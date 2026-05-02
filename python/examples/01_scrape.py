#!/usr/bin/env python3
"""Example: Scrape a single URL and extract markdown content."""

from quickcrawl import QuickCrawlClient

def main():
    with QuickCrawlClient() as client:
        result = client.scrape("https://example.com")

        print("Title:", result["metadata"].get("title"))
        print("\nMarkdown content:")
        print(result.get("markdown", "")[:500])

if __name__ == "__main__":
    main()
