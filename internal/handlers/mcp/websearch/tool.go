package websearch

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"web-search/internal/services/search"
)

type (
	Searcher interface {
		Search(ctx context.Context, in search.Input) (search.Output, error)
	}
	Handler struct{ service Searcher }
)

func New(service Searcher) *Handler { return &Handler{service: service} }

func (h *Handler) Register(server *mcp.Server) {
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name: "web_search",
			Description: "Find public web pages using DuckDuckGo ordinary search results, without AI answers. Only " +
				"send public search terms; never include private project data or secrets.",
		},
		h.Handle,
	)
}

func (h *Handler) Handle(ctx context.Context, _ *mcp.CallToolRequest, in search.Input) (*mcp.CallToolResult, search.Output, error) {
	out, err := h.service.Search(ctx, in)
	return nil, out, err
}
