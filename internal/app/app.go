package app

import (
	"fmt"

	"github.com/ashep/go-app/httpserver"
	"github.com/ashep/go-app/runner"
	"github.com/ashep/simshop/api"
	"github.com/ashep/simshop/internal/handler"
	"github.com/ashep/simshop/internal/loader"
	"github.com/ashep/simshop/internal/openapi"
	"github.com/ashep/simshop/internal/product"
)

func Run(rt *runner.Runtime[Config]) error {
	cfg := rt.Cfg
	l := rt.Log

	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}

	catalog, err := loader.Load(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}

	prodSvc := product.NewService(catalog.Products)

	openAPI, err := openapi.New(api.Spec)
	if err != nil {
		return fmt.Errorf("create openapi: %w", err)
	}

	hdl := handler.NewHandler(prodSvc, openAPI.Responder(), l)
	openapiMw := openAPI.Middleware()

	srv := httpserver.New(httpserver.WithAddr(cfg.Server.Addr))

	srv.HandleFunc("GET /products", openapiMw(hdl.ListProducts))

	l.Info().Str("addr", srv.Listener().Addr().String()).Msg("starting server")

	if err := srv.Run(rt.Ctx); err != nil {
		return fmt.Errorf("server run: %w", err)
	}

	return nil
}
