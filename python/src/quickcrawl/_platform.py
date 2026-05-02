"""Platform detection and GitHub Release asset mapping."""

import platform
import sys

PLATFORM_MAP = {
    ("Darwin", "arm64"): "quickcrawl-mcp-darwin-arm64.tar.gz",
    ("Darwin", "x86_64"): "quickcrawl-mcp-darwin-x64.tar.gz",
    ("Linux", "x86_64"): "quickcrawl-mcp-linux-x64.tar.gz",
    ("Linux", "aarch64"): "quickcrawl-mcp-linux-arm64.tar.gz",
    ("Windows", "AMD64"): "quickcrawl-mcp-win32-x64.zip",
    ("Windows", "ARM64"): "quickcrawl-mcp-win32-arm64.zip",
}

BINARY_NAME = "quickcrawl-mcp.exe" if sys.platform == "win32" else "quickcrawl-mcp"


def get_asset_name() -> str | None:
    """Return the GitHub Release asset filename for the current platform."""
    key = (platform.system(), platform.machine())
    return PLATFORM_MAP.get(key)
