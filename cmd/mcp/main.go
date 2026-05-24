package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/MabudAlam/quickcrawl/internal/api"
	"github.com/MabudAlam/quickcrawl/internal/mcp"
	"github.com/MabudAlam/quickcrawl/internal/renderer"
)

const (
	serverName    = "quickcrawl"
	serverVersion = "1.0.0"
)

func main() {
	cfg, err := api.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %s", err.Error())
	}

	state, stateErr := api.NewAppState(cfg)
	if stateErr != nil {
		log.Fatalf("failed to initialize server state: %s", stateErr.Message)
	}

	rend, rendererErr := renderer.NewFallbackRendererWithConfig(
		&cfg.Renderer,
		cfg.Crawler.UserAgent,
		&cfg.Crawler.Stealth,
		cfg.Renderer.RenderJSDefault,
	)
	if rendererErr != nil {
		log.Fatalf("failed to initialize renderer: %s", rendererErr.Message)
	}

	state.Renderer = rend
	defer state.Close()

	browsers := rend.BrowsersInfo()
	if len(browsers) > 0 {
		for _, b := range browsers {
			log.Printf("browser started: %s (%s)", b.Name, b.WSURL)
		}
	}

	server := mcp.NewServer(
		&mcp.Implementation{Name: serverName, Version: serverVersion},
		nil,
	)

	serverImpl := quickcrawl.NewServer(state, cfg)
	quickcrawl.AddTools(server, serverImpl)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("shutting down MCP server...")
		cancel()
	}()

	log.Println("MCP server starting...")
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Printf("MCP server error: %v", err)
	}

	log.Println("MCP server exited gracefully")
	os.Exit(0)
}
