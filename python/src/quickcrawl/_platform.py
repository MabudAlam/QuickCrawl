"""Platform detection and GitHub Release asset mapping."""

import platform
import sys

PLATFORM_MAP = {
    ("Darwin", "arm64"): "quickcrawl_0.1.0_darwin_arm64.tar.gz",
    ("Darwin", "x86_64"): "quickcrawl_0.1.0_darwin_amd64.tar.gz",
    ("Linux", "x86_64"): "quickcrawl_0.1.0_linux_amd64.tar.gz",
    ("Linux", "aarch64"): "quickcrawl_0.1.0_linux_arm64.tar.gz",
    ("Windows", "AMD64"): "quickcrawl_0.1.0_win32_x64.zip",
    ("Windows", "ARM64"): "quickcrawl_0.1.0_win32_arm64.zip",
}

BINARY_NAME = "quickcrawl.exe" if sys.platform == "win32" else "quickcrawl"


def get_asset_name() -> str | None:
    """Return the GitHub Release asset filename for the current platform."""
    key = (platform.system(), platform.machine())
    return PLATFORM_MAP.get(key)
