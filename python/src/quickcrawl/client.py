"""QuickCrawl Python SDK - CLI-based web scraping, crawling, and URL discovery.

This module provides a Python client that shells out to the quickcrawl CLI binary.
It's a simpler alternative to the MCP-based approach, using direct CLI invocation
instead of JSON-RPC over stdio.

Architecture:
    Python SDK → CLI subprocess → JSON stdout → Python dict

This is simpler than MCP because:
    1. No protocol overhead (JSON-RPC framing)
    2. No long-running process management
    3. Each call is independent (no state between calls)
    4. Easier to debug (just run the CLI manually)

For HTTP mode (remote server), set api_url to connect to a running server.
"""

from __future__ import annotations

import json
import subprocess
import time
from typing import Any

from quickcrawl._binary import ensure_binary
from quickcrawl.exceptions import QuickCrawlApiError, QuickCrawlError, QuickCrawlTimeoutError


class QuickCrawlClient:
    """QuickCrawl web scraper client.

    The client shells out to the quickcrawl CLI binary for scraping operations.
    No server process is required - each call spawns a short-lived subprocess.

    Args:
        api_url: QuickCrawl server URL for HTTP mode. If None, uses CLI subprocess mode.
        api_key: API key for authentication (HTTP mode).

    Examples:
        # CLI mode (zero config, no server):
        client = QuickCrawlClient()
        result = client.scrape("https://example.com")

        # HTTP mode (remote server):
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="...")
        result = client.scrape("https://example.com")
    """

    def __init__(self, api_url: str | None = None, api_key: str | None = None):
        self._api_url = api_url
        self._api_key = api_key

    def scrape(
        self,
        url: str,
        formats: list[str] | None = None,
        include_tags: list[str] | None = None,
        exclude_tags: list[str] | None = None,
        render_mode: str = "auto",
        ttl: int | None = None,
        **kwargs: Any,
    ) -> dict:
        """Scrape a single URL and return its content.

        Args:
            url: The URL to scrape.
            formats: Output formats (markdown, html, links, json). Default: ["markdown"].
            include_tags: CSS selectors to include.
            exclude_tags: CSS selectors to exclude.
            render_mode: Renderer mode - "auto" (default), "http", or "browser".
            ttl: Cache TTL in seconds (0=bypass cache, >0=accept cached if younger).
            **kwargs: Additional arguments passed to the CLI.

        Returns:
            Dict with keys like 'markdown', 'html', 'metadata', etc.
        """
        args = ["scrape", url]

        if formats:
            args.extend(["--formats", ",".join(formats)])
        if include_tags:
            args.extend(["--include-tags", ",".join(include_tags)])
        if exclude_tags:
            args.extend(["--exclude-tags", ",".join(exclude_tags)])
        if render_mode != "auto":
            args.extend(["--render", render_mode])
        if ttl is not None:
            args.extend(["--ttl", str(ttl)])

        for key, value in kwargs.items():
            if isinstance(value, bool):
                if value:
                    args.append(f"--{key.replace('_', '-')}")
            elif isinstance(value, (list, tuple)):
                args.extend([f"--{key.replace('_', '-')}", ",".join(str(v) for v in value)])
            else:
                args.extend([f"--{key.replace('_', '-')}", str(value)])

        if self._api_url:
            body = {"url": url, "formats": formats}
            if render_mode == "http":
                body["renderJs"] = False
            elif render_mode == "browser":
                body["renderJs"] = True
            if ttl is not None:
                body["ttl"] = ttl
            return self._http_post("/v1/scrape", body)
        return self._cli_call(args)

    def crawl(
        self,
        url: str,
        max_depth: int = 2,
        max_pages: int = 10,
        poll_interval: float = 2.0,
        timeout: float = 300.0,
        render_mode: str = "auto",
        **kwargs: Any,
    ) -> list[dict]:
        """Crawl a website and return all page results.

        Starts a crawl job, polls for completion, and returns all scraped pages.

        Args:
            url: The starting URL to crawl.
            max_depth: Maximum link depth to follow. Default: 2.
            max_pages: Maximum number of pages to scrape. Default: 10.
            poll_interval: Seconds between status checks. Default: 2.0.
            timeout: Maximum seconds to wait. Default: 300.0.
            render_mode: Renderer mode - "auto" (default), "http", or "browser".
            **kwargs: Additional arguments passed to the CLI.

        Returns:
            List of dicts, each containing scraped page data.
        """
        args = ["crawl", url, "--max-depth", str(max_depth), "--max-pages", str(max_pages)]

        if render_mode != "auto":
            args.extend(["--render", render_mode])

        for key, value in kwargs.items():
            if isinstance(value, bool):
                if value:
                    args.append(f"--{key.replace('_', '-')}")
            elif not isinstance(value, (list, tuple)):
                args.extend([f"--{key.replace('_', '-')}", str(value)])

        if self._api_url:
            return self._http_crawl(args, poll_interval, timeout)

        result = self._cli_call(args)
        return result

    def map(
        self,
        url: str,
        max_depth: int = 2,
        use_sitemap: bool = True,
        **kwargs: Any,
    ) -> list[str]:
        """Discover URLs on a website without scraping content.

        Args:
            url: The starting URL for discovery.
            max_depth: Maximum link depth. Default: 2.
            use_sitemap: Use sitemap.xml as seed. Default: True.
            **kwargs: Additional arguments passed to the CLI.

        Returns:
            List of discovered URLs.
        """
        args = ["map", url, "--max-depth", str(max_depth)]
        if not use_sitemap:
            args.append("--no-sitemap")

        if self._api_url:
            data = self._http_post("/v1/map", {"url": url, "maxDepth": max_depth, "useSitemap": use_sitemap})
            return data.get("links", [])

        result = self._cli_call(args)
        return result.get("links", [])

    def search(
        self,
        query: str,
        formats: list[str] | None = None,
        region: str = "us-en",
        safesearch: str = "moderate",
        scrape: bool = False,
        render_mode: str = "auto",
        page: int = 0,
        timelimit: str | None = None,
        use_bm25: bool = False,
        **kwargs: Any,
    ) -> dict:
        """Search the web and optionally scrape results.

        Args:
            query: Search query string.
            formats: Output formats for scraped pages (default: ["markdown"]).
            region: Region code for search results (e.g., "us-en", "gb-en").
            safesearch: SafeSearch mode: "moderate", "strict", "off".
            scrape: Whether to scrape content from each result URL.
            render_mode: Renderer mode for scraping - "auto" (default), "http", or "browser".
            page: 0-indexed page number to fetch. Default: 0.
            timelimit: Time-range filter ("d"=day, "w"=week, "m"=month, "y"=year). Default: None.
            use_bm25: If True, re-rank results with BM25 and add a `bm25_score` field.
            **kwargs: Additional arguments passed to the CLI.

        Returns:
            A dict with the full search response:
              {
                "query": str,
                "results": [{"position", "score", "bm25_score", "title", "url", "site_name", "snippet", ...}],
                "total_results": int,
                "page": int,
              }
        """
        args = ["search", query]

        if formats:
            args.extend(["--formats", ",".join(formats)])
        if region and region != "us-en":
            args.extend(["--region", region])
        if safesearch and safesearch != "moderate":
            args.extend(["--safesearch", safesearch])
        if scrape:
            args.append("--scrape")
        if render_mode != "auto":
            args.extend(["--render", render_mode])
        if page:
            args.extend(["--page", str(page)])
        if timelimit:
            args.extend(["--timelimit", timelimit])
        if use_bm25:
            args.append("--use-bm25")

        for key, value in kwargs.items():
            if isinstance(value, bool):
                if value:
                    args.append(f"--{key.replace('_', '-')}")
            elif isinstance(value, (list, tuple)):
                args.extend([f"--{key.replace('_', '-')}", ",".join(str(v) for v in value)])
            else:
                args.extend([f"--{key.replace('_', '-')}", str(value)])

        if self._api_url:
            body = {
                "query": query,
                "formats": formats or ["markdown"],
                "region": region,
                "safesearch": safesearch,
                "page": page,
            }
            if timelimit:
                body["timelimit"] = timelimit
            if use_bm25:
                body["use_bm25"] = True
            data = self._http_post("/v1/search", body)
            return {
                "query": data.get("query", query),
                "results": data.get("results", []),
                "total_results": data.get("total_results", len(data.get("results", []))),
                "page": data.get("page", page),
            }

        return self._cli_call(args)

    def close(self) -> None:
        """No-op for CLI mode. Kept for API compatibility with MCP mode."""
        pass

    def __enter__(self) -> QuickCrawlClient:
        return self

    def __exit__(self, *_: Any) -> None:
        self.close()

    def __del__(self) -> None:
        self.close()

    # --- CLI subprocess mode ---

    def _cli_call(self, args: list[str]) -> dict:
        """Execute the quickcrawl CLI with the given arguments.

        Args:
            args: CLI subcommand and flags, e.g. ["scrape", "https://example.com"]

        Returns:
            Parsed JSON response from the CLI.

        Raises:
            QuickCrawlError: If the CLI can't be found or executed.
            QuickCrawlApiError: If the CLI returns an error.
        """
        binary = ensure_binary()

        cmd = [str(binary)] + args

        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=300,
            )
        except subprocess.TimeoutExpired:
            raise QuickCrawlTimeoutError(f"Command timed out after 300s: {' '.join(cmd)}")
        except FileNotFoundError:
            raise QuickCrawlError(
                f"quickcrawl binary not found. "
                f"Install manually: go install github.com/MabudAlam/quickcrawl/cmd/cli"
            )
        except Exception as e:
            raise QuickCrawlError(f"Failed to execute quickcrawl: {e}")

        if result.returncode != 0:
            error_msg = result.stderr.strip() if result.stderr else f"Exit code {result.returncode}"
            raise QuickCrawlApiError(f"quickcrawl error: {error_msg}")

        if not result.stdout.strip():
            raise QuickCrawlError("quickcrawl returned empty output")

        try:
            return json.loads(result.stdout.strip())
        except json.JSONDecodeError as e:
            raise QuickCrawlError(f"Failed to parse quickcrawl output: {e}\nOutput: {result.stdout[:500]}")

    # --- HTTP mode ---

    def _http_request(self, method: str, path: str, body: dict | None = None, *, raw: bool = False) -> dict:
        """Make an HTTP request to the API server."""
        import urllib.request

        url = f"{self._api_url.rstrip('/')}{path}"
        headers = {"Content-Type": "application/json"}
        if self._api_key:
            headers["Authorization"] = f"Bearer {self._api_key}"

        data = json.dumps(body).encode() if body else None
        req = urllib.request.Request(url, data=data, headers=headers, method=method)

        with urllib.request.urlopen(req, timeout=120) as resp:
            result = json.loads(resp.read())

        if not result.get("success"):
            raise QuickCrawlApiError(result.get("error", "API error"))
        if raw:
            return result
        return result.get("data", result)

    def _http_post(self, path: str, body: dict) -> dict:
        return self._http_request("POST", path, body)

    def _http_get(self, path: str) -> dict:
        return self._http_request("GET", path)

    def _http_crawl(self, args: dict, poll_interval: float, timeout: float) -> list[dict]:
        result = self._http_post("/v1/crawl", args)
        job_id = result.get("id")
        if not job_id:
            raise QuickCrawlError(f"Crawl did not return job ID: {result}")

        start = time.monotonic()
        while True:
            if time.monotonic() - start > timeout:
                raise QuickCrawlTimeoutError(f"Crawl {job_id} timed out after {timeout}s")

            status_result = self._http_request("GET", f"/v1/crawl/{job_id}", raw=True)
            status = status_result.get("status")

            if status == "completed":
                return status_result.get("data", [])
            if status == "failed":
                raise QuickCrawlError(f"Crawl failed: {status_result.get('error', 'unknown')}")

            time.sleep(poll_interval)