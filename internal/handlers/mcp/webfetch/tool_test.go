package webfetch_test

import (
	"context"
	"errors"
	"testing"

	"web-search/internal/handlers/mcp/webfetch"
	"web-search/internal/services/fetch"
)

type fetcher struct{ fail bool }

func (f fetcher) Fetch(_ context.Context, in fetch.Input) (fetch.Output, error) {
	if f.fail {
		return fetch.Output{}, errors.New("failed")
	}

	return fetch.Output{URL: in.URL}, nil
}

func TestHandle(t *testing.T) {
	t.Parallel()

	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "error"}[fail], func(t *testing.T) {
			t.Parallel()

			_, out, err := webfetch.New(fetcher{fail: fail}).Handle(context.Background(), nil, fetch.Input{URL: "https://go.dev"})
			if (err != nil) != fail || !fail && out.URL != "https://go.dev" {
				t.Fatalf("%+v %v", out, err)
			}
		})
	}
}
