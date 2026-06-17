"use client"

import Link from "next/link"
import { useState, useEffect } from "react"
import { useTheme } from "next-themes"
import { ArrowLeft, ExternalLink, Copy, Check, BarChart3, Zap, Globe, Shield, Clock, TrendingUp, Sun, Moon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

const benchmarkData = {
  schema: "quickcrawl.bench/v3",
  timestamp_ist: "2026-06-16T09:57:24.265672+05:30",
  scraper: "quickcrawl",
  dataset: "firecrawl/scrape-content-dataset-v1",
  config: {
    endpoint: "http://localhost:3000",
    max_concurrent_requests: 5,
    request_timeout_seconds: 120,
    max_urls_to_test: 1000,
    total_urls: 1000
  },
  coverage: {
    total_urls: 1000,
    successful_scrapes: 864,
    failed_scrapes: 136,
    success_rate_pct: 86.4
  },
  response_time_ms: {
    average_ms: 3776.8,
    p50_ms: 2242.6,
    p90_ms: 7590.8,
    p95_ms: 13000.8,
    p99_ms: 21188.8,
    successful_request_count: 864
  },
  content_accuracy: {
    urls_with_ground_truth: 769,
    total_phrases_to_find: 11231,
    total_phrases_found: 3660,
    urls_with_matched_phrases: 412,
    phrase_match_rate_pct: 53.58,
    phrase_match_rate_overall_pct: 47.69,
    urls_with_leaked_phrases: 83,
    clean_content_rate_pct: 90.39
  },
  most_common_failures: {
    "target returned HTTP Not Found": 59,
    "target returned HTTP Forbidden": 36,
    "no markdown": 12,
    "target returned HTTP Not Acceptable": 3,
    "target returned HTTP Internal Server Err": 2,
    "target returned HTTP Bad Request": 1,
    "target returned HTTP Service Unavailable": 1,
    "page load timed out for https://www.bgky": 1,
    "page load timed out for http://fanduel.c": 1,
    "target returned HTTP Unauthorized": 1
  }
}

const rawJson = JSON.stringify(benchmarkData, null, 2)

export default function BenchmarkBlogPage() {
  const { resolvedTheme, setTheme } = useTheme()
  const [mounted, setMounted] = useState(false)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setMounted(true)
  }, [])

  const copyToClipboard = () => {
    navigator.clipboard.writeText(rawJson)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="min-h-screen bg-background">
      <nav className="sticky top-0 z-50 border-b-2 border-border bg-background/80 backdrop-blur-sm">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4">
          <Link href="/" className="flex items-center -ml-2">
            {mounted && resolvedTheme === "dark" ? (
              <img src="/qc-dark.svg" alt="QuickCrawl" className="h-10 w-auto" />
            ) : (
              <img src="/qc.svg" alt="QuickCrawl" className="h-10 w-auto" />
            )}
          </Link>

          <div className="flex items-center gap-3">
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
            <Link href="/">
              <Button variant="noShadow" size="sm">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back
              </Button>
            </Link>
          </div>
        </div>
      </nav>

      <main className="mx-auto max-w-5xl px-4 py-12">
        <div className="mb-8">
          <Badge variant="default" className="mb-4">
            Benchmark Results
          </Badge>
          <h1 className="mb-4 font-heading text-4xl font-bold md:text-5xl">
            QuickCrawl vs Firecrawl Dataset: 86.4% Success Rate
          </h1>
          <p className="text-lg text-foreground/70">
            June 16, 2026 · Benchmark against the Firecrawl Scrape Content Dataset v1
          </p>
        </div>

        <div className="mb-12 rounded-lg border-2 border-border bg-secondary-background p-6">
          <h2 className="mb-4 font-heading text-2xl font-bold">Key Highlights</h2>
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Card>
              <CardContent className="pt-6">
                <div className="flex items-center gap-3 mb-2">
                  <Zap className="h-5 w-5 text-main" />
                  <span className="text-sm text-foreground/70">Success Rate</span>
                </div>
                <div className="font-heading text-3xl font-bold text-main">86.4%</div>
                <div className="text-xs text-foreground/50 mt-1">864/1000 URLs</div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <div className="flex items-center gap-3 mb-2">
                  <Clock className="h-5 w-5 text-main" />
                  <span className="text-sm text-foreground/70">P50 Latency</span>
                </div>
                <div className="font-heading text-3xl font-bold text-main">2.2s</div>
                <div className="text-xs text-foreground/50 mt-1">Median response</div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <div className="flex items-center gap-3 mb-2">
                  <Shield className="h-5 w-5 text-main" />
                  <span className="text-sm text-foreground/70">Clean Content</span>
                </div>
                <div className="font-heading text-3xl font-bold text-main">90.4%</div>
                <div className="text-xs text-foreground/50 mt-1">Quality rate</div>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-6">
                <div className="flex items-center gap-3 mb-2">
                  <TrendingUp className="h-5 w-5 text-main" />
                  <span className="text-sm text-foreground/70">P90 Latency</span>
                </div>
                <div className="font-heading text-3xl font-bold text-main">7.6s</div>
                <div className="text-xs text-foreground/50 mt-1">90th percentile</div>
              </CardContent>
            </Card>
          </div>
        </div>

        <div className="prose prose-invert max-w-none mb-12">
          <h2 className="font-heading text-2xl font-bold mb-4">Methodology</h2>
          <p className="text-foreground/70 mb-4">
            We tested QuickCrawl against the Firecrawl Scrape Content Dataset v1, a comprehensive
            collection of 1,000 diverse URLs designed to benchmark web scraping tools. This dataset
            includes a variety of websites with different architectures, anti-bot measures, and content
            types.
          </p>
          <div className="rounded-lg border border-border bg-background p-4 mb-4">
            <h3 className="font-heading text-lg font-semibold mb-2">Test Configuration</h3>
            <ul className="list-disc list-inside text-foreground/70 space-y-1 text-sm">
              <li>Endpoint: <code className="bg-secondary-background px-2 py-0.5 rounded">http://localhost:3000</code></li>
              <li>Max concurrent requests: <code className="bg-secondary-background px-2 py-0.5 rounded">5</code></li>
              <li>Request timeout: <code className="bg-secondary-background px-2 py-0.5 rounded">120 seconds</code></li>
              <li>Total URLs tested: <code className="bg-secondary-background px-2 py-0.5 rounded">1,000</code></li>
            </ul>
          </div>
        </div>

        <div className="mb-12">
          <h2 className="mb-4 font-heading text-2xl font-bold">Detailed Results</h2>
          <div className="grid gap-6 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <BarChart3 className="h-5 w-5 text-main" />
                  Coverage Metrics
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex justify-between items-center">
                  <span className="text-foreground/70">Successful Scrapes</span>
                  <span className="font-mono font-bold">864</span>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-foreground/70">Failed Scrapes</span>
                  <span className="font-mono font-bold text-red-400">136</span>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-foreground/70">Success Rate</span>
                  <span className="font-mono font-bold text-main">86.4%</span>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Clock className="h-5 w-5 text-main" />
                  Response Time
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex justify-between items-center">
                  <span className="text-foreground/70">Average</span>
                  <span className="font-mono font-bold">3,776.8ms</span>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-foreground/70">P50 (Median)</span>
                  <span className="font-mono font-bold text-main">2,242.6ms</span>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-foreground/70">P90</span>
                  <span className="font-mono font-bold">7,590.8ms</span>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-foreground/70">P95</span>
                  <span className="font-mono font-bold">13,000.8ms</span>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-foreground/70">P99</span>
                  <span className="font-mono font-bold">21,188.8ms</span>
                </div>
              </CardContent>
            </Card>

            <Card className="md:col-span-2">
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Globe className="h-5 w-5 text-main" />
                  Content Accuracy
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="grid gap-4 md:grid-cols-4">
                  <div className="text-center p-4 rounded-lg bg-secondary-background">
                    <div className="font-heading text-2xl font-bold text-main">90.4%</div>
                    <div className="text-xs text-foreground/70 mt-1">Clean Content Rate</div>
                  </div>
                  <div className="text-center p-4 rounded-lg bg-secondary-background">
                    <div className="font-heading text-2xl font-bold text-main">53.6%</div>
                    <div className="text-xs text-foreground/70 mt-1">Phrase Match Rate</div>
                  </div>
                  <div className="text-center p-4 rounded-lg bg-secondary-background">
                    <div className="font-heading text-2xl font-bold text-main">769</div>
                    <div className="text-xs text-foreground/70 mt-1">URLs with Ground Truth</div>
                  </div>
                  <div className="text-center p-4 rounded-lg bg-secondary-background">
                    <div className="font-heading text-2xl font-bold text-main">412</div>
                    <div className="text-xs text-foreground/70 mt-1">URLs with Matched Phrases</div>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card className="md:col-span-2">
              <CardHeader>
                <CardTitle>Common Failure Reasons</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  {Object.entries(benchmarkData.most_common_failures).map(([reason, count]) => (
                    <div key={reason} className="flex justify-between items-center">
                      <span className="text-sm text-foreground/70 truncate mr-4">{reason}</span>
                      <span className="font-mono text-sm font-bold text-red-400">{count as number}</span>
                    </div>
                  ))}
                </div>
                <p className="mt-4 text-xs text-foreground/50">
                  Note: Most failures are due to target websites returning 404/403 errors,
                  not QuickCrawl issues.
                </p>
              </CardContent>
            </Card>
          </div>
        </div>

        <div className="mb-12">
          <h2 className="mb-4 font-heading text-2xl font-bold">Raw JSON Data</h2>
          <Tabs defaultValue="preview">
            <TabsList className="mb-4">
              <TabsTrigger value="preview">Preview</TabsTrigger>
              <TabsTrigger value="json">JSON</TabsTrigger>
            </TabsList>
            <TabsContent value="preview">
              <Card>
                <CardContent className="pt-6">
                  <pre className="overflow-x-auto text-sm font-mono whitespace-pre-wrap">
                    {rawJson}
                  </pre>
                </CardContent>
              </Card>
            </TabsContent>
            <TabsContent value="json">
              <Card>
                <CardHeader className="flex flex-row items-center justify-between">
                  <CardTitle className="text-base">summary.json</CardTitle>
                  <Button
                    variant="noShadow"
                    size="sm"
                    onClick={copyToClipboard}
                  >
                    {copied ? (
                      <>
                        <Check className="mr-2 h-4 w-4" />
                        Copied!
                      </>
                    ) : (
                      <>
                        <Copy className="mr-2 h-4 w-4" />
                        Copy JSON
                      </>
                    )}
                  </Button>
                </CardHeader>
                <CardContent>
                  <pre className="overflow-x-auto text-sm font-mono">
                    {rawJson}
                  </pre>
                </CardContent>
              </Card>
            </TabsContent>
          </Tabs>
        </div>

        <div className="flex justify-center">
          <Link href="/">
            <Button size="lg">
              Try QuickCrawl
              <ExternalLink className="ml-2 h-4 w-4" />
            </Button>
          </Link>
        </div>
      </main>

      <footer className="border-t-2 border-border bg-background px-4 py-8">
        <div className="mx-auto max-w-7xl text-center">
          <p className="text-sm text-foreground/50">
            &copy; 2026 QuickCrawl. AGPL-3.0 Licensed.
          </p>
        </div>
      </footer>
    </div>
  )
}
