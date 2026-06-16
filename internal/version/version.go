// Package version exposes build-time version metadata for QuickCrawl.
//
// The Version, Commit, and BuildDate variables are populated by goreleaser
// via -ldflags at release time. When the package is built locally with
// `go build`, the values fall back to the placeholders below so the
// `--version` output is never empty.
package version

var (
	// Version is the semantic version of the build (e.g., "0.1.0").
	Version = "0.0.0-dev"

	// Commit is the short git SHA the build was cut from.
	Commit = "none"

	// BuildDate is the ISO 8601 timestamp of the build.
	BuildDate = "unknown"
)

// String returns a one-line human-readable version summary.
func String() string {
	return "quickcrawl " + Version + " (" + Commit + ") built " + BuildDate
}
