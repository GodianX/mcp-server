package duckduckgo_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"web-search/internal/adapters/duckduckgo"
	"web-search/internal/adapters/web"
)

type getter struct {
	t    *testing.T
	fail bool
}

func (g getter) Get(_ context.Context, raw string) (web.Document, error) {
	u, err := url.Parse(raw)
	if err != nil {
		g.t.Fatal(err)
	}

	if u.Host != "html.duckduckgo.com" || u.Query().Get("q") != "pgx docs" {
		g.t.Fatal(raw)
	}

	if g.fail {
		return web.Document{}, errors.New("upstream unavailable")
	}

	return web.Document{
		Body: []byte(`<div class="result"><a class="result__a" href="https://go.dev/">Go</a><a class="result__snippet">Docs</a></div>`),
	}, nil
}

func TestParse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, body string
		count      int
		fail       bool
	}{
		{"results", `<div class="result">` +
			`<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgo.dev%2F">Go</a>` +
			`<a class="result__snippet">Language docs</a>` +
			`</div>` +
			`<a class="result__a" href="https://go.dev/">Duplicate</a>`, 1, false},
		{"empty", `<div class="no-results">No results found</div>`, 0, false},
		{"blocked", `<form action="anomaly.js">challenge</form>`, 0, true},
		{"changed", `<h1>Unexpected markup</h1>`, 0, true},
		{"captcha documentation", `<div class="result">` +
			`<a class="result__a" href="https://go.dev/">Go</a>` +
			`<a class="result__snippet">Language docs about CAPTCHA</a>` +
			`</div>`, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := duckduckgo.Parse([]byte(tc.body), 5)
			if (err != nil) != tc.fail || len(out) != tc.count {
				t.Fatalf("results=%+v err=%v", out, err)
			}

			if len(out) > 0 && (out[0].URL != "https://go.dev/" || !strings.HasPrefix(out[0].Snippet, "Language docs")) {
				t.Fatal(out)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()

	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "error"}[fail], func(t *testing.T) {
			t.Parallel()

			out, err := duckduckgo.New(getter{t: t, fail: fail}).Search(context.Background(), "pgx docs", 5)
			if (err != nil) != fail || !fail && len(out) != 1 {
				t.Fatalf("%+v %v", out, err)
			}
		})
	}
}
