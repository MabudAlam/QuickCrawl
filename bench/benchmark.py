#!/usr/bin/env python3
"""QuickCrawl Scrape-Evals Benchmark.

Drives QuickCrawl's /v1/scrape against every row of
`firecrawl/scrape-content-dataset-v1` (configurable cap), records one
result per URL to a streaming JSONL, then prints and persists a
per-run aggregate. Mirrors the methodology documented in
docs/bench-methodology.md (-comparable): phrase filters, link-syntax
collapse, calibrated-recall denominator, p50/p90/p95/p99 latency,
noise-rejection.
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import re
import sys
import time
from dataclasses import asdict, dataclass, field
from datetime import datetime
from typing import Any
from zoneinfo import ZoneInfo

import aiohttp

# ── dataset cache pin ────────────────────────────────────────────────
# Set HF_DATASETS_CACHE BEFORE importing `datasets`; the library reads
# it at import time. Cache lives inside the bench/ folder so re-runs
# skip the HuggingFace download.
BENCH_DIR = os.path.dirname(os.path.abspath(__file__))
DATASET_CACHE = os.getenv("BENCH_DATASET_CACHE", os.path.join(BENCH_DIR, "dataset_cache"))
os.makedirs(DATASET_CACHE, exist_ok=True)
os.environ["HF_DATASETS_CACHE"] = DATASET_CACHE
os.environ.setdefault("HF_HUB_DISABLE_TELEMETRY", "1")

from datasets import load_dataset  # noqa: E402  (must follow env setup)

QUICKCRAWL_URL = os.getenv("QUICKCRAWL_API_URL", "http://localhost:3000")
MAX_CONCURRENT_REQUESTS = int(os.getenv("BENCH_CONCURRENCY", "4"))
REQUEST_TIMEOUT_SECONDS = int(os.getenv("BENCH_TIMEOUT", "30"))
MAX_URLS_TO_TEST = int(os.getenv("BENCH_MAX_URLS", "0"))  # 0 = all

# Each run lands in its own folder: <OUTPUT_FOLDER>/{summary.json,
# per_url_details.jsonl, failure_log.json}. Default is
# server-runs/Run_bench_<IST-timestamp>/. The override env is
# BENCH_RUN_DIR; the override CLI flag is --run-dir.
DEFAULT_OUTPUT_FOLDER = os.path.join(
    BENCH_DIR,
    "server-runs",
    f"Run_bench_{datetime.now(ZoneInfo('Asia/Kolkata')).strftime('%Y-%m-%dT%H-%M-%S-IST')}",
)
OUTPUT_FOLDER = os.getenv("BENCH_RUN_DIR", DEFAULT_OUTPUT_FOLDER)
SUMMARY_FILE = os.path.join(OUTPUT_FOLDER, "summary.json")
PER_URL_DETAILS_FILE = os.path.join(OUTPUT_FOLDER, "per_url_details.jsonl")
FAILURE_LOG_FILE = os.path.join(OUTPUT_FOLDER, "failure_log.json")

# Legacy single-file overrides. If a caller pins BENCH_RESULTS_PATH or
# BENCH_JSONL_PATH we honor it and put artifacts alongside that file, but
# the new layout (BENCH_RUN_DIR) is preferred.
_LEGACY_RESULTS_PATH = os.getenv("BENCH_RESULTS_PATH")
_LEGACY_JSONL_PATH = os.getenv("BENCH_JSONL_PATH")
if _LEGACY_RESULTS_PATH or _LEGACY_JSONL_PATH:
    SUMMARY_FILE = _LEGACY_RESULTS_PATH or os.path.join(OUTPUT_FOLDER, "summary.json")
    legacy_dir = os.path.dirname(SUMMARY_FILE) or "."
    PER_URL_DETAILS_FILE = _LEGACY_JSONL_PATH or os.path.join(legacy_dir, "per_url_details.jsonl")
    FAILURE_LOG_FILE = os.path.join(legacy_dir, "failure_log.json")

MARKDOWN_LINK_PATTERN = re.compile(r"\[([^\]]+)\]\([^)]+\)")


@dataclass
class UrlBenchmarkResult:
    """Per-URL outcome. One of these is written to the JSONL stream
    AND folded into RunTotals. expected_phrases_to_find is recorded so
    the calibrated denominator (urls_with_ground_truth) is reproducible
    from the JSONL alone.
    """

    url: str
    scrape_succeeded: bool = False
    api_status_code: int = 0
    extracted_text_length: int = 0
    has_ground_truth_to_check: bool = False
    enough_phrases_found: bool = False
    phrases_we_expected_to_find: int = 0
    phrases_we_actually_found: int = 0
    leaked_unwanted_phrases: bool = False
    forbidden_phrases_in_dataset: int = 0
    forbidden_phrases_we_leaked: int = 0
    response_time_ms: float = 0.0
    failure_reason: str = ""


@dataclass
class RunTotals:
    total_urls: int = 0
    successful_scrapes: int = 0
    failed_scrapes: int = 0
    response_times_ms: list[float] = field(default_factory=list)
    urls_with_ground_truth: int = 0
    urls_with_matched_phrases: int = 0
    urls_with_leaked_phrases: int = 0
    total_expected_phrases: int = 0
    total_phrases_found: int = 0
    failure_counts: dict[str, int] = field(default_factory=dict)
    failed_url_details: list[dict[str, Any]] = field(default_factory=list)


def remove_markdown_link_syntax(markdown_text: str) -> str:
    """Replace `[text](url)` with `text`. The dataset's
    ground_truth_phrases are written as a human would read them on the
    page; matching against the raw markdown would falsely miss phrases
    wrapped in link syntax. See docs/bench-methodology.md#anchor-collapse.
    """
    return MARKDOWN_LINK_PATTERN.sub(r"\1", markdown_text or "")


def split_into_phrases(raw_text_to_split: str, minimum_length: int) -> list[str]:
    """Newline-split a dataset annotation, dropping entries shorter
    than `minimum_length`. 20 chars for ground_truth_phrases (filters
    single words / accidental matches), 10 for forbidden_phrases
    (catches short injected noise)."""
    return [w.strip() for w in (raw_text_to_split or "").split("\n") if len(w.strip()) > minimum_length]


def count_substring_matches(searchable_text: str, phrases_to_search_for: list[str]) -> tuple[int, int]:
    """Lower-cased substring scan. Returns (matches, total_phrases)."""
    if not phrases_to_search_for:
        return 0, 0
    matches = sum(1 for phrase in phrases_to_search_for if phrase in searchable_text)
    return matches, len(phrases_to_search_for)


def build_searchable_text(markdown_text: str) -> str:
    """Build the search index. Concatenate the raw markdown with its
    link-syntax-removed form so a phrase matches whether the scraper
    emitted `[text](url)` or just `text` — applied uniformly to every
    tool, this is a fairness control (not a looser rule)."""
    if not markdown_text:
        return ""
    return (markdown_text + "\n" + remove_markdown_link_syntax(markdown_text)).lower()


def score_extracted_content(
    markdown_text: str,
    ground_truth_phrases: str,
    forbidden_phrases: str,
) -> tuple[bool, bool, bool, int, int, int, int]:
    """Score a single scrape.

    Returns
    -------
    (enough_phrases_found, has_ground_truth_to_check,
     leaked_unwanted_phrases, phrases_we_expected_to_find,
     phrases_we_actually_found, forbidden_phrases_in_dataset,
     forbidden_phrases_we_leaked)

    has_ground_truth_to_check = the row has at least one >20-char
    ground-truth phrase.
    enough_phrases_found = >=30% of expected phrases appear in the
    searchable text (calibrated denominator excludes rows with no
    truth label).
    leaked_unwanted_phrases = >=50% of forbidden phrases leaked into
    the searchable text (stricter threshold because forbidden phrases
    are shorter and a single match is more likely to be a false
    positive).
    """
    searchable_text = build_searchable_text(markdown_text)

    expected = [p.lower() for p in split_into_phrases(ground_truth_phrases, 20)]
    forbidden = [p.lower() for p in split_into_phrases(forbidden_phrases, 10)]

    num_expected = len(expected)
    num_forbidden = len(forbidden)

    has_ground_truth_to_check = num_expected > 0
    enough_phrases_found = False
    num_found = 0
    if has_ground_truth_to_check:
        num_found, _ = count_substring_matches(searchable_text, expected)
        enough_phrases_found = (num_found / num_expected) >= 0.3

    leaked_unwanted_phrases = False
    num_leaked = 0
    if num_forbidden:
        num_leaked, _ = count_substring_matches(searchable_text, forbidden)
        leaked_unwanted_phrases = (num_leaked / num_forbidden) >= 0.5

    return (
        enough_phrases_found,
        has_ground_truth_to_check,
        leaked_unwanted_phrases,
        num_expected,
        num_found,
        num_forbidden,
        num_leaked,
    )


async def scrape_url_via_api(
    session: aiohttp.ClientSession,
    target_url: str,
    timeout_seconds: int,
) -> dict[str, Any]:
    """POST /v1/scrape with the canonical benchmark payload. Returns
    a small dict the caller folds into a UrlBenchmarkResult — keeping
    the I/O shape here makes the rest of the pipeline testable without
    a running server."""
    payload = {"url": target_url, "formats": ["markdown"], "renderMode": "auto"}
    t0 = time.monotonic()
    async with session.post(
        f"{QUICKCRAWL_URL}/v1/scrape",
        json=payload,
        timeout=aiohttp.ClientTimeout(total=timeout_seconds),
    ) as resp:
        response_time_ms = (time.monotonic() - t0) * 1000
        api_response = await resp.json()
        extracted_markdown = ((api_response.get("data") or {}).get("markdown")) or ""
        return {
            "response_time_ms": response_time_ms,
            "api_status_code": resp.status,
            "extracted_markdown": extracted_markdown,
            "api_returned_content": bool(api_response.get("success")) and bool(extracted_markdown),
            "error_message": api_response.get("error"),
        }


SCRAPER_REGISTRY = {"quickcrawl": scrape_url_via_api}


async def score_one_url(
    session: aiohttp.ClientSession,
    scraper_name: str,
    dataset_entry: dict[str, Any],
    concurrency_limit: asyncio.Semaphore,
    timeout_seconds: int,
) -> UrlBenchmarkResult:
    """Run one row through the scraper registry, score it, and return
    a result. Network/timeout errors become populated `failure_reason`
    fields rather than raising — we want a result for every URL."""
    target_url = dataset_entry["url"]
    ground_truth_phrases = dataset_entry.get("truth_text", "") or ""
    forbidden_phrases = dataset_entry.get("lie_text", "") or ""

    result = UrlBenchmarkResult(url=target_url)
    async with concurrency_limit:
        try:
            api_response = await SCRAPER_REGISTRY[scraper_name](session, target_url, timeout_seconds)
            result.response_time_ms = api_response["response_time_ms"]
            result.api_status_code = api_response["api_status_code"]
            if api_response["api_returned_content"]:
                result.scrape_succeeded = True
                result.extracted_text_length = len(api_response["extracted_markdown"])
                (
                    result.enough_phrases_found,
                    result.has_ground_truth_to_check,
                    result.leaked_unwanted_phrases,
                    result.phrases_we_expected_to_find,
                    result.phrases_we_actually_found,
                    result.forbidden_phrases_in_dataset,
                    result.forbidden_phrases_we_leaked,
                ) = score_extracted_content(
                    api_response["extracted_markdown"], ground_truth_phrases, forbidden_phrases,
                )
            else:
                result.failure_reason = (api_response.get("error_message") or "no markdown")[:120]
        except asyncio.TimeoutError:
            result.response_time_ms = timeout_seconds * 1000
            result.failure_reason = "timeout"
        except Exception as exc:  # noqa: BLE001 — bench loop must not crash on row error
            result.failure_reason = str(exc)[:120]
    return result


def update_run_totals(result: UrlBenchmarkResult, totals: RunTotals) -> None:
    """Update aggregate counters from a single result. Kept separate
    from the result-construction path so the streaming JSONL write and
    the aggregate update can be ordered independently."""
    if result.scrape_succeeded:
        totals.successful_scrapes += 1
        totals.response_times_ms.append(result.response_time_ms)
        if result.has_ground_truth_to_check:
            totals.urls_with_ground_truth += 1
            totals.total_expected_phrases += result.phrases_we_expected_to_find
            totals.total_phrases_found += result.phrases_we_actually_found
            if result.enough_phrases_found:
                totals.urls_with_matched_phrases += 1
        if result.leaked_unwanted_phrases:
            totals.urls_with_leaked_phrases += 1
    else:
        totals.failed_scrapes += 1
        label = result.failure_reason[:40] or "unknown"
        totals.failure_counts[label] = totals.failure_counts.get(label, 0) + 1
        totals.failed_url_details.append(
            {
                "url": result.url,
                "status_code": result.api_status_code,
                "failure_reason": result.failure_reason or "unknown",
                "response_time_ms": round(result.response_time_ms, 1),
            }
        )


def compute_percentile(sorted_samples: list[float], q: float) -> float:
    """Linear-interpolated percentile, q in [0, 1]. Empty → 0."""
    if not sorted_samples:
        return 0.0
    if len(sorted_samples) == 1:
        return sorted_samples[0]
    pos = q * (len(sorted_samples) - 1)
    lo = int(pos)
    hi = min(lo + 1, len(sorted_samples) - 1)
    frac = pos - lo
    return sorted_samples[lo] * (1 - frac) + sorted_samples[hi] * frac


def build_run_summary(
    totals: RunTotals,
    timestamp_iso: str,
    scraper_name: str,
) -> dict[str, Any]:
    """Shape the aggregate report. Every config knob is echoed so the
    JSON is self-describing — max concurrent requests, request timeout,
    dataset identity, scraper identity, output folder, artifact paths."""
    sorted_response_times = sorted(totals.response_times_ms)
    avg = (sum(sorted_response_times) / len(sorted_response_times)) if sorted_response_times else 0.0
    p50 = compute_percentile(sorted_response_times, 0.50)
    p90 = compute_percentile(sorted_response_times, 0.90)
    p95 = compute_percentile(sorted_response_times, 0.95)
    p99 = compute_percentile(sorted_response_times, 0.99)

    successful = totals.successful_scrapes
    with_truth = totals.urls_with_ground_truth
    phrase_match_rate = (totals.urls_with_matched_phrases / with_truth * 100) if with_truth else 0.0
    phrase_match_rate_overall = (totals.urls_with_matched_phrases / successful * 100) if successful else 0.0
    clean_content_rate = ((successful - totals.urls_with_leaked_phrases) / successful * 100) if successful else 0.0
    success_rate = (successful / totals.total_urls * 100) if totals.total_urls else 0.0

    return {
        "schema": "quickcrawl.bench/v3",
        "timestamp_ist": timestamp_iso,
        "scraper": scraper_name,
        "dataset": "firecrawl/scrape-content-dataset-v1",
        "config": {
            "endpoint": QUICKCRAWL_URL,
            "max_concurrent_requests": MAX_CONCURRENT_REQUESTS,
            "request_timeout_seconds": REQUEST_TIMEOUT_SECONDS,
            "max_urls_to_test": MAX_URLS_TO_TEST,
            "total_urls": totals.total_urls,
        },
        "artifacts": {
            "output_folder": OUTPUT_FOLDER,
            "summary_file": SUMMARY_FILE,
            "per_url_details_file": PER_URL_DETAILS_FILE,
            "failure_log_file": FAILURE_LOG_FILE,
        },
        "coverage": {
            "total_urls": totals.total_urls,
            "successful_scrapes": successful,
            "failed_scrapes": totals.failed_scrapes,
            "success_rate_pct": round(success_rate, 2),
        },
        "response_time_ms": {
            "average_ms": round(avg, 1),
            "p50_ms": round(p50, 1),
            "p90_ms": round(p90, 1),
            "p95_ms": round(p95, 1),
            "p99_ms": round(p99, 1),
            "successful_request_count": len(sorted_response_times),
        },
        "content_accuracy": {
            "urls_with_ground_truth": with_truth,
            "total_phrases_to_find": totals.total_expected_phrases,
            "total_phrases_found": totals.total_phrases_found,
            "urls_with_matched_phrases": totals.urls_with_matched_phrases,
            "phrase_match_rate_pct": round(phrase_match_rate, 2),
            "phrase_match_rate_overall_pct": round(phrase_match_rate_overall, 2),
            "urls_with_leaked_phrases": totals.urls_with_leaked_phrases,
            "clean_content_rate_pct": round(clean_content_rate, 2),
        },
        "most_common_failures": dict(
            sorted(totals.failure_counts.items(), key=lambda kv: -kv[1])[:10]
        ),
    }


def print_run_summary(summary: dict[str, Any]) -> None:
    cfg = summary["config"]
    cov = summary["coverage"]
    rt = summary["response_time_ms"]
    qa = summary["content_accuracy"]
    print("\n" + "=" * 64)
    print("RESULTS — QuickCrawl scrape-evals")
    print("=" * 64)
    print(f"\nConfig: max_concurrent={cfg['max_concurrent_requests']}  "
          f"timeout={cfg['request_timeout_seconds']}s  "
          f"max_urls={cfg['max_urls_to_test']}  endpoint={cfg['endpoint']}")
    print(f"\n[Coverage]")
    print(f"  total_urls={cov['total_urls']}  successful={cov['successful_scrapes']}  "
          f"failed={cov['failed_scrapes']}  success_rate={cov['success_rate_pct']:.2f}%")
    print(f"\n[Response time ms]  successful_request_count={rt['successful_request_count']}")
    print(f"  avg={rt['average_ms']:.0f}  p50={rt['p50_ms']:.0f}  "
          f"p90={rt['p90_ms']:.0f}  p95={rt['p95_ms']:.0f}  p99={rt['p99_ms']:.0f}")
    print(f"\n[Content accuracy]")
    print(f"  urls_with_ground_truth={qa['urls_with_ground_truth']}")
    print(f"  total_phrases_to_find={qa['total_phrases_to_find']}  "
          f"total_phrases_found={qa['total_phrases_found']}")
    print(f"  phrase_match_rate={qa['phrase_match_rate_pct']:.2f}%  "
          f"phrase_match_rate_overall={qa['phrase_match_rate_overall_pct']:.2f}%")
    print(f"  urls_with_leaked_phrases={qa['urls_with_leaked_phrases']}  "
          f"clean_content_rate={qa['clean_content_rate_pct']:.2f}%")
    if summary["most_common_failures"]:
        print(f"\n[Most common failures]")
        for label, n in summary["most_common_failures"].items():
            print(f"  {n:4d}x  {label}")


def apply_command_line_overrides() -> None:
    """Allow CLI flags to win over env vars without losing the
    process-level config block. argparse is local to this function so
    the rest of the module reads module-level constants."""
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--concurrent-workers", type=int, default=MAX_CONCURRENT_REQUESTS)
    parser.add_argument("--per-request-timeout", type=int, default=REQUEST_TIMEOUT_SECONDS)
    parser.add_argument("--url-cap", type=int, default=MAX_URLS_TO_TEST)
    parser.add_argument("--endpoint", default=QUICKCRAWL_URL)
    parser.add_argument("--run-dir", default=OUTPUT_FOLDER,
                        help="Folder to write summary.json, per_url_details.jsonl, failure_log.json into. "
                             "Default: <BENCH_DIR>/server-runs/Run_bench_<IST-timestamp>/")
    parser.add_argument("--tool", default="quickcrawl")
    args, _unknown = parser.parse_known_args()

    globals()["MAX_CONCURRENT_REQUESTS"] = args.concurrent_workers
    globals()["REQUEST_TIMEOUT_SECONDS"] = args.per_request_timeout
    globals()["MAX_URLS_TO_TEST"] = args.url_cap
    globals()["QUICKCRAWL_URL"] = args.endpoint
    globals()["OUTPUT_FOLDER"] = args.run_dir
    globals()["SUMMARY_FILE"] = os.path.join(args.run_dir, "summary.json")
    globals()["PER_URL_DETAILS_FILE"] = os.path.join(args.run_dir, "per_url_details.jsonl")
    globals()["FAILURE_LOG_FILE"] = os.path.join(args.run_dir, "failure_log.json")
    globals()["SELECTED_SCRAPER"] = args.tool


def load_test_urls() -> list[dict[str, Any]]:
    cache_ok = os.path.isdir(DATASET_CACHE) and any(
        fname.endswith(".arrow")
        for root, _, files in os.walk(DATASET_CACHE)
        for fname in files
    )
    if cache_ok:
        print(f"[dataset] using cache: {DATASET_CACHE}")
    else:
        print(f"[dataset] downloading once into {DATASET_CACHE}")

    try:
        ds = load_dataset("firecrawl/scrape-content-dataset-v1", split="train")
    except Exception:
        print(f"[dataset] cache corrupt — clearing {DATASET_CACHE} and re-downloading")
        import shutil
        shutil.rmtree(DATASET_CACHE, ignore_errors=True)
        os.makedirs(DATASET_CACHE, exist_ok=True)
        ds = load_dataset("firecrawl/scrape-content-dataset-v1", split="train")

    rows = list(ds)
    if MAX_URLS_TO_TEST > 0:
        rows = rows[:MAX_URLS_TO_TEST]
    return rows


async def run_full_benchmark() -> None:
    apply_command_line_overrides()
    rows = load_test_urls()
    total = len(rows)
    scraper = globals().get("SELECTED_SCRAPER", "quickcrawl")
    if scraper not in SCRAPER_REGISTRY:
        raise SystemExit(f"unknown scraper {scraper!r}; available: {list(SCRAPER_REGISTRY)}")

    print(
        f"[plan] scraper={scraper} urls={total} max_concurrent={MAX_CONCURRENT_REQUESTS} "
        f"request_timeout={REQUEST_TIMEOUT_SECONDS}s endpoint={QUICKCRAWL_URL}"
    )

    totals = RunTotals(total_urls=total)
    concurrency_limit = asyncio.Semaphore(MAX_CONCURRENT_REQUESTS)

    os.makedirs(OUTPUT_FOLDER, exist_ok=True)
    print(f"[output-folder] {OUTPUT_FOLDER}")

    finished = 0
    with open(PER_URL_DETAILS_FILE, "w", encoding="utf-8") as sink:
        async with aiohttp.ClientSession() as session:
            tasks = [
                score_one_url(session, scraper, row, concurrency_limit, REQUEST_TIMEOUT_SECONDS)
                for row in rows
            ]
            for coro in asyncio.as_completed(tasks):
                result = await coro
                update_run_totals(result, totals)
                sink.write(json.dumps(asdict(result), ensure_ascii=False) + "\n")
                finished += 1
                if finished % 50 == 0 or finished == total:
                    cov_pct = totals.successful_scrapes / finished * 100 if finished else 0
                    print(
                        f"  [progress] {finished}/{total}  "
                        f"success_rate_so_far={cov_pct:.1f}%"
                    )

    now_ist = datetime.now(ZoneInfo("Asia/Kolkata"))
    timestamp_iso = now_ist.isoformat()
    summary = build_run_summary(totals, timestamp_iso, scraper)
    print_run_summary(summary)

    with open(SUMMARY_FILE, "w", encoding="utf-8") as fp:
        json.dump(summary, fp, indent=2)

    failure_log = {
        "schema": "quickcrawl.bench/v3",
        "timestamp_ist": timestamp_iso,
        "scraper": scraper,
        "endpoint": QUICKCRAWL_URL,
        "config": summary["config"],
        "total_failures": len(totals.failed_url_details),
        "failure_summary": dict(
            sorted(totals.failure_counts.items(), key=lambda kv: -kv[1])
        ),
        "failed_url_details": totals.failed_url_details,
    }
    with open(FAILURE_LOG_FILE, "w", encoding="utf-8") as fp:
        json.dump(failure_log, fp, indent=2)

    print(f"\n[artifacts] output-folder: {OUTPUT_FOLDER}")
    print(f"  summary            : {SUMMARY_FILE}")
    print(f"  per_url_details    : {PER_URL_DETAILS_FILE}  ({finished} records)")
    print(f"  failure_log        : {FAILURE_LOG_FILE}  "
          f"({len(totals.failed_url_details)} entries)")


if __name__ == "__main__":
    try:
        asyncio.run(run_full_benchmark())
    except KeyboardInterrupt:
        sys.exit(130)
