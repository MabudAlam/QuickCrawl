"""Download, cache, and locate the quickcrawl-mcp binary."""

from __future__ import annotations

import json
import os
import stat
import tarfile
import zipfile
from io import BytesIO
from pathlib import Path
from urllib.request import Request, urlopen

from platformdirs import user_cache_dir

from quickcrawl._platform import BINARY_NAME, get_asset_name
from quickcrawl.exceptions import QuickCrawlBinaryNotFoundError

GITHUB_REPO = "MabudAlam/quickcrawl"
GITHUB_API = f"https://api.github.com/repos/{GITHUB_REPO}/releases/latest"


def _get_latest_version() -> str:
    """Fetch the latest release tag from GitHub API."""
    req = Request(GITHUB_API, headers={
        "User-Agent": "quickcrawl-python",
        "Accept": "application/vnd.github+json",
    })
    with urlopen(req, timeout=10) as resp:
        data = json.loads(resp.read())
    tag = data.get("tag_name", "")
    return tag.lstrip("v")


def _cache_dir(version: str) -> Path:
    return Path(user_cache_dir("quickcrawl")) / f"v{version}"


def _find_cached_latest() -> tuple[Path, str] | None:
    """Find the newest cached binary version."""
    cache_root = Path(user_cache_dir("quickcrawl"))
    if not cache_root.is_dir():
        return None
    versions = sorted(
        (d.name for d in cache_root.iterdir() if d.is_dir() and d.name.startswith("v")),
        reverse=True,
    )
    for v in versions:
        binary = cache_root / v / BINARY_NAME
        if binary.is_file() and os.access(binary, os.X_OK):
            return binary, v.lstrip("v")
    return None


def _download_binary(version: str) -> Path:
    """Download the binary from GitHub Releases and cache it."""
    asset = get_asset_name()
    if asset is None:
        raise QuickCrawlBinaryNotFoundError(
            "No prebuilt binary for this platform. "
            "Install from source: go install github.com/MabudAlam/quickcrawl/cmd/mcp"
        )

    url = f"https://github.com/{GITHUB_REPO}/releases/download/v{version}/{asset}"
    cache = _cache_dir(version)
    cache.mkdir(parents=True, exist_ok=True)

    req = Request(url, headers={"User-Agent": f"quickcrawl-python/{version}"})
    with urlopen(req, timeout=120) as resp:
        data = resp.read()

    if asset.endswith(".tar.gz"):
        with tarfile.open(fileobj=BytesIO(data), mode="r:gz") as tar:
            for member in tar.getmembers():
                if member.name.endswith("quickcrawl-mcp"):
                    member.name = BINARY_NAME
                    tar.extract(member, path=cache)
                    break
            else:
                raise QuickCrawlBinaryNotFoundError(f"quickcrawl-mcp not found in {asset}")
    elif asset.endswith(".zip"):
        with zipfile.ZipFile(BytesIO(data)) as zf:
            for name in zf.namelist():
                if name.endswith("quickcrawl-mcp.exe") or name.endswith("quickcrawl-mcp"):
                    target = cache / BINARY_NAME
                    target.write_bytes(zf.read(name))
                    break
            else:
                raise QuickCrawlBinaryNotFoundError(f"quickcrawl-mcp not found in {asset}")

    binary = cache / BINARY_NAME
    binary.chmod(binary.stat().st_mode | stat.S_IEXEC | stat.S_IXGRP | stat.S_IXOTH)
    return binary


def _is_native_binary(path: Path) -> bool:
    """Check if a file is a native binary (not a Python/shell script)."""
    try:
        with open(path, "rb") as f:
            header = f.read(4)
        return header[:4] in (
            b"\x7fELF", b"\xcf\xfa\xed\xfe", b"\xce\xfa\xed\xfe",
            b"MZ\x90\x00", b"MZ\x00\x00",
        )
    except OSError:
        return False


def ensure_binary() -> Path:
    """Return path to quickcrawl-mcp binary, downloading latest if necessary."""
    env_path = os.environ.get("QUICKCRAWL_BINARY")
    if env_path:
        p = Path(env_path)
        if p.is_file():
            return p
        raise QuickCrawlBinaryNotFoundError(f"QUICKCRAWL_BINARY={env_path} does not exist")

    for directory in os.environ.get("PATH", "").split(os.pathsep):
        candidate = Path(directory) / BINARY_NAME
        if candidate.is_file() and _is_native_binary(candidate):
            return candidate

    try:
        version = _get_latest_version()
    except Exception:
        cached = _find_cached_latest()
        if cached:
            return cached[0]
        raise QuickCrawlBinaryNotFoundError(
            "Cannot reach GitHub API and no cached binary found. "
            "Install manually: go install github.com/MabudAlam/quickcrawl/cmd/mcp"
        )

    binary = _cache_dir(version) / BINARY_NAME
    if binary.is_file() and os.access(binary, os.X_OK):
        return binary

    return _download_binary(version)
