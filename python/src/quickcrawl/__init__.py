"""QuickCrawl Python SDK — scrape, crawl, and map any website."""

from quickcrawl.client import QuickCrawlClient
from quickcrawl.exceptions import QuickCrawlError, QuickCrawlBinaryNotFoundError, QuickCrawlTimeoutError

__all__ = ["QuickCrawlClient", "QuickCrawlError", "QuickCrawlBinaryNotFoundError", "QuickCrawlTimeoutError"]
__version__ = "0.1.0"
