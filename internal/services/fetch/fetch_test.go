package fetch_test

import (
	"context"
	"errors"
	"testing"

	"web-search/internal/adapters/web"
	"web-search/internal/services/fetch"
)

const greeting = "Привет мир"

type getter struct {
	body, media string
	fail        bool
}

func (g getter) Get(_ context.Context, _ string) (web.Document, error) {
	if g.fail {
		return web.Document{}, errors.New("failed")
	}

	return web.Document{URL: "https://example.com/final", ContentType: g.media, Body: []byte(g.body)}, nil
}

func TestFetch(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		g    getter
		in   fetch.Input
		want string
		next int
		fail bool
	}{
		{
			"html",
			getter{body: `<title>Docs</title>` +
				`<main>` +
				`<p>Привет мир</p>` +
				`</main>`, media: "text/html"},
			fetch.Input{URL: "https://example.com", MaxChars: 6},
			"Привет", 6, false,
		},
		{"continuation", getter{body: greeting, media: "text/plain"}, fetch.Input{URL: "https://example.com", Offset: 7}, "мир", 0, false},
		{"private", getter{}, fetch.Input{URL: "http://127.0.0.1"}, "", 0, true},
		{"limit", getter{}, fetch.Input{URL: "https://example.com", MaxChars: 101}, "", 0, true},
		{"offset", getter{body: "text"}, fetch.Input{URL: "https://example.com", Offset: 9}, "", 0, true},
		{"upstream", getter{fail: true}, fetch.Input{URL: "https://example.com"}, "", 0, true},
		{"empty", getter{body: "<script></script>", media: "text/html"}, fetch.Input{URL: "https://example.com"}, "", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := fetch.New(tc.g, 50, 100).Fetch(context.Background(), tc.in)
			if (err != nil) != tc.fail {
				t.Fatalf("%+v %v", out, err)
			}

			if !tc.fail && (out.Text != tc.want || out.NextOffset != tc.next || len(out.ContentHash) != 64) {
				t.Fatalf("%+v", out)
			}
		})
	}
}
