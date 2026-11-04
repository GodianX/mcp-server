# Local Web Search MCP

A local MCP server written in Go **1.27.3** for finding public web pages through DuckDuckGo and extracting their text. The application runs locally as a regular process.

Agent setup: [docs/PI.md](docs/PI.md).

## Getting started

Requirements: Go 1.27.3 and Task. Run all commands from the project directory.

```sh
task install
task build
./bin/web-search -config config.yaml
```

`task install` downloads the application and tool dependencies. The golangci-lint version is pinned in a separate `tools/go.mod`; commands run it through `go tool -modfile=tools/go.mod`. Building may contact the Go module proxy and checksum database. These requests are separate from the running server's network activity.

MCP endpoint: `http://127.0.0.1:8080/mcp`. Alternatively, start the application with `task run`.

```sh
./bin/web-search -config config.yaml -transport stdio
```

In stdio mode, stdout contains only MCP messages. JSON logs go to stderr by default; file logging can be enabled through YAML. SIGINT/SIGTERM shut down the application.

## Tools

### web_search

```json
{"query":"pgx v5 connection pool documentation","domains":["pkg.go.dev","github.com"],"limit":5}
```

Returns `provider` and `results`, with `title`, `url`, and `snippet` for each result. `domains` is optional and accepts up to five domains. The tool uses DuckDuckGo's HTML interface without executing JavaScript, extracting only ordinary links and snippets, without Search Assist or Duck.ai. No accounts or API keys are required. `!bang` redirects are rejected.

### web_fetch

```json
{"url":"https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool","offset":0,"max_chars":20000}
```

Returns `url`, `final_url`, `title`, `text`, `links`, `total_chars`, `truncated`, `next_offset`, and `content_hash`. Supports HTML/XHTML, plain text, and Markdown, preserving text structure, tables, lists, and code blocks. The link list is limited to 200 URLs.

`offset` and `max_chars` count Unicode characters. When `truncated=true`, use `next_offset` in the next call. Each call downloads the URL again: restart reading if `content_hash` changes. The tool does not automatically crawl links or index pages.

Images, stylesheets, scripts, video, and audio are not downloaded. PDF parsing, website authentication, and JavaScript rendering are outside the scope of the first version.

## Configuration

[config.yaml](config.yaml) contains all default values for a single local agent without subagents. Unknown fields and invalid values cause startup to fail. Omitted fields use their default values.

| Setting | Default | Purpose |
|---|---|---|
| `server.transport` | `http` | `http` or `stdio`; the `-transport` flag overrides this value |
| `server.host`, `server.port` | `127.0.0.1`, `8080` | Loopback IP addresses only |
| `server.request_timeout` | `45s` | MCP request processing, including waiting for an available execution slot |
| `server.shutdown_timeout` | `5s` | Graceful HTTP server shutdown |
| `server.max_concurrent_requests` | `2` | Maximum number of tools executing concurrently |
| `server.max_request_bytes` | `16384` | Maximum incoming MCP message size |
| `http.timeout` | `20s` | Total timeout for an outgoing HTTP request |
| `http.max_response_bytes` | `5242880` | Maximum 5 MiB of original and decoded text |
| `http.max_redirects` | `5` | Redirect limit |
| `search.interval` | `2s` | Minimum interval between searches; only one search runs at a time |
| `search.default_limit`, `search.max_limit` | `5`, `10` | Number of search results |
| `fetch.default_max_chars`, `fetch.max_chars` | `20000`, `50000` | Text chunk size |
| `logging.level` | `INFO` | slog level |
| `logging.output` | `console` | Output to `console` (stderr) or `file` |
| `logging.directory` | `logs` | JSONL file directory when `output: file` |
| `logging.log_responses` | `false` | Log complete successful tool responses |

Relative paths are resolved from the process's working directory. Go and golangci-lint use their standard system caches; no cache directory inside the project is required.

## Logging

Both output modes write JSON logs through `slog`. Console output is the default:

```yaml
logging:
  level: INFO
  output: console
  directory: logs
  log_responses: false
```

In `console` mode, records go to stderr. The `logs` directory and log files are not created. This also reserves stdout for the MCP protocol when using stdio.

To enable file logging, change the configuration:

```yaml
logging:
  level: INFO
  output: file
  directory: logs
  log_responses: false
```

In `file` mode, the application creates the directory and a separate `mcp-*.jsonl` file on each startup. Logs go only to that file. `logs/` is included in `.gitignore`. There is no automatic rotation or deletion of old files; the machine's owner controls retention. Application startup errors are written to stderr.

Every record contains a `trace_id`. A tool request, its outgoing HTTP requests, and its result share the same identifier. It is also returned in the tool result's `_meta.trace_id`; HTTP responses include `X-Trace-ID`. Background records use an identifier assigned when the logger is created. Use this value to find related lines in console output or a log file.

Tool arguments, errors, and durations are logged at the INFO level. Enable `logging.log_responses` to log complete successful responses. Logs may contain private data if the agent includes it in tool arguments.

## Privacy boundaries

There is no third-party MCP hosting, cloud parsing, LLM invocation, or external log collector. The running server contacts DuckDuckGo, the requested public websites, and the system DNS resolver. The search engine sees `query`; a website sees the requested URL and the server's IP address. The server does not add project context or read project files, but it cannot determine whether arbitrary query text contains a secret.

Cookies, authorization, pi headers, and conversation history are not forwarded to websites. Proxy environment variables are not used. HTTP connections are allowed only on ports 80/443. Private and special IP addresses are blocked, including during DNS resolution and redirects. DNS addresses are checked before connecting, and the connection is then opened to a verified IP. HTTPS certificate verification is enabled. Trace headers are not sent to external websites.

Website HTML is untrusted content. The server returns text; the agent remains responsible for deciding whether to follow any instructions it contains.

DuckDuckGo may change its HTML, rate-limit requests, or present a CAPTCHA. The tool returns an error in these cases; it does not automatically switch search engines or bypass CAPTCHAs. The live test checks access from the machine running it.

## Checks

```sh
task lc
task lcf
task test
task test-live
```

`task lc` (`lint_check`) checks the code using `.golangci.yml`. `task lcf` (`lint_fix`) applies the gofumpt, golines, and swaggo formatters, followed by linter fixes. The `noinlineerr` check is disabled during automatic fixes.

`task test` tests all layers with the race detector and creates `coverage.out`. Regular tests use local data and do not contact external websites. They cover both logging modes, the presence of `trace_id`, and the absence of files when logging to the console. The CLI test builds the binary and checks the actual stdio transport.

`task test-live` performs a real search for pgx documentation and downloads a result page. It requires internet access and runs separately from the regular test suite.

## Structure

```text
cmd/app/                       CLI and application startup
internal/config/               YAML and validation
internal/root/                 dependency wiring and tool registration
internal/server/               MCP transports, limits, and tracing
internal/handlers/mcp/websearch/ MCP contract for web_search
internal/handlers/mcp/webfetch/  MCP contract for web_fetch
internal/services/search/      search rules and rate limiting
internal/services/fetch/       text extraction and chunking
internal/adapters/duckduckgo/   search requests and result parsing
internal/adapters/web/          HTTP, DNS, and address restrictions
internal/adapters/htmltext/     HTML text extraction
internal/observability/        slog and trace_id
docs/PI.md                     pi setup
```

Add each new tool as a separate package under `internal/handlers/mcp/`. Call a service when business logic is needed, and register the tool in `internal/root/`.
