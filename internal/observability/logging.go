package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"web-search/internal/config"
)

const (
	directoryPermissions = 0o700
	filePermissions      = 0o600
)

type (
	traceKey      struct{}
	consoleOutput struct{ io.Writer }
	Handler       struct {
		next     slog.Handler
		fallback string
	}
)

func New(cfg config.Logging, stderr io.Writer) (*slog.Logger, io.Closer, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, nil, err
	}

	output, err := newOutput(cfg, stderr)
	if err != nil {
		return nil, nil, err
	}

	h := slog.NewJSONHandler(output, &slog.HandlerOptions{Level: cfg.Level})

	return slog.New(&Handler{next: h, fallback: NewTraceID()}), output, nil
}

func NewTraceID() string {
	var b [16]byte

	_, err := rand.Read(b[:])
	if err != nil {
		panic(err)
	}

	return hex.EncodeToString(b[:])
}

func WithTrace(ctx context.Context) context.Context {
	return context.WithValue(ctx, traceKey{}, NewTraceID())
}

func TraceID(ctx context.Context) string {
	id, ok := ctx.Value(traceKey{}).(string)
	if !ok {
		return ""
	}

	return id
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	id := TraceID(ctx)
	if id == "" {
		id = h.fallback
	}

	r.AddAttrs(slog.String("trace_id", id))

	return h.next.Handle(ctx, r)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{next: h.next.WithAttrs(attrs), fallback: h.fallback}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name), fallback: h.fallback}
}

func (c consoleOutput) Close() error { return nil }

func newOutput(cfg config.Logging, stderr io.Writer) (io.WriteCloser, error) {
	if cfg.Output == "console" {
		return consoleOutput{Writer: stderr}, nil
	}

	err := os.MkdirAll(cfg.Directory, directoryPermissions)
	if err != nil {
		return nil, err
	}

	return os.OpenFile(
		filepath.Clean(filepath.Join(cfg.Directory, "mcp-"+time.Now().UTC().Format("20060102T150405.000000000")+".jsonl")),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		filePermissions,
	)
}
