package app

import (
	"fmt"
	"net/http"

	"github.com/ashep/go-app/dbmigrator"
	"github.com/ashep/go-app/httpserver"
	"github.com/ashep/go-app/runner"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ashep/simshop/api"
	"github.com/ashep/simshop/internal/geo"
	"github.com/ashep/simshop/internal/handler"
	"github.com/ashep/simshop/internal/loader"
	"github.com/ashep/simshop/internal/monobank"
	"github.com/ashep/simshop/internal/novaposhta"
	"github.com/ashep/simshop/internal/openapi"
	"github.com/ashep/simshop/internal/order"
	"github.com/ashep/simshop/internal/orderdb"
	"github.com/ashep/simshop/internal/page"
	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/internal/shop"
	appsql "github.com/ashep/simshop/internal/sql"
)

func Run(rt *runner.Runtime[Config]) error {
	cfg := rt.Cfg
	l := rt.Log

	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}

	if cfg.Monobank.APIKey == "" {
		return fmt.Errorf("monobank api key is required")
	}
	if cfg.Monobank.RedirectURL == "" {
		return fmt.Errorf("monobank redirect url is required")
	}
	if cfg.Monobank.WebhookURL == "" {
		return fmt.Errorf("monobank webhook url is required")
	}

	migRes, err := dbmigrator.RunPostgres(cfg.Database.DSN, l, dbmigrator.Source{FS: appsql.FS, Path: "."})
	if err != nil {
		return fmt.Errorf("migrate db: %w", err)
	}

	if migRes.PrevVersion != migRes.NewVersion {
		l.Info().
			Uint("from", migRes.PrevVersion).
			Uint("to", migRes.NewVersion).
			Msg("database migrated")
	}

	db, err := pgxpool.New(rt.Ctx, cfg.Database.DSN)
	if err != nil {
		return fmt.Errorf("connect to db: %w", err)
	}
	defer db.Close()

	catalog, err := loader.Load(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}

	prodSvc := product.NewService(catalog.ProductItems)
	pageSvc := page.NewService(catalog.Pages)
	shopSvc := shop.NewService(catalog.Shop)
	npClient := novaposhta.NewClient(cfg.NovaPoshta.APIKey, cfg.NovaPoshta.ServiceURL)
	mbClient := monobank.NewClient(cfg.Monobank.APIKey, cfg.Monobank.ServiceURL)
	mbVerifier := monobank.NewVerifier(cfg.Monobank.APIKey, cfg.Monobank.ServiceURL)
	if err := mbVerifier.Fetch(rt.Ctx); err != nil {
		return fmt.Errorf("fetch monobank pubkey: %w", err)
	}

	ordersWriter := orderdb.New(db)
	ordersReader := orderdb.NewReader(db)
	// orderdb.Writer satisfies Writer, InvoiceWriter, and PaymentEventWriter.
	orderSvc := order.NewService(ordersWriter, ordersReader, ordersWriter, ordersWriter)

	openAPI, err := openapi.New(api.Spec)
	if err != nil {
		return fmt.Errorf("create openapi: %w", err)
	}

	hdl := handler.NewHandler(
		prodSvc, pageSvc, shopSvc, npClient, mbClient, mbVerifier, orderSvc,
		geo.NewDetector(), openAPI.Responder(),
		cfg.DataDir, cfg.Monobank.RedirectURL, cfg.Monobank.WebhookURL, cfg.Monobank.TaxIDs, l,
	)
	openapiMw := openAPI.Middleware()
	corsMw := handler.CORSMiddleware()

	srv := httpserver.New(httpserver.WithAddr(cfg.Server.Addr))

	nop := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusMethodNotAllowed) }

	srv.HandleFunc("GET /products", corsMw(openapiMw(hdl.ListProducts)))
	srv.HandleFunc("OPTIONS /products", corsMw(nop))
	srv.HandleFunc("GET /products/{id}/{lang}", corsMw(openapiMw(hdl.ServeProductContent)))
	srv.HandleFunc("OPTIONS /products/{id}/{lang}", corsMw(nop))
	srv.HandleFunc("GET /images/{product_id}/{file_name}", corsMw(hdl.ServeImage))
	srv.HandleFunc("OPTIONS /images/{product_id}/{file_name}", corsMw(nop))
	srv.HandleFunc("GET /pages", corsMw(openapiMw(hdl.ListPages)))
	srv.HandleFunc("OPTIONS /pages", corsMw(nop))
	srv.HandleFunc("GET /pages/{id}/{lang}", corsMw(hdl.ServePage))
	srv.HandleFunc("OPTIONS /pages/{id}/{lang}", corsMw(nop))
	srv.HandleFunc("GET /shop", corsMw(openapiMw(hdl.ServeShop)))
	srv.HandleFunc("OPTIONS /shop", corsMw(nop))
	srv.HandleFunc("GET /nova-poshta/cities", corsMw(openapiMw(hdl.SearchNPCities)))
	srv.HandleFunc("OPTIONS /nova-poshta/cities", corsMw(nop))
	srv.HandleFunc("GET /nova-poshta/branches", corsMw(openapiMw(hdl.SearchNPBranches)))
	srv.HandleFunc("OPTIONS /nova-poshta/branches", corsMw(nop))
	srv.HandleFunc("GET /nova-poshta/streets", corsMw(openapiMw(hdl.SearchNPStreets)))
	srv.HandleFunc("OPTIONS /nova-poshta/streets", corsMw(nop))

	var ordersHandler http.HandlerFunc
	if cfg.RateLimit < 0 {
		// Negative rate limit disables rate limiting
		ordersHandler = hdl.CreateOrder
	} else {
		rateLimit := cfg.RateLimit
		if rateLimit == 0 {
			rateLimit = 1 // default: 1 request per minute
		}
		rateLimitMw := handler.RateLimitMiddleware(rateLimit)
		ordersHandler = rateLimitMw(hdl.CreateOrder)
	}
	srv.HandleFunc("POST /orders", corsMw(openapiMw(ordersHandler)))
	srv.HandleFunc("OPTIONS /orders", corsMw(nop))
	srv.HandleFunc("POST /monobank/webhook", hdl.MonobankWebhook)
	if cfg.Server.APIKey != "" {
		apiKeyMw := handler.APIKeyMiddleware(cfg.Server.APIKey)
		srv.HandleFunc("GET /orders", corsMw(apiKeyMw(openapiMw(hdl.ListOrders))))
	}

	l.Info().Str("addr", srv.Listener().Addr().String()).Msg("starting server")

	if err := srv.Run(rt.Ctx); err != nil {
		return fmt.Errorf("server run: %w", err)
	}

	return nil
}
