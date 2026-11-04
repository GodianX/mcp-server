package duckduckgo

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/net/html"

	"web-search/internal/adapters/htmltext"
	"web-search/internal/adapters/web"
)

type (
	Result struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Snippet string `json:"snippet"`
	}
	Getter interface {
		Get(ctx context.Context, raw string) (web.Document, error)
	}
	Client struct{ http Getter }
)

func New(http Getter) *Client { return &Client{http: http} }

func (c *Client) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	u := "https://html.duckduckgo.com/html/?" + url.Values{"q": {query}}.Encode()

	doc, err := c.http.Get(ctx, u)
	if err != nil {
		return nil, err
	}

	return Parse(doc.Body, limit)
}

func Parse(body []byte, limit int) ([]Result, error) {
	raw := strings.ToLower(string(body))
	for _, marker := range []string{"anomaly.js", "anomaly-modal", `id="captcha"`, `class="captcha-container"`} {
		if strings.Contains(raw, marker) {
			return nil, errors.New("DuckDuckGo blocked the request with a challenge; try again later")
		}
	}

	root, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	results := []Result{}
	seen := map[string]bool{}
	collectResults(root, limit, &results, seen)

	if len(results) == 0 && !strings.Contains(raw, "no-results") && !strings.Contains(raw, "no results found") {
		return nil, errors.New("unrecognized DuckDuckGo response; markup may have changed")
	}

	return results, nil
}

func class(n *html.Node, value string) bool {
	return slices.Contains(strings.Fields(htmltext.Attr(n, "class")), value)
}

func collectResults(n *html.Node, limit int, results *[]Result, seen map[string]bool) {
	if len(*results) >= limit {
		return
	}

	if n.Type == html.ElementNode && n.Data == "a" && class(n, "result__a") {
		result, err := parseResult(n)
		if err == nil && !seen[result.URL] {
			*results = append(*results, result)
			seen[result.URL] = true
		}
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectResults(child, limit, results, seen)
	}
}

func parseResult(n *html.Node) (Result, error) {
	u, err := url.Parse(htmltext.Attr(n, "href"))
	if err != nil {
		return Result{}, err
	}

	if (u.Hostname() == "duckduckgo.com" || u.Hostname() == "html.duckduckgo.com") && u.Query().Get("uddg") != "" {
		u, err = url.Parse(u.Query().Get("uddg"))
		if err != nil {
			return Result{}, err
		}
	}

	err = web.ValidateURL(u.String())
	if err != nil {
		return Result{}, err
	}

	container := n.Parent
	for container != nil && !class(container, "result") {
		container = container.Parent
	}

	return Result{Title: htmltext.Text(n), URL: u.String(), Snippet: findSnippet(container)}, nil
}

func findSnippet(n *html.Node) string {
	if n == nil {
		return ""
	}

	if class(n, "result__snippet") {
		return htmltext.Text(n)
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		text := findSnippet(child)
		if text != "" {
			return text
		}
	}

	return ""
}
