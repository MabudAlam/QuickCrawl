// Package cmd contains the Cobra command definitions for the QuickCrawl CLI.
// Each subcommand (scrape, crawl, map, search) is defined in its own file
// to keep the codebase organized and manageable.
package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// verbose controls whether to print verbose output.
// It's set by the --verbose flag on the root command.
var verbose bool

// outputWriter is where command output is written (default: stdout).
// This is configurable to allow redirecting output, e.g., for testing.
var outputWriter io.Writer = os.Stdout

// errorWriter is where error messages are written (default: stderr).
// Using a separate writer for errors allows proper separation of output streams.
var errorWriter io.Writer = os.Stderr

// rootCmd is the root command for the QuickCrawl CLI.
// All other subcommands are children of this command.
var rootCmd = &cobra.Command{
	// Use is the command name that invokes this command.
	// With "quickcrawl" as the binary name, users would run:
	//   quickcrawl scrape <url>
	//   quickcrawl crawl <url>
	Use: "quickcrawl",

	// Short is a short description shown in help output.
	// This appears when running "quickcrawl --help".
	Short: "QuickCrawl - Web scraping and crawling CLI",

	// Long is the full description shown in help more.
	// This appears when running "quickcrawl --help" and shows
	// the complete description of what QuickCrawl does.
	Long: `QuickCrawl is a fast, configurable web scraping and crawling tool.

It can scrape single pages, crawl entire websites, discover URLs via sitemaps,
and search SearXNG. Supports JavaScript rendering, proxy configuration,
stealth mode, and structured data extraction via LLM.

Examples:
  # Scrape a single URL and output markdown
  quickcrawl scrape https://example.com

  # Crawl a website up to 10 pages
  quickcrawl crawl https://example.com --max-pages 10

  # Discover URLs on a site using sitemap
  quickcrawl map https://example.com --max-depth 3

  # Search SearXNG and scrape results
  quickcrawl search "golang web scraping" --formats markdown`,

	// Run is executed when no subcommand is specified.
	// For the root command, we just show help since a tool like
	// QuickCrawl needs a subcommand (scrape, crawl, map, search).
	Run: func(cmd *cobra.Command, args []string) {
		// Print help to stdout when no arguments provided.
		// Users typically expect help on stdout, not stderr.
		cmd.Help()
	},

	// PersistentPreRun is executed before any subcommand's Run function.
	// It sets up the environment (like verbose logging) before
	// any actual command logic runs.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			// When verbose mode is enabled, we could configure
			// logging to show more details about what's happening.
			// For now, this is a placeholder for future enhancement.
		}
	},
}

// Execute runs the root command and all its subcommands.
// This is called from main() and handles the full command lifecycle.
// Returns an error if command execution fails, nil otherwise.
func Execute() error {
	return rootCmd.Execute()
}

// SetOutput sets the output writer for command results.
// This is primarily used for testing to capture output instead of
// writing to stdout. It returns the previous writer for chaining.
func SetOutput(w io.Writer) io.Writer {
	old := outputWriter
	outputWriter = w
	return old
}

// SetErrorOutput sets the error writer for error messages.
// Returns the previous writer for chaining.
func SetErrorOutput(w io.Writer) io.Writer {
	old := errorWriter
	errorWriter = w
	return old
}

// output prints the given format string and arguments to the output writer.
// It's used by subcommands to emit their results in a consistent way.
// The format string is followed by arguments (like fmt.Printf).
func output(format string, args ...interface{}) {
	fmt.Fprintf(outputWriter, format, args...)
}

// errPrint prints the given format string and arguments to the error writer.
// It's used for error messages and warnings, separating them from
// actual command output (which might be piped to other commands).
func errPrint(format string, args ...interface{}) {
	fmt.Fprintf(errorWriter, format, args...)
}

func init() {
	//Verbose flag at the root level means all subcommands inherit it
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}