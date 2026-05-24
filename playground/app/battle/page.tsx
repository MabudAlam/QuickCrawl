"use client"

import { useState } from "react"
import {
  Loader2,
  Play,
  AlertCircle,
  CheckCircle,
  XCircle,
  Clock,
  ExternalLink,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Switch } from "@/components/ui/switch"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Alert, AlertDescription } from "@/components/ui/alert"
import ReactMarkdown from "react-markdown"

interface ScrapeResult {
  success: boolean
  data?: {
    markdown?: string
    html?: string
    plainText?: string
    metadata?: {
      title?: string
      description?: string
      timeTaken?: number
      [key: string]: unknown
    }
  }
  error?: string
  timeTaken?: number
}

interface BattleResult {
  quickcrawl: ScrapeResult
  tinyfish: ScrapeResult
}

function getBaseUrl() {
  return process.env.NEXT_PUBLIC_BASE_URL
}

async function fetchWithTimeout(
  url: string,
  options: RequestInit,
  timeoutMs: number = 60000
): Promise<Response> {
  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs)

  try {
    const response = await fetch(url, {
      ...options,
      signal: controller.signal,
    })
    clearTimeout(timeoutId)
    return response
  } catch (error) {
    clearTimeout(timeoutId)
    throw error
  }
}

async function scrapeWithQuickCrawl(
  url: string,
  sharedStartTime: number,
  renderJs: boolean = false
): Promise<ScrapeResult> {
  const baseUrl = getBaseUrl()

  try {
    const response = await fetchWithTimeout(`${baseUrl}/v1/scrape`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        url,
        formats: ["markdown"],
        renderJs,
      }),
    })

    const timeTaken = Date.now() - sharedStartTime
    const data = await response.json()

    if (!response.ok) {
      return {
        success: false,
        error: data.error || response.statusText,
        timeTaken,
      }
    }

    return {
      success: true,
      data: {
        markdown: data.data?.markdown,
        html: data.data?.html,
        plainText: data.data?.plainText,
        metadata: {
          title: data.data?.metadata?.title,
          description: data.data?.metadata?.description,
          timeTaken: data.data?.metadata?.timeTaken || timeTaken,
        },
      },
      timeTaken,
    }
  } catch (error) {
    const timeTaken = Date.now() - sharedStartTime
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
      timeTaken,
    }
  }
}

async function scrapeWithTinyFish(
  url: string,
  sharedStartTime: number
): Promise<ScrapeResult> {
  const apiKey = process.env.NEXT_PUBLIC_TINY_FISH_API_KEY

  if (!apiKey) {
    return {
      success: false,
      error: "TINY_FISH_API_KEY not configured",
      timeTaken: Date.now() - sharedStartTime,
    }
  }

  try {
    const response = await fetch("https://api.fetch.tinyfish.ai", {
      method: "POST",
      headers: {
        "X-API-Key": apiKey,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        urls: [url],
        format: "markdown",
      }),
    })

    const timeTaken = Date.now() - sharedStartTime
    const data = await response.json()

    if (!response.ok) {
      return {
        success: false,
        error: data.message || response.statusText,
        timeTaken,
      }
    }

    const result = data.results?.[0]

    if (!result || result.error) {
      return {
        success: false,
        error: result?.error || "No content returned",
        timeTaken,
      }
    }

    return {
      success: true,
      data: {
        markdown: result.text,
        html: result.html,
        plainText: result.text,
        metadata: {
          title: result.title,
          description: result.description,
          timeTaken,
        },
      },
      timeTaken,
    }
  } catch (error) {
    const timeTaken = Date.now() - sharedStartTime
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
      timeTaken,
    }
  }
}

function ResultCard({
  title,
  result,
  badgeColor,
}: {
  title: string
  result: ScrapeResult
  badgeColor: string
}) {
  return (
    <Card className="min-w-0 flex-1">
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-lg">{title}</CardTitle>
          <div className="flex items-center gap-2">
            {result.timeTaken && (
              <Badge variant="neutral" className="flex items-center gap-1">
                <Clock className="h-3 w-3" />
                {result.timeTaken}ms
              </Badge>
            )}
            {result.success ? (
              <CheckCircle className={`h-5 w-5 ${badgeColor}`} />
            ) : (
              <XCircle className="h-5 w-5 text-red-500" />
            )}
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {!result.success && !("Pending" === result.error) ? (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>{result.error}</AlertDescription>
          </Alert>
        ) : result.error === "Pending" ? (
          <div className="text-muted-foreground flex items-center justify-center py-8">
            <Loader2 className="mr-2 h-6 w-6 animate-spin" />
            <span>Waiting for result...</span>
          </div>
        ) : (
          <Tabs defaultValue="markdown">
            <TabsList className="mb-3">
              <TabsTrigger value="markdown">Markdown</TabsTrigger>
              <TabsTrigger value="preview">Preview</TabsTrigger>
            </TabsList>
            <TabsContent value="markdown">
              <div className="bg-muted max-h-[500px] overflow-auto rounded-md p-3">
                <pre className="font-mono text-xs break-words whitespace-pre-wrap">
                  {result.data?.markdown || "No content"}
                </pre>
              </div>
            </TabsContent>
            <TabsContent value="preview">
              <div className="prose prose-sm max-h-[500px] max-w-none overflow-auto rounded-md border p-4">
                <ReactMarkdown>
                  {result.data?.markdown || "No content"}
                </ReactMarkdown>
              </div>
            </TabsContent>
          </Tabs>
        )}
      </CardContent>
    </Card>
  )
}

export default function BattlePage() {
  const [url, setUrl] = useState("")
  const [isLoading, setIsLoading] = useState(false)
  const [results, setResults] = useState<BattleResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [renderJs, setRenderJs] = useState(false)

  const handleCompare = async () => {
    if (!url.trim()) {
      setError("Please enter a URL")
      return
    }

    let normalizedUrl = url.trim()
    if (
      !normalizedUrl.startsWith("http://") &&
      !normalizedUrl.startsWith("https://")
    ) {
      normalizedUrl = "https://" + normalizedUrl
    }

    setIsLoading(true)
    setError(null)
    setResults(null)

    try {
      // Clear any cached data to ensure fair comparison
      if (typeof window !== "undefined" && "caches" in window) {
        try {
          const cacheNames = await caches.keys()
          await Promise.all(cacheNames.map((name) => caches.delete(name)))
        } catch {
          // Caching APIs not available, continue
        }
      }

      // Start both at the exact same time
      const startTime = Date.now()

      const quickcrawlPromise = scrapeWithQuickCrawl(
        normalizedUrl,
        startTime,
        renderJs
      )
      const tinyfishPromise = scrapeWithTinyFish(normalizedUrl, startTime)

      // Update results as each one completes (independently)
      quickcrawlPromise.then((result) => {
        setResults((prev) =>
          prev
            ? { ...prev, quickcrawl: result }
            : {
                quickcrawl: result,
                tinyfish: { success: false, error: "Pending" },
              }
        )
      })

      tinyfishPromise.then((result) => {
        setResults((prev) =>
          prev
            ? { ...prev, tinyfish: result }
            : {
                quickcrawl: { success: false, error: "Pending" },
                tinyfish: result,
              }
        )
      })

      // Wait for both to complete before hiding loader
      await Promise.all([quickcrawlPromise, tinyfishPromise])
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to compare")
    } finally {
      setIsLoading(false)
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !isLoading) {
      handleCompare()
    }
  }

  return (
    <div className="min-h-screen bg-background">
      <div className="container mx-auto max-w-7xl px-4 py-8">
        <div className="mb-8">
          <h1 className="mb-2 text-3xl font-bold tracking-tight">Battle</h1>
          <p className="text-muted-foreground">
            Compare QuickCrawl and TinyFish scrape results side by side
          </p>
        </div>

        <Card className="mb-8">
          <CardContent className="pt-6">
            <div className="flex gap-3">
              <Input
                placeholder="Enter URL to compare (e.g., https://example.com)"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                onKeyDown={handleKeyDown}
                disabled={isLoading}
                className="flex-1"
              />
              <div className="flex items-center gap-2 border-l pl-3">
                <Switch
                  id="render-js"
                  checked={renderJs}
                  onCheckedChange={setRenderJs}
                  disabled={isLoading}
                />
                <Label htmlFor="render-js" className="cursor-pointer">
                  JS
                </Label>
              </div>
              <Button
                onClick={handleCompare}
                disabled={isLoading || !url.trim()}
              >
                {isLoading ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Comparing...
                  </>
                ) : (
                  <>
                    <Play className="mr-2 h-4 w-4" />
                    Compare
                  </>
                )}
              </Button>
            </div>
            {error && (
              <Alert variant="destructive" className="mt-3">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
          </CardContent>
        </Card>

        {results && (
          <div className="space-y-6">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <a
                  href={url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-2 text-lg font-medium hover:underline"
                >
                  {url}
                  <ExternalLink className="h-4 w-4" />
                </a>
              </div>
              <div className="flex gap-4 text-sm">
                <div className="flex items-center gap-2">
                  <div className="h-3 w-3 rounded-full bg-green-500" />
                  <span>QuickCrawl</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className="h-3 w-3 rounded-full bg-blue-500" />
                  <span>TinyFish</span>
                </div>
              </div>
            </div>

            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <ResultCard
                title="QuickCrawl"
                result={results.quickcrawl}
                badgeColor="text-green-500"
              />
              <ResultCard
                title="TinyFish"
                result={results.tinyfish}
                badgeColor="text-blue-500"
              />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
