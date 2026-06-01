// Package api provides the HTTP API server for the scraping service.
package api

import (
	context "context"
	cryptoRand "crypto/rand"
	"sync"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/config"
	"github.com/MabudAlam/quickcrawl/internal/core"
	"github.com/MabudAlam/quickcrawl/internal/renderer"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

// AppState holds the application state including configuration and running jobs.
// It is the central state manager for the API server, coordinating between
// the HTTP handlers, renderer, and crawl job tracking.
type AppState struct {
	Config      *types.AppConfig           // Application configuration
	Renderer    *renderer.FallbackRenderer // Page renderer (HTTP/browser)
	CoreScraper *core.Scraper              // Core scraper using chromedp
	CrawlJobs   map[string]CrawlJob        // Active crawl jobs (protected by mu)
	mu          sync.RWMutex               // Mutex for thread-safe CrawlJobs access
	ctx         context.Context            // Context for controlling background goroutines
	cancel      context.CancelFunc         // Cancel function to stop background goroutines
}

// CrawlJob represents a single crawl job with its metadata and current state.
type CrawlJob struct {
	ID        string           // Unique job identifier (format: YYYYMMDDHHMMSS-random8)
	CreatedAt time.Time        // Timestamp when the job was created
	State     types.CrawlState // Current state including status, progress, and data
}

// NewAppState creates a new application state instance with the given configuration.
// It initializes the crawl job map and starts a background goroutine to clean up
// expired jobs. Returns the initialized state or an error if configuration fails.
func NewAppState(cfg *types.AppConfig) (*AppState, *types.QuickCrawlError) {
	if cfg == nil {
		cfg = &types.AppConfig{}
	}
	cfg.Defaults()

	ctx, cancel := context.WithCancel(context.Background())

	state := &AppState{
		Config:    cfg,
		Renderer:  nil,
		CrawlJobs: make(map[string]CrawlJob),
		ctx:       ctx,
		cancel:    cancel,
	}

	go state.removeExpiredJobs()
	return state, nil
}

// removeExpiredJobs periodically scans for and removes completed/failed jobs
// that have exceeded the configured TTL (time-to-live). Runs every minute.
// It respects context cancellation and will exit when the context is done.
func (s *AppState) removeExpiredJobs() {
	ttl := time.Duration(s.Config.Crawler.JobTTLSecs) * time.Second
	if ttl <= 0 {
		ttl = time.Hour // Default TTL of 1 hour if not configured
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			for id, job := range s.CrawlJobs {
				if job.State.Status == types.CrawlStatusCompleted || job.State.Status == types.CrawlStatusFailed {
					if time.Since(job.CreatedAt) >= ttl {
						delete(s.CrawlJobs, id)
					}
				}
			}
			s.mu.Unlock()
		}
	}
}

// StartCrawlJob registers a new crawl job with initial state and returns its unique ID.
// The returned ID can be used to track progress and retrieve results via GetCrawlJob.
func (s *AppState) StartCrawlJob(req *types.CrawlRequest) string {
	id := createJobID()
	initial := types.CrawlState{
		ID:        id,
		Success:   true,
		Status:    types.CrawlStatusInProgress,
		Total:     0,
		Completed: 0,
		Data:      []types.ScrapeData{},
		Error:     nil,
	}

	s.mu.Lock()
	s.CrawlJobs[id] = CrawlJob{
		ID:        id,
		CreatedAt: time.Now(),
		State:     initial,
	}
	s.mu.Unlock()

	return id
}

// GetCrawlJob returns the current state of a crawl job by its ID.
// Returns nil if the job ID is not found.
func (s *AppState) GetCrawlJob(id string) *types.CrawlState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if job, ok := s.CrawlJobs[id]; ok {
		return &job.State
	}
	return nil
}

// UpdateCrawlJob atomically updates the state of a crawl job.
// Used by the crawler to report progress during execution.
func (s *AppState) UpdateCrawlJob(id string, state types.CrawlState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.CrawlJobs[id]
	if !ok {
		return
	}
	job.State = state
	s.CrawlJobs[id] = job
}

// DeleteCrawlJob removes a crawl job from tracking, effectively canceling it.
// If the job is currently running, calling this will prevent further updates.
func (s *AppState) DeleteCrawlJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.CrawlJobs, id)
}

// ActiveCrawlJobCount returns the total number of crawl jobs currently being tracked.
func (s *AppState) ActiveCrawlJobCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.CrawlJobs)
}

// RendererBrowsersInfo returns information about currently running browser instances.
// Each BrowserInfo contains the browser name and its WebSocket endpoint URL.
func (s *AppState) RendererBrowsersInfo() []renderer.BrowserInfo {
	if s.Renderer == nil {
		return nil
	}
	return s.Renderer.BrowsersInfo()
}

// Close releases all resources held by the application state, including
// terminating any running browser processes used by the renderer and
// stopping background goroutines.
func (s *AppState) Close() {
	s.cancel()
	if s.Renderer != nil {
		s.Renderer.Close()
	}
}

// createJobID generates a unique job identifier string.
// Format: YYYYMMDDHHMMSS-random8 (e.g., "20260428130046-AYNLwr4e")
// The timestamp part helps with ordering and the random part ensures uniqueness.
func createJobID() string {
	return time.Now().Format("20060102150405") + "-" + generateRandomString(8)
}

// generateRandomString creates a random alphanumeric string of the specified length.
// Used for generating unique identifiers when combined with timestamps.
func generateRandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	if _, err := cryptoRand.Read(b); err == nil {
		for i := range b {
			b[i] = letters[int(b[i])%len(letters)]
		}
		return string(b)
	}

	// Fallback to time-based random if crypto/rand fails
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}

// LoadConfig loads application configuration from TOML file and environment variables.
// Configuration is layered: defaults -> TOML file -> environment variables.
func LoadConfig() (*types.AppConfig, error) {
	return config.LoadAppConfig()
}
