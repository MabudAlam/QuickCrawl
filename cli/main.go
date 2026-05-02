// Package main is the entry point for the QuickCrawl CLI binary.
// The CLI provides subcommands for web scraping, crawling, URL discovery,
// and search via Cobra.
package main

import (
	"os"

	"github.com/MabudAlam/quickcrawl/cli/cmd"
)

// main delegates to cmd.Execute() which sets up the Cobra command tree
// and handles routing to subcommands (scrape, crawl, map, search) based on os.Args.
func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}