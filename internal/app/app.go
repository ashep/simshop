package app

import (
	"fmt"

	"github.com/ashep/go-app/dbmigrator"
	"github.com/ashep/go-app/httpserver"
	"github.com/ashep/go-app/runner"
	"github.com/ashep/simshop/api"
	"github.com/ashep/simshop/internal/auth"
	"github.com/ashep/simshop/internal/contenttype"
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

	openAPI, err := openapi.New(api.Spec)
	if err != nil {
		return fmt.Errorf("create openapi: %w", err)
	}

	authSvc := auth.NewService(db)

	hdl := handler.NewHandler(shopSvc, prodSvc, propSvc, openAPI.Responder(), l)
	authMw := auth.Middleware(authSvc)
	optionalAuthMw := auth.OptionalMiddleware(authSvc)
	ctypeMw := contenttype.Middleware()
	openapiMw := openAPI.Middleware()

	srv := httpserver.New(httpserver.WithAddr(cfg.Server.Addr))

	srv.HandleFunc("GET /shops", authMw(openapiMw(hdl.ListShops)))
	srv.HandleFunc("GET /shops/{id}", optionalAuthMw(openapiMw(hdl.GetShop)))
	srv.HandleFunc("POST /shops", ctypeMw(authMw(openapiMw(hdl.CreateShop))))
	srv.HandleFunc("PATCH /shops/{id}", ctypeMw(authMw(openapiMw(hdl.UpdateShop))))

	srv.HandleFunc("POST /products", ctypeMw(authMw(openapiMw(hdl.CreateProduct))))
	srv.HandleFunc("PATCH /products/{id}", ctypeMw(authMw(openapiMw(hdl.UpdateProduct))))
	srv.HandleFunc("GET /products/{id}", optionalAuthMw(openapiMw(hdl.GetProduct)))
	srv.HandleFunc("GET /shops/{id}/products", optionalAuthMw(openapiMw(hdl.ListShopProducts)))
	srv.HandleFunc("GET /properties", openapiMw(hdl.ListProperties))
	srv.HandleFunc("POST /properties", ctypeMw(authMw(openapiMw(hdl.CreateProperty))))
	srv.HandleFunc("PATCH /properties/{id}", ctypeMw(authMw(openapiMw(hdl.UpdateProperty))))

	l.Info().Str("addr", srv.Listener().Addr().String()).Msg("starting server")

	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("server run: %w", err)
	}

	return nil
}
