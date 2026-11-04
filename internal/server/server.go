package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"web-search/internal/config"
	"web-search/internal/observability"
)

func Middleware(cfg config.Server, logger *slog.Logger, logResponses bool) mcp.Middleware {
	gate := make(chan struct{}, cfg.MaxConcurrentRequests)

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if observability.TraceID(ctx) == "" {
				ctx = observability.WithTrace(ctx)
			}

			ctx, cancel := context.WithTimeout(ctx, cfg.RequestTimeout)
			defer cancel()

			start := time.Now()

			attrs := []any{"method", method}
			if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
				attrs = append(attrs, "tool", p.Name, "arguments", string(p.Arguments))
			}

			logger.InfoContext(ctx, "mcp.request", attrs...)

			if method == "tools/call" {
				select {
				case gate <- struct{}{}:
				case <-ctx.Done():
					logger.ErrorContext(ctx, "mcp.error", "error", ctx.Err().Error())
					return nil, ctx.Err()
				}

				defer func() { <-gate }()
			}

			result, err := next(ctx, method, req)

			attrs = append(attrs, "duration_ms", time.Since(start).Milliseconds())
			if err != nil {
				logger.ErrorContext(ctx, "mcp.error", append(attrs, "error", err.Error())...)
				return result, err
			}

			attrs = appendResult(ctx, attrs, result, logResponses)

			logger.InfoContext(ctx, "mcp.response", attrs...)

			return result, nil
		}
	}
}

func HTTPHandler(mcpServer *mcp.Server, cfg config.Server, logger *slog.Logger) http.Handler {
	h := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{
			Stateless:                    true,
			JSONResponse:                 true,
			Logger:                       logger,
			MaxRequestBodyBytes:          cfg.MaxRequestBytes,
			PropagateRequestCancellation: true,
		},
	)
	protected := http.NewCrossOriginProtection().Handler(h)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := observability.WithTrace(r.Context())
		w.Header().Set("X-Trace-ID", observability.TraceID(ctx))

		start := time.Now()

		logger.InfoContext(ctx, "mcp.http.request", "http_method", r.Method, "path", r.URL.Path)

		host, _, hostErr := net.SplitHostPort(r.Host)
		if hostErr != nil {
			host = r.Host
		}

		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			http.Error(w, "non-local Host rejected", http.StatusForbidden)
			return
		}

		if r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}

		protected.ServeHTTP(w, r.WithContext(ctx))
		logger.InfoContext(ctx, "mcp.http.response", "duration_ms", time.Since(start).Milliseconds())
	})
}

func Run(ctx context.Context, mcpServer *mcp.Server, cfg config.Server, logger *slog.Logger) error {
	if cfg.Transport == "stdio" {
		logger.InfoContext(ctx, "server.started", "transport", "stdio")
		return mcpServer.Run(ctx, &mcp.StdioTransport{MaxLineLength: int(cfg.MaxRequestBytes)})
	}

	srv := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Handler:           HTTPHandler(mcpServer, cfg, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16384,
	}

	listenConfig := &net.ListenConfig{}

	listener, err := listenConfig.Listen(ctx, "tcp", srv.Addr)
	if err != nil {
		return err
	}

	logger.InfoContext(ctx, "server.started", "transport", "http", "address", srv.Addr)

	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()

	select {
	case err = <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}

		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
		defer cancel()

		err = srv.Shutdown(shutdownCtx)
		if err != nil {
			closeErr := srv.Close()
			if closeErr != nil {
				err = errors.Join(err, closeErr)
			}

			return fmt.Errorf("shutdown: %w", err)
		}

		return nil
	}
}

func appendResult(ctx context.Context, attrs []any, result mcp.Result, logResponses bool) []any {
	if toolResult, ok := result.(*mcp.CallToolResult); ok {
		attrs = append(attrs, "is_error", toolResult.IsError)
		if toolResult.IsError || logResponses {
			attrs = append(attrs, "response", toolResult)
		}

		if toolResult.Meta == nil {
			toolResult.Meta = mcp.Meta{}
		}

		toolResult.Meta["trace_id"] = observability.TraceID(ctx)
	}

	return attrs
}
