package fetch

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"web-search/internal/adapters/htmltext"
	"web-search/internal/adapters/web"
)

type (
	Input struct {
		URL      string `json:"url"                 jsonschema:"Absolute public HTTP(S) URL to read."`
		Offset   int    `json:"offset,omitempty"    jsonschema:"Character offset; defaults to zero."`
		MaxChars int    `json:"max_chars,omitempty" jsonschema:"Maximum characters; default 20000, maximum 50000."`
	}
	Output struct {
		URL         string          `json:"url"`
		FinalURL    string          `json:"final_url"`
		Title       string          `json:"title"`
		Text        string          `json:"text"`
		Links       []htmltext.Link `json:"links"`
		TotalChars  int             `json:"total_chars"`
		Truncated   bool            `json:"truncated"`
		NextOffset  int             `json:"next_offset"`
		ContentHash string          `json:"content_hash"`
	}
	Getter interface {
		Get(ctx context.Context, raw string) (web.Document, error)
	}
	Service struct {
		http     Getter
		defaults int
		max      int
	}
)

func New(http Getter, defaults, maximum int) *Service {
	return &Service{http: http, defaults: defaults, max: maximum}
}

func (s *Service) Fetch(ctx context.Context, in Input) (Output, error) {
	err := web.ValidateURL(in.URL)
	if err != nil {
		return Output{}, err
	}

	limit := in.MaxChars
	if limit == 0 {
		limit = s.defaults
	}

	if limit < 1 || limit > s.max || in.Offset < 0 {
		return Output{}, fmt.Errorf("invalid offset or max_chars; maximum %d", s.max)
	}

	doc, err := s.http.Get(ctx, in.URL)
	if err != nil {
		return Output{}, err
	}

	page := htmltext.Page{Text: strings.TrimSpace(string(doc.Body)), Links: []htmltext.Link{}}
	if doc.ContentType == "text/html" || doc.ContentType == "application/xhtml+xml" {
		page, err = htmltext.Parse(doc.Body, doc.URL)
		if err != nil {
			return Output{}, err
		}
	}

	if page.Text == "" {
		return Output{}, errors.New("document contains no text")
	}

	chars := []rune(page.Text)
	if in.Offset > len(chars) {
		return Output{}, errors.New("offset exceeds document length")
	}

	end := min(in.Offset+limit, len(chars))

	next := 0
	if end < len(chars) {
		next = end
	}

	return Output{
		URL:         in.URL,
		FinalURL:    doc.URL,
		Title:       page.Title,
		Text:        string(chars[in.Offset:end]),
		Links:       page.Links,
		TotalChars:  len(chars),
		Truncated:   end < len(chars),
		NextOffset:  next,
		ContentHash: fmt.Sprintf("%x", sha256.Sum256([]byte(page.Text))),
	}, nil
}
