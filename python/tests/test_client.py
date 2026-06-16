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
            result = client.scrape(
                "https://example.com",
                formats=["markdown", "html"],
                wait_for=2000,
                render_mode="browser",
            )

        mock_post.assert_called_once()
        call_args = mock_post.call_args
        assert call_args[0][0] == "/v1/scrape"
        body = call_args[0][1]
        assert body["url"] == "https://example.com"
        assert body["formats"] == ["markdown", "html"]
        assert body["waitFor"] == 2000
        assert body["renderMode"] == "browser"
        assert result == mock_response

    def test_scrape_rejects_invalid_url(self) -> None:
        client = QuickCrawlClient()
        with pytest.raises(ValueError, match="url must start with http"):
            client.scrape("not-a-url")

    def test_scrape_subprocess_calls_cli(self) -> None:
        client = QuickCrawlClient()
        mock_response = {"markdown": "# Hello", "metadata": {"title": "Hello"}}

        with patch.object(client, "_cli_call", return_value=mock_response) as mock_cli:
            result = client.scrape("https://example.com")

        mock_cli.assert_called_once()
        call_args = mock_cli.call_args[0][0]
        assert call_args[0] == "scrape"
        assert call_args[1] == "https://example.com"
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
            "page": 1,
        }

        with patch.object(client, "_http_post", return_value=mock_response) as mock_post:
            result = client.search("golang")

        assert result == mock_response
        call_args = mock_post.call_args
        assert call_args[0][0] == "/v1/search"
        body = call_args[0][1]
        assert body["query"] == "golang"
        assert body["page"] == 1
        assert "use_bm25" not in body

    def test_search_http_with_bm25_and_page(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")
        mock_response = {"query": "x", "results": [], "total_results": 0, "page": 2}

        with patch.object(client, "_http_post", return_value=mock_response) as mock_post:
            result = client.search("x", use_bm25=True, page=2, time_range="week")

        body = mock_post.call_args[0][1]
        assert body["use_bm25"] is True
        assert body["page"] == 2
        assert body["timeRange"] == "week"
        assert result["page"] == 2

    def test_search_http_with_time_range(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")
        mock_response = {"query": "golang", "results": [], "total_results": 0, "page": 1}

        with patch.object(client, "_http_post", return_value=mock_response) as mock_post:
            result = client.search("golang", time_range="day")

        body = mock_post.call_args[0][1]
        assert body["timeRange"] == "day"

    def test_search_http_rejects_invalid_time_range(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")
        with pytest.raises(ValueError, match="time_range must be one of"):
            client.search("golang", time_range="hour")

    def test_search_http_rejects_invalid_page(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")
        with pytest.raises(ValueError, match="page must be between 1 and 1000"):
            client.search("golang", page=0)

    def test_search_subprocess_passes_all_flags(self) -> None:
        client = QuickCrawlClient()
        mock_response = {"query": "go", "results": [], "total_results": 0, "page": 1}

        with patch.object(client, "_cli_call", return_value=mock_response) as mock_cli:
            result = client.search(
                "go",
                page=2,
                time_range="month",
                use_bm25=True,
                scrape=True,
                region="gb-en",
            )

        args = mock_cli.call_args[0][0]
        assert "search" in args
        assert "go" in args
        assert "--page" in args and "2" in args
        assert "--time-range" in args and "month" in args
        assert "--use-bm25" in args
        assert "--scrape" in args
        assert "--region" in args and "gb-en" in args
        assert result == mock_response

    def test_search_rejects_empty_query(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000", api_key="test")
        with pytest.raises(ValueError, match="query is required"):
            client.search("")


@pytest.mark.unit
class TestLifecycle:
    def test_close_noop_for_http_mode(self) -> None:
        client = QuickCrawlClient(api_url="http://localhost:3000")
        client.close()

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

    def test_scrape_raises_on_invalid_url(self) -> None:
        client = QuickCrawlClient()
        with pytest.raises(ValueError, match="url must start with http"):
            client.scrape("ftp://example.com")

    def test_search_raises_on_empty_query(self) -> None:
        client = QuickCrawlClient()
        with pytest.raises(ValueError, match="query is required"):
            client.search("")

    def test_crawl_raises_on_invalid_max_depth(self) -> None:
        client = QuickCrawlClient()
        with pytest.raises(ValueError, match="max_depth must be between"):
            client.crawl("https://example.com", max_depth=101)

    def test_crawl_raises_on_negative_max_depth(self) -> None:
        client = QuickCrawlClient()
        with pytest.raises(ValueError, match="max_depth must be between"):
            client.crawl("https://example.com", max_depth=-1)
