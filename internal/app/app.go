package app

import (
	"fmt"

	"github.com/ashep/go-app/dbmigrator"
	"github.com/ashep/go-app/httpserver"
	"github.com/ashep/go-app/runner"
	"github.com/ashep/simshop/api"
	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/contenttype"
	"github.com/ashep/simshop/internal/file"
	"github.com/ashep/simshop/internal/handler"
	"github.com/ashep/simshop/internal/openapi"
	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/internal/property"
	"github.com/ashep/simshop/internal/shop"
	appsql "github.com/ashep/simshop/internal/sql"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Run(rt *runner.Runtime[Config]) error {
	ctx := rt.Ctx
	cfg := rt.Cfg
	l := rt.Log

	// Apply Files config defaults.
	if cfg.Files.MaxSize == 0 {
		cfg.Files.MaxSize = 10 * 1024 * 1024
	}
	if cfg.Files.MaxNumPerUser == 0 {
		cfg.Files.MaxNumPerUser = 50
	}
	if len(cfg.Files.AllowedTypes) == 0 {
		cfg.Files.AllowedTypes = []string{
			"image/jpeg",
			"image/png",
			"image/gif",
			"application/pdf",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		}
	}

	// Migrate DB
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

	shopSvc := shop.NewService(db, l)
	prodSvc := product.NewService(db, l)
	propSvc := property.NewService(db, l)
	fileSvc := file.NewService(db, cfg.Files.MaxNumPerUser, l)

	openAPI, err := openapi.New(api.Spec)
	if err != nil {
		return fmt.Errorf("create openapi: %w", err)
	}

	authSvc := auth.NewService(db)

	hdl := handler.NewHandler(shopSvc, prodSvc, propSvc, fileSvc, cfg.Files.MaxSize, cfg.Files.AllowedTypes, openAPI.Responder(), l)
	authMw := auth.Middleware(authSvc)
	optionalAuthMw := auth.OptionalMiddleware(authSvc)
	jsonContentType := contenttype.Middleware("application/json")
	MultipartContentType := contenttype.Middleware("multipart/form-data")
	openapiMw := openAPI.Middleware()

	srv := httpserver.New(httpserver.WithAddr(cfg.Server.Addr))

	srv.HandleFunc("GET /shops", authMw(openapiMw(hdl.ListShops)))
	srv.HandleFunc("GET /shops/{id}", optionalAuthMw(openapiMw(hdl.GetShop)))
	srv.HandleFunc("POST /shops", jsonContentType(authMw(openapiMw(hdl.CreateShop))))
	srv.HandleFunc("PATCH /shops/{id}", jsonContentType(authMw(openapiMw(hdl.UpdateShop))))

	srv.HandleFunc("POST /products", jsonContentType(authMw(openapiMw(hdl.CreateProduct))))
	srv.HandleFunc("PATCH /products/{id}", jsonContentType(authMw(openapiMw(hdl.UpdateProduct))))
	srv.HandleFunc("PUT /products/{id}/prices", jsonContentType(authMw(openapiMw(hdl.SetProductPrices))))
	srv.HandleFunc("GET /products/{id}/prices", optionalAuthMw(openapiMw(hdl.GetProductPrice)))
	srv.HandleFunc("GET /products/{id}", optionalAuthMw(openapiMw(hdl.GetProduct)))
	srv.HandleFunc("GET /shops/{id}/products", optionalAuthMw(openapiMw(hdl.ListShopProducts)))
	srv.HandleFunc("GET /properties", openapiMw(hdl.ListProperties))
	srv.HandleFunc("POST /properties", jsonContentType(authMw(openapiMw(hdl.CreateProperty))))
	srv.HandleFunc("PATCH /properties/{id}", jsonContentType(authMw(openapiMw(hdl.UpdateProperty))))

	srv.HandleFunc("POST /files", MultipartContentType(authMw(openapiMw(hdl.UploadFile))))

	l.Info().Str("addr", srv.Listener().Addr().String()).Msg("starting server")

	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("server run: %w", err)
	}

	return nil
}
