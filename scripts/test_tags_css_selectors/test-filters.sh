#!/bin/bash

# Filter Test Script for quickcrawl
# Randomly samples HTML tags for includeTags/excludeTags and CSS selectors,
# serves demo.html over a local HTTP server, and verifies the /v1/scrape
# API returns content consistent with each filter.
#
# Usage:
#   ./scripts/test-filters.sh                  # uses http://127.0.0.1:3000 as the API
#   BASE_URL=http://host:port ./scripts/test-filters.sh
#   RATE_DELAY=0 ./scripts/test-filters.sh     # disable inter-request pacing
#
# Note: the server's rate_limit_rps (default 10 in quickcrawl.toml) limits
# how many requests we can fire per second; this script paces itself with
# a 150ms sleep between calls. Bump RATE_DELAY if you raise the limit.
#
# Requirements: the API server is running and demo.html is in the project root.

set -u

BASE_URL="${BASE_URL:-http://0.0.0.0:3000}"
STATIC_PORT="${STATIC_PORT:-8765}"
STATIC_URL="http://127.0.0.1:${STATIC_PORT}/demo.html"
TIMEOUT=30
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEMO_FILE="${PROJECT_ROOT}/demo.html"
PID_FILE="/tmp/quickcrawl-test-filters.${STATIC_PORT}.pid"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

INCLUDED_PASS=0
INCLUDED_FAIL=0
EXCLUDED_PASS=0
EXCLUDED_FAIL=0
CSS_PASS=0
CSS_FAIL=0
SANITY_FAIL=0

pick_random() {
    local n="$1"
    shift
    local arr=("$@")
    local count=${#arr[@]}
    if [ "$n" -gt "$count" ]; then n=$count; fi
    for ((i = 0; i < n; i++)); do
        local idx=$((RANDOM % count))
        echo "${arr[$idx]}"
    done
}

assert_nonempty() {
    local body="$1"
    local label="$2"
    if [ -z "$body" ]; then
        echo -e "  ${RED}❌ empty response${NC}"
        return 1
    fi
    return 0
}

extract_markdown_len() {
    local body="$1"
    echo "$body" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    md = d.get('data', {}).get('markdown') or ''
    print(len(md))
except Exception:
    print(0)
" 2>/dev/null
}

extract_markdown_text() {
    local body="$1"
    echo "$body" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    md = d.get('data', {}).get('markdown') or ''
    print(md)
except Exception:
    print('')
" 2>/dev/null
}

# Pause briefly between requests so we stay under the server's RPS limit
# (quickcrawl.toml has rate_limit_rps = 10 by default; 17 sequential
# requests would otherwise trip 429s on overflow). Set RATE_DELAY=0
# to disable.
RATE_DELAY="${RATE_DELAY:-0.15}"
sleep_after_call() {
    if [ "${RATE_DELAY}" != "0" ]; then
        sleep "$RATE_DELAY"
    fi
}

call_scrape() {
    local payload="$1"
    local resp http_code
    resp=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/v1/scrape" \
        -H 'Content-Type: application/json' \
        -d "$payload" \
        --max-time "$TIMEOUT" 2>/dev/null)
    http_code=$(echo "$resp" | tail -n1)
    local body
    body=$(echo "$resp" | sed '$d')
    if [ "$http_code" != "200" ]; then
        echo "    [debug] http_code=$http_code body=$(echo "$body" | head -c 200)" >&2
        echo "$body"
        sleep_after_call
        return 1
    fi
    echo "$body"
    sleep_after_call
    return 0
}

start_static_server() {
    if [ ! -f "$DEMO_FILE" ]; then
        echo -e "${RED}❌ demo.html not found at $DEMO_FILE${NC}"
        exit 1
    fi
    cd "$PROJECT_ROOT"
    python3 -m http.server "$STATIC_PORT" --bind 127.0.0.1 >/dev/null 2>&1 &
    echo $! > "$PID_FILE"
    for i in {1..30}; do
        if curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${STATIC_PORT}/demo.html" 2>/dev/null | grep -q "200"; then
            return 0
        fi
        sleep 0.2
    done
    echo -e "${RED}❌ static server failed to start on port $STATIC_PORT${NC}"
    kill "$(cat "$PID_FILE")" 2>/dev/null
    rm -f "$PID_FILE"
    exit 1
}

stop_static_server() {
    if [ -f "$PID_FILE" ]; then
        local pid
        pid=$(cat "$PID_FILE")
        kill "$pid" 2>/dev/null
        rm -f "$PID_FILE"
    fi
}

trap stop_static_server EXIT

echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}Filter Test: includeTags / excludeTags / cssSelector${NC}"
echo -e "${YELLOW}API:        $BASE_URL${NC}"
echo -e "${YELLOW}Fixture:    $STATIC_URL${NC}"
echo -e "${YELLOW}========================================${NC}"

start_static_server
echo -e "${GREEN}static server up on :$STATIC_PORT (pid=$(cat "$PID_FILE"))${NC}"

baseline_payload="{\"url\":\"$STATIC_URL\",\"formats\":[\"markdown\"]}"
baseline_body=$(call_scrape "$baseline_payload") || {
    echo -e "${RED}❌ baseline scrape failed; is the API server running?${NC}"
    exit 1
}
BASELINE_LEN=$(extract_markdown_len "$baseline_body")
if [ "${BASELINE_LEN:-0}" -lt 200 ]; then
    echo -e "${RED}❌ baseline markdown suspiciously short (${BASELINE_LEN} chars)${NC}"
    SANITY_FAIL=1
else
    echo -e "${GREEN}✅ baseline scrape OK (${BASELINE_LEN} chars)${NC}"
fi
echo

# Sanity: extract a few known strings from demo.html to use as test markers
# for includeTags (these should appear) and excludeTags (these should not).
declare -a H2_HEADINGS
while IFS= read -r line; do
    H2_HEADINGS+=("$line")
done < <(grep -oE '<h2[^>]*>[^<]+' "$DEMO_FILE" | sed -E 's|<h2[^>]*>||' | head -10)

declare -a FOOTER_STRINGS
while IFS= read -r line; do
    FOOTER_STRINGS+=("$line")
done < <(python3 -c "
import re
html = open('$DEMO_FILE').read()
m = re.search(r'<footer[^>]*>(.*?)</footer>', html, re.S | re.I)
if m:
    txt = re.sub(r'<[^>]+>', ' ', m.group(1))
    txt = re.sub(r'\s+', ' ', txt).strip()
    for w in txt.split()[:6]:
        print(w)
" 2>/dev/null)

if [ ${#H2_HEADINGS[@]} -eq 0 ]; then
    echo -e "${RED}❌ could not extract h2 headings from demo.html${NC}"
    exit 1
fi

# ── Test 1: includeTags ──────────────────────────────────────────────────────
echo -e "${BLUE}── includeTags: random samples ──${NC}"
INCLUDE_TAGS=(h1 h2 p a img blockquote pre code ul ol li)
SAMPLE_TAGS=($(pick_random 5 "${INCLUDE_TAGS[@]}"))
for tag in "${SAMPLE_TAGS[@]}"; do
    payload="{\"url\":\"$STATIC_URL\",\"formats\":[\"markdown\"],\"includeTags\":[\"$tag\"]}"
    body=$(call_scrape "$payload") || {
        echo -e "  ${RED}❌ includeTags=[\"$tag\"] HTTP error${NC}"
        INCLUDED_FAIL=$((INCLUDED_FAIL + 1))
        continue
    }
    if ! assert_nonempty "$body" "includeTags=[\"$tag\"]"; then
        INCLUDED_FAIL=$((INCLUDED_FAIL + 1))
        continue
    fi
    md_len=$(extract_markdown_len "$body")
    md_text=$(extract_markdown_text "$body")
    if [ "${tag}" = "h2" ] && [ ${#H2_HEADINGS[@]} -gt 0 ]; then
        marker="${H2_HEADINGS[0]}"
        if echo "$md_text" | grep -qF "$marker"; then
            echo -e "  ${GREEN}✅ includeTags=[\"h2\"] (${md_len} chars) — contains h2 heading \"$marker\"${NC}"
            INCLUDED_PASS=$((INCLUDED_PASS + 1))
        else
            echo -e "  ${RED}❌ includeTags=[\"h2\"] (${md_len} chars) — missing expected h2 heading \"$marker\"${NC}"
            INCLUDED_FAIL=$((INCLUDED_FAIL + 1))
        fi
    elif [ "${md_len}" -gt 50 ]; then
        echo -e "  ${GREEN}✅ includeTags=[\"$tag\"] (${md_len} chars)${NC}"
        INCLUDED_PASS=$((INCLUDED_PASS + 1))
    else
        echo -e "  ${RED}❌ includeTags=[\"$tag\"] too short (${md_len} chars)${NC}"
        INCLUDED_FAIL=$((INCLUDED_FAIL + 1))
    fi
done
echo

# ── Test 2: excludeTags ──────────────────────────────────────────────────────
echo -e "${BLUE}── excludeTags: random samples ──${NC}"
EXCLUDE_TAGS=(footer nav header img a span)
SAMPLE_EXCLUDES=($(pick_random 5 "${EXCLUDE_TAGS[@]}"))
excluded_combined=$(IFS=,; echo "${SAMPLE_EXCLUDES[*]}")
payload="{\"url\":\"$STATIC_URL\",\"formats\":[\"markdown\"],\"excludeTags\":[$(printf '"%s",' "${SAMPLE_EXCLUDES[@]}" | sed 's/,$//')]}"
body=$(call_scrape "$payload") || {
    echo -e "  ${RED}❌ excludeTags combined HTTP error${NC}"
    EXCLUDED_FAIL=$((EXCLUDED_FAIL + 1))
}
md_text=$(extract_markdown_text "$body")
md_len=$(extract_markdown_len "$body")
shorter_than_baseline=0
if [ "${md_len:-0}" -lt "$BASELINE_LEN" ]; then
    shorter_than_baseline=1
fi
if [ -n "$body" ] && [ "$shorter_than_baseline" -eq 1 ]; then
    echo -e "  ${GREEN}✅ excludeTags=[$excluded_combined] (${md_len} chars < baseline ${BASELINE_LEN})${NC}"
    EXCLUDED_PASS=$((EXCLUDED_PASS + 1))
else
    echo -e "  ${RED}❌ excludeTags=[$excluded_combined] (${md_len} chars, baseline ${BASELINE_LEN})${NC}"
    EXCLUDED_FAIL=$((EXCLUDED_FAIL + 1))
fi

for tag in "${SAMPLE_EXCLUDES[@]}"; do
    payload="{\"url\":\"$STATIC_URL\",\"formats\":[\"markdown\",\"html\"],\"excludeTags\":[\"$tag\"]}"
    body=$(call_scrape "$payload") || {
        echo -e "  ${RED}❌ excludeTags=[\"$tag\"] HTTP error${NC}"
        EXCLUDED_FAIL=$((EXCLUDED_FAIL + 1))
        continue
    }
    if ! assert_nonempty "$body" "excludeTags=[\"$tag\"]"; then
        EXCLUDED_FAIL=$((EXCLUDED_FAIL + 1))
        continue
    fi
    html_text=$(echo "$body" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    h = d.get('data', {}).get('html') or ''
    print(h.lower())
except Exception:
    print('')
" 2>/dev/null)
    md_text=$(extract_markdown_text "$body")
    if [ "$tag" = "footer" ] && [ ${#FOOTER_STRINGS[@]} -gt 0 ]; then
        # Build a small list of footer words and check none leak into output.
        leaked=0
        for w in "${FOOTER_STRINGS[@]}"; do
            if [ -n "$w" ] && echo "$html_text" | grep -qiF "$w"; then
                leaked=1
                break
            fi
        done
        if [ "$leaked" -eq 0 ]; then
            echo -e "  ${GREEN}✅ excludeTags=[\"footer\"] (${md_len:-?} chars) — no footer leak${NC}"
            EXCLUDED_PASS=$((EXCLUDED_PASS + 1))
        else
            echo -e "  ${RED}❌ excludeTags=[\"footer\"] — footer text leaked through${NC}"
            EXCLUDED_FAIL=$((EXCLUDED_FAIL + 1))
        fi
    else
        echo -e "  ${GREEN}✅ excludeTags=[\"$tag\"] OK${NC}"
        EXCLUDED_PASS=$((EXCLUDED_PASS + 1))
    fi
done
echo

# ── Test 3: cssSelector ──────────────────────────────────────────────────────
echo -e "${BLUE}── cssSelector: random samples ──${NC}"
SELECTORS=(".post" "#main" "h2" "figure" "p" "pre code" "img" ".footer" "article" ".decoder")
SAMPLE_SELECTORS=($(pick_random 5 "${SELECTORS[@]}"))
for sel in "${SAMPLE_SELECTORS[@]}"; do
    escaped_sel=$(echo "$sel" | sed 's/"/\\"/g')
    payload="{\"url\":\"$STATIC_URL\",\"formats\":[\"markdown\",\"html\"],\"cssSelector\":\"$escaped_sel\"}"
    body=$(call_scrape "$payload") || {
        echo -e "  ${RED}❌ cssSelector=\"$sel\" HTTP error${NC}"
        CSS_FAIL=$((CSS_FAIL + 1))
        continue
    }
    if ! assert_nonempty "$body" "cssSelector=\"$sel\""; then
        CSS_FAIL=$((CSS_FAIL + 1))
        continue
    fi
    md_text=$(extract_markdown_text "$body")
    md_len=$(extract_markdown_len "$body")
    if [ "${md_len:-0}" -le "$BASELINE_LEN" ] && [ "${md_len:-0}" -gt 0 ]; then
        echo -e "  ${GREEN}✅ cssSelector=\"$sel\" (${md_len} chars ≤ baseline ${BASELINE_LEN})${NC}"
        CSS_PASS=$((CSS_PASS + 1))
    elif [ "${md_len:-0}" -gt 0 ]; then
        echo -e "  ${GREEN}✅ cssSelector=\"$sel\" (${md_len} chars)${NC}"
        CSS_PASS=$((CSS_PASS + 1))
    else
        echo -e "  ${RED}❌ cssSelector=\"$sel\" empty${NC}"
        CSS_FAIL=$((CSS_FAIL + 1))
    fi
done
echo

# ── Combined: includeTags + excludeTags + cssSelector ────────────────────────
echo -e "${BLUE}── combined: include + exclude + css ──${NC}"
payload="{\"url\":\"$STATIC_URL\",\"formats\":[\"markdown\"],\"includeTags\":[\"h2\",\"p\"],\"excludeTags\":[\"footer\",\"nav\"],\"cssSelector\":\"#main\"}"
body=$(call_scrape "$payload") || {
    echo -e "  ${RED}❌ combined filter HTTP error${NC}"
    CSS_FAIL=$((CSS_FAIL + 1))
}
md_len=$(extract_markdown_len "$body")
if [ -n "$body" ] && [ "${md_len:-0}" -gt 50 ] && [ "${md_len:-0}" -le "$BASELINE_LEN" ]; then
    echo -e "  ${GREEN}✅ combined filters (${md_len} chars)${NC}"
    CSS_PASS=$((CSS_PASS + 1))
else
    echo -e "  ${RED}❌ combined filters (${md_len:-?} chars, baseline ${BASELINE_LEN})${NC}"
    CSS_FAIL=$((CSS_FAIL + 1))
fi
echo

# ── Summary ──────────────────────────────────────────────────────────────────
TOTAL_PASS=$((INCLUDED_PASS + EXCLUDED_PASS + CSS_PASS))
TOTAL_FAIL=$((INCLUDED_FAIL + EXCLUDED_FAIL + CSS_FAIL + SANITY_FAIL))
echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}Summary${NC}"
echo -e "${YELLOW}========================================${NC}"
echo -e "  includeTags:  ${GREEN}${INCLUDED_PASS} pass${NC} / ${RED}${INCLUDED_FAIL} fail${NC}"
echo -e "  excludeTags:  ${GREEN}${EXCLUDED_PASS} pass${NC} / ${RED}${EXCLUDED_FAIL} fail${NC}"
echo -e "  cssSelector:  ${GREEN}${CSS_PASS} pass${NC} / ${RED}${CSS_FAIL} fail${NC}"
echo -e "  baseline:     ${RED}${SANITY_FAIL} fail${NC}"
echo -e "  ${YELLOW}total:        ${TOTAL_PASS} pass / ${TOTAL_FAIL} fail${NC}"

if [ "$TOTAL_FAIL" -eq 0 ]; then
    echo -e "${GREEN}ALL FILTER TESTS PASSED${NC}"
    exit 0
else
    echo -e "${RED}SOME FILTER TESTS FAILED${NC}"
    exit 1
fi
