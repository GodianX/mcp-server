package main_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCLI(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(t.TempDir(), "web-search")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./cmd/app")
	build.Dir = root

	body, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %s %v", body, err)
	}

	for _, tc := range []struct {
		name string
		args []string
		fail bool
	}{
		{"help", []string{"-help"}, false},
		{"unknown flag", []string{"-unknown"}, true},
		{"missing config", []string{"-config", "missing.yaml"}, true},
		{"transport", []string{"-config", filepath.Join(root, "config.yaml"), "-transport", "sse"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			command := exec.CommandContext(t.Context(), binary, tc.args...)

			out, runErr := command.CombinedOutput()
			if (runErr != nil) != tc.fail {
				t.Fatalf("%s %v", out, runErr)
			}
		})
	}

	t.Run("stdio", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		command := exec.CommandContext(ctx, binary, "-config", filepath.Join(root, "config.yaml"), "-transport", "stdio")
		command.Dir = root

		session, connectErr := mcp.NewClient(
			&mcp.Implementation{Name: "stdio-test", Version: "1"},
			nil,
		).Connect(
			ctx,
			&mcp.CommandTransport{Command: command},
			nil,
		)
		if connectErr != nil {
			t.Fatal(connectErr)
		}
		defer session.Close()

		tools, listErr := session.ListTools(ctx, nil)
		if listErr != nil || len(tools.Tools) != 2 {
			t.Fatalf("%+v %v", tools, listErr)
		}

		firstID := ""

		for range 2 {
			result, callErr := session.CallTool(
				ctx,
				&mcp.CallToolParams{Name: "web_fetch", Arguments: map[string]any{"url": "http://127.0.0.1"}},
			)
			if callErr != nil || !result.IsError {
				t.Fatalf("%+v %v", result, callErr)
			}

			id, _ := result.Meta["trace_id"].(string)
			if len(id) != 32 || id == firstID {
				body, _ := json.Marshal(result)
				t.Fatalf("invalid trace: %s", body)
			}

			firstID = id
		}
	})
}
