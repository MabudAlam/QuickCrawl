#!/usr/bin/env python3
"""Example: Search the web with QuickCrawl.

Search SearXNG and optionally scrape content from each result.
This is useful for research, competitor analysis, and content aggregation.
"""

from quickcrawl import QuickCrawlClient

def main():
    with QuickCrawlClient() as client:
        # Simple search (returns title, URL, snippet)
        response = client.search("golang web scraping")
        results = response["results"]
        print(f"Query: {response['query']}")
        print(f"Found {response['total_results']} results (page {response['page']}):\n")

        for r in results[:5]:
            print(f"[{r.get('position')}] {r.get('title', 'No title')}")
            print(f"   URL: {r.get('url', 'No URL')}")
            print(f"   Site: {r.get('site_name', '')}")
            print(f"   Score: {r.get('score', 0)}")
            snippet = r.get("snippet", "")
            if snippet:
                print(f"   Snippet: {snippet[:120]}...")
            print()

        # Search with BM25 re-ranking
        print("=" * 50)
        print("With BM25 re-ranking:\n")

        response = client.search(
            "python web scraping",
            use_bm25=True,
        )
        for r in response["results"][:5]:
            print(f"[{r.get('position')}] {r.get('title', 'No title')}")
            print(f"   native score: {r.get('score', 0)} | bm25_score: {r.get('bm25_score', 0):.3f}")
            print()

        # Search with scraping (fetches content from each result)
        print("=" * 50)
        print("With scraping enabled:\n")

        response = client.search(
            "python web scraping",
            scrape=True,
            formats=["markdown"],
        )

        for r in response["results"][:3]:
            print(f"[{r.get('position')}] {r.get('title', 'No title')}")
            if r.get("markdown"):
                print(f"   Content preview: {r['markdown'][:150]}...")
            print()

        # Fetch a specific page of results
        print("=" * 50)
        print("Page 1 of results:\n")
        page1 = client.search("golang", page=1)
        print(f"Got {page1['total_results']} results on page {page1['page']}")

if __name__ == "__main__":
    main()
