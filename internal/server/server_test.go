package server_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"web-search/internal/config"
	"web-search/internal/server"
)

func TestHTTPBoundary(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, path, host, origin, body string
		status                         int
	}{
		{"path", "/other", "localhost", "", "{}", 404},
		{"origin", "/mcp", "localhost", "https://evil.example", "{}", 403},
		{"host", "/mcp", "evil.example", "", "{}", 403},
		{"large", "/mcp", "localhost", "", strings.Repeat("x", 20000), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
			h := server.HTTPHandler(s, config.Default().Server, slog.New(slog.DiscardHandler))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Host = tc.host
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")

			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}

			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)

			if res.Code != tc.status || len(res.Header().Get("X-Trace-Id")) != 32 {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
		})
	}
}

func TestRunShutdown(t *testing.T) {
	t.Parallel()

	for _, delay := range []time.Duration{time.Millisecond, 10 * time.Millisecond} {
		t.Run(delay.String(), func(t *testing.T) {
			t.Parallel()

			cfg := config.Default().Server
			cfg.Port = 0

			ctx, cancel := context.WithTimeout(context.Background(), delay)
			defer cancel()

			s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)

			err := server.Run(ctx, s, cfg, slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
