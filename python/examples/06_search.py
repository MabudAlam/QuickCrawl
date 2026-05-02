#!/usr/bin/env python3
"""Example: Search the web with QuickCrawl.

Search DuckDuckGo and optionally scrape content from each result.
This is useful for research, competitor analysis, and content aggregation.
"""

from quickcrawl import QuickCrawlClient

def main():
    with QuickCrawlClient() as client:
        # Simple search (returns title, URL, description)
        results = client.search("golang web scraping")
        print(f"Found {len(results)} results:\n")

        for i, r in enumerate(results[:5], 1):
            print(f"{i}. {r.get('title', 'No title')}")
            print(f"   URL: {r.get('url', 'No URL')}")
            print(f"   Description: {r.get('description', 'No description')[:100]}...")
            print()

        # Search with scraping (fetches content from each result)
        print("=" * 50)
        print("With scraping enabled:\n")

        results = client.search(
            "python web scraping",
            scrape=True,
            formats=["markdown"],
        )

        for i, r in enumerate(results[:3], 1):
            print(f"{i}. {r.get('title', 'No title')}")
            if r.get("markdown"):
                print(f"   Content preview: {r['markdown'][:150]}...")
            print()

if __name__ == "__main__":
    main()
