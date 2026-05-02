#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NPM_DIR="$ROOT_DIR/npm"
GITHUB_REPO_OWNER="MabudAlam"

usage() {
  cat <<'EOF'
Usage:
  scripts/release.sh patch
  scripts/release.sh minor
  scripts/release.sh major
  scripts/release.sh 1.2.3

Behavior:
  - bumps npm/package.json version
  - creates release commit and annotated git tag
  - runs goreleaser release --clean (creates draft GitHub release with binaries)
  - pushes Docker images to GHCR

Examples:
  scripts/release.sh patch
  scripts/release.sh minor
EOF
}

if [[ "${1:-}" == "" ]] || [[ "${1:-}" == "-h" ]] || [[ "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

for cmd in git node npm goreleaser; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "$cmd is required but was not found in PATH." >&2
    exit 1
  fi
done

if [[ ! -f "$ROOT_DIR/npm/package.json" ]]; then
  echo "Could not find $ROOT_DIR/npm/package.json" >&2
  exit 1
fi

cd "$ROOT_DIR"

TARGET="${1}"
PACKAGE_JSON="$NPM_DIR/package.json"
CURRENT_VERSION="$(node -e "console.log(require('$PACKAGE_JSON').version)")"

if [[ "$TARGET" =~ ^(patch|minor|major)$ ]]; then
  (cd "$NPM_DIR" && npm version "$TARGET" --no-git-tag-version)
else
  node -e "const fs=require('fs'); const pkg=JSON.parse(fs.readFileSync('$NPM_DIR/package.json','utf8')); pkg.version='$TARGET'; fs.writeFileSync('$NPM_DIR/package.json', JSON.stringify(pkg, null, 2)+'\n');"
fi

NEW_VERSION="$(node -e "console.log(require('$PACKAGE_JSON').version)")"
NEW_TAG="v$NEW_VERSION"

if git rev-parse "$NEW_TAG" >/dev/null 2>&1; then
  echo "Git tag $NEW_TAG already exists. Deleting..."
  git tag -d "$NEW_TAG"
fi

if [[ "$CURRENT_VERSION" != "$NEW_VERSION" ]]; then
  git add npm/package.json npm/package-lock.json
  git commit -m "release: $NEW_TAG"
  echo "Version: $CURRENT_VERSION -> $NEW_VERSION"
else
  echo "Version unchanged: $NEW_VERSION"
fi

git tag -a "$NEW_TAG" -m "Release $NEW_TAG"

echo "Pushing tag to origin..."
git push origin "$NEW_TAG" --force || true

echo "Tag: $NEW_TAG"
echo ""
echo "Running goreleaser..."

goreleaser release --clean

echo ""
echo "## Release complete"
echo ""
echo "Draft release created on GitHub with binaries"
echo ""
echo "Docker Images:"
echo "  ghcr.io/$GITHUB_REPO_OWNER/quickcrawl/quickcrawl-server:$NEW_TAG"
echo "  ghcr.io/$GITHUB_REPO_OWNER/quickcrawl/quickcrawl-playground:$NEW_TAG"