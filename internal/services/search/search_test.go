package search_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"web-search/internal/adapters/duckduckgo"
	"web-search/internal/services/search"
)

type provider struct {
	t      *testing.T
	fail   bool
	domain bool
}

func (p provider) Search(_ context.Context, q string, limit int) ([]duckduckgo.Result, error) {
	if limit != 5 {
		p.t.Fatalf("limit=%d", limit)
	}

	if p.domain && !strings.Contains(q, "site:go.dev") {
		p.t.Fatal(q)
	}

	if p.fail {
		return nil, errors.New("failed")
	}

	return []duckduckgo.Result{
		{URL: "https://go.dev/doc/", Title: "Go"},
		{URL: "https://example.com", Title: "Other"},
	}, nil
}

func TestSearch(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		in           search.Input
		upstreamFail bool
		count        int
		fail         bool
	}{
		{"success", search.Input{Query: "docs"}, false, 2, false},
		{"domain", search.Input{Query: "docs", Domains: []string{"go.dev"}}, false, 1, false},
		{"empty", search.Input{}, false, 0, true},
		{"limit", search.Input{Query: "docs", Limit: 100}, false, 0, true},
		{"invalid domain", search.Input{Query: "docs", Domains: []string{"go.dev/evil"}}, false, 0, true},
		{"bang", search.Input{Query: "!g docs"}, false, 0, true},
		{"upstream", search.Input{Query: "docs"}, true, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := search.New(provider{t: t, fail: tc.upstreamFail, domain: len(tc.in.Domains) > 0}, time.Millisecond, 5, 10)

			out, err := s.Search(context.Background(), tc.in)
			if (err != nil) != tc.fail || len(out.Results) != tc.count {
				t.Fatalf("%+v %v", out, err)
			}
		})
	}
}

func TestSearchCancellation(t *testing.T) {
	t.Parallel()
	s := search.New(provider{t: t}, time.Hour, 5, 10)

	_, err := s.Search(context.Background(), search.Input{Query: "docs"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = s.Search(ctx, search.Input{Query: "docs"})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
