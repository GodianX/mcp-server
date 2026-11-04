package htmltext_test

import (
	"strings"
	"testing"

	"web-search/internal/adapters/htmltext"
)

func TestParse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, html, want, absent string
		fail                     bool
	}{
		{"article", `<title>Docs</title>` +
			`<nav>MENU</nav>` +
			`<main>` +
			`<h1>API</h1>` +
			`<p>Hello <b>world</b>` +
			`</p>` +
			`<a href="/next">Next</a>` +
			`<pre>  code\n    indent</pre>` +
			`<script>secret()</script>` +
			`<audio src="track">` +
			`</audio>` +
			`</main>`, "# API", "MENU", false},
		{"fallback", `<html><body><p>Content &amp; text</p><div hidden>HIDDEN</div></body></html>`, "Content & text", "HIDDEN", false},
		{"empty", `<script>render()</script>`, "", "", true},
		{"malformed", `<article><p>Unclosed <b>text`, "Unclosed text", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, err := htmltext.Parse([]byte(tc.html), "https://example.com/docs")
			if (err != nil) != tc.fail {
				t.Fatal(err)
			}

			if tc.fail {
				return
			}

			if !strings.Contains(p.Text, tc.want) || tc.absent != "" && strings.Contains(p.Text, tc.absent) ||
				strings.Contains(p.Text, "secret()") {
				t.Fatalf("%+v", p)
			}

			if tc.name == "article" &&
				(len(p.Links) != 1 || p.Links[0].URL != "https://example.com/next" || !strings.Contains(p.Text, "  code")) {
				t.Fatalf("%+v", p)
			}
		})
	}
}
