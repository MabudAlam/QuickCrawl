# Local Development Setup

This guide walks you through cloning, building, and running QuickCrawl from source.

## Prerequisites

- **Go 1.23+** — [install](https://go.dev/doc/install)
- **Node.js 20+** (for the playground frontend only)
- **Python 3.10+** (for the Python SDK)
- **Docker** (for local SearXNG search and CloakBrowser rendering)

## Local Docker Services

QuickCrawl uses two optional backend services — **SearXNG** (search) and **CloakBrowser** (headless Chrome with anti-detection). Both are Docker-based and live in `infra/`. You only need them if you're testing search functionality or browser rendering locally.

### SearXNG (search backend)

SearXNG is the search engine used by QuickCrawl's `/v1/search` endpoint. The `infra/SearXNG/` directory contains a custom `Dockerfile` and `settings.yml`.

```bash
# Build and run SearXNG on port 8888
docker build -f infra/SearXNG/Dockerfile.searxng -t quickcrawl-searxng .
docker run -d --name searxng -p 8888:8080 quickcrawl-searxng
```

Then point QuickCrawl at it:

```bash
SEARCH__BASE_URL=http://localhost:8888/ ./bin/quickcrawl search "hello world"
# or in quickcrawl.toml:
# [search]
# base_url = "http://localhost:8888/"
```

Stop with: `docker stop searxng && docker rm searxng`

### CloakBrowser (headless Chrome rendering)

CloakBrowser is a headless Chrome with the "Bypass Paywalls Clean" extension pre-installed. It's used when `renderMode = "browser"` on scrape/crawl/map requests.

```bash
# Build and run CloakBrowser with CDP on port 9222
docker build -f infra/cloakbrowser/Dockerfile.cloak -t quickcrawl-cloak .
docker run -d --name cloak -p 9222:9222 quickcrawl-cloak
```

Then configure QuickCrawl to use it:

```bash
RENDERER__CHROME__WS_URL=ws://localhost:9222/ ./bin/quickcrawl scrape https://example.com --render browser
# or in quickcrawl.toml:
# [renderer.chrome]
# ws_url = "ws://localhost:9222/"
```

Stop with: `docker stop cloak && docker rm cloak`

### CloakBrowser with a proxy

CloakBrowser supports routing browser traffic through a SOCKS5 proxy. Pass the proxy URL when starting the container:

```bash
# With an authenticated SOCKS5 proxy
docker run -d --name cloak \
  -p 9222:9222 \
  -e PROXY_ENABLED=true \
  -e PROXY_SERVER="socks5://user:pass@proxy.example.com:1080" \
  quickcrawl-cloak

# With an unauthenticated SOCKS5 proxy
docker run -d --name cloak \
  -p 9222:9222 \
  -e PROXY_ENABLED=true \
  -e PROXY_SERVER="socks5://proxy.example.com:1080" \
  quickcrawl-cloak
```

> **Proxy URL format:** `socks5://[user:pass@]host:port`
> Omit `user:pass@` for unauthenticated proxies.
>
> **Proxy type:** Only SOCKS5 is supported by CloakBrowser natively. For HTTP proxies, consider routing traffic through a local SOCKS5 proxy (e.g. `ssh -D 1080 user@host`) first.

After starting with a proxy, use CloakBrowser normally — all browser traffic will be routed through the proxy automatically:

```bash
RENDERER__CHROME__WS_URL=ws://localhost:9222/ \
  ./bin/quickcrawl scrape https://example.com --render browser
```

### Using both together

```bash
# Start both
docker build -f infra/SearXNG/Dockerfile.searxng -t quickcrawl-searxng .
docker run -d --name searxng -p 8888:8080 quickcrawl-searxng

docker build -f infra/cloakbrowser/Dockerfile.cloak -t quickcrawl-cloak .
docker run -d --name cloak -p 9222:9222 quickcrawl-cloak

# Run with overrides
SEARCH__BASE_URL=http://localhost:8888/ \
RENDERER__CHROME__WS_URL=ws://localhost:9222/ \
./bin/quickcrawl scrape https://example.com --render browser

# Tear down both
docker stop searxng cloak && docker rm searxng cloak
```

> **Without these containers**, QuickCrawl still works — the CLI and MCP auto-launch a local LightPanda for browser rendering, and search falls back to whatever `SEARCH__BASE_URL` is configured (defaults to the hosted SearXNG instance).

```bash
git clone https://github.com/MabudAlam/quickcrawl
cd quickcrawl
```

## Install Go dependencies

```bash
make install
# or manually:
go mod download
go mod tidy
```

## Configuration: `quickcrawl.toml`

`quickcrawl.toml` is the primary configuration file. It lives next to the binary and is **required** at startup.

Key sections:

| Section | Purpose |
|---|---|
| `[server]` | HTTP API host, port, timeouts, rate limits |
| `[renderer]` | Browser pool size, page timeout, render mode |
| `[renderer.chrome]` | Chrome DevTools WebSocket URL |
| `[crawler]` | Concurrency, rate, robots.txt, user-agent, defaults |
| `[crawler.stealth]` | Stealth browser strategy and jitter |
| `[extraction.llm]` | LLM API key, model, base URL for JSON extraction |
| `[cache]` | Redis URL and TTL |
| `[search]` | SearXNG base URL and BM25 weights |



## Configuration: `.env` (environment overrides)

`.env` is **optional**. It overrides `quickcrawl.toml` values using the format `<SECTION>__<KEY>` with a **double underscore**. The `.env` file is gitignored — never commit secrets.

```bash
cp .env.example .env
```

### How override precedence works

1. `quickcrawl.toml` is loaded first as the base config.
2. If a matching `_<KEY>` env var is set, it **replaces** the TOML value.
3. Unset env vars are simply ignored — TOML values stand.

### Override naming convention

The env key is the TOML section name in **uppercase**, followed by `__`, followed by the key name in **uppercase**.

| TOML value | Environment variable |
|---|---|
| `server.port` | `SERVER__PORT` |
| `renderer.render_mode` | `RENDERER__RENDER_MODE` |
| `renderer.chrome.ws_url` | `RENDERER__CHROME__WS_URL` |
| `crawler.max_concurrency` | `CRAWLER__MAX_CONCURRENCY` |
| `crawler.stealth.enabled` | `CRAWLER__STEALTH__ENABLED` |
| `extraction.llm.api_key` | `EXTRACTION__LLM__API_KEY` |
| `search.base_url` | `SEARCH__BASE_URL` |
| `cache.redis_url` | `REDIS_URL` |

### Common local development overrides

```bash
# Use a different SearXNG instance locally
SEARCH__BASE_URL=http://localhost:8888/

# Bump concurrency for benchmarking
CRAWLER__MAX_CONCURRENCY=80
CRAWLER__REQUESTS_PER_SECOND=80.0

# Disable cache during development
CACHE__ENABLED=false

# Point at a remote Chrome CDP
RENDERER__CHROME__WS_URL=ws://remote-chrome:9222/

# Use a different LLM for extraction testing
EXTRACTION__LLM__API_KEY=sk-...
EXTRACTION__LLM__MODEL=gpt-4o
```

### Config file path

To use a non-standard TOML path, set `CONFIG`:

```bash
CONFIG=/etc/quickcrawl/production.toml ./bin/quickcrawl-server
```

---

## Building

Three binaries live in this repo:

| Binary | Package | Purpose |
|---|---|---|
| **CLI** | `cli` | `scrape`, `crawl`, `map`, `search` commands |
| **MCP server** | `cmd/mcp` | MCP stdio server for AI tool integrations |
| **HTTP API server** | `cmd/server` | REST API + playground |

### Development builds (with verbose logs)

Dev builds use `DefaultLevel=info` — logs are visible, useful when working on the codebase.

```bash
make build && ./bin/quickcrawl --help
```

### Production builds (silent by default)

Production builds use `DefaultLevel=error` via ldflags — all info/warn logs are suppressed. Binaries distributed via GitHub Releases and `goreleaser` are built this way.

```bash
make build-prod && ./bin/quickcrawl --help
```

### Build individually

```bash
# Development (verbose)
go build -o bin/quickcrawl        ./cli
go build -o bin/quickcrawl-mcp   ./cmd/mcp
go build -o bin/quickcrawl-server ./cmd/server

# Production (silent)
go build -ldflags "-X github.com/MabudAlam/quickcrawl/internal/utils.DefaultLevel=error" -o bin/quickcrawl        ./cli
go build -ldflags "-X github.com/MabudAlam/quickcrawl/internal/utils.DefaultLevel=error" -o bin/quickcrawl-mcp   ./cmd/mcp
go build -ldflags "-X github.com/MabudAlam/quickcrawl/internal/utils.DefaultLevel=error" -o bin/quickcrawl-server ./cmd/server
```

### Via Makefile targets

```bash
# Development
make build        # CLI + MCP + server (into bin/)
make build-mcp   # MCP only
make build-server # server only
make run-server  # build + run server
make run-mcp     # build + run MCP

# Production (silent)
make build-prod        # CLI + MCP + server (silent)
make build-cli-prod   # CLI only (silent)
make build-mcp-prod   # MCP only (silent)
make build-server-prod # server only (silent)
```

### With `air` (live reload)

```bash
make dev   # requires air: go install github/air@latest
```

### Overriding log level at runtime

Even in production builds, set `LOG_LEVEL` to re-enable logs:

```bash
LOG_LEVEL=info ./bin/quickcrawl scrape https://example.com   # re-enable verbose logs
LOG_LEVEL=debug ./bin/quickcrawl scrape https://example.com  # even more detail
```

---

## Running

### HTTP API server

```bash
./bin/quickcrawl-server
# API available at http://localhost:3000
# Playground at http://localhost:3000/playground
```

### MCP server (stdio mode)

```bash
./bin/quickcrawl-mcp
# Connect via stdio to your AI client (Claude Code, Cursor, etc.)
```

### CLI (standalone — no server needed)

```bash
./bin/quickcrawl scrape https://example.com
./bin/quickcrawl crawl https://example.com --max-pages 10
./bin/quickcrawl map https://example.com
./bin/quickcrawl search "what is quickcrawl"
```

Run `quickcrawl --help` and `quickcrawl <command> --help` for all flags.

---

### Test MCP on MCP Inspector 

CONFIG=/Users/skmabudalam/Desktop/quickcrawl/quickcrawl.toml \
npx @modelcontextprotocol/inspector /Users/skmabudalam/Desktop/quickcrawl/bin/quickcrawl-mcp

Change the path to your env


## Testing

```bash
# All tests
make test

# With coverage report
make test-cover

# Go tests only
go test -race ./...

# Python SDK tests
cd python && python3 -m pytest tests/ -v
```

---

## Project layout

```
quickcrawl/
├── cli/cmd/          CLI commands (scrape, crawl, map, search)
├── cmd/
│   ├── mcp/          MCP server entry point
│   └── server/       HTTP API server entry point
├── internal/
│   ├── api/          HTTP handlers, routes, middleware
│   ├── browser/      Chrome/LightPanda CDP wrapper
│   ├── config/       Config loading + env override logic
│   ├── core/         Core scraping/crawling logic
│   ├── crawler/      BFS crawler, robots.txt, politeness
│   ├── extractor/    HTML/markdown extraction
│   ├── mcp/          MCP tool schemas and handlers
│   ├── search/       SearXNG search integration
│   └── types/        Request/response types and validation
├── playground/       React playground (Next.js)
├── python/           Python SDK
└── scripts/          Dev and benchmark scripts
```

---

## Common development tasks

### Add a new CLI flag

1. Add the field to `cli/cmd/your_command.go`
2. Add validation in the parse function
3. Add tests in `cli/cmd/your_command_test.go`
4. Rebuild: `go build -o bin/quickcrawl ./cli`

### Add a new API endpoint

1. Add request/response types in `internal/types/types.go`
2. Add handler in `internal/api/handlers/handler.go`
3. Add route in `internal/api/routes/routes.go`
4. Add validation test in `internal/types/types_test.go`
5. Rebuild server: `make build-server`

### Add a new MCP tool

1. Define the input schema in `internal/mcp/server.go` (`*InputSchema` func)
2. Add the handler method (`Handle*`)
3. Wire it into `newHandlers()` and the tool registry
4. Run `make build-mcp` to rebuild

### Update validation rules

All request validation lives in `internal/types/types.go` on each `*Request.Validate()` method. The same validators are used by the API, MCP, CLI, and Python SDK — changing one place propagates everywhere.

---

## Code quality

```bash
make fmt    # format all Go code
make lint   # run golangci-lint
make test   # run full test suite
```

---

## Troubleshooting

**`quickcrawl: command not found` after install**
```bash
export PATH="$HOME/go/bin:$PATH"
# or reinstall with: go install github.com/MabudAlam/quickcrawl/cli
```

**Browser/CDP connection refused**
- Ensure Chrome is running with DevTools enabled: `chrome --remote-debugging-port=9222`
- Or set `RENDERER__CHROME__WS_URL=ws://localhost:9222/`
- The CLI and MCP auto-launch a headless LightPanda if no Chrome is configured

**Playground won't start**
```bash
cd playground && npm install && npm run dev
# then run the server separately: ./bin/quickcrawl-server
```

**Python SDK import error**
```bash
cd python && pip install -e .
```
