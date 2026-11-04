package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

const httpsScheme = "https"

type (
	Document struct {
		URL         string
		ContentType string
		Body        []byte
	}
	Client struct {
		client   *http.Client
		maxBytes int64
		logger   *slog.Logger
	}
)

func New(timeout time.Duration, maxBytes int64, maxRedirects int, logger *slog.Logger) *Client {
	transport := &http.Transport{
		Proxy: nil, MaxIdleConns: 4, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 2,
		IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		DialContext: safeDial,
	}
	client := &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: redirectPolicy(maxRedirects)}

	return &Client{client: client, maxBytes: maxBytes, logger: logger}
}

func PublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}

	for _, prefix := range []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24",
		"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		"2001:db8::/32", "2001::/32", "2002::/16", "64:ff9b::/96",
	} {
		if netip.MustParsePrefix(prefix).Contains(ip) {
			return false
		}
	}

	return true
}

func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid URL")
	}

	if (u.Scheme != "http" && u.Scheme != httpsScheme) || u.Hostname() == "" || u.User != nil || len(raw) > 8192 {
		return errors.New("only absolute HTTP(S) URLs without credentials are allowed")
	}

	if u.Port() != "" && u.Port() != "80" && u.Port() != "443" {
		return errors.New("only ports 80 and 443 are allowed")
	}

	ip, err := netip.ParseAddr(u.Hostname())
	if err == nil && !PublicIP(ip) {
		return errors.New("non-public destination rejected")
	}

	if strings.EqualFold(u.Hostname(), "localhost") || strings.HasSuffix(strings.ToLower(u.Hostname()), ".localhost") {
		return errors.New("localhost rejected")
	}

	return nil
}

func (c *Client) Get(ctx context.Context, raw string) (Document, error) {
	err := ValidateURL(raw)
	if err != nil {
		return Document{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, http.NoBody)
	if err != nil {
		return Document{}, err
	}

	req.Header.Set("User-Agent", "LocalWebSearch/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,text/markdown;q=0.9")

	start := time.Now()

	c.logger.InfoContext(ctx, "http.request", "url", raw)

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.ErrorContext(ctx, "http.error", "error", err.Error())
		return Document{}, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	c.logger.InfoContext(
		ctx,
		"http.response",
		"url",
		resp.Request.URL.String(),
		"status",
		resp.StatusCode,
		"duration_ms",
		time.Since(start).Milliseconds(),
	)

	return ReadDocument(resp, c.maxBytes)
}

func ReadDocument(resp *http.Response, maxBytes int64) (Document, error) {
	if resp.StatusCode != http.StatusOK {
		return Document{}, fmt.Errorf("upstream HTTP %d", resp.StatusCode)
	}

	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return Document{}, errors.New("missing or invalid Content-Type")
	}

	if media != "text/html" && media != "application/xhtml+xml" && media != "text/plain" && media != "text/markdown" {
		return Document{}, fmt.Errorf("unsupported Content-Type: %s", media)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return Document{}, err
	}

	if int64(len(body)) > maxBytes {
		return Document{}, errors.New("response exceeds byte limit")
	}

	if (media == "text/plain" || media == "text/markdown") &&
		!strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "charset=") {
		return Document{URL: resp.Request.URL.String(), ContentType: media, Body: body}, nil
	}

	reader, err := charset.NewReader(strings.NewReader(string(body)), resp.Header.Get("Content-Type"))
	if err != nil {
		return Document{}, err
	}

	body, err = io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return Document{}, err
	}

	if int64(len(body)) > maxBytes {
		return Document{}, errors.New("decoded response exceeds byte limit")
	}

	return Document{URL: resp.Request.URL.String(), ContentType: media, Body: body}, nil
}

func (c *Client) Close() { c.client.CloseIdleConnections() }

func safeDial(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}

	if len(ips) == 0 {
		return nil, errors.New("host has no addresses")
	}

	for _, ip := range ips {
		if !PublicIP(ip) {
			return nil, errors.New("destination resolves to a non-public address")
		}
	}

	var last error

	for _, ip := range ips {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}

		last = dialErr
	}

	return nil, last
}

func redirectPolicy(maxRedirects int) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		req.Header.Del("Referer")

		if len(via) > maxRedirects {
			return errors.New("redirect limit exceeded")
		}

		err := ValidateURL(req.URL.String())
		if err != nil {
			return err
		}

		if via[0].URL.Scheme == httpsScheme && req.URL.Scheme != httpsScheme {
			return errors.New("HTTPS downgrade rejected")
		}

		return nil
	}
}
