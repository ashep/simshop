package app

import (
	"fmt"
	"net/http"

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

	hdl := handler.NewHandler(prodSvc, openAPI.Responder(), cfg.DataDir, l)
	openapiMw := openAPI.Middleware()
	corsMw := handler.CORSMiddleware(cfg.Server.CORSOrigins)

	srv := httpserver.New(httpserver.WithAddr(cfg.Server.Addr))

	nop := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusMethodNotAllowed) }

	srv.HandleFunc("GET /products", corsMw(openapiMw(hdl.ListProducts)))
	srv.HandleFunc("OPTIONS /products", corsMw(nop))
	srv.HandleFunc("GET /images/{product_id}/{file_name}", corsMw(hdl.ServeImage))
	srv.HandleFunc("OPTIONS /images/{product_id}/{file_name}", corsMw(nop))
	srv.HandleFunc("GET /pages", corsMw(openapiMw(hdl.ListPages)))
	srv.HandleFunc("OPTIONS /pages", corsMw(nop))
	srv.HandleFunc("GET /pages/{id}/{lang}", corsMw(hdl.ServePage))
	srv.HandleFunc("OPTIONS /pages/{id}/{lang}", corsMw(nop))

	l.Info().Str("addr", srv.Listener().Addr().String()).Msg("starting server")

	if err := srv.Run(rt.Ctx); err != nil {
		return fmt.Errorf("server run: %w", err)
	}

	return nil
}
