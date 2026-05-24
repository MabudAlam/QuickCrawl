package crawler

import (
	"context"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/renderer"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

// MapOptions contains parameters for URL mapping operations.
type MapOptions struct {
	BaseURL           string
	MaxDepth          uint32
	UseSitemap        bool
	RespectRobots     bool
	Renderer          *renderer.FallbackRenderer
	MaxConcurrency    int
	RequestsPerSecond float64
	UserAgent         string
	Timeout           *int // Timeout in milliseconds
}

// Map discovers URLs starting from a base URL using BFS traversal.
// It respects robots.txt and rate limits, and optionally uses sitemaps.
func Map(opts MapOptions) (*types.MapData, *types.QuickCrawlError) {
	maxDepth := uint32(2)
	if opts.MaxDepth > 0 {
		maxDepth = opts.MaxDepth
		if maxDepth > 10 {
			maxDepth = 10
		}
	}

	// Create context with timeout if provided
	ctx := context.Background()
	if opts.Timeout != nil && *opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*opts.Timeout)*time.Millisecond)
		defer cancel()
	}

	urls, err := DiscoverUrls(
		opts.BaseURL,
		maxDepth,
		opts.UseSitemap,
		opts.Renderer,
		opts.RespectRobots,
		opts.MaxConcurrency,
		opts.RequestsPerSecond,
		opts.UserAgent,
		ctx,
	)
	if err != nil {
		return nil, err
	}

	return &types.MapData{Links: urls}, nil
}
