# Connecting to pi

This guide assumes pi coding agent with built-in MCP support. The project does not modify pi configuration automatically. Official pi documentation: [MCP Servers](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/mcp.md).

First, run these commands from the project directory:

```sh
task install
task build
```

## Option 1: a separate HTTP process

Start the application and leave it running:

```sh
./bin/web-search -config config.yaml
```

Register the local endpoint:

```sh
pi mcp add local-web-search --url http://127.0.0.1:8080/mcp
pi mcp list
```

Alternatively, add an entry to `~/.pi/agent/mcp.json` manually, preserving any existing entries:

```json
{
  "mcpServers": {
    "local-web-search": {
      "url": "http://127.0.0.1:8080/mcp",
      "timeout": 60,
      "description": "Search public documentation with DuckDuckGo and read public pages locally"
    }
  }
}
```

If you change the port in YAML, update the URL in the pi configuration. The server uses Streamable HTTP at `/mcp`.

## Option 2: pi starts the server through stdio

Replace `/absolute/path/to/web-search` with the absolute path to your project directory:

```sh
WEB_SEARCH_DIR="/absolute/path/to/web-search"
pi mcp add local-web-search -- "$WEB_SEARCH_DIR/bin/web-search" -config "$WEB_SEARCH_DIR/config.yaml" -transport stdio
```

Logs go to stderr by default. When `logging.output: file` is enabled, a relative `logging.directory` is resolved from the process's working directory. To keep log files in a consistent location, set `cwd` manually or use an absolute `logging.directory` in YAML. Replace `/absolute/path/to/web-search` in every field below with your own path:

```json
{
  "mcpServers": {
    "local-web-search": {
      "command": "/absolute/path/to/web-search/bin/web-search",
      "args": [
        "-config", "/absolute/path/to/web-search/config.yaml",
        "-transport", "stdio"
      ],
      "cwd": "/absolute/path/to/web-search",
      "timeout": 60,
      "description": "Search public documentation with DuckDuckGo and read public pages locally"
    }
  }
}
```

Choose one configuration option for the `local-web-search` entry. In stdio mode, pi manages the server process's lifecycle, so you do not need to start the application separately.

## Verification

```sh
pi mcp list
```

In an open pi session, run `/reload`, then `/mcp` to check the connection. Two tools should be available:

- `mcp__local_web_search__web_search`
- `mcp__local_web_search__web_fetch`

Example task for the agent: “Find the official pgx v5 documentation for connection pools, open the relevant URL, and show a configuration example.”

The agent first calls `web_search`, then `web_fetch` for the selected URL. The tools do not receive conversation history or project files, except for text the agent explicitly includes in their arguments. Search queries should contain only public data.

With HTTP, console logs appear in the server's terminal. With stdio, pi receives stderr; how to view it depends on the client version. To save logs, enable `logging.output: file` in YAML. Records will be written to `logging.directory` (`logs` by default). Use the `trace_id` returned in the tool result's `_meta.trace_id` to find a particular request. See [logging](../README.md#logging) for details.

If `pi mcp` is unavailable, check your pi version and its documentation: this guide assumes built-in MCP support. If search fails, start by running `task test-live`. CAPTCHAs, HTTP 429 responses, and changes to DuckDuckGo's result markup are reported as tool errors.
