#!/usr/bin/env python3
"""Example: Discover URLs on a website without scraping content.

This is useful for building a URL list before crawling, site audits,
or generating sitemaps.
"""

from quickcrawl import QuickCrawlClient

def main():
    with QuickCrawlClient() as client:
        links = client.map(
            "https://example.com",
            max_depth=2,
            use_sitemap=True,
        )

        print(f"Discovered {len(links)} URLs:\n")

        for url in links[:10]:
            print(f"  {url}")

        if len(links) > 10:
            print(f"\n  ... and {len(links) - 10} more")

if __name__ == "__main__":
    main()
