#!/usr/bin/env python3
"""Benchmark script to test quickcrawl fetch quality against article URLs."""

import json
import time
from pathlib import Path
from quickcrawl import QuickCrawlClient

URLS = [
    "https://www.dailymail.co.uk/news/article-15881367/ex-Nato-chief-chilling-warning-Starmer-spend-defence-Britain-cost-blood.html",
    "https://www.dailymail.co.uk/news/article-15881685/8-2-earthquake-Philippines.html",
    "https://www.dailymail.co.uk/news/article-15869813/Glamorous-sheep-farmer-faces-jail-secretly-building-second-home-inside-barn-40-acre-farm-despite-paying-council-tax.html",
    "https://www.hindustantimes.com/india-news/3-reasons-why-vijay-led-tvk-will-not-be-a-part-of-todays-india-bloc-meeting-101780894736751.html",
    "https://www.hindustantimes.com/india-news/3-reasons-why-vijay-led-tvk-will-not-be-a-part-of-todays-india-bloc-meeting-101780894736751.html",
    "https://www.hindustantimes.com/cricket/ind-vs-afg-live-cricket-score-india-vs-afghanistan-one-off-test-match-day-3-2026-new-chandigarh-june-8-101780882653311.html",
    "https://www.hindustantimes.com/trending/bengaluru-founder-says-finding-a-flat-was-a-nightmare-despite-80-000-monthly-budget-paid-5-lakh-deposit-101780890684540.html",
    "https://www.scmp.com/news/china/military/article/3356269/china-adds-warheads-nuclear-powers-walk-away-disarmament-sipri",
    "https://www.scmp.com/native/business/topics/how-design-drives-global-growth/article/3353635/how-changan-automobiles-design-driven-brand-strategy-shapes-its-global-expansion-plans?module=top_story&pgtype=homepage",
    "https://www.scmp.com/news/china/science/article/3356271/tibet-quartz-discovery-boosts-chinas-self-sufficiency-push-hi-tech-materials?module=top_story&pgtype=homepage",
    "https://www.theguardian.com/travel/2026/jun/08/ireland-joyce-country-western-lakes-unesco-geopark-county-galway-mayo",
    "https://www.theguardian.com/commentisfree/2026/jun/07/the-guardian-view-on-cancer-treatments-new-hope-for-patients-now-and-in-the-future",
    "https://www.theguardian.com/commentisfree/2026/jun/07/the-guardian-view-on-the-french-presidential-election-campaign-only-the-far-right-will-profit-from-division",
    "https://www.nytimes.com/2026/06/08/world/asia/north-korea-kim-jong-un-pandemic-economy.html",
    "https://www.nytimes.com/2026/06/05/dining/craft-restaurant-closing-tom-colicchio.html",
    "https://www.nytimes.com/2026/06/03/travel/uk-travelers-eta-outage.html",
]

OUTPUT_DIR = Path("bench_results")
OUTPUT_DIR.mkdir(exist_ok=True)


def main():
    results = []
    total_start = time.time()

    client = QuickCrawlClient(api_url="http://localhost:3000")

    for i, url in enumerate(URLS, 1):
        print(f"\n[{i}/{len(URLS)}] Fetching: {url[:60]}...")
        url_start = time.time()

        try:
            result = client.scrape(url, formats=["markdown", "html", "links"])
            elapsed = time.time() - url_start

            markdown = result.get("markdown", "")
            html = result.get("html", "")
            links = result.get("links", [])
            metadata = result.get("metadata", {})

            char_count = len(markdown)
            token_estimate = char_count // 4
            link_count = len(links) if links else 0

            publisher = "dailymail" if "dailymail" in url else "hindustantimes" if "hindustantimes" in url else "scmp" if "scmp" in url else "guardian" if "guardian" in url else "nytimes"

            print(f"  Title: {metadata.get('title', 'N/A')[:60]}")
            print(f"  Chars: {char_count:,} | Est Tokens: ~{token_estimate:,} | Links: {link_count} | Time: {elapsed:.1f}s")

            results.append({
                "url": url,
                "publisher": publisher,
                "title": metadata.get("title", ""),
                "char_count": char_count,
                "token_estimate": token_estimate,
                "link_count": link_count,
                "elapsed_secs": round(elapsed, 2),
                "success": True,
            })

            with open(OUTPUT_DIR / f"url_{i:02d}_{publisher}.md", "w") as f:
                f.write(f"# {metadata.get('title', 'Untitled')}\n\n")
                f.write(f"Source: {url}\n\n")
                f.write(markdown)

        except Exception as e:
            elapsed = time.time() - url_start
            print(f"  ERROR: {e}")
            results.append({
                "url": url,
                "publisher": "unknown",
                "char_count": 0,
                "token_estimate": 0,
                "link_count": 0,
                "elapsed_secs": round(elapsed, 2),
                "success": False,
                "error": str(e),
            })

    total_elapsed = time.time() - total_start

    print("\n" + "=" * 70)
    print("BENCHMARK SUMMARY")
    print("=" * 70)

    successful = [r for r in results if r["success"]]
    failed = [r for r in results if not r["success"]]

    print(f"\nTotal URLs: {len(URLS)} | Success: {len(successful)} | Failed: {len(failed)}")
    print(f"Total time: {total_elapsed:.1f}s")

    if successful:
        total_chars = sum(r["char_count"] for r in successful)
        total_tokens = sum(r["token_estimate"] for r in successful)
        avg_chars = total_chars / len(successful)
        avg_tokens = total_tokens / len(successful)

        print(f"\nTotal chars returned: {total_chars:,}")
        print(f"Avg chars/page: {avg_chars:,.0f}")
        print(f"Avg est tokens/page: ~{avg_tokens:,.0f}")

        by_publisher = {}
        for r in successful:
            pub = r["publisher"]
            if pub not in by_publisher:
                by_publisher[pub] = []
            by_publisher[pub].append(r)

        print("\n--- By Publisher ---")
        for pub, rows in by_publisher.items():
            avg_c = sum(r["char_count"] for r in rows) / len(rows)
            avg_t = sum(r["token_estimate"] for r in rows) / len(rows)
            print(f"  {pub}: {len(rows)} pages | avg chars: {avg_c:,.0f} | avg tokens: ~{avg_t:,.0f}")

    summary_path = OUTPUT_DIR / "summary.json"
    with open(summary_path, "w") as f:
        json.dump({
            "total_urls": len(URLS),
            "successful": len(successful),
            "failed": len(failed),
            "total_time_secs": round(total_elapsed, 2),
            "results": results,
        }, f, indent=2)

    print(f"\nResults saved to: {OUTPUT_DIR}/")
    print(f"Summary: {summary_path}")


if __name__ == "__main__":
    main()
