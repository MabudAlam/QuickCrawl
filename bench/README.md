# QuickCrawl Scrape-Evals Benchmark

Evaluates QuickCrawl's scrape quality against
[`firecrawl/scrape-content-dataset-v1`](https://huggingface.co/datasets/firecrawl/scrape-content-dataset-v1)
(1000 real URLs with ground-truth content and noise annotations).
Methodology is CRW-comparable: same phrase filters, same link-syntax collapse,
same calibrated-recall denominator. See [Methodology](#methodology) for details.

## What It Measures

| Metric | Description |
|--------|-------------|
| **Success rate** | % of requests that returned a non-empty markdown (HTTP 2xx with body) |
| **Response time** | average / p50 / p90 / p95 / p99 for successful requests (ms) |
| **Phrase match rate** | % of URLs with ground-truth where ≥30% of expected phrases (≥20 chars) appear in the output |
| **Phrase match rate (overall)** | same numerator, but denominator is all successful scrapes (no labeling filter) |
| **Clean content rate** | % of successful scrapes that did **not** leak annotated forbidden phrases |
| **Phrase totals** | total phrases we tried to find vs. total phrases we actually found |
| **Most common failures** | Top 10 failure reasons with counts |

## Setup

The benchmark uses [uv](https://github.com/astral-sh/uv) for env + deps.

```bash
# Install uv (one-time)
brew install uv
# or: curl -LsSf https://astral.sh/uv/install.sh | sh

# From the bench/ directory
cd bench
uv sync                         # creates .venv and installs deps
```

## What "concurrency" and "timeout" mean

| Flag | What it controls | Where it lives |
|------|------------------|----------------|
| `BENCH_CONCURRENCY` | **Number of in-flight requests at once** (an `asyncio.Semaphore` around the scraper loop) | Client side |
| `BENCH_TIMEOUT` | **Client-side deadline for the HTTP round-trip** (`aiohttp.ClientTimeout(total=...)`); the benchmark gives up waiting for QuickCrawl's response after this many seconds | Client side |

> **Important — the timeout does NOT bound the scrape on the server.**
> `BENCH_TIMEOUT` is the wait-for-response budget on the benchmark side. It is
> **not** sent in the request body. If QuickCrawl's internal scrape takes
> longer than `BENCH_TIMEOUT`, the client records a `timeout` failure even
> though the server may eventually finish successfully.
>
> For accurate results, set QuickCrawl's **server-side** per-scrape budget
> (in `quickcrawl.toml` or server flags) to be **≤** `BENCH_TIMEOUT`. The
> client then always sees a clean response within its deadline.

## Recommended Settings

| Scenario | `BENCH_CONCURRENCY` | `BENCH_TIMEOUT` | `BENCH_MAX_URLS` | Why |
|----------|---------------------|-----------------|------------------|-----|
| Smoke test (verify setup) | `5` | `30` | `10` | Fast, low cost, catches wiring issues |
| Local full run (dev box) | `10` | `120` | `1000` | Mirrors the CRW published run; headroom for browser-render path |
| Railway / free-tier deploy | `3` | `120` | `1000` | Lower concurrency avoids CPU throttling on shared CPU plans |
| Railway / paid deploy | `10` | `120` | `1000` | Matches local numbers for honest comparison |
| CI regression check | `5` | `60` | `150` | Sub-15-min run, statistically useful on this dataset |

> Set QuickCrawl's **server-side** scrape budget to **≤ `BENCH_TIMEOUT − 10s`**
> so the server self-aborts slightly before the client gives up. With
> `BENCH_TIMEOUT=120`, configure the server for `~110s` per scrape.

## Running the Benchmark

QuickCrawl must be running on the target port. The default is
`http://localhost:3000`; override with `QUICKCRAWL_API_URL` for a remote
instance (Railway, staging, etc.).

### Smoke test (10 URLs, ~30 seconds)

```bash
QUICKCRAWL_API_URL=http://localhost:3000 \
BENCH_CONCURRENCY=5 \
BENCH_TIMEOUT=30 \
BENCH_MAX_URLS=10 \
uv run benchmark.py
```

### Local full run (1000 URLs, ~25 minutes at concurrency 10)

```bash
QUICKCRAWL_API_URL=http://localhost:3000 \
BENCH_CONCURRENCY=10 \
BENCH_TIMEOUT=120 \
BENCH_MAX_URLS=1000 \
uv run benchmark.py
```

### Remote / Railway run

```bash
QUICKCRAWL_API_URL=https://your-app.up.railway.app \
BENCH_CONCURRENCY=5 \
BENCH_TIMEOUT=120 \
BENCH_MAX_URLS=1000 \
uv run benchmark.py
```

### CLI flags (override env vars)

```bash
uv run benchmark.py \
  --concurrent-workers 5 \
  --per-request-timeout 120 \
  --url-cap 1000 \
  --endpoint http://localhost:3000 \
  --run-dir server-runs/Run_bench_2026-06-16T09-13-16-IST
```

`--run-dir` overrides the default `bench/server-runs/Run_bench_<IST-timestamp>/`
folder. Inside that folder the script writes the three fixed-name artifacts
listed in [Output Artifacts](#output-artifacts).

### Override the dataset cache location

```bash
BENCH_DATASET_CACHE=/persistent/volume/bench_cache uv run benchmark.py
```

Useful on Railway / containers where `bench/` may be ephemeral.

### Pin the output folder

```bash
BENCH_RUN_DIR=server-runs/Run_bench_2026-06-16T09-13-16-IST uv run benchmark.py
```

Default is `bench/server-runs/Run_bench_<IST-timestamp>/` — each run lands in
its own folder so multiple runs never overwrite each other's artifacts.

## Environment Variables

| Var | Default | Description |
|-----|---------|-------------|
| `QUICKCRAWL_API_URL` | `http://localhost:3000` | Base URL of QuickCrawl |
| `BENCH_CONCURRENCY` | `4` | Max concurrent in-flight scrape requests |
| `BENCH_TIMEOUT` | `30` | Per-request client-side deadline (seconds) |
| `BENCH_MAX_URLS` | `0` | URL cap (0 = all 1000 in the dataset) |
| `BENCH_RUN_DIR` | `bench/server-runs/Run_bench_<IST-timestamp>/` | Output folder (preferred) |
| `BENCH_RESULTS_PATH` | *legacy* — see below | Summary JSON path (single-file override) |
| `BENCH_JSONL_PATH` | *legacy* — see below | Per-URL JSONL path (single-file override) |
| `BENCH_DATASET_CACHE` | `bench/dataset_cache/` | HuggingFace dataset cache override |

`BENCH_RUN_DIR` is the **preferred** way to pin artifact output — it sets the
folder and the three files inside it. The legacy `BENCH_RESULTS_PATH` /
`BENCH_JSONL_PATH` overrides still work for callers that want a single-file
layout; when set, `failure_log.json` is auto-placed alongside them.

CLI flags win over env vars; env vars win over defaults. See
`uv run benchmark.py --help` for the full list.

## Output Artifacts

Every run lands in its own folder so multiple runs never overwrite each other:

```
bench/server-runs/
└── Run_bench_2026-06-16T09-13-16-IST/   ← BENCH_RUN_DIR / --run-dir
    ├── summary.json                  ← aggregate report (see Summary Schema)
    ├── per_url_details.jsonl         ← streaming per-URL records (one per line)
    └── failure_log.json              ← failures only, with failure_summary + failed_url_details
```

| File | Contents |
|------|----------|
| `summary.json` | Aggregate report: config, coverage, response time p50/p90/p95/p99, phrase match rate, clean content rate, most common failures, artifact paths |
| `per_url_details.jsonl` | **Streaming per-URL JSONL** — one result per line, written as each request completes (crash-safe on long runs). Includes `phrases_we_expected_to_find` / `phrases_we_actually_found` per row so the headline phrase match rate is reproducible from this file alone. |
| `failure_log.json` | Per-URL failure list with `url`, `status_code`, `failure_reason`, `response_time_ms`, plus `failure_summary` for aggregation. Same `config` block as the summary, so the file is self-describing. |

The JSONL stream is the **primary record of the run**. If the script crashes
mid-run (OOM, SIGTERM, network blip), the JSONL still has every result
written so far — re-aggregate by streaming `wc -l` + `jq` over the file.

To run multiple variants back-to-back, each lands in its own folder:

```bash
for cfg in "5:30:http://localhost:3000" "10:120:http://localhost:3000" "5:120:https://staging.example.com"; do
  IFS=: read -r c t e <<< "$cfg"
  QUICKCRAWL_API_URL="$e" BENCH_CONCURRENCY=$c BENCH_TIMEOUT=$t \
  BENCH_MAX_URLS=150 uv run benchmark.py
done
ls server-runs/
# Run_bench_2026-06-16T09-13-16-IST/
# Run_bench_2026-06-16T09-14-02-IST/
# Run_bench_2026-06-16T09-15-11-IST/
```

## Summary Schema

```jsonc
{
  "schema": "quickcrawl.bench/v3",
  "timestamp_ist": "2026-06-15T22:32:34.820391+05:30",
  "scraper": "quickcrawl",
  "dataset": "firecrawl/scrape-content-dataset-v1",
  "config": {
    "endpoint": "http://localhost:3000",
    "max_concurrent_requests": 10,
    "request_timeout_seconds": 120,
    "max_urls_to_test": 1000,
    "total_urls": 1000
  },
  "artifacts": {
    "output_folder": "bench/server-runs/Run_bench_2026-06-15T22-32-34-IST",
    "summary_file": "bench/server-runs/Run_bench_2026-06-15T22-32-34-IST/summary.json",
    "per_url_details_file": "bench/server-runs/Run_bench_2026-06-15T22-32-34-IST/per_url_details.jsonl",
    "failure_log_file": "bench/server-runs/Run_bench_2026-06-15T22-32-34-IST/failure_log.json"
  },
  "coverage": {
    "total_urls": 1000,
    "successful_scrapes": 877,
    "failed_scrapes": 123,
    "success_rate_pct": 87.7
  },
  "response_time_ms": {
    "average_ms": 1234.0,
    "p50_ms":  890.0,
    "p90_ms":  4250.0,
    "p95_ms":  4210.0,
    "p99_ms":  7890.0,
    "successful_request_count": 877
  },
  "content_accuracy": {
    "urls_with_ground_truth": 819,
    "total_phrases_to_find": 3210,
    "total_phrases_found": 2890,
    "urls_with_matched_phrases": 522,
    "phrase_match_rate_pct": 63.74,
    "phrase_match_rate_overall_pct": 59.52,
    "urls_with_leaked_phrases": 26,
    "clean_content_rate_pct": 97.03
  },
  "most_common_failures": {
    "timeout": 31,
    "blocked by anti-bot protection": 12,
    "page load timed out": 8
  }
}
```

### Per-URL JSONL record shape

```jsonc
{
  "url": "https://example.com/article",
  "scrape_succeeded": true,
  "api_status_code": 200,
  "extracted_text_length": 32147,
  "has_ground_truth_to_check": true,
  "enough_phrases_found": true,
  "phrases_we_expected_to_find": 4,        // = truth_total in CRW terms
  "phrases_we_actually_found": 3,          // phrases that actually matched
  "leaked_unwanted_phrases": false,
  "forbidden_phrases_in_dataset": 2,
  "forbidden_phrases_we_leaked": 0,
  "response_time_ms": 1283.5,
  "failure_reason": ""
}
```

The `phrases_we_expected_to_find` and `phrases_we_actually_found` per row are
the denominators behind phrase match rate — recompute the headline number
yourself from the JSONL with `jq` if you want to verify the aggregate.

## Console Output

```
================================================================
RESULTS — QuickCrawl scrape-evals
================================================================

Config: max_concurrent=10  timeout=120s  max_urls=1000  endpoint=http://localhost:3000

[Coverage]
  total_urls=1000  successful=877  failed=123  success_rate=87.70%

[Response time ms]  successful_request_count=877
  avg=1234  p50=890  p90=4250  p95=4210  p99=7890

[Content accuracy]
  urls_with_ground_truth=819
  total_phrases_to_find=3210  total_phrases_found=2890
  phrase_match_rate=63.74%  phrase_match_rate_overall=59.52%
  urls_with_leaked_phrases=26  clean_content_rate=97.03%

[Most common failures]
    31x  timeout
    12x  blocked by anti-bot protection
     8x  page load timed out

[artifacts] output-folder: bench/server-runs/Run_bench_2026-06-15T22-32-34-IST
  summary            : bench/server-runs/Run_bench_2026-06-15T22-32-34-IST/summary.json
  per_url_details    : bench/server-runs/Run_bench_2026-06-15T22-32-34-IST/per_url_details.jsonl
  failure_log        : bench/server-runs/Run_bench_2026-06-15T22-32-34-IST/failure_log.json
```

## Methodology

The benchmark is **CRW-comparable**: identical phrase filters, the same
link-syntax collapse fairness control, the same calibrated-recall denominator.
Concretely:

- **Phrase filters.** Ground-truth phrases (`truth_text`) are split on newline
  and any line ≤ 20 chars is dropped. Forbidden phrases (`lie_text`) drop
  lines ≤ 10 chars (looser because noise can be short).
- **Link-syntax collapse.** A second copy of the markdown is built with
  every `[text](url)` replaced by `text`. The search index is
  `lowered(md + "\n" + remove_markdown_link_syntax(md))` so a phrase matches
  whether the scraper emitted link syntax or just the visible text. Applied
  uniformly to every tool — a fairness control, not a looser rule.
- **Phrase match threshold.** A row counts as `enough_phrases_found` if
  ≥ 30% of expected phrases appear in the index. Stricter than a
  single-match rule but tolerant of pages where the scraper surfaced 9/10
  facts.
- **Forbidden phrase threshold.** A row counts as `leaked_unwanted_phrases`
  if ≥ 50% of forbidden phrases leaked into the output. Stricter threshold
  than recall because single-match false positives are more likely on
  short forbidden phrases.
- **Calibrated denominator.** `phrase_match_rate_pct` divides by the count
  of `urls_with_ground_truth` (successful scrape AND has at least one
  >20-char expected phrase). This matches the 819-style denominator CRW
  uses and excludes bench artifacts — rows with no truth label are not
  the scraper's fault.
- **Streaming JSONL.** Every result is written to disk before the next is
  awaited, so a 30-minute run is crash-safe.
- **Percentiles.** Linear-interpolated (not floored-index), reported as
  p50 / p90 / p95 / p99 over successful-request response times only.



## What This Benchmark Does Not Measure

Like any honest benchmark, the omissions are listed explicitly:

- **Anti-bot rotation under adversarial load.** The dataset URLs are
  mostly cooperative; aggressive WAF / Cloudflare Turnstile / PerimeterX
  flows are not in scope.
- **Captcha-solver economics.** No scraper in this run was configured
  with a paid captcha-solving provider.
- **JS-heavy SPAs with auth walls.** Pages that need a logged-in session,
  OAuth bounce, or a CSRF handshake are excluded because labeling them
  objectively is intractable.
- **Geo-restricted content.** The run is executed from a single region;
  multi-region latency variance is a separate measurement.
- **Long-tail format coverage.** Phrase-match here judges text extraction.
  Screenshot, PDF, and structured-JSON extraction modes have their own
  evaluation harnesses.
