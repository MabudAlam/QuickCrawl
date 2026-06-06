#!/bin/bash
#
# Comprehensive benchmark + feature test for /v1/scrape vs /v1/scrape-core.
#
# For each test site, runs both endpoints under several feature configurations
# and reports wall-clock time, byte size of returned markdown, and success status.
#
# Usage:
#   ./scripts/benchmark.sh                    # run with defaults
#   BASE_URL=http://localhost:3000 ./scripts/benchmark.sh
#   SITES="https://go.dev/doc/" ./scripts/benchmark.sh   # custom site list
#   SKIP_FEATURES=1 ./scripts/benchmark.sh   # time only, no feature matrix
#
# Requirements: curl, python3, jq (optional, falls back to python3).

set -u

BASE_URL="${BASE_URL:-http://0.0.0.0:3000}"
TIMEOUT="${TIMEOUT:-60}"
OUT_DIR="${OUT_DIR:-./tmp/bench-$(date +%Y%m%d-%H%M%S)}"

# Default sites — these are the ones the user flagged as slower than original.
SITES="${SITES:-https://stripe.com/docs/api https://app.slack.com/ https://go.dev/doc/ https://www.cloudflare.com/}"

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

mkdir -p "$OUT_DIR"
RESULTS_CSV="$OUT_DIR/results.csv"
echo "endpoint,site,feature,config,status,time_ms,md_bytes,links_count,html_bytes,note" > "$RESULTS_CSV"

log()  { echo -e "$*"; }
hr()   { echo -e "${CYAN}────────────────────────────────────────────────────────────────${NC}"; }
hdr()  { echo -e "\n${BOLD}${YELLOW}$*${NC}"; }
ok()   { echo -e "${GREEN}$*${NC}"; }
warn() { echo -e "${YELLOW}$*${NC}"; }
err()  { echo -e "${RED}$*${NC}"; }

# Extract a JSON field with python (no jq dependency).
jget() {
  local body="$1" expr="$2"
  python3 -c "import sys, json
try:
    d = json.loads(sys.argv[1])
    print(eval(sys.argv[2]))
except Exception as e:
    print('ERR')" "$body" "$expr" 2>/dev/null
}

# Run one scrape. Echoes a single-line CSV record (without the header prefix)
# on stdout. Captures full body to $OUT_DIR/<endpoint>__<site>__<feature>.json
#
# Args:
#   $1 endpoint   - "scrape" or "scrape-core"
#   $2 site       - URL to scrape
#   $3 feature    - human label for the feature under test
#   $4 json_body  - request body
#   $5 extra_note - optional note appended to CSV
run_one() {
  local endpoint="$1" site="$2" feature="$3" body="$4" extra_note="${5:-}"
  local url="$BASE_URL/v1/$endpoint"
  local safe_site safe_feat
  safe_site=$(echo "$site" | sed 's|https\?://||;s|/|_|g;s|[^A-Za-z0-9._-]||g' | cut -c1-60)
  safe_feat=$(echo "$feature" | sed 's|/|_|g;s|[^A-Za-z0-9._-]||g' | cut -c1-40)
  local outfile="$OUT_DIR/${endpoint}__${safe_site}__${safe_feat}.json"

  # Use curl's -w for timing; write body to file. Timeout is per request.
  local tmp
  tmp=$(curl -sS -o "$outfile" -w '%{http_code}|%{time_total}' \
    -X POST "$url" \
    -H 'Content-Type: application/json' \
    -d "$body" \
    --max-time "$TIMEOUT" 2>/dev/null) || tmp="000|0"

  local http_code="${tmp%%|*}"
  local time_s="${tmp##*|}"
  local time_ms
  time_ms=$(python3 -c "print(int(float('$time_s') * 1000))")

  # Parse metrics from the response.
  local body_content=""
  body_content=$(cat "$outfile" 2>/dev/null || echo "{}")
  local success md_bytes links_count html_bytes note
  success=$(jget "$body_content" "d.get('success', False)")
  md_bytes=$(jget "$body_content" "(len(d.get('data', {}).get('markdown')) if d.get('data') and d.get('data', {}).get('markdown') else 0)")
  links_count=$(jget "$body_content" "(len(d.get('data', {}).get('links')) if d.get('data') and isinstance(d.get('data', {}).get('links'), list) else 0)")
  html_bytes=$(jget "$body_content" "(len(d.get('data', {}).get('html')) if d.get('data') and d.get('data', {}).get('html') else 0)")
  note="$extra_note"

  # Status column shows pass/fail.
  local status
  if [ "$http_code" = "200" ] && [ "$success" = "True" ]; then
    status="OK"
  elif [ "$http_code" = "000" ]; then
    status="TIMEOUT"
    note="${note:+$note; }curl failed/timeout"
  else
    status="FAIL"
    local err_msg
    err_msg=$(jget "$body_content" "(d.get('error') or '')")
    note="${note:+$note; }http=$http_code err=$err_msg"
  fi

  # Sanitize commas/newlines in CSV fields.
  local cfg_csv="${body//,/_}"
  echo "${endpoint},${site},${feature},${cfg_csv},${status},${time_ms},${md_bytes},${links_count},${html_bytes},${note}" \
    | sed 's/\t/ /g' \
    | tr -d '\n' \
    >> "$RESULTS_CSV"
  echo "" >> "$RESULTS_CSV"

  printf "  %-11s | %-9s | %6s ms | md=%6s B | links=%4s | %s\n" \
    "$endpoint" "$status" "$time_ms" "$md_bytes" "$links_count" "$feature"
}

# Pre-flight: server health check.
hdr "Pre-flight: $BASE_URL/health"
if ! curl -sS --max-time 5 "$BASE_URL/health" > /dev/null 2>&1; then
  err "Server not responding. Start it with: make run-server (or: go run ./cmd/server/main.go)"
  exit 1
fi
ok "Server is up."

# ---------------------------------------------------------------------------
# Feature matrix
# ---------------------------------------------------------------------------
#
# Each row: feature_label | json_body_template
# The placeholder __URL__ is replaced with the actual site.
#
# Tests are deliberately run in this order so the report reads:
#   F1 baseline HTTP       (fastest path, should be near-instant)
#   F2 renderJs=true       (forces chromedp, the path the user is benchmarking)
#   F3 renderJs+waitFor    (user-supplied wait budget)
#   F4 multi-format        (markdown + html + links + imageLinks)
#   F5 cssSelector         (targeted extraction)
#   F6 include/excludeTags (sanitization path)
#   F7 browser=chrome      (pin to chrome for /scrape chain)
#
FEATURE_BODIES=(
  "F1_http_baseline|{\"url\":\"__URL__\",\"formats\":[\"markdown\"]}"
  "F2_renderJs|{\"url\":\"__URL__\",\"formats\":[\"markdown\"],\"renderJs\":true,\"browser\":\"chrome\"}"
  "F3_renderJs_wait2s|{\"url\":\"__URL__\",\"formats\":[\"markdown\"],\"renderJs\":true,\"browser\":\"chrome\",\"waitFor\":2000}"
  "F4_multi_format|{\"url\":\"__URL__\",\"formats\":[\"markdown\",\"html\",\"links\",\"imageLinks\"],\"renderJs\":true,\"browser\":\"chrome\"}"
  "F5_css_selector|{\"url\":\"__URL__\",\"formats\":[\"markdown\"],\"renderJs\":true,\"browser\":\"chrome\",\"cssSelector\":\"body\"}"
  "F6_tag_filter|{\"url\":\"__URL__\",\"formats\":[\"markdown\"],\"renderJs\":true,\"browser\":\"chrome\",\"includeTags\":[\"h1\",\"h2\",\"p\"],\"excludeTags\":[\"script\",\"style\",\"nav\"]}"
  "F7_browser_chrome_pin|{\"url\":\"__URL__\",\"formats\":[\"markdown\"],\"renderJs\":true,\"browser\":\"chrome\"}"
)

# ---------------------------------------------------------------------------
# Run the matrix
# ---------------------------------------------------------------------------
hdr "Running feature matrix"
hdr "Sites: $SITES"
log "Output dir: $OUT_DIR"
log ""

# Sites in turn.
for site in $SITES; do
  hdr "Site: $site"
  for spec in "${FEATURE_BODIES[@]}"; do
    label="${spec%%|*}"
    tmpl="${spec##*|}"
    body="${tmpl//__URL__/$site}"
    run_one "scrape"      "$site" "$label" "$body"
    run_one "scrape-core" "$site" "$label" "$body"
  done
  hr
done

# ---------------------------------------------------------------------------
# Summary report
# ---------------------------------------------------------------------------
hdr "Summary (median of nothing — single run, raw wall-clock)"
hr
log ""
printf "${BOLD}%-13s %-45s %-18s %-9s %9s %9s${NC}\n" \
  "ENDPOINT" "SITE" "FEATURE" "STATUS" "TIME_MS" "MD_BYTES"
hr
# Pretty-print the CSV (skip header).
tail -n +2 "$RESULTS_CSV" | awk -F, '{
  printf "%-13s %-45s %-18s %-9s %9s %9s\n",
    $1, substr($2, 1, 45), $3, $4, $5, $7
}'
hr

# Compute per-site speedup.
hdr "Per-site comparison (renderJs feature F2)"
log ""
printf "${BOLD}%-45s %12s %12s %10s${NC}\n" "SITE" "SCRAPE(ms)" "CORE(ms)" "DELTA"
hr
tail -n +2 "$RESULTS_CSV" | awk -F, '
  $1=="scrape"      && $3=="F2_renderJs" { s[$2]=$5 }
  $1=="scrape-core" && $3=="F2_renderJs" { c[$2]=$5 }
  END {
    for (k in s) {
      sc = s[k] + 0
      co = c[k] + 0
      delta = co - sc
      printf "%-45s %12s %12s %+10s\n", substr(k,1,45), sc, co, delta
    }
  }
' | sort
hr

log ""
ok "Full CSV:        $RESULTS_CSV"
ok "Per-run bodies:  $OUT_DIR/*.json"
log ""
log "Open the CSV in a spreadsheet, or run:"
log "  column -ts, $RESULTS_CSV | less -S"
