package search

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"web-search/internal/adapters/duckduckgo"
)

const maxDomains = 5

type (
	Input struct {
		Query   string   `json:"query"             jsonschema:"Public search terms. Do not include secrets or private project content."`
		Domains []string `json:"domains,omitempty" jsonschema:"Optional domains to restrict the search."`
		Limit   int      `json:"limit,omitempty"   jsonschema:"Maximum results; default 5, maximum 10."`
	}
	Output struct {
		Provider string              `json:"provider"`
		Results  []duckduckgo.Result `json:"results"`
	}
	Provider interface {
		Search(ctx context.Context, query string, limit int) ([]duckduckgo.Result, error)
	}
	Service struct {
		provider Provider
		interval time.Duration
		defaults int
		max      int
		gate     chan struct{}
		last     time.Time
	}
)

func New(provider Provider, interval time.Duration, defaults, maximum int) *Service {
	return &Service{provider: provider, interval: interval, defaults: defaults, max: maximum, gate: make(chan struct{}, 1)}
}

func (s *Service) Search(ctx context.Context, in Input) (Output, error) {
	query, limit, err := s.prepare(in)
	if err != nil {
		return Output{}, err
	}

	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return Output{}, ctx.Err()
	}

	defer func() { <-s.gate }()

	wait := time.Until(s.last.Add(s.interval))
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()

		select {
		case <-timer.C:
		case <-ctx.Done():
			return Output{}, ctx.Err()
		}
	}

	s.last = time.Now()

	results, err := s.provider.Search(ctx, query, limit)
	if err != nil {
		return Output{}, err
	}

	results = filterResults(results, in.Domains)

	return Output{Provider: "duckduckgo", Results: results}, nil
}

func validDomain(s string) bool {
	if s == "" || len(s) > 253 || !strings.Contains(s, ".") {
		return false
	}

	for label := range strings.SplitSeq(s, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}

		for _, r := range label {
			if r != '-' && (r >= 128 || !unicode.IsLetter(r) && !unicode.IsDigit(r)) {
				return false
			}
		}
	}

	return true
}

func (s *Service) prepare(in Input) (query string, limit int, err error) {
	query = strings.TrimSpace(in.Query)
	if query == "" || len(query) > 1000 {
		return "", 0, errors.New("query must contain 1–1000 bytes")
	}

	if strings.Contains(query, "!") || strings.ContainsAny(query, "\r\n\x00") {
		return "", 0, errors.New("bang redirects and control characters are not allowed")
	}

	limit = in.Limit
	if limit == 0 {
		limit = s.defaults
	}

	if limit < 1 || limit > s.max {
		return "", 0, fmt.Errorf("limit must be between 1 and %d", s.max)
	}

	if len(in.Domains) > maxDomains {
		return "", 0, errors.New("at most five domains are allowed")
	}

	var sites []string

	for _, domain := range in.Domains {
		domain = strings.ToLower(domain)
		if !validDomain(domain) {
			return "", 0, fmt.Errorf("invalid domain: %s", domain)
		}

		sites = append(sites, "site:"+domain)
	}

	if len(sites) > 0 {
		query += " (" + strings.Join(sites, " OR ") + ")"
	}

	return query, limit, nil
}

func filterResults(results []duckduckgo.Result, domains []string) []duckduckgo.Result {
	if len(domains) == 0 {
		return results
	}

	filtered := make([]duckduckgo.Result, 0, len(results))
	for _, result := range results {
		u, err := url.Parse(result.URL)
		if err != nil {
			continue
		}

		if matchesDomain(u.Hostname(), domains) {
			filtered = append(filtered, result)
		}
	}

	return filtered
}

func matchesDomain(host string, domains []string) bool {
	host = strings.ToLower(host)
	for _, domain := range domains {
		domain = strings.ToLower(domain)
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}

	return false
}
