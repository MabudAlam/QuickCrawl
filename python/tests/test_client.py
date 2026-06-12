"""Unit tests for QuickCrawlClient — all network/subprocess calls are mocked."""

from __future__ import annotations

import json
from unittest.mock import MagicMock, patch

import pytest

from quickcrawl.client import QuickCrawlClient
from quickcrawl.exceptions import QuickCrawlApiError, QuickCrawlError, QuickCrawlTimeoutError


@pytest.mark.unit
class TestInit:
    def test_init_subprocess_mode(self) -> None:
        client = QuickCrawlClient()
        assert client._api_url is None
        assert client._api_key is None

    def test_init_http_mode(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")
        assert client._api_url == "http://localhost:3000"
        assert client._api_key == "test"


@pytest.mark.unit
class TestScrape:
    def test_scrape_http_builds_correct_request(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")
        mock_response = {"markdown": "# Hello", "metadata": {"title": "Hello"}}

        with patch.object(client, "_http_post", return_value=mock_response) as mock_post:
            result = client.scrape("https://example.com", formats=["markdown", "html"])

        mock_post.assert_called_once()
        call_args = mock_post.call_args
        assert call_args[0][0] == "/v1/scrape"
        body = call_args[0][1]
        assert body["url"] == "https://example.com"
        assert body["formats"] == ["markdown", "html"]
        assert result == mock_response

    def test_scrape_subprocess_calls_tool(self) -> None:
        client = QuickCrawlClient()
        mock_response = {"markdown": "# Hello"}

        with patch.object(client, "_tool_call", return_value=mock_response) as mock_tool:
            result = client.scrape("https://example.com")

        mock_tool.assert_called_once()
        call_args = mock_tool.call_args
        assert call_args[0][0] == "quickcrawl_scrape"
        assert call_args[0][1]["url"] == "https://example.com"
        assert result == mock_response


@pytest.mark.unit
class TestCrawl:
    def test_crawl_http_polls_until_complete(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")

        crawl_start_response = {"id": "job-123"}
        poll_in_progress = {"success": True, "status": "scraping", "data": []}
        poll_completed = {
            "success": True,
            "status": "completed",
            "data": [{"url": "https://example.com", "markdown": "# Hello"}],
        }

        with (
            patch.object(client, "_http_post", return_value=crawl_start_response) as mock_post,
            patch.object(
                client, "_http_request", side_effect=[poll_in_progress, poll_completed]
            ) as mock_req,
            patch("quickcrawl.client.time.sleep"),
        ):
            result = client.crawl("https://example.com", poll_interval=0.01, timeout=10)

        assert mock_req.call_count == 2
        assert len(result) == 1
        assert result[0]["url"] == "https://example.com"

    def test_crawl_raises_timeout(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")

        crawl_start_response = {"id": "job-timeout"}
        poll_in_progress = {"success": True, "status": "scraping", "data": []}

        with (
            patch.object(client, "_http_post", return_value=crawl_start_response),
            patch.object(client, "_http_request", return_value=poll_in_progress),
            patch("quickcrawl.client.time.sleep"),
            patch("quickcrawl.client.time.monotonic", side_effect=[0.0, 0.0, 100.0]),
        ):
            with pytest.raises(QuickCrawlTimeoutError, match="timed out"):
                client.crawl("https://example.com", poll_interval=0.01, timeout=5)

    def test_crawl_raises_on_failure(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")

        crawl_start_response = {"id": "job-fail"}
        poll_failed = {"success": True, "status": "failed", "error": "Something went wrong"}

        with (
            patch.object(client, "_http_post", return_value=crawl_start_response),
            patch.object(client, "_http_request", return_value=poll_failed),
            patch("quickcrawl.client.time.sleep"),
        ):
            with pytest.raises(QuickCrawlError, match="Crawl failed"):
                client.crawl("https://example.com", poll_interval=0.01, timeout=10)


@pytest.mark.unit
class TestMap:
    def test_map_http_returns_links(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")
        mock_response = {"links": ["https://example.com/a", "https://example.com/b"]}

        with patch.object(client, "_http_post", return_value=mock_response):
            result = client.map("https://example.com")

        assert result == ["https://example.com/a", "https://example.com/b"]


@pytest.mark.unit
class TestSearch:
    def test_search_http_returns_full_response(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")
        mock_response = {
            "query": "golang",
            "results": [
                {"position": 1, "score": 9.0, "title": "Go", "url": "https://go.dev", "site_name": "go.dev", "snippet": "..."},
            ],
            "total_results": 1,
            "page": 0,
        }

        with patch.object(client, "_http_post", return_value=mock_response) as mock_post:
            result = client.search("golang")

        assert result == mock_response
        call_args = mock_post.call_args
        assert call_args[0][0] == "/v1/search"
        body = call_args[0][1]
        assert body["query"] == "golang"
        assert body["page"] == 0
        assert "use_bm25" not in body

    def test_search_http_with_bm25_and_page(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")
        mock_response = {"query": "x", "results": [], "total_results": 0, "page": 2}

        with patch.object(client, "_http_post", return_value=mock_response) as mock_post:
            result = client.search("x", use_bm25=True, page=2, timelimit="w")

        body = mock_post.call_args[0][1]
        assert body["use_bm25"] is True
        assert body["page"] == 2
        assert body["timelimit"] == "w"
        assert result["page"] == 2

    def test_search_subprocess_passes_all_flags(self) -> None:
        client = QuickCrawlClient()
        mock_response = {"query": "go", "results": [], "total_results": 0, "page": 1}

        with patch.object(client, "_cli_call", return_value=mock_response) as mock_cli:
            result = client.search(
                "go",
                page=1,
                timelimit="d",
                use_bm25=True,
                scrape=True,
                region="gb-en",
                safesearch="strict",
            )

        args = mock_cli.call_args[0][0]
        assert "search" in args
        assert "go" in args
        assert "--page" in args and "1" in args
        assert "--timelimit" in args and "d" in args
        assert "--use-bm25" in args
        assert "--scrape" in args
        assert "--region" in args and "gb-en" in args
        assert "--safesearch" in args and "strict" in args
        assert result == mock_response


@pytest.mark.unit
class TestLifecycle:
    def test_close_terminates_process(self) -> None:
        client = QuickCrawlClient()
        mock_proc = MagicMock()
        mock_proc.poll.return_value = None
        mock_proc.wait.return_value = 0
        client._process = mock_proc

        client.close()

        mock_proc.stdin.close.assert_called_once()
        assert client._process is None

    def test_context_manager(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000")
        with patch.object(client, "close") as mock_close:
            with client as c:
                assert c is client
            mock_close.assert_called_once()


@pytest.mark.unit
class TestErrors:
    def test_api_error_raised_on_failure(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")

        with patch.object(
            client,
            "_http_request",
            side_effect=QuickCrawlApiError("Bad request", status_code=400),
        ):
            with pytest.raises(QuickCrawlApiError) as exc_info:
                client.scrape("https://example.com")
            assert exc_info.value.status_code == 400

    def test_jsonrpc_error_handling(self) -> None:
        client = QuickCrawlClient()
        mock_proc = MagicMock()
        error_response = json.dumps({
            "jsonrpc": "2.0",
            "id": 1,
            "error": {"code": -32600, "message": "Invalid request"},
        })
        mock_proc.stdout.readline.return_value = error_response + "\n"
        mock_proc.poll.return_value = None
        client._process = mock_proc

        with pytest.raises(QuickCrawlApiError, match="Invalid request"):
            client._jsonrpc("tools/call", {"name": "quickcrawl_scrape", "arguments": {}})
