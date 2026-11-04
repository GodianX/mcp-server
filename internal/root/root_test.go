package root_test

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"web-search/internal/config"
	"web-search/internal/observability"
	"web-search/internal/root"
	"web-search/internal/server"
	"web-search/internal/services/fetch"
	"web-search/internal/services/search"
)

var live = flag.Bool("live", false, "Run the DuckDuckGo network smoke test")

func TestMCP(t *testing.T) {
	t.Parallel()

	for _, transport := range []string{"memory", "http"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()

			cfg := config.Default()
			logger := slog.New(slog.DiscardHandler)

			app, err := root.New(cfg, logger)
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var clientTransport mcp.Transport

			if transport == "http" {
				srv := httptest.NewServer(server.HTTPHandler(app.MCP, cfg.Server, logger))
				defer srv.Close()

				clientTransport = &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp"}
			} else {
				a, b := mcp.NewInMemoryTransports()

				ss, connectErr := app.MCP.Connect(ctx, a, nil)
				if connectErr != nil {
					t.Fatal(connectErr)
				}
				defer ss.Close()

				clientTransport = b
			}

			session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()

			tools, err := session.ListTools(ctx, nil)
			if err != nil || len(tools.Tools) != 2 {
				t.Fatalf("%+v %v", tools, err)
			}

			result, err := session.CallTool(
				ctx,
				&mcp.CallToolParams{Name: "web_fetch", Arguments: map[string]any{"url": "http://127.0.0.1"}},
			)
			if err != nil || !result.IsError || result.Meta["trace_id"] == "" {
				t.Fatalf("%+v %v", result, err)
			}

			firstID := result.Meta["trace_id"]

			result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "web_search", Arguments: map[string]any{"query": "!g secret"}})
			if err != nil || !result.IsError || result.Meta["trace_id"] == firstID {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestLive(t *testing.T) {
	if os.Getenv("WEB_SEARCH_LIVE") != "1" && !*live {
		t.Skip("set WEB_SEARCH_LIVE=1 or run task test-live")
	}

	cfg := config.Default()

	logger, closer, err := observability.New(cfg.Logging, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	app, err := root.New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(server.HTTPHandler(app.MCP, cfg.Server, logger))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	session, err := mcp.NewClient(
		&mcp.Implementation{Name: "live-check", Version: "1"},
		nil,
	).Connect(
		ctx,
		&mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp"},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.CallTool(
		ctx,
		&mcp.CallToolParams{Name: "web_search", Arguments: map[string]any{"query": "golang pgx v5 documentation", "limit": 3}},
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Fatalf("search failed: %+v", result.Content)
	}

	found := decodeStructured[search.Output](t, result)
	if len(found.Results) == 0 {
		t.Fatal("empty search results")
	}

	result, err = session.CallTool(
		ctx,
		&mcp.CallToolParams{Name: "web_fetch", Arguments: map[string]any{"url": found.Results[0].URL, "max_chars": 5000}},
	)
	if err != nil || result.IsError {
		t.Fatalf("fetch failed: %+v %v", result, err)
	}

	page := decodeStructured[fetch.Output](t, result)
	if strings.TrimSpace(page.Text) == "" {
		t.Fatal("empty page")
	}

	t.Logf(
		"Found %d results; fetched %s (%d chars); trace_id=%v",
		len(found.Results),
		page.FinalURL,
		page.TotalChars,
		result.Meta["trace_id"],
	)
}

func decodeStructured[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()

	body, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}

	var value T

	err = json.Unmarshal(body, &value)
	if err != nil {
		t.Fatal(err)
	}

	return value
}
