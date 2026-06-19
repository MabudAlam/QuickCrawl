"use client"

import Image from "next/image"
import Link from "next/link"
import { useState, useEffect } from "react"
import { useTheme } from "next-themes"
import {
  Globe,
  RefreshCw,
  Map,
  Search,
  Bot,
  Database,
  Zap,
  Code2,
  ChevronRight,
  CheckCircle2,
  ArrowRight,
  ArrowUpRight,
  Star,
  Terminal as TerminalIcon,
  Copy,
  Sun,
  Moon,
  Menu,
  X,
} from "lucide-react"
import { toast } from "sonner"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Switch } from "@/components/ui/switch"
import { BackgroundRippleEffect } from "@/components/ui/background-ripple-effect"
import { Terminal } from "@/components/ui/terminal"

const features = [
  {
    icon: Search,
    title: "Search",
    subtitle: "Fresh results from the live web. Never cached.",
    description:
      "Real browser-rendered search that returns structured JSON from dynamic pages and sources that change too fast for traditional search.",
    cta: "Try Search",
    badge: "Built for pricing shifts, earnings monitoring, and breaking news.",
    command: 'quickcrawl search "golang web scraping" --scrape',
    outputs: {
      0: [
        '{',
        '  "query": "golang web scraping",',
        '  "results": [',
        '    {',
        '      "title": "Building Web Scrapers in Go",',
        '      "url": "https://example.com/golang",',
        '      "snippet": "A comprehensive guide..."',
        '    },',
        '    {',
        '      "title": "Best Go Scraping Libraries",',
        '      "url": "https://example.com/libs",',
        '      "snippet": "Compare top libraries..."',
        '    }',
        '  ],',
        '  "total_results": 2,',
        '  "page": 1',
        '}',
      ],
    },
  },
  {
    icon: Globe,
    title: "Scrape",
    subtitle: "Any URL to clean, structured data.",
    description:
      "Convert any URL to Markdown, HTML, Plain Text, or Links. Full JS rendering without Puppeteer overhead. Handles anti-bot protections automatically.",
    cta: "Try Scrape",
    badge: "86.4% success rate across 1,000 URLs tested.",
    command: "quickcrawl scrape https://example.com",
    outputs: {
      0: [
        '{',
        '  "markdown": "# Example Domain",',
        '  "html": "<h1>Example Domain</h1>...",',
        '  "links": ["https://www.iana.org/..."],',
        '  "metadata": {',
        '    "title": "Example Domain",',
        '    "statusCode": 200,',
        '    "timeTaken": 281',
        '    "renderedMode": "http"',
        '  }',
        '}',
      ],
    },
  },
  {
    icon: RefreshCw,
    title: "Crawl",
    subtitle: "Map entire sites in minutes.",
    description:
      "BFS crawl entire websites respecting robots.txt. All pages returned as clean, structured data. Perfect for building knowledge bases.",
    cta: "Try Crawl",
    badge: "Handles 100+ pages per minute with rate limiting.",
    command: "quickcrawl crawl https://docs.example.com --max-depth 2 --max-pages 50",
    outputs: {
      0: [
        '{',
        '  "markdown": "# Getting Started...",',
        '  "metadata": { "sourceURL": "https://docs.example.com/start" }',
        '}',
        '{',
        '  "markdown": "# Installation Guide...",',
        '  "metadata": { "sourceURL": "https://docs.example.com/install" }',
        '}',
        '...',
        'crawl completed: 47 pages scraped',
      ],
    },
  },
  {
    icon: Map,
    title: "Map",
    subtitle: "Discover all URLs instantly.",
    description:
      "Fast sitemap generation without full page loads. Discover every endpoint on a domain for security audits and link analysis.",
    cta: "Try Map",
    badge: "Sitemap-aware discovery respects robots.txt.",
    command: "quickcrawl map https://example.com --max-depth 3",
    outputs: {
      0: [
        '{',
        '  "links": [',
        '    "https://example.com/",',
        '    "https://example.com/about",',
        '    "https://example.com/blog",',
        '    "https://example.com/contact"',
        '  ],',
        '  "count": 23',
        '}',
      ],
    },
  },
  {
    icon: Bot,
    title: "MCP Server",
    subtitle: "Drop into any AI agent instantly.",
    description:
      "Built-in stdio transport for seamless AI agent integration. Works with Claude Code, Cursor, Roo Code, and any MCP-compatible client.",
    cta: "View Docs",
    badge: "One config. Zero infrastructure.",
    command: "npx -y @mabudalam/quickcrawl-mcp",
    outputs: {
      0: [
        "Installing quickcrawl-mcp...",
        "added 1 package in 2s",
        "",
        "Configure in your agent:",
        '{',
        '  "mcpServers": {',
        '    "quickcrawl": {',
        '      "url": "http://localhost:3000/mcp"',
        '    }',
        '  }',
        '}',
      ],
    },
  },
  {
    icon: Database,
    title: "LLM Extraction",
    subtitle: "Structured data from any page.",
    description:
      "Send a JSON schema, get validated structured data back. Perfect for RAG pipelines, data enrichment, and content parsing.",
    cta: "Try Extract",
    badge: "Uses OpenAI for intelligent extraction.",
    command: 'quickcrawl scrape https://news.example.com --formats json --json-schema \'{"type":"object","properties":{"title":{"type":"string"},"author":{"type":"string"}}}\'',
    outputs: {
      0: [
        '{',
        '  "json": {',
        '    "title": "AI Agents Making Waves",',
        '    "author": "Jane Doe"',
        '  },',
        '  "metadata": {',
        '    "sourceURL": "https://news.example.com/ai-agents",',
        '    "timeTaken": 2100',
        '  }',
        '}',
      ],
    },
  },
]

const benchmarks = [
  { value: "86.4%", label: "Success Rate", sublabel: "864/1000 URLs" },
  { value: "2.2s", label: "P50 Latency", sublabel: "Median response" },
  { value: "7.6s", label: "P90 Latency", sublabel: "90th percentile" },
  { value: "90.4%", label: "Clean Content", sublabel: "Quality rate" },
]

const pricingPlans = [
  {
    name: "Free",
    description: "Self-host or use MCP. Zero cost, unlimited requests.",
    features: [
      "API Server Support",
      "MCP integration",
      "CLI tool",
      "Playground to test endpoints",
      "Self Host and scale freely",
      "No credit cards needed",
    ],
    cta: "Get Started",
  },
]

export default function LandingPage() {
  const { resolvedTheme, setTheme } = useTheme()
  const [mounted, setMounted] = useState(false)
  const [includeEnv, setIncludeEnv] = useState(false)
  const [searxngUrl, setSearxngUrl] = useState("")
  const [apiKey, setApiKey] = useState("")
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setMounted(true)
  }, [])

  return (
    <div className="min-h-screen bg-background">
      <nav className="fixed top-0 left-0 right-0 z-50 border-b-2 border-border bg-background/80 backdrop-blur-sm">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4">
          <Link href="/" className="flex items-center -ml-2">
            {mounted && resolvedTheme === "dark" ? (
              <img src="/qc-dark.svg" alt="QuickCrawl" className="h-10 w-auto" />
            ) : (
              <img src="/qc.svg" alt="QuickCrawl" className="h-10 w-auto" />
            )}
          </Link>

          <div className="hidden items-center gap-6 lg:gap-8 md:flex">
            <Link
              href="#features"
              className="text-sm font-base text-foreground/70 hover:text-foreground"
            >
              Features
            </Link>
            <Link
              href="#cli"
              className="text-sm font-base text-foreground/70 hover:text-foreground"
            >
              CLI
            </Link>
            <Link
              href="#agents"
              className="text-sm font-base text-foreground/70 hover:text-foreground"
            >
              Agents
            </Link>
            <Link
              href="#skills"
              className="text-sm font-base text-foreground/70 hover:text-foreground"
            >
              Skills
            </Link>
            <Link
              href="#pricing"
              className="text-sm font-base text-foreground/70 hover:text-foreground"
            >
              Pricing
            </Link>
            <Link
              href="/blog/benchmark"
              className="text-sm font-base text-foreground/70 hover:text-foreground"
            >
              Benchmark
            </Link>
            <Button
              variant="noShadow"
              size="icon"
              onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
            >
              {mounted && resolvedTheme === "dark" ? (
                <Sun className="h-4 w-4" />
              ) : (
                <Moon className="h-4 w-4" />
              )}
            </Button>
          </div>

          <div className="flex items-center gap-2">
            <Button
              variant="noShadow"
              size="icon"
              className="md:hidden"
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            >
              {mobileMenuOpen ? (
                <X className="h-5 w-5" />
              ) : (
                <Menu className="h-5 w-5" />
              )}
            </Button>
            <div className="hidden items-center gap-2 md:flex">
              <Button variant="noShadow" size="sm" asChild>
                <a
                  href="https://github.com/MabudAlam/quickcrawl"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
                  </svg>
                  <span className="ml-2">GitHub</span>
                </a>
              </Button>
              <Button size="sm" asChild>
                <Link href="/playground">
                  Playground
                  <ArrowRight className="ml-2 h-4 w-4" />
                </Link>
              </Button>
            </div>
          </div>
        </div>

        {mobileMenuOpen && (
          <div className="border-t-2 border-border md:hidden">
            <div className="flex flex-col gap-4 px-4 py-4">
              <Link
                href="#features"
                className="text-sm font-base text-foreground/70 hover:text-foreground"
                onClick={() => setMobileMenuOpen(false)}
              >
                Features
              </Link>
              <Link
                href="#cli"
                className="text-sm font-base text-foreground/70 hover:text-foreground"
                onClick={() => setMobileMenuOpen(false)}
              >
                CLI
              </Link>
              <Link
                href="#agents"
                className="text-sm font-base text-foreground/70 hover:text-foreground"
                onClick={() => setMobileMenuOpen(false)}
              >
                Agents
              </Link>
              <Link
                href="#skills"
                className="text-sm font-base text-foreground/70 hover:text-foreground"
                onClick={() => setMobileMenuOpen(false)}
              >
                Skills
              </Link>
              <Link
                href="#pricing"
                className="text-sm font-base text-foreground/70 hover:text-foreground"
                onClick={() => setMobileMenuOpen(false)}
              >
                Pricing
              </Link>
              <Link
                href="/blog/benchmark"
                className="text-sm font-base text-foreground/70 hover:text-foreground"
                onClick={() => setMobileMenuOpen(false)}
              >
                Benchmark
              </Link>
              <div className="flex items-center gap-3 pt-4 border-t-2 border-border">
                <Button variant="noShadow" size="sm" asChild>
                  <a
                    href="https://github.com/MabudAlam/quickcrawl"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
                      <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
                    </svg>
                    <span className="ml-2">GitHub</span>
                  </a>
                </Button>
                <Button size="sm" asChild>
                  <Link href="/playground">
                    Playground
                    <ArrowRight className="ml-2 h-4 w-4" />
                  </Link>
                </Button>
              </div>
            </div>
          </div>
        )}
      </nav>

      <section className="relative flex min-h-screen flex-col items-center justify-center overflow-hidden px-4 pt-16">
        <BackgroundRippleEffect />

        <div className="absolute inset-0 bg-gradient-to-b from-transparent via-background/50 to-background z-[1]" />

        <div className="relative z-10 flex flex-col items-center text-center max-w-5xl mx-auto px-4">
          <Badge variant="default" className="mb-6 md:mb-8 px-3 md:px-4 py-1.5 md:py-2 text-xs md:text-base shadow-shadow">
            <Zap className="mr-1.5 md:mr-2 h-3 w-3 md:h-4 md:w-4" />
            86.4% scrape success rate · 2.2s median latency
          </Badge>

          <h1 className="mb-4 md:mb-6 font-heading text-3xl sm:text-4xl md:text-5xl lg:text-7xl font-bold tracking-tight leading-tight">
            <span className="text-foreground">Your AI agents&apos;</span>
            <br />
            <span className="text-main">window to the web</span>
          </h1>

          <p className="mb-8 md:mb-12 max-w-2xl text-base md:text-xl text-foreground/70 leading-relaxed px-2">
            Give your AI agents real-time web access. Scrape, crawl, map, and search
            — one API, zero infrastructure headaches.
          </p>

          <div className="flex flex-col sm:flex-row gap-3 sm:gap-4 mb-10 md:mb-16 w-full sm:w-auto">
            <Button size="lg" className="text-sm sm:text-base px-6 sm:px-8 py-4 sm:py-6 w-full sm:w-auto" asChild>
              <a
                href={process.env.NEXT_PUBLIC_DOCS_URL || "https://quickcrawl.dev"}
                target="_blank"
                rel="noopener noreferrer"
              >
                Docs
                <ArrowRight className="ml-2 h-4 w-4 sm:h-5 sm:w-5" />
              </a>
            </Button>
            <Button variant="neutral" size="lg" className="text-sm sm:text-base px-6 sm:px-8 py-4 sm:py-6 w-full sm:w-auto" asChild>
              <Link href="#cli">
                <Code2 className="mr-2 h-4 w-4 sm:h-5 sm:w-5" />
                Get Started
              </Link>
            </Button>
          </div>

          <Link href="/blog/benchmark" className="grid grid-cols-2 md:grid-cols-4 gap-3 md:gap-6 w-full max-w-3xl">
            {benchmarks.map((stat) => (
              <Card key={stat.label} className="text-center py-4 md:py-6 px-2 md:px-4 hover:-translate-y-1 transition-all duration-300 cursor-pointer">
                <CardContent className="pt-0">
                  <div className="font-heading text-2xl sm:text-3xl md:text-4xl font-bold text-main mb-1 md:mb-2">
                    {stat.value}
                  </div>
                  <div className="font-base text-xs md:text-sm text-foreground/70">
                    {stat.label}
                  </div>
                </CardContent>
              </Card>
            ))}
          </Link>
        </div>
      </section>

      <section id="features" className="border-y-2 border-border bg-secondary-background px-4 py-24 scroll-mt-20">
        <div className="mx-auto max-w-7xl">
          <div className="mb-16 text-center">
            <h2 className="mb-4 font-heading text-4xl font-bold md:text-5xl">
              Everything you need to scrape the web
            </h2>
            <p className="mx-auto max-w-2xl text-lg text-foreground/70">
              Five endpoints cover the whole workflow. One tool instead of five
              different services.
            </p>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
            {features.map((feature) => (
              <Card key={feature.title} className="overflow-hidden hover:shadow-none transition-all duration-300 hover:-translate-y-1">
                <CardHeader className="pb-4">
                  <div className="flex items-center gap-4">
                    <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-xl bg-main/10 border-2 border-border shadow-shadow">
                      <feature.icon className="h-7 w-7 text-main" />
                    </div>
                    <div>
                      <CardTitle className="text-2xl">{feature.title}</CardTitle>
                      <p className="text-sm text-main/80 mt-1">{feature.subtitle}</p>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-4">
                  <p className="text-foreground/70">{feature.description}</p>
                  <Badge variant="neutral" className="text-xs">
                    {feature.badge}
                  </Badge>
                  <Terminal
                    commands={[feature.command]}
                    outputs={feature.outputs}
                    username="skmabudalam"
                    className="max-w-full"
                    typingSpeed={30}
                    delayBetweenCommands={2000}
                    enableSound={false}
                  />
                  <Link
                    href="#"
                    className="inline-flex items-center gap-2 text-sm text-main hover:text-main/80 font-medium pt-2"
                  >
                    {feature.cta}
                    <ArrowRight className="h-4 w-4" />
                  </Link>
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      </section>

      <section id="cli" className="border-y-2 border-border px-4 py-24 scroll-mt-20">
        <div className="mx-auto max-w-7xl">
          <div className="mb-16 text-center">
            <h2 className="mb-4 font-heading text-4xl font-bold md:text-5xl">
              Install the CLI
            </h2>
            <p className="mx-auto max-w-2xl text-lg text-foreground/70">
              Standalone command-line access. No server or Python needed.
            </p>
          </div>

          <Card className="max-w-xl mx-auto">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <TerminalIcon className="h-5 w-5" />
                Quick Install
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex gap-2">
                <Input
                  readOnly
                  value="curl -fsSL https://raw.githubusercontent.com/MabudAlam/quickcrawl/main/install.sh | sh"
                  className="font-mono text-sm"
                />
                <Button
                  variant="neutral"
                  size="icon"
                  onClick={() => {
                    navigator.clipboard.writeText("curl -fsSL https://raw.githubusercontent.com/MabudAlam/quickcrawl/main/install.sh | sh")
                    toast.success("Copied to clipboard")
                  }}
                  className="shrink-0"
                >
                  <Copy className="h-4 w-4" />
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      </section>

      <section id="agents" className="border-y-2 border-border bg-secondary-background px-4 py-24 scroll-mt-20">
        <div className="mx-auto max-w-7xl">
          <div className="mb-16 text-center">
            <h2 className="mb-4 font-heading text-4xl font-bold md:text-5xl">
              Works with every major AI agent
            </h2>
            <p className="mx-auto max-w-2xl text-lg text-foreground/70">
              Install QuickCrawl MCP with one command. Auto-detects installed agents or target a specific one.
            </p>
          </div>

          <Card className="mb-8">
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle className="flex items-center gap-2">
                  <TerminalIcon className="h-5 w-5" />
                  Quick Install (all agents)
                </CardTitle>
                <div className="flex items-center gap-3">
                  <span className="text-sm text-foreground/70">Environment variables</span>
                  <Switch
                    checked={includeEnv}
                    onCheckedChange={setIncludeEnv}
                  />
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {includeEnv && (
                <div className="grid gap-3">
                  <div>
                    <label className="text-sm text-foreground/70 mb-1 block">SearXNG URL</label>
                    <Input
                      placeholder="https://searxng-qc.up.railway.app/"
                      value={searxngUrl}
                      onChange={(e) => setSearxngUrl(e.target.value)}
                      className="font-mono text-sm"
                    />
                  </div>
                  <div>
                    <label className="text-sm text-foreground/70 mb-1 block">OpenAI API Key</label>
                    <Input
                      placeholder="sk-proj-..."
                      type="password"
                      value={apiKey}
                      onChange={(e) => setApiKey(e.target.value)}
                      className="font-mono text-sm"
                    />
                  </div>
                  <p className="text-xs text-foreground/50">
                    Note: Override any <code className="bg-background px-1 rounded">quickcrawl.toml</code> value using env vars like <code className="bg-background px-1 rounded">SEARCH__BASE_URL</code>, <code className="bg-background px-1 rounded">EXTRACTION__LLM__API_KEY</code>, etc.
                  </p>
                  <p className="text-xs text-main/80">
                    Search and JSON output require both SearXNG URL and API key to be set.
                  </p>
                </div>
              )}
              <div className="flex gap-2">
                <Input
                  readOnly
                  value={includeEnv && (searxngUrl || apiKey)
                    ? `npx agent-install@latest mcp add @mabudalam/quickcrawl-mcp -g${searxngUrl ? ` --env "SEARCH__BASE_URL=${searxngUrl}"` : ""}${apiKey ? ` --env "EXTRACTION__LLM__API_KEY=${apiKey}"` : ""}`
                    : "npx agent-install@latest mcp add @mabudalam/quickcrawl-mcp -g"
                  }
                  className="font-mono text-sm"
                />
                <Button
                  variant="neutral"
                  size="icon"
                  onClick={() => {
                    const cmd = includeEnv && (searxngUrl || apiKey)
                      ? `npx agent-install@latest mcp add @mabudalam/quickcrawl-mcp -g${searxngUrl ? ` --env "SEARCH__BASE_URL=${searxngUrl}"` : ""}${apiKey ? ` --env "EXTRACTION__LLM__API_KEY=${apiKey}"` : ""}`
                      : "npx agent-install@latest mcp add @mabudalam/quickcrawl-mcp -g"
                    navigator.clipboard.writeText(cmd)
                    toast.success("Copied to clipboard")
                  }}
                  className="shrink-0"
                >
                  <Copy className="h-4 w-4" />
                </Button>
              </div>
            </CardContent>
          </Card>

          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {[
              { name: "Claude Code", image: "/claude.webp", agent: "claude-code", desc: "Anthropic's CLI agent" },
              { name: "Cursor", image: "/cursor.webp", agent: "cursor", desc: "AI-first code editor" },
              { name: "Gemini CLI", image: "/gemini.webp", agent: "gemini-cli", desc: "Google's CLI agent" },
              { name: "OpenAI Codex", image: "/openai.webp", agent: "codex", desc: "OpenAI's coding agent" },
              { name: "OpenCode", image: "/qc.svg", agent: "opencode", desc: "Open-source CLI agent" },
              { name: "Windsurf", image: "/windsurf.webp", agent: "windsurf", desc: "Codeium's AI IDE" },
            ].map((item) => {
              const cmd = `npx agent-install@latest mcp add @mabudalam/quickcrawl-mcp -g -a ${item.agent}`
              return (
                <Card key={item.name}>
                  <CardHeader className="pb-2">
                    <div className="flex items-center gap-3">
                      <Image src={item.image} alt={item.name} width={24} height={24} className="rounded" />
                      <CardTitle className="text-lg">{item.name}</CardTitle>
                    </div>
                    <p className="text-sm text-foreground/50">{item.desc}</p>
                  </CardHeader>
                  <CardContent>
                    <div className="flex gap-2">
                      <Input
                        readOnly
                        value={cmd}
                        className="font-mono text-xs"
                      />
                      <Button
                        variant="neutral"
                        size="icon"
                        onClick={() => {
                          navigator.clipboard.writeText(cmd)
                          toast.success("Copied to clipboard")
                        }}
                        className="shrink-0"
                      >
                        <Copy className="h-4 w-4" />
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              )
            })}
          </div>
        </div>
      </section>

      <section id="skills" className="border-y-2 border-border px-4 py-24 scroll-mt-20">
        <div className="mx-auto max-w-7xl">
          <div className="mb-16 text-center">
            <h2 className="mb-4 font-heading text-4xl font-bold md:text-5xl">
              Teach your agent to use QuickCrawl
            </h2>
            <p className="mx-auto max-w-2xl text-lg text-foreground/70">
              QuickCrawl ships with SKILL.md files that teach AI coding agents how to use it. Install with one command.
            </p>
          </div>

          <Card className="max-w-xl mx-auto">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Bot className="h-5 w-5" />
                Install Skill
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex gap-2">
                <Input
                  readOnly
                  value="npx skills add MabudAlam/QuickCrawl"
                  className="font-mono text-sm"
                />
                <Button
                  variant="neutral"
                  size="icon"
                  onClick={() => {
                    navigator.clipboard.writeText("npx skills add MabudAlam/QuickCrawl")
                    toast.success("Copied to clipboard")
                  }}
                  className="shrink-0"
                >
                  <Copy className="h-4 w-4" />
                </Button>
              </div>
              <p className="mt-4 text-sm text-foreground/60">
                Installs both quickcrawl-cli and quickcrawl-mcp skills to your agent.
              </p>
            </CardContent>
          </Card>
        </div>
      </section>

      <section
        id="pricing"
        className="border-y-2 border-border bg-secondary-background px-4 py-24 scroll-mt-20"
      >
        <div className="mx-auto max-w-7xl">
          <div className="mb-16 text-center">
            <h2 className="mb-4 font-heading text-4xl font-bold md:text-5xl">
              Free and open source
            </h2>
           
          </div>

          <div className="flex justify-center">
            <Card className="w-full max-w-md transition-all duration-300 hover:-translate-y-1 hover:shadow-none">
              <CardHeader className="text-center">
                <CardTitle className="text-2xl">Free</CardTitle>
                <p className="mt-2 text-sm text-foreground/70">
                  {pricingPlans[0].description}
                </p>
              </CardHeader>
              <CardContent>
                <ul className="mb-6 space-y-3">
                  {pricingPlans[0].features.map((feature) => (
                    <li key={feature} className="flex items-center gap-2 text-sm">
                      <CheckCircle2 className="h-4 w-4 text-main" />
                      <span className="text-foreground/80">{feature}</span>
                    </li>
                  ))}
                </ul>
                <div className="flex flex-col gap-3">
                  <Button className="w-full" asChild>
                    <Link href="https://github.com/MabudAlam/quickcrawl">
                      <svg className="mr-2 h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
                        <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
                      </svg>
                      Star on GitHub
                    </Link>
                  </Button>
                  <Button variant="neutral" className="w-full" asChild>
                    <Link href="/playground">
                      Try Playground
                      <ChevronRight className="ml-2 h-4 w-4" />
                    </Link>
                  </Button>
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      </section>

      <section className="border-y-2 border-border px-4 py-24 scroll-mt-20">
        <div className="mx-auto max-w-3xl text-center">
          <h2 className="mb-6 font-heading text-4xl font-bold md:text-5xl">
            The web scraping API built for AI.
          </h2>
          <p className="mb-10 text-lg text-foreground/70">
            Scrape, crawl, and search — one API, zero infrastructure.
          </p>
          <div className="flex flex-col gap-4 sm:flex-row sm:justify-center">
            <Button size="lg" asChild>
              <a
                href={process.env.NEXT_PUBLIC_DOCS_URL || "https://quickcrawl.dev"}
                target="_blank"
                rel="noopener noreferrer"
              >
                Docs
                <ArrowRight className="ml-2 h-5 w-5" />
              </a>
            </Button>
            <Button variant="neutral" size="lg" asChild>
              <Link href="https://github.com/MabudAlam/quickcrawl">
                <Code2 className="mr-2 h-5 w-5" />
                View on GitHub
              </Link>
            </Button>
          </div>
        </div>
      </section>

      <footer className="border-t-2 border-border bg-background px-4 py-16">
        <div className="mx-auto max-w-7xl">
          <div className="grid gap-8 md:grid-cols-4">
            <div>
              <Link href="/" className="flex items-center gap-2">
                {mounted && resolvedTheme === "dark" ? (
                  <img src="/qc-dark.svg" alt="QuickCrawl" className="h-10 w-auto" />
                ) : (
                  <img src="/qc.svg" alt="QuickCrawl" className="h-10 w-auto" />
                )}
              </Link>
              <p className="mt-4 text-sm text-foreground/50">
                Open source web scraping API for AI agents. Self-host or use the
                cloud — same API.
              </p>

            </div>

            <div>
              <h5 className="mb-4 font-heading font-bold">Product</h5>
              <ul className="space-y-3 text-sm">
                {["Features", "Benchmark", "Documentation"].map((item) => (
                  <li key={item}>
                    <Link
                      href="#"
                      className="text-foreground/50 hover:text-foreground"
                    >
                      {item}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>

            <div>
              <h5 className="mb-4 font-heading font-bold">Resources</h5>
              <ul className="space-y-3 text-sm">
                {["API Reference", "SDKs", "Examples", "Blog"].map((item) => (
                  <li key={item}>
                    <Link
                      href="#"
                      className="text-foreground/50 hover:text-foreground"
                    >
                      {item}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          </div>

          <div className="mt-12 flex flex-col items-center justify-between gap-4 border-t-2 border-border pt-8 md:flex-row">
            <p className="text-sm text-foreground/50">
              &copy; 2026 QuickCrawl.
            </p>
          </div>
        </div>
      </footer>
    </div>
  )
}
