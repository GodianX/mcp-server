package htmltext

import (
	"errors"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

const (
	preTag   = "pre"
	maxLinks = 200
)

type (
	Link struct {
		Text string `json:"text"`
		URL  string `json:"url"`
	}
	Page struct {
		Title string
		Text  string
		Links []Link
	}
	renderer struct {
		out  strings.Builder
		page Page
		base *url.URL
		seen map[string]bool
	}
)

func Parse(body []byte, baseURL string) (Page, error) {
	root, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return Page{}, err
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return Page{}, err
	}

	content, title := selectContent(root)
	r := renderer{page: Page{Title: title, Links: []Link{}}, base: base, seen: map[string]bool{}}
	r.walk(content, false)

	r.page.Text = normalizeText(r.out.String())
	if r.page.Text == "" {
		return Page{}, errors.New("page has no readable text; JavaScript rendering is not supported")
	}

	return r.page, nil
}

func Attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}

	return ""
}

func Text(n *html.Node) string {
	var (
		out  strings.Builder
		walk func(*html.Node)
	)

	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			out.WriteString(node.Data)
			out.WriteByte(' ')
		}

		for ch := node.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)

	return strings.Join(strings.Fields(out.String()), " ")
}

func selectContent(root *html.Node) (content *html.Node, title string) {
	content = root

	var main *html.Node

	title = ""

	var find func(*html.Node)

	find = func(n *html.Node) {
		switch {
		case n.Type != html.ElementNode:
		case n.Data == "title":
			title = Text(n)
		case n.Data == "body":
			content = n
		case n.Data == "main":
			main = n
		case n.Data == "article" && main == nil:
			main = n
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(root)

	if main != nil {
		content = main
	}

	return content, title
}

func (r *renderer) walk(n *html.Node, pre bool) {
	if n.Type == html.TextNode {
		r.writeText(n.Data, pre)
		return
	}

	if n.Type == html.ElementNode && !visible(n) {
		return
	}

	if n.Type == html.ElementNode {
		r.start(n)
	}

	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		r.walk(ch, pre || n.Data == preTag)
	}

	if n.Type == html.ElementNode {
		r.end(n)
	}
}

func (r *renderer) writeText(text string, pre bool) {
	if pre {
		r.out.WriteString(text)
		return
	}

	text = strings.Join(strings.Fields(text), " ")
	if text != "" {
		r.out.WriteString(text)
		r.out.WriteByte(' ')
	}
}

func (r *renderer) start(n *html.Node) {
	if blockNode(n.Data) {
		r.out.WriteString("\n\n")
	}

	if len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
		r.out.WriteString(strings.Repeat("#", int(n.Data[1]-'0')) + " ")
	}

	switch n.Data {
	case "li":
		r.out.WriteString("- ")
	case "br":
		r.out.WriteByte('\n')
	case preTag:
		r.out.WriteString("```\n")
	case "a":
		r.addLink(n)
	}
}

func (r *renderer) end(n *html.Node) {
	switch n.Data {
	case "td", "th":
		r.out.WriteString(" | ")
	case preTag:
		r.out.WriteString("\n```\n")
	}

	if blockNode(n.Data) {
		r.out.WriteString("\n\n")
	}
}

func (r *renderer) addLink(n *html.Node) {
	href := Attr(n, "href")
	if href == "" || len(r.page.Links) >= maxLinks {
		return
	}

	u, err := url.Parse(href)
	if err != nil {
		return
	}

	u = r.base.ResolveReference(u)
	if u.Scheme != "http" && u.Scheme != "https" {
		return
	}

	if r.seen[u.String()] {
		return
	}

	r.page.Links = append(r.page.Links, Link{Text: Text(n), URL: u.String()})
	r.seen[u.String()] = true
}

func visible(n *html.Node) bool {
	switch n.Data {
	case "script", "style", "noscript", "nav", "footer", "header", "aside", "form", "svg", "video", "audio", "iframe", "template":
		return false
	}

	for _, a := range n.Attr {
		if a.Key == "hidden" || a.Key == "aria-hidden" && a.Val == "true" {
			return false
		}
	}

	return true
}

func blockNode(tag string) bool {
	switch tag {
	case "p", "div", "section", "article", "main", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", preTag, "blockquote", "tr":
		return true
	default:
		return false
	}
}

func normalizeText(text string) string {
	lines := strings.Split(text, "\n")
	clean := make([]string, 0, len(lines))
	blank, inCode := false, false

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCode = !inCode
		}

		if !inCode {
			line = strings.TrimSpace(line)
		}

		if strings.TrimSpace(line) == "" {
			if !blank {
				clean = append(clean, "")
			}

			blank = true

			continue
		}

		clean = append(clean, line)
		blank = false
	}

	return strings.TrimSpace(strings.Join(clean, "\n"))
}
