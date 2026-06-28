#!/usr/bin/env bash
# Test all QuickCrawl CLI commands against www.mabud.dev and write results to response.md.
# Usage: ./scripts/test-cli.sh
set -u

BIN="$(cd "$(dirname "$0")/.." && pwd)/bin/quickcrawl"
TARGET="${TARGET:-https://www.mabud.dev/}"
OUT="$(cd "$(dirname "$0")/.." && pwd)/response.md"

if [ ! -x "$BIN" ]; then
  echo "binary not found or not executable: $BIN" >&2
  exit 1
fi

# Strip trailing slash so --max-depth works the same on http and https forms.
SEED="${TARGET%/}"

run() {
  local label="$1"; shift
  local cmd="$*"
  local out exit
  echo "running: $label"
  out=$($cmd 2>&1)
  exit=$?
  {
    printf '\n## %s\n\n' "$label"
    printf '**command:**\n\n```sh\n%s\n```\n\n' "$cmd"
    printf '**exit code:** `%d`\n\n' "$exit"
    if [ $exit -eq 0 ]; then
      printf '**output:**\n\n```\n%s\n```\n' "$out"
    else
      printf '**error:**\n\n```\n%s\n```\n' "$out"
    fi
  } >> "$OUT"
}

{
  printf '# QuickCrawl CLI smoke test\n\n'
  printf '**binary:** `%s`\n\n' "$BIN"
  printf '**target:** `%s`\n\n' "$TARGET"
  printf '**timestamp:** `%s`\n\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
} > "$OUT"

run "scrape (auto)"          "$BIN scrape $TARGET"
run "scrape (browser)"       "$BIN scrape $TARGET --render browser --wait-for 1500"
run "scrape (markdown+links)" "$BIN scrape $TARGET --formats markdown,links"
run "map"                    "$BIN map $SEED --max-depth 2"
run "crawl (max-pages=5)"    "$BIN crawl $SEED --max-depth 2 --max-pages 5"
run "search"                 "$BIN search site:mabud.dev --region us-en --page 1"

printf '\n---\nwritten to %s\n' "$OUT"
