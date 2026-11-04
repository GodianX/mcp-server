package websearch_test

import (
	"context"
	"errors"
	"testing"

	"web-search/internal/handlers/mcp/websearch"
	"web-search/internal/services/search"
)

type searcher struct{ fail bool }

func (s searcher) Search(_ context.Context, in search.Input) (search.Output, error) {
	if s.fail {
		return search.Output{}, errors.New("failed")
	}

	return search.Output{Provider: in.Query}, nil
}

func TestHandle(t *testing.T) {
	t.Parallel()

	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "error"}[fail], func(t *testing.T) {
			t.Parallel()

			_, out, err := websearch.New(searcher{fail: fail}).Handle(context.Background(), nil, search.Input{Query: "docs"})
			if (err != nil) != fail || !fail && out.Provider != "docs" {
				t.Fatalf("%+v %v", out, err)
			}
		})
	}
}
