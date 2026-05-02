#!/usr/bin/env python3
"""Example: Connect to a cloud-hosted QuickCrawl server.

This mode is useful when:
- You want to deploy QuickCrawl on your own server
- You need scalable cloud infrastructure
- You want to offer API access to users

First, deploy a QuickCrawl server:
    go install github.com/MabudAlam/quickcrawl/cmd/server
    quickcrawl-server  # runs on port 3000

Then connect using HTTP mode:
"""

from quickcrawl import QuickCrawlClient

def main():
    # Connect to your deployed server
    # Get your API key from the server configuration
    client = QuickCrawlClient(
        api_url="http://localhost:3000",  # or your cloud endpoint
        api_key="optional-api-key",       # if server requires auth
    )

    # All operations go via HTTP instead of CLI subprocess
    with client:
        result = client.scrape("https://example.com")
        print("Success! Title:", result["metadata"].get("title"))

if __name__ == "__main__":
    main()
