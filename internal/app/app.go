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
	"github.com/ashep/simshop/internal/resend"
	"github.com/ashep/simshop/internal/shop"
	appsql "github.com/ashep/simshop/internal/sql"
	"github.com/ashep/simshop/internal/telegram"
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
	if cfg.Server.PublicURL == "" {
		return fmt.Errorf("server public url is required")
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

	var telegramNotifier order.Notifier
	switch {
	case cfg.Telegram.Token != "" && cfg.Telegram.ChatID != "":
		tgClient := telegram.NewClient(cfg.Telegram.Token, cfg.Telegram.ServiceURL)
		tn := telegram.NewNotifier(tgClient, cfg.Telegram.ChatID, ordersReader, prodSvc, l)
		tn.Start()
		defer tn.Stop()
		telegramNotifier = tn
		l.Info().Msg("telegram notifier enabled")
	case cfg.Telegram.Token != "" || cfg.Telegram.ChatID != "":
		return fmt.Errorf("telegram: token and chat_id must be set together")
	default:
		l.Info().Msg("telegram notifier disabled")
	}

	var resendNotifier order.Notifier
	if cfg.Resend.APIKey != "" {
		if cfg.Mail.From == "" {
			return fmt.Errorf("mail.from is required when resend.api_key is set")
		}
		if cfg.Mail.OrderURL == "" {
			return fmt.Errorf("mail.order_url is required when resend.api_key is set")
		}
		if err := validateEmailTemplates(catalog.EmailTemplates); err != nil {
			return fmt.Errorf("validate email templates: %w", err)
		}
		rc := resend.NewClient(cfg.Resend.APIKey, cfg.Resend.ServiceURL)
		rn := resend.NewNotifier(
			rc, cfg.Mail.From, cfg.Mail.OrderURL,
			ordersReader, prodSvc, shopSvc, catalog.EmailTemplates, l,
		)
		rn.Start()
		defer rn.Stop()
		resendNotifier = rn
		l.Info().Msg("resend notifier enabled")
	} else {
		l.Info().Msg("resend notifier disabled")
	}

	var orderNotifier order.Notifier
	notifiers := make([]order.Notifier, 0, 2)
	if telegramNotifier != nil {
		notifiers = append(notifiers, telegramNotifier)
	}
	if resendNotifier != nil {
		notifiers = append(notifiers, resendNotifier)
	}
	switch len(notifiers) {
	case 0:
		orderNotifier = nil
	case 1:
		orderNotifier = notifiers[0]
	default:
		orderNotifier = order.NewMultiNotifier(notifiers...)
	}

	// orderdb.Writer satisfies Writer, InvoiceWriter, InvoiceEventWriter, and OperatorWriter.
	orderSvc := order.NewService(ordersWriter, ordersReader, ordersWriter, ordersWriter, ordersWriter, orderNotifier)

	openAPI, err := openapi.New(api.Spec)
	if err != nil {
		return fmt.Errorf("create openapi: %w", err)
	}

	hdl := handler.NewHandler(
		prodSvc, pageSvc, shopSvc, npClient, mbClient, mbVerifier, orderSvc,
		geo.NewDetector(), openAPI.Responder(),
		cfg.DataDir, cfg.Monobank.RedirectURL, cfg.Server.PublicURL, cfg.Monobank.TaxIDs, l,
	)
	openapiMw := openAPI.Middleware()
	corsMw := handler.CORSMiddleware()

	srv := httpserver.New(httpserver.WithAddr(cfg.Server.Addr))

	nop := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusMethodNotAllowed) }

	srv.HandleFunc("OPTIONS /shop", corsMw(nop))
	srv.HandleFunc("GET /shop", corsMw(openapiMw(hdl.ServeShop)))

	srv.HandleFunc("OPTIONS /pages", corsMw(nop))
	srv.HandleFunc("GET /pages", corsMw(openapiMw(hdl.ListPages)))

	srv.HandleFunc("OPTIONS /products", corsMw(nop))
	srv.HandleFunc("GET /products", corsMw(openapiMw(hdl.ListProducts)))
	srv.HandleFunc("OPTIONS /products/{id}/{lang}", corsMw(nop))
	srv.HandleFunc("GET /products/{id}/{lang}", corsMw(openapiMw(hdl.ServeProductContent)))
	srv.HandleFunc("OPTIONS /images/{product_id}/{file_name}", corsMw(nop))
	srv.HandleFunc("GET /images/{product_id}/{file_name}", corsMw(hdl.ServeImage))

	srv.HandleFunc("OPTIONS /pages/{id}/{lang}", corsMw(nop))
	srv.HandleFunc("GET /pages/{id}/{lang}", corsMw(hdl.ServePage))

	srv.HandleFunc("OPTIONS /nova-poshta/cities", corsMw(nop))
	srv.HandleFunc("GET /nova-poshta/cities", corsMw(openapiMw(hdl.SearchNPCities)))
	srv.HandleFunc("OPTIONS /nova-poshta/branches", corsMw(nop))
	srv.HandleFunc("GET /nova-poshta/branches", corsMw(openapiMw(hdl.SearchNPBranches)))
	srv.HandleFunc("OPTIONS /nova-poshta/streets", corsMw(nop))
	srv.HandleFunc("GET /nova-poshta/streets", corsMw(openapiMw(hdl.SearchNPStreets)))

	srv.HandleFunc("OPTIONS /orders", corsMw(nop))
	createOrderHandler := hdl.CreateOrder
	if cfg.RateLimit > 0 {
		rateLimitMw := handler.RateLimitMiddleware(cfg.RateLimit)
		createOrderHandler = rateLimitMw(createOrderHandler)
	}
	srv.HandleFunc("POST /orders", corsMw(openapiMw(createOrderHandler)))
	srv.HandleFunc("POST /monobank/webhook", hdl.MonobankWebhook)
	srv.HandleFunc("OPTIONS /orders/{id}", corsMw(nop))
	srv.HandleFunc("GET /orders/{id}", corsMw(openapiMw(hdl.GetOrderStatus)))
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
