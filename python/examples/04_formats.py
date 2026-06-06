#!/usr/bin/env python3
"""Example: Scrape with multiple output formats.

QuickCrawl supports multiple output formats:
- markdown: Converted markdown text
- html: Clean HTML content
- links: All URLs found on the page
- json: LLM-extracted structured data (requires jsonSchema)
"""

from quickcrawl import QuickCrawlClient

def main():
    with QuickCrawlClient() as client:
        result = client.scrape(
            "https://example.com",
            formats=["markdown", "html", "links"],
        )

        print("=== Markdown ===")
        print(result.get("markdown", "")[:300])

        print("\n=== Links Found ===")
        links = result.get("links", [])
        for link in links[:5]:
            print(f"  {link}")
        if len(links) > 5:
            print(f"  ... and {len(links) - 5} more")

        print("\n=== HTML Snippet ===")
        html = result.get("html", "")
        print(html[:200] if html else "No HTML")

if __name__ == "__main__":
    main()
