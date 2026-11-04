package root

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"web-search/internal/adapters/duckduckgo"
	"web-search/internal/adapters/web"
	"web-search/internal/config"
	"web-search/internal/handlers/mcp/webfetch"
	"web-search/internal/handlers/mcp/websearch"
	"web-search/internal/server"
	"web-search/internal/services/fetch"
	"web-search/internal/services/search"
)

type (
	App struct {
		MCP    *mcp.Server
		cfg    config.Config
		logger *slog.Logger
		http   *web.Client
	}
)

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	httpClient := web.New(cfg.HTTP.Timeout, cfg.HTTP.MaxResponseBytes, cfg.HTTP.MaxRedirects, logger)
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "local-web-search", Version: "0.1.0"}, &mcp.ServerOptions{Logger: logger})
	websearch.New(
		search.New(duckduckgo.New(httpClient), cfg.Search.Interval, cfg.Search.DefaultLimit, cfg.Search.MaxLimit),
	).Register(
		mcpServer,
	)
	webfetch.New(fetch.New(httpClient, cfg.Fetch.DefaultMaxChars, cfg.Fetch.MaxChars)).Register(mcpServer)
	mcpServer.AddReceivingMiddleware(server.Middleware(cfg.Server, logger, cfg.Logging.LogResponses))

	return &App{MCP: mcpServer, cfg: cfg, logger: logger, http: httpClient}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.http.Close()
	return server.Run(ctx, a.MCP, a.cfg.Server, a.logger)
}
