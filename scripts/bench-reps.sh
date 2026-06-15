#!/bin/bash
#
# Focused micro-benchmark: runs each (site, feature) combo N times and reports
# median, min, max. Cuts through the per-run variance the single-run
# benchmark shows on sites like cloudflare.com (~30% run-to-run variance).
#
# Usage:
#   ./scripts/bench-reps.sh                       # default: 3 reps, 4 sites, F1+F2
#   REPS=5 ./scripts/bench-reps.sh               # 5 reps
#   SITES="https://go.dev/doc/" ./scripts/bench-reps.sh

set -u

BASE_URL="${BASE_URL:-http://0.0.0.0:3000}"
TIMEOUT="${TIMEOUT:-60}"
REPS="${REPS:-3}"

SITES="${SITES:-https://stripe.com/docs/api https://app.slack.com/ https://go.dev/doc/ https://www.cloudflare.com/}"

FEATURES=(
  "F1_http|{\"url\":\"__URL__\",\"formats\":[\"markdown\"]}"
  "F2_renderMode|{\"url\":\"__URL__\",\"formats\":[\"markdown\"],\"renderMode\":\"browser\"}"
)

OUT=$(mktemp -t bench-reps.XXXXXX.csv)
trap 'rm -f "$OUT"' EXIT
echo "site,endpoint,feature,rep,ms" > "$OUT"

run_one() {
  local endpoint="$1" site="$2" feat="$3" body="$4" rep="$5"
  local t
  t=$(curl -sS -o /dev/null -w '%{time_total}' \
    -X POST "$BASE_URL/v1/$endpoint" \
    -H 'Content-Type: application/json' \
    -d "$body" \
    --max-time "$TIMEOUT" 2>/dev/null) || t="0"
  local ms
  ms=$(python3 -c "print(int(float('$t') * 1000))")
  echo "$site,$endpoint,$feat,$rep,$ms" >> "$OUT"
}

echo "Running $REPS reps per (site, endpoint, feature)..."
for site in $SITES; do
  for spec in "${FEATURES[@]}"; do
    label="${spec%%|*}"
    tmpl="${spec##*|}"
    body="${tmpl//__URL__/$site}"
    for endpoint in scrape scrape-core; do
      for ((i=1; i<=REPS; i++)); do
        run_one "$endpoint" "$site" "$label" "$body" "$i"
        printf "."
      done
    done
  done
  printf "  (%s done)\n" "$site"
done
echo
echo

# Compute medians with python (no python aggregation; we already wrote per-rep).
python3 - "$OUT" <<'PY'
import csv, statistics, sys
rows = list(csv.DictReader(open(sys.argv[1])))

# Group by (site, endpoint, feature)
groups = {}
for r in rows:
    k = (r['site'], r['endpoint'], r['feature'])
    groups.setdefault(k, []).append(int(r['ms']))

# F2 (renderMode) summary table.
sites = sorted({k[0] for k in groups})
print()
print("F2 (renderMode=browser) median over N=3 reps")
print("─" * 78)
print(f"{'Site':<40} {'/scrape':>12} {'/scrape-core':>14} {'Δ (ms)':>10} {'Δ (%)':>8}")
print("─" * 78)

for site in sites:
    s_ms = statistics.median(groups.get((site, 'scrape', 'F2_renderMode'), [0]))
    c_ms = statistics.median(groups.get((site, 'scrape-core', 'F2_renderMode'), [0]))
    delta = c_ms - s_ms
    pct = (delta / s_ms * 100) if s_ms else 0
    sign = "+" if delta >= 0 else ""
    site_s = site if len(site) < 40 else site[:37] + "..."
    print(f"{site_s:<40} {s_ms:>10.0f}ms {c_ms:>12.0f}ms {sign}{delta:>7.0f}ms {sign}{pct:>6.1f}%")

# F2 - F1 (JS overhead) summary
print()
print(f"JS overhead (F2 - F1, median)")
print("─" * 78)
print(f"{'Site':<40} {'/scrape':>12} {'/scrape-core':>14} {'core-savings':>12}")
print("─" * 78)
for site in sites:
    s_f1 = statistics.median(groups.get((site, 'scrape', 'F1_http'), [0]))
    s_f2 = statistics.median(groups.get((site, 'scrape', 'F2_renderMode'), [0]))
    c_f1 = statistics.median(groups.get((site, 'scrape-core', 'F1_http'), [0]))
    c_f2 = statistics.median(groups.get((site, 'scrape-core', 'F2_renderMode'), [0]))
    s_js = s_f2 - s_f1
    c_js = c_f2 - c_f1
    savings = s_js - c_js
    site_s = site if len(site) < 40 else site[:37] + "..."
    sign = "+" if savings >= 0 else ""
    print(f"{site_s:<40} {s_js:>10.0f}ms {c_js:>12.0f}ms {sign}{savings:>9.0f}ms")
PY
