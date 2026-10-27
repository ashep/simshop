package app

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ashep/go-app/dbmigrator"
	"github.com/ashep/go-app/httpserver"
	"github.com/ashep/go-app/runner"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ashep/simshop/api"
	"github.com/ashep/simshop/internal/cloudflare"
	"github.com/ashep/simshop/internal/ganalytics"
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
	"github.com/ashep/simshop/internal/stats"
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

	cfEnabled := cfg.Cloudflare.APIToken != "" && cfg.Cloudflare.AccountID != "" && cfg.Cloudflare.SiteTag != ""
	if !cfEnabled && (cfg.Cloudflare.APIToken != "" || cfg.Cloudflare.AccountID != "" || cfg.Cloudflare.SiteTag != "") {
		return fmt.Errorf("cloudflare api_token, account_id and site_tag must be set together")
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

	var gaNotifier order.Notifier
	if cfg.GoogleAnalytics.APISecret != "" {
		// The measurement id is public and lives in shop.yaml next to the gtag
		// setup, so browser and server events land in the same GA property.
		if catalog.Shop.GoogleAnalytics == nil || catalog.Shop.GoogleAnalytics.ID == "" {
			return fmt.Errorf("google_analytics.api_secret requires google-analytics.id in shop.yaml")
		}
		gaClient := ganalytics.NewClient(
			catalog.Shop.GoogleAnalytics.ID, cfg.GoogleAnalytics.APISecret, cfg.GoogleAnalytics.ServiceURL)
		gn := ganalytics.NewNotifier(gaClient, ordersReader, prodSvc, l)
		gn.Start()
		defer gn.Stop()
		gaNotifier = gn
		l.Info().Msg("google analytics notifier enabled")
	} else {
		l.Info().Msg("google analytics notifier disabled")
	}

	var orderNotifier order.Notifier
	notifiers := make([]order.Notifier, 0, 3)
	if telegramNotifier != nil {
		notifiers = append(notifiers, telegramNotifier)
	}
	if resendNotifier != nil {
		notifiers = append(notifiers, resendNotifier)
	}
	if gaNotifier != nil {
		notifiers = append(notifiers, gaNotifier)
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
	if cfEnabled {
		cfClient := cloudflare.NewClient(
			cfg.Cloudflare.APIToken, cfg.Cloudflare.AccountID, cfg.Cloudflare.SiteTag, cfg.Cloudflare.ServiceURL)
		hdl.SetStats(stats.NewService(cfClient, prodSvc, time.Now))
		l.Info().Msg("visit stats enabled")
	} else {
		l.Info().Msg("visit stats disabled")
	}
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
	srv.HandleFunc("OPTIONS /products/{id}/promo-codes/{code}", corsMw(nop))
	// Own limiter: this is the only brute-force guard on promo codes, and sharing the order
	// limiter would let storefront code checks eat into the customer's order budget.
	checkPromoHandler := hdl.CheckPromoCode
	if cfg.RateLimit > 0 {
		checkPromoHandler = handler.RateLimitMiddleware(cfg.RateLimit)(checkPromoHandler)
	}
	srv.HandleFunc("GET /products/{id}/promo-codes/{code}", corsMw(openapiMw(checkPromoHandler)))
	srv.HandleFunc("GET /product/{id}", hdl.ServeProductPreview)
	srv.HandleFunc("GET /product/{lang}/{id}", hdl.ServeProductPreview)
	srv.HandleFunc("OPTIONS /images/{product_id}/{file_name}", corsMw(nop))
	srv.HandleFunc("GET /images/{product_id}/{file_name}", corsMw(hdl.ServeImage))
	srv.HandleFunc("OPTIONS /assets/{path...}", corsMw(nop))
	srv.HandleFunc("GET /assets/{path...}", corsMw(hdl.ServeAsset))

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
		srv.HandleFunc("OPTIONS /orders/{id}/status", corsMw(nop))
		srv.HandleFunc("PATCH /orders/{id}/status", corsMw(apiKeyMw(openapiMw(hdl.UpdateOrderStatus))))
		if cfEnabled {
			srv.HandleFunc("OPTIONS /stats", corsMw(nop))
			srv.HandleFunc("GET /stats", corsMw(apiKeyMw(openapiMw(hdl.ServeStats))))
		}
	}

	l.Info().Str("addr", srv.Listener().Addr().String()).Msg("starting server")

	if err := srv.Run(rt.Ctx); err != nil {
		return fmt.Errorf("server run: %w", err)
	}

	return nil
}
