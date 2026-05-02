"""QuickCrawl SDK exceptions."""


class QuickCrawlError(Exception):
    """Base exception for QuickCrawl SDK."""


class QuickCrawlBinaryNotFoundError(QuickCrawlError):
    """Binary could not be found or downloaded."""


class QuickCrawlTimeoutError(QuickCrawlError):
    """Operation timed out."""


class QuickCrawlApiError(QuickCrawlError):
    """API returned an error."""

    def __init__(self, message: str, status_code: int | None = None):
        super().__init__(message)
        self.status_code = status_code
