package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"web-search/internal/config"
	"web-search/internal/observability"
)

func TestLogging(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, output string
		withTrace    bool
	}{
		{name: "console request", output: "console", withTrace: true},
		{name: "console background", output: "console"},
		{name: "file request", output: "file", withTrace: true},
		{name: "file background", output: "file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stderr bytes.Buffer

			cfg := config.Default().Logging
			cfg.Output = tc.output
			cfg.Directory = filepath.Join(t.TempDir(), "logs")

			logger, closer, err := observability.New(cfg, &stderr)
			if err != nil {
				t.Fatal(err)
			}

			ctx := t.Context()
			if tc.withTrace {
				ctx = observability.WithTrace(ctx)
			}

			logger.With("service", "mcp").InfoContext(ctx, "test", "duration_ms", 1)

			err = closer.Close()
			if err != nil {
				t.Fatal(err)
			}

			body := readOutput(t, cfg, &stderr)

			var record map[string]any

			err = json.Unmarshal(body, &record)
			if err != nil {
				t.Fatal(err)
			}

			id, ok := record["trace_id"].(string)
			if !ok || len(id) != 32 || tc.withTrace && id != observability.TraceID(ctx) {
				t.Fatal(record)
			}

			if record["service"] != "mcp" || record["msg"] != "test" {
				t.Fatal(record)
			}
		})
	}
}

func TestNewErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, output     string
		blockedDirectory bool
	}{
		{name: "invalid output", output: "other"},
		{name: "missing directory", output: "file"},
		{name: "blocked directory", output: "file", blockedDirectory: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := config.Default().Logging
			cfg.Output = tc.output

			cfg.Directory = ""
			if tc.blockedDirectory {
				cfg.Directory = filepath.Join(t.TempDir(), "not-a-directory")

				err := os.WriteFile(cfg.Directory, []byte("occupied"), 0o600)
				if err != nil {
					t.Fatal(err)
				}
			}

			_, closer, err := observability.New(cfg, io.Discard)
			if err == nil {
				closer.Close()
				t.Fatal("invalid output accepted")
			}
		})
	}
}

func TestDisabledLevel(t *testing.T) {
	t.Parallel()

	cfg := config.Default().Logging
	cfg.Level = slog.LevelError

	l, closer, err := observability.New(cfg, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	if l.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info enabled")
	}
}

func readLog(t *testing.T, directory string) []byte {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(directory, "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%v error=%v", files, err)
	}

	body, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}

	return body
}

func readOutput(t *testing.T, cfg config.Logging, stderr *bytes.Buffer) []byte {
	t.Helper()

	if cfg.Output == "file" {
		if stderr.Len() != 0 {
			t.Fatal("file output also wrote to console")
		}

		return readLog(t, cfg.Directory)
	}

	_, err := os.Stat(cfg.Directory)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("console output created log directory: %v", err)
	}

	return stderr.Bytes()
}
