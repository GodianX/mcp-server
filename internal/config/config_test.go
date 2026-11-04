package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"web-search/internal/config"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, body string
		fail       bool
	}{
		{"defaults", "{}", false},
		{"partial", "search:\n  default_limit: 3", false},
		{"unknown", "typo: true", true},
		{"port", "server:\n  port: 0", true},
		{"host", "server:\n  host: 0.0.0.0", true},
		{"duration", "http:\n  timeout: -1s", true},
		{"documents", "{}\n---\n{}", true},
		{"limits", "fetch:\n  max_chars: 1", true},
		{"transport", "server:\n  transport: sse", true},
		{"console", "logging:\n  output: console\n  directory: ''", false},
		{"file", "logging:\n  output: file", false},
		{"logging output", "logging:\n  output: other", true},
		{"file directory", "logging:\n  output: file\n  directory: ''", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "config.yaml")

			err := os.WriteFile(path, []byte(tc.body), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			c, err := config.Load(path)
			if (err != nil) != tc.fail {
				t.Fatalf("config=%+v error=%v", c, err)
			}

			if !tc.fail && c.Server.Host != "127.0.0.1" {
				t.Fatal(c)
			}
		})
	}
}

func TestShippedDefaults(t *testing.T) {
	t.Parallel()

	c, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if c != config.Default() {
		t.Fatalf("shipped YAML differs from defaults: %+v", c)
	}
}
