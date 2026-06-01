package routes

import (
	"time"

	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/api/handlers"
	"github.com/MabudAlam/quickcrawl/internal/api/middleware"
	"github.com/gin-gonic/gin"
)

// Router holds the Gin engine and application state for setting up HTTP routes.
type Router struct {
	Engine       *gin.Engine   // Gin HTTP engine
	State        *api.AppState // Application state (config, jobs, renderer)
	RateLimitRPS uint64        // Requests per second rate limit (0 = disabled)
}

// NewRouter creates a new Router with the given application state and rate limit.
// It sets Gin to release mode (no debug logging) and adds recovery/logger middleware.
func NewRouter(state *api.AppState, rateLimitRPS uint64) *Router {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(gin.Logger())

	return &Router{
		Engine:       engine,
		State:        state,
		RateLimitRPS: rateLimitRPS,
	}
}

// Setup configures all routes and middleware, returning the Gin engine.
func (r *Router) Setup() *gin.Engine {
	engine := r.Engine

	// Add global middleware (CORS, rate limiting)
	r.setupGlobalMiddleware(engine)

	// Register all HTTP route handlers
	r.setupRoutes(engine)

	return engine
}

// setupGlobalMiddleware adds middleware that applies to all routes.
// Currently includes CORS headers and optional rate limiting.
func (r *Router) setupGlobalMiddleware(engine *gin.Engine) {
	engine.Use(middleware.CORSMiddlewareGin())

	if r.RateLimitRPS > 0 {
		// Create a token bucket rate limiter: rate limit requests per second
		limiter := middleware.NewRateLimiter(int(r.RateLimitRPS), time.Second)
		engine.Use(middleware.RateLimitMiddlewareGin(limiter))
	}
}

// setupRoutes registers all API route handlers.
func (r *Router) setupRoutes(engine *gin.Engine) {
	h := handlers.NewHandler(r.State)
	ch := handlers.NewCoreHandler(r.State)

	// Health check endpoint - returns renderer/browser availability and active job count
	engine.GET("/health", h.Health)

	// v1 API group
	v1 := engine.Group("/v1")
	{
		// POST /v1/scrape - Scrape a single URL and return content in various formats
		v1.POST("/scrape", h.Scrape)

		// POST /v1/scrape-core - Scrape using chromedp-based core implementation
		v1.POST("/scrape-core", ch.ScrapeHandler)

		// POST /v1/crawl - Start an async BFS crawl of a website
		v1.POST("/crawl", h.StartCrawl)

		// GET /v1/crawl/:id - Get the status/results of a crawl job
		v1.GET("/crawl/:id", h.GetCrawlStatus)

		// DELETE /v1/crawl/:id - Cancel a running crawl job
		v1.DELETE("/crawl/:id", h.CancelCrawl)

		// POST /v1/map - Discover all URLs on a website without scraping content
		v1.POST("/map", h.Map)

		// POST /v1/search - Search DuckDuckGo
		v1.POST("/search", h.Search)
	}
}
