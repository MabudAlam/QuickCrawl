#!/usr/bin/env python3
"""Example: Crawl a website and get all discovered pages.

This example crawls a website starting from a seed URL, following links
up to a configurable depth, and returns scraped content from each page.
"""

from quickcrawl import QuickCrawlClient

def main():
    with QuickCrawlClient() as client:
        results = client.crawl(
            "https://example.com",
            max_depth=2,
            max_pages=5,
        )

        print(f"Crawled {len(results)} pages:\n")

        for i, result in enumerate(results, 1):
            title = result.get("metadata", {}).get("title", "No title")
            url = result.get("metadata", {}).get("sourceURL", "Unknown URL")
            print(f"{i}. {title}")
            print(f"   {url}")
            print()

if __name__ == "__main__":
    main()
