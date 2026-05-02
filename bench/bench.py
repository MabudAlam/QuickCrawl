
import json
import time
import sys
import os
import asyncio
import aiohttp
from dataclasses import dataclass, field
from datetime import datetime
from datasets import load_dataset

API_ENDPOINT = os.getenv("API_URL", "http://localhost:3000")
PARALLEL_REQUESTS = int(os.getenv("BENCH_CONCURRENCY", "10"))
REQUEST_TIMEOUT = int(os.getenv("BENCH_TIMEOUT", "30"))
URL_BATCH_LIMIT = int(os.getenv("BENCH_MAX_URLS", "0"))

@dataclass
class ScrapeResult:
    url: str
    succeeded: bool = False
    http_status: int = 0
    markdown_retrieved: bool = False
    content_verified: bool = False
    noise_detected: bool = False
    response_time_ms: float = 0
    failure_reason: str = ""
    content_size: int = 0

@dataclass
class AggregateStats:
    total_count: int = 0
    success_count: int = 0
    failure_count: int = 0
    content_matches: int = 0
    noise_catches: int = 0
    response_times: list = field(default_factory=list)
    failure_log: dict = field(default_factory=dict)
    failed_endpoints: list = field(default_factory=list)
    all_results: list = field(default_factory=list)

def evaluate_extraction(markdown: str, expected_text: str, junk_text: str):
    print(f"[DEBUG] evaluate_extraction called with markdown length={len(markdown)}")
    md_lower = markdown.lower()
    expected_phrases = [p.strip().lower() for p in expected_text.split("\n") if len(p.strip()) > 20]
    content_found = False
    if expected_phrases:
        matches = sum(1 for phrase in expected_phrases if phrase in md_lower)
        content_found = matches / len(expected_phrases) >= 0.3
        print(f"[DEBUG] expected_phrases count={len(expected_phrases)}, matches={matches}, content_found={content_found}")

    junk_phrases = [p.strip().lower() for p in junk_text.split("\n") if len(p.strip()) > 10]
    noise_found = False
    if junk_phrases:
        matches = sum(1 for phrase in junk_phrases if phrase in md_lower)
        noise_found = matches / len(junk_phrases) >= 0.5
        print(f"[DEBUG] junk_phrases count={len(junk_phrases)}, matches={matches}, noise_found={noise_found}")

    print(f"[DEBUG] evaluate_extraction returning content_found={content_found}, noise_found={noise_found}")
    return content_found, noise_found

async def fetch_page(session: aiohttp.ClientSession, url: str, expected: str, junk: str, limiter: asyncio.Semaphore) -> ScrapeResult:
    print(f"[DEBUG] fetch_page started for url={url}")
    record = ScrapeResult(url=url)
    async with limiter:
        try:
            start = time.monotonic()
            print(f"[DEBUG] Sending POST request to {API_ENDPOINT}/v1/scrape with url={url}")
            async with session.post(
                f"{API_ENDPOINT}/v1/scrape",
                json={"url": url, "formats": ["markdown"]},
                timeout=aiohttp.ClientTimeout(total=REQUEST_TIMEOUT),
            ) as response:
                record.response_time_ms = (time.monotonic() - start) * 1000
                record.http_status = response.status
                print(f"[DEBUG] Received response for {url}: status={response.status}, time_ms={record.response_time_ms:.2f}")
                payload = await response.json()

                if payload.get("success") and payload.get("data", {}).get("markdown"):
                    record.succeeded = True
                    record.markdown_retrieved = True
                    md = payload["data"]["markdown"]
                    record.content_size = len(md)
                    print(f"[DEBUG] Markdown retrieved for {url}: size={record.content_size}")
                    if expected and junk:
                        record.content_verified, record.noise_detected = evaluate_extraction(md, expected, junk)
                else:
                    record.failure_reason = payload.get("error", "no markdown")
                    print(f"[DEBUG] Scraping failed for {url}: {record.failure_reason}")
        except asyncio.TimeoutError:
            record.response_time_ms = REQUEST_TIMEOUT * 1000
            record.failure_reason = "timeout"
            print(f"[DEBUG] Timeout occurred for {url}")
        except Exception as e:
            record.failure_reason = str(e)[:100]
            print(f"[DEBUG] Exception occurred for {url}: {record.failure_reason}")
    print(f"[DEBUG] fetch_page completed for url={url}: succeeded={record.succeeded}, content_verified={record.content_verified}, noise_detected={record.noise_detected}")
    return record

async def run_benchmark():
    cache_location = os.path.join(os.path.dirname(os.path.abspath(__file__)), "dataset_cache")
    print(f"[DEBUG] Loading dataset from HuggingFace (cache: {cache_location})...")
    dataset = load_dataset("firecrawl/scrape-content-dataset-v1", split="train", cache_dir=cache_location)
    print(f"[DEBUG] Dataset loaded: type={type(dataset)}, length={len(dataset)}")

    url_list = list(dataset)
    if URL_BATCH_LIMIT > 0:
        url_list = url_list[:URL_BATCH_LIMIT]
    print(f"[DEBUG] URL batch limit: {URL_BATCH_LIMIT}, resulting url_list length: {len(url_list)}")

    total = len(url_list)
    print(f"[DEBUG] Starting benchmark: total={total}, parallelism={PARALLEL_REQUESTS}, timeout={REQUEST_TIMEOUT}s, target={API_ENDPOINT}")
    print("=" * 60)

    metrics = AggregateStats(total_count=total)
    gate = asyncio.Semaphore(PARALLEL_REQUESTS)
    finished = 0
    print(f"[DEBUG] Initialized metrics and semaphore with {PARALLEL_REQUESTS} permits")

    async with aiohttp.ClientSession() as session:
        print(f"[DEBUG] Created aiohttp ClientSession, starting to create {len(url_list)} tasks...")
        jobs = [
            fetch_page(session, row["url"], row.get("truth_text", ""), row.get("lie_text", ""), gate)
            for row in url_list
        ]
        print(f"[DEBUG] All {len(jobs)} tasks created, waiting for completion...")

        for coro in asyncio.as_completed(jobs):
            result = await coro
            finished += 1

            if result.succeeded:
                metrics.success_count += 1
                metrics.response_times.append(result.response_time_ms)
                if result.content_verified:
                    metrics.content_matches += 1
                if result.noise_detected:
                    metrics.noise_catches += 1
                print(f"[DEBUG] Result {finished}/{total}: SUCCESS - url={result.url}, time_ms={result.response_time_ms:.2f}, content_verified={result.content_verified}, noise_detected={result.noise_detected}")
            else:
                metrics.failure_count += 1
                err_key = result.failure_reason[:40] if result.failure_reason else "unknown"
                metrics.failure_log[err_key] = metrics.failure_log.get(err_key, 0) + 1
                metrics.failed_endpoints.append({
                    "url": result.url,
                    "reason": result.failure_reason,
                    "status": result.http_status,
                })
                print(f"[DEBUG] Result {finished}/{total}: FAILED - url={result.url}, reason={result.failure_reason}")

            metrics.all_results.append({
                "url": result.url,
                "success": result.succeeded,
                "status_code": result.http_status,
                "response_time_ms": round(result.response_time_ms, 2),
                "content_verified": result.content_verified,
                "noise_detected": result.noise_detected,
                "error": result.failure_reason,
            })

            if finished % 50 == 0 or finished == total:
                progress = finished / total * 100
                success_ratio = metrics.success_count / finished * 100 if finished else 0
                print(f"  [{finished}/{total}] {progress:.0f}% done | success rate: {success_ratio:.1f}%")

    print(f"[DEBUG] All tasks completed. Computing latency percentiles...")
    latency_samples = sorted(metrics.response_times)
    median = latency_samples[len(latency_samples) // 2] if latency_samples else 0
    p95 = latency_samples[int(len(latency_samples) * 0.95)] if latency_samples else 0
    p99 = latency_samples[int(len(latency_samples) * 0.99)] if latency_samples else 0
    mean = sum(latency_samples) / len(latency_samples) if latency_samples else 0
    print(f"[DEBUG] Latency computed: mean={mean:.2f}ms, median={median:.2f}ms, p95={p95:.2f}ms, p99={p99:.2f}ms")

    valid_samples = metrics.success_count
    precision_score = ((valid_samples - metrics.noise_catches) / valid_samples * 100) if valid_samples else 0
    recall_score = (metrics.content_matches / valid_samples * 100) if valid_samples else 0
    print(f"[DEBUG] Quality scores: precision={precision_score:.2f}%, recall={recall_score:.2f}%")

    print("\n" + "=" * 60)
    print("BENCHMARK RESULTS")
    print("=" * 60)
    print(f"\n📊 Coverage:")
    print(f"  Total endpoints:   {metrics.total_count}")
    print(f"  Successful:        {metrics.success_count} ({metrics.success_count/metrics.total_count*100:.1f}%)")
    print(f"  Failed:            {metrics.failure_count} ({metrics.failure_count/metrics.total_count*100:.1f}%)")

    print(f"\n⚡ Response time (successful requests):")
    print(f"  Mean:              {mean:.0f}ms")
    print(f"  Median:            {median:.0f}ms")
    print(f"  P95:              {p95:.0f}ms")
    print(f"  P99:              {p99:.0f}ms")

    print(f"\n📝 Content quality (on {valid_samples} successful scrapes):")
    print(f"  Content recall:    {recall_score:.1f}% (core content found)")
    print(f"  Noise rejection:   {precision_score:.1f}% (noise NOT in output)")
    print(f"  Content matches:  {metrics.content_matches}")
    print(f"  Noise leaks:      {metrics.noise_catches}")

    if metrics.failure_log:
        print(f"\n❌ Top failures:")
        for reason, count in sorted(metrics.failure_log.items(), key=lambda x: -x[1])[:10]:
            print(f"  {count:4d}x  {reason}")

    report = {
        "server": API_ENDPOINT,
        "total": metrics.total_count,
        "concurrency": PARALLEL_REQUESTS,
        "timeout_s": REQUEST_TIMEOUT,
        "coverage": {
            "success": metrics.success_count,
            "failed": metrics.failure_count,
            "success_rate": round(metrics.success_count / metrics.total_count * 100, 2),
        },
        "latency_ms": {
            "mean": round(mean, 1),
            "median": round(median, 1),
            "p95": round(p95, 1),
            "p99": round(p99, 1),
        },
        "quality": {
            "content_recall": round(recall_score, 2),
            "noise_rejection": round(precision_score, 2),
            "content_matches": metrics.content_matches,
            "noise_leaks": metrics.noise_catches,
        },
        "errors": dict(sorted(metrics.failure_log.items(), key=lambda x: -x[1])[:10]),
    }
    timestamp = datetime.now().strftime("%Y-%m-%d-%H-%M-%S")
    report_file = f"results_quickcrawl_{timestamp}.json"
    print(f"[DEBUG] Writing report to {report_file}")
    with open(report_file, "w") as f:
        json.dump(report, f, indent=2)
    print(f"\nDetailed results saved to {report_file}")

    flush = {
        "errors": dict(sorted(metrics.failure_log.items(), key=lambda x: -x[1])[:10]),
        "failed_urls": metrics.failed_endpoints,
        "results": metrics.all_results,
    }
    timestamp = datetime.now().strftime("%Y-%m-%d-%H-%M-%S")
    flush_file = f"flush_{timestamp}.json"
    print(f"[DEBUG] Writing flush data to {flush_file}")
    with open(flush_file, "w") as f:
        json.dump(flush, f, indent=2)
    print(f"Flush data saved to {flush_file}")

if __name__ == "__main__":
    asyncio.run(run_benchmark())
