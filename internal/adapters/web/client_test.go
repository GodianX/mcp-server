package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"web-search/internal/adapters/web"
)

func TestValidateURL(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		url   string
		valid bool
	}{
		{"https://go.dev/doc/", true},
		{"http://example.com", true},
		{"https://example.com:443/a", true},
		{"file:///etc/passwd", false},
		{"http://user:secret@example.com", false},
		{"http://127.0.0.1", false},
		{"http://[::ffff:127.0.0.1]", false},
		{"http://169.254.169.254", false},
		{"http://localhost", false},
		{"http://x.localhost", false},
		{"https://example.com:9200", false},
		{"/relative", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()

			err := web.ValidateURL(tc.url)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestPublicIP(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		ip     string
		public bool
	}{
		{"8.8.8.8", true},
		{"2606:4700:4700::1111", true},
		{"10.1.2.3", false},
		{"192.168.0.1", false},
		{"100.64.1.1", false},
		{"0.0.0.0", false},
		{"::1", false},
		{"fc00::1", false},
		{"2002:7f00:1::", false},
		{"64:ff9b::7f00:1", false},
		{"198.18.1.2", false},
	} {
		t.Run(tc.ip, func(t *testing.T) {
			t.Parallel()

			if web.PublicIP(netip.MustParseAddr(tc.ip)) != tc.public {
				t.Fatal(tc)
			}
		})
	}
}

func TestGetRejectsPrivate(t *testing.T) {
	t.Parallel()

	c := web.New(time.Second, 1024, 2, slog.New(slog.DiscardHandler))
	defer c.Close()

	_, err := c.Get(context.Background(), "http://127.0.0.1/")
	if err == nil {
		t.Fatal("private destination accepted")
	}
}

func TestReadDocument(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, media, body string
		status            int
		fail              bool
	}{
		{"html", "text/html; charset=utf-8", "<p>Привет</p>", 200, false},
		{"text", "text/plain", "Привет", 200, false},
		{"latin", "text/html; charset=windows-1252", "<p>caf\xe9</p>", 200, false},
		{"media", "video/mp4", "binary", 200, true},
		{"missing", "", "text", 200, true},
		{"status", "text/html", "blocked", 429, true},
		{"large", "text/plain", strings.Repeat("x", 1025), 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u, err := url.Parse("https://example.com/")
			if err != nil {
				t.Fatal(err)
			}

			resp := &http.Response{
				StatusCode: tc.status,
				Header:     http.Header{"Content-Type": {tc.media}},
				Body:       io.NopCloser(strings.NewReader(tc.body)),
				Request:    &http.Request{URL: u},
			}
			defer resp.Body.Close()

			doc, err := web.ReadDocument(resp, 1024)
			if (err != nil) != tc.fail {
				t.Fatalf("%+v %v", doc, err)
			}

			if tc.name == "text" && string(doc.Body) != "Привет" || tc.name == "latin" && string(doc.Body) != "<p>café</p>" {
				t.Fatalf("%q", doc.Body)
			}
		})
	}
}
