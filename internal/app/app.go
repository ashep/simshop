package app

import (
	"fmt"
	"os"

	"github.com/ashep/go-app/httpserver"
	"github.com/ashep/go-app/runner"
	"github.com/ashep/simshop/api"
	"github.com/ashep/simshop/internal/file"
	"github.com/ashep/simshop/internal/handler"
	"github.com/ashep/simshop/internal/loader"
	"github.com/ashep/simshop/internal/openapi"
	"github.com/ashep/simshop/internal/product"
	"github.com/ashep/simshop/internal/property"
)

func Run(rt *runner.Runtime[Config]) error {
	cfg := rt.Cfg
	l := rt.Log

	if cfg.Server.PublicDir == "" {
		cfg.Server.PublicDir = "./public"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}

	if err := os.MkdirAll(cfg.Server.PublicDir, 0755); err != nil {
		return fmt.Errorf("create public dir: %w", err)
	}

	catalog, err := loader.Load(cfg.DataDir, cfg.Server.PublicDir, l)
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}

	prodSvc := product.NewService(catalog.Products)
	propSvc := property.NewService(catalog.Properties)
	fileSvc := file.NewService(catalog.Files)

	openAPI, err := openapi.New(api.Spec)
	if err != nil {
		return fmt.Errorf("create openapi: %w", err)
	}

	hdl := handler.NewHandler(prodSvc, propSvc, fileSvc, openAPI.Responder(), l)
	openapiMw := openAPI.Middleware()

	srv := httpserver.New(httpserver.WithAddr(cfg.Server.Addr))

	srv.HandleFunc("GET /products", openapiMw(hdl.ListProducts))
	srv.HandleFunc("GET /products/{id}", openapiMw(hdl.GetProduct))
	srv.HandleFunc("GET /products/{id}/prices", openapiMw(hdl.GetProductPrice))
	srv.HandleFunc("GET /products/{id}/files", openapiMw(hdl.GetProductFiles))
	srv.HandleFunc("GET /properties", openapiMw(hdl.ListProperties))

	l.Info().Str("addr", srv.Listener().Addr().String()).Msg("starting server")

	if err := srv.Run(rt.Ctx); err != nil {
		return fmt.Errorf("server run: %w", err)
	}

	return nil
}
