# QuickCrawl Scrape-Evals Benchmark

Tests QuickCrawl against Firecrawl's [scrape-content-dataset-v1](https://huggingface.co/datasets/firecrawl/scrape-content-dataset-v1) — 1000 real URLs with ground-truth content and noise annotations.

## What It Measures

| Metric | Description |
|--------|-------------|
| **Coverage** | Success rate of scrape calls (HTTP 2xx with markdown) |
| **Latency** | avg / P50 / P95 / P99 for successful requests |
| **Truth recall** | % of expected content phrases (≥20 chars) found in the output |
| **Noise rejection** | % of successful scrapes that do NOT leak the annotated "lie" text |

## Setup

This benchmark uses [uv](https://github.com/astral-sh/uv) for environment and package management.

```bash
# Install
brew install uv
# or: curl -LsSf https://astral.sh/uv/install.sh | sh

# From the bench/ dir
uv sync                    # creates .venv and installs deps
```

## Dataset Caching

The HuggingFace dataset is downloaded once and cached in `bench/dataset_cache/`. Subsequent runs reuse the cache — no network round-trip.

- First run: `Downloading dataset to .../bench/dataset_cache (one-time, ~ a few MB)...`
- Later runs: `Loading dataset from local cache: .../bench/dataset_cache`

To force a fresh download, delete the cache directory:

```bash
rm -rf bench/dataset_cache
```

Override the cache location with `BENCH_DATASET_CACHE=/path/to/cache uv run benchmark.py`.

## Run

Make sure QC is running on the target port (default: `http://localhost:3000`):

```bash
# Default: 1000 URLs, 10 concurrent, 30s timeout
uv run benchmark.py

# Smoke test with 50 URLs
BENCH_MAX_URLS=50 uv run benchmark.py

# Higher concurrency
BENCH_CONCURRENCY=20 uv run benchmark.py

# Stricter timeout
BENCH_TIMEOUT=15 uv run benchmark.py

# Target a different QuickCrawl instance
QUICKCRAWL_API_URL=https://quickcrawl.example.com uv run benchmark.py
```

## Env Vars

| Var | Default | Description |
|-----|---------|-------------|
| `QUICKCRAWL_API_URL` | `http://localhost:3000` | Base URL of QuickCrawl |
| `BENCH_CONCURRENCY` | `10` | Max concurrent scrape calls |
| `BENCH_TIMEOUT` | `30` | Per-request timeout (seconds) |
| `BENCH_MAX_URLS` | `0` | Cap on URLs (0 = all 1000) |
| `BENCH_RESULTS_PATH` | `bench/results-{ts}-IST.json` | Override the timestamped results path |
| `BENCH_DATASET_CACHE` | `bench/dataset_cache/` | Override the HuggingFace dataset cache |

## Output Files

Each run writes two timestamped JSONs (IST = Asia/Kolkata, UTC+05:30):

| File | Contents |
|------|----------|
| `results-2026-06-04T17-39-20-IST.json` | Full run report: coverage, latency percentiles, truth recall, noise rejection, top errors |
| `failed-2026-06-04T17-39-20-IST.json` | Per-URL failure list with `url`, `status_code`, `error`, `latency_ms`, plus an `errors_summary` for quick aggregation |

Both files include a `date_time_ist` field with the full ISO-8601 timestamp including the `+05:30` offset, e.g. `"2026-06-04T17:39:20.219004+05:30"`.

The filename timestamp uses `-` instead of `:` to keep the file portable across filesystems (Windows, FAT, etc.).

```
================================================================
RESULTS
================================================================

📊 Coverage:
  Total URLs:      1000
  Successful:      942 (94.2%)
  Failed:          58 (5.8%)

⚡ Latency (successful requests):
  Average:         1234ms
  P50:             890ms
  P95:             4210ms
  P99:             7890ms

📝 Content Quality:
  Successful scrapes:   942
  Matchable (truth ok): 920  (rest = dataset has no usable truth_text)
  Truth recall (fair):  88.4% = 813/920
  Truth recall (naive): 86.3% = 813/942
  Noise rejection:      97.2% (noise NOT in output)
  Noise leaks:          26

❌ Top errors:
    31x  timeout
    12x  blocked by anti-bot protection
     8x  page load timed out
     7x  ...

Results saved to:        bench/results-2026-06-04T17-39-20-IST.json
Failed URLs saved to:    bench/failed-2026-06-04T17-39-20-IST.json
  (58 failed entries; inspect for retry / debugging)
```

The JSON report contains the same data plus percentile breakdowns, suitable for diffing across runs or tracking regressions.
