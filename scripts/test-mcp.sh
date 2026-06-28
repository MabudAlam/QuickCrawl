#!/usr/bin/env bash
# Test QuickCrawl MCP server over stdio JSON-RPC against www.mabud.dev.
# Usage: ./scripts/test-mcp.sh
set -u

BIN="$(cd "$(dirname "$0")/.." && pwd)/bin/quickcrawl-mcp"
TARGET="${TARGET:-https://www.mabud.dev/}"
SEED="${TARGET%/}"
OUT="$(cd "$(dirname "$0")/.." && pwd)/response-mcp.md"

if [ ! -x "$BIN" ]; then
  echo "binary not found or not executable: $BIN" >&2
  exit 1
fi

# Call a JSON-RPC method on the MCP server over stdio.
# Args: <label> <json-payload> ... (extra payload lines appended as-is)
# Writes section to $OUT with the raw stdout response.
call() {
  local label="$1"; shift
  local payload
  payload=$(printf '%s\n' "$@")
  {
    printf '\n## %s\n\n' "$label"
    printf '**request(s):**\n\n```json\n%s```\n\n' "$payload"
  } >> "$OUT"

  local response exit
  response=$(printf '%s\n' "$@" | "$BIN" 2>/dev/null)
  exit=$?

  {
    printf '**exit code:** `%d`\n\n' "$exit"
    printf '**response:**\n\n```json\n%s\n```\n' "$response"
  } >> "$OUT"
}

{
  printf '# QuickCrawl MCP smoke test\n\n'
  printf '**binary:** `%s`\n\n' "$BIN"
  printf '**target:** `%s`\n\n' "$TARGET"
  printf '**timestamp:** `%s`\n\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf '**transport:** newline-delimited JSON-RPC over stdio\n\n'
} > "$OUT"

call "initialize" \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}'

call "notifications/initialized" \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}'

call "tools/list" \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'

call "tools/call: scrape" \
  '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"scrape","arguments":{"url":"'"$TARGET"'","formats":["markdown"],"renderMode":"auto"}}}'

call "tools/call: scrape (browser)" \
  '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"scrape","arguments":{"url":"'"$TARGET"'","formats":["markdown"],"renderMode":"browser","waitFor":1500}}}'

call "tools/call: site_map" \
  '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"site_map","arguments":{"url":"'"$SEED"'","maxDepth":2}}}'

call "tools/call: crawl" \
  '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"crawl","arguments":{"url":"'"$SEED"'","maxDepth":2,"maxPages":5,"formats":["markdown"]}}}'

call "tools/call: search" \
  '{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"search","arguments":{"query":"site:mabud.dev","region":"us-en","page":1}}}'

printf '\n---\nwritten to %s\n' "$OUT"
