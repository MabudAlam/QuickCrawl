---
name: quickcrawl-mcp
description: "Scrape URLs, crawl sites, map pages, and search the web via the QuickCrawl MCP server. Use when the agent has an MCP socket available (Claude Code, OpenCode, Cursor, Roo Code, Windsurf) and needs structured typed tool calls to fetch web content, discover site structure, or run web searches."
license: AGPL-3.0
metadata:
  author: MabudAlam
  version: "0.3.0"
  homepage: https://github.com/MabudAlam/quickcrawl
---

# QuickCrawl MCP — Web Data Toolkit for AI Agents

## When to use this skill

Use this skill when:
- The agent is Claude Code, OpenCode, Cursor, Roo Code, or Windsurf (MCP-supported)
- The agent needs structured typed tool calls rather than shell commands
- The agent needs to fetch a page, crawl a site, discover URLs, or search the web
- The agent is building a RAG corpus, researching a topic, or doing competitive analysis

## Installation

### 1. Install the MCP server binary

```bash
# From source (recommended for latest)
go install github.com/MabudAlam/quickcrawl/cmd/mcp

# Or download a pre-built binary from releases
# https://github.com/MabudAlam/quickcrawl/releases
```

Verify: `quickcrawl-mcp --help` should produce no output (stdio mode).

> **Config file:** The MCP server needs `quickcrawl.toml` in the same directory as the binary (or set `CONFIG=/path/to/quickcrawl.toml`). Copy the default from the repo: `curl -fsSL https://raw.githubusercontent.com/MabudAlam/quickcrawl/main/quickcrawl.toml > ./quickcrawl.toml`

### 2. Connect to your agent

**Claude Code** (recommended):
```bash
claude mcp add quickcrawl -- bin/quickcrawl-mcp
```

**Claude Desktop** (`~/.claude_desktop_config.json`):
```json
{
  "mcpServers": {
    "quickcrawl": {
      "command": "/path/to/quickcrawl-mcp",
      "args": []
    }
  }
}
```

**OpenCode** (`~/.config/opencode/config.json`):
```json
{
  "mcpServers": {
    "quickcrawl": {
      "command": "quickcrawl-mcp",
      "args": []
    }
  }
}
```

**Cursor** — Settings → MCP → Add new MCP server → command: `quickcrawl-mcp`

### 3. Verify

Restart the agent. The following tools should be available:
- `quickcrawl_scrape` — fetch a single URL
- `quickcrawl_crawl` — start an async site crawl
- `quickcrawl_check_crawl_status` — poll a crawl job for results
- `quickcrawl_map` — discover URLs without scraping
- `quickcrawl_search` — web search via SearXNG

## Tools Reference

| Tool | What it does | Blocking? |
|---|---|---|
| `quickcrawl_scrape` | Fetch one URL, extract content in any format | Yes |
| `quickcrawl_crawl` | Start a BFS crawl of a site; returns job ID | No (returns immediately) |
| `quickcrawl_check_crawl_status` | Poll a running/completed crawl job | Yes |
| `quickcrawl_map` | Discover all URLs on a site (no content scraping) | Yes |
| `quickcrawl_search` | Search SearXNG, optionally scrape result pages | Yes |

See [references/tools.md](skills/quickcrawl-mcp/references/tools.md) for full parameter schemas with defaults and ranges.

## Validation Rules

All parameters validated server-side. Know these before calling:

| Parameter | Tools | Allowed values |
|---|---|---|
| `maxDepth` | crawl, map | `0`–`100` (default `2`, negative rejected) |
| `maxPages` | crawl | `1`–`100` (default `100`, zero/negative rejected) |
| `waitFor` | scrape, crawl | `0`–`120000` |
| `timeout` | map | `1`–`600000` (default `30000`) |
| `timeRange` | search | `day`, `week`, `month`, `year` (omit for no filter) |
| `page` | search | `1`–`1000` (default `1`, 1-indexed) |
| `renderMode` | scrape, crawl, search | `auto`, `browser`, `http` |
| `formats` | scrape, crawl, search | Per-tool allowlist (`json` rejected on crawl) |
| `query` | search | Required, non-empty |
| `url` | scrape, crawl, map | Required, valid URL |

## Quick Examples

```json
// Scrape a page
{ "tool": "quickcrawl_scrape", "params": { "url": "https://example.com", "formats": ["markdown"] } }

// Start a crawl, then poll for results
{ "tool": "quickcrawl_crawl", "params": { "url": "https://docs.example.com", "maxPages": 50 } }
// → { "id": "abc-123", "status": "pending" }
// Then poll with:
{ "tool": "quickcrawl_check_crawl_status", "params": { "id": "abc-123" } }

// Map a site
{ "tool": "quickcrawl_map", "params": { "url": "https://example.com", "maxDepth": 3 } }

// Search and scrape results
{ "tool": "quickcrawl_search", "params": { "query": "golang web scraping", "page": 1, "scrape": true, "timeRange": "month" } }
```

## Orchestration Patterns

**Scrape + crawl pattern for RAG:**
1. `quickcrawl_map` to discover site structure and estimate size
2. `quickcrawl_crawl` with a bounded `maxPages` to extract all content
3. `quickcrawl_check_crawl_status` to poll until `completed`
4. Process the `data` array — each item has `markdown` + `metadata.sourceURL`

**Search + scrape pattern for research:**
1. `quickcrawl_search(query=..., scrape=true)` to get results + scraped content
2. Each result contains `content.markdown` with full page text
3. Pass the combined markdown through the LLM for synthesis

> **Content bounds:** Scraped content can exceed the agent's context window. After fetching, consider truncating long markdown fields or extracting only key sections before passing to the LLM.

## Common Edge Cases

- **JavaScript-heavy sites**: Set `renderMode: "browser"` to force headless Chrome
- **Rate limiting**: Large crawls may be blocked by sites; use `maxPages` to limit scope
- **Large sites**: Use `quickcrawl_map` first to estimate site size before committing to a crawl
- **Empty markdown**: Try `formats: ["html"]` instead
- **Crawl timeout**: Jobs expire after 1 hour; poll `check_crawl_status` regularly

## Configuration

QuickCrawl MCP reads `quickcrawl.toml` in the same directory as the binary. Key env var overrides:

```bash
# Use a local SearXNG
SEARCH__BASE_URL=http://localhost:8888/ quickcrawl-mcp

# Use a local CloakBrowser
RENDERER__CHROME__WS_URL=ws://localhost:9222/ quickcrawl-mcp

# Production: both are pre-configured; no env vars needed
```

## Links

- GitHub: https://github.com/MabudAlam/quickcrawl
- Full docs: https://github.com/MabudAlam/quickcrawl#readme
- Report issues: https://github.com/MabudAlam/quickcrawl/issues
