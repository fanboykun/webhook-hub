package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
	"github.com/iweka-dev/webhook-hub/internal/app"
	"github.com/iweka-dev/webhook-hub/internal/config"
)

type Server struct {
	engine *gin.Engine
	http   *http.Server
}

func New(cfg config.Config, service *app.Service) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	apiConfig := huma.DefaultConfig("Webhook Notification Gateway", "0.1.0")
	apiConfig.DocsPath = ""
	apiConfig.Components = &huma.Components{
		Schemas: huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer),
		SecuritySchemes: map[string]*huma.SecurityScheme{
			"bearerAuth": {
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "opaque",
				Description:  "Static admin bearer token configured by the gateway.",
			},
		},
	}
	api := humagin.New(engine, apiConfig)

	engine.GET("/docs", func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.String(http.StatusOK, scalarDocsHTML("Webhook Notification Gateway Reference", "/openapi.json"))
	})

	registerRoutes(api, cfg, NewHandler(service))
	applyDestinationEnums(api.OpenAPI(), cfg)

	server := &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           engine,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	return &Server{engine: engine, http: server}
}

func (s *Server) Handler() http.Handler {
	return s.engine
}

func (s *Server) Run() error {
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func scalarDocsHTML(title, openAPIPath string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="referrer" content="no-referrer">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>%s</title>
  </head>
  <body>
    <div id="app"></div>
    <script src="https://unpkg.com/@scalar/api-reference@1.44.20/dist/browser/standalone.js" crossorigin integrity="sha384-tMz7GAo6dMy55x9tLFtH+sHtogji6Scmb+feBR31TAHmvSPRUTboK9H3M5NFaP4R"></script>
    <script>
      Scalar.createApiReference('#app', {
        url: '%s',
        persistAuth: true
      })
    </script>
  </body>
</html>`, title, openAPIPath)
}

func applyDestinationEnums(spec *huma.OpenAPI, cfg config.Config) {
	if spec == nil || spec.Components == nil || spec.Components.Schemas == nil {
		return
	}

	enum := destinationEnumValues(cfg)
	if len(enum) == 0 {
		return
	}

	for _, schema := range spec.Components.Schemas.Map() {
		if schema == nil || schema.Properties == nil {
			continue
		}
		property := schema.Properties["destinations"]
		if property == nil || property.Items == nil {
			continue
		}
		property.Items.Enum = enum
	}
}

func destinationEnumValues(cfg config.Config) []any {
	keys := make([]string, 0, len(cfg.Destinations))
	for key := range cfg.Destinations {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	enum := make([]any, 0, len(keys))
	for _, key := range keys {
		enum = append(enum, key)
	}
	return enum
}
