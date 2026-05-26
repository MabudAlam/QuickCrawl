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
  Copy,
  LayoutDashboard,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Switch } from "@/components/ui/switch"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { NeumorphCombobox } from "@/components/ui/neumorph-combobox"
import ReactMarkdown from "react-markdown"

interface ScrapeResult {
  success: boolean
  data?: {
    markdown?: string
    html?: string
    plainText?: string
    imageLinks?: string[]
    rawHtml?: string
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
  renderJs: boolean = false,
  formatList: string[] = ["markdown"]
): Promise<ScrapeResult> {
  const baseUrl = getBaseUrl()

  try {
    const response = await fetchWithTimeout(`${baseUrl}/v1/scrape`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        url,
        formats: formatList,
        renderJs,
        onlyMain: true,
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
        imageLinks: data.data?.imageLinks,
        rawHtml: data.data?.rawHtml,
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
  sharedStartTime: number,
  formatList: string[] = ["markdown"]
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
    const stringResults: Record<string, string> = {}
    let imageLinks: string[] | undefined

    if (formatList.includes("markdown") && formatList.includes("imageLinks")) {
      const combinedResponse = await fetch("https://api.fetch.tinyfish.ai", {
        method: "POST",
        headers: {
          "X-API-Key": apiKey,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          urls: [url],
          format: "markdown",
          image_links: true,
        }),
      })

      const combinedData = await combinedResponse.json()
      const combinedResult = combinedData.results?.[0]
      if (combinedResult && !combinedResult.error) {
        stringResults.markdown = combinedResult.text
        imageLinks = combinedResult.image_links
      }
    } else if (formatList.includes("markdown")) {
      const mdResponse = await fetch("https://api.fetch.tinyfish.ai", {
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

      const mdData = await mdResponse.json()
      const mdResult = mdData.results?.[0]
      if (mdResult && !mdResult.error) {
        stringResults.markdown = mdResult.text
      }
    }

    if (formatList.includes("html")) {
      const htmlResponse = await fetch("https://api.fetch.tinyfish.ai", {
        method: "POST",
        headers: {
          "X-API-Key": apiKey,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          urls: [url],
          format: "html",
          image_links: formatList.includes("imageLinks"),
        }),
      })

      const htmlData = await htmlResponse.json()
      const htmlResult = htmlData.results?.[0]
      if (htmlResult && !htmlResult.error) {
        stringResults.html = htmlResult.text
        if (formatList.includes("imageLinks") && !imageLinks) {
          imageLinks = htmlResult.image_links
        }
      }
    }

    const timeTaken = Date.now() - sharedStartTime

    const firstResult = Object.values(stringResults)[0]
    if (!firstResult && !imageLinks) {
      return {
        success: false,
        error: "No content returned from TinyFish",
        timeTaken,
      }
    }

    return {
      success: true,
      data: {
        markdown: stringResults.markdown || stringResults.html,
        html: stringResults.html || stringResults.markdown,
        plainText: stringResults.markdown || stringResults.html,
        imageLinks,
        metadata: {
          title: undefined,
          description: undefined,
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
  showHtml,
  showImageLinks,
  showRawHtml,
}: {
  title: string
  result: ScrapeResult
  badgeColor: string
  showHtml?: boolean
  showImageLinks?: boolean
  showRawHtml?: boolean
}) {
  const [copied, setCopied] = useState<string | null>(null)

  const copyToClipboard = async (text: string, key: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(key)
      setTimeout(() => setCopied(null), 2000)
    } catch {
      // Clipboard API not available
    }
  }
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
          <Tabs defaultValue={showHtml ? "html" : showImageLinks ? "imageLinks" : "markdown"}>
<TabsList className="mb-3">
              {showImageLinks && <TabsTrigger value="imageLinks">Image Links</TabsTrigger>}
              {showHtml && <TabsTrigger value="html">HTML</TabsTrigger>}
              {showHtml && <TabsTrigger value="html-preview">HTML Preview</TabsTrigger>}
              <TabsTrigger value="markdown">Markdown</TabsTrigger>
              <TabsTrigger value="preview">Preview</TabsTrigger>
              {showRawHtml && <TabsTrigger value="rawHtml">Raw HTML</TabsTrigger>}
            </TabsList>
            {showImageLinks && (
              <TabsContent value="imageLinks">
                <div className="bg-muted max-h-[500px] overflow-auto rounded-md p-3">
                  {result.data?.imageLinks && result.data.imageLinks.length > 0 ? (
                    <ul className="space-y-2">
                      {result.data.imageLinks.map((link, i) => (
                        <li key={i} className="text-xs break-all">
                          <a href={link} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline">
                            {link}
                          </a>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="text-muted-foreground text-sm">No image links found</p>
                  )}
                </div>
              </TabsContent>
            )}
            {showHtml && (
              <>
                <TabsContent value="html">
                  <div className="bg-muted max-h-[500px] overflow-auto rounded-md p-3">
                    <div className="flex justify-end mb-1">
                      <Button
                        variant="noShadow"
                        size="sm"
                        className="h-7 text-xs"
                        onClick={() => copyToClipboard(result.data?.html || "", "html")}
                      >
                        <Copy className="h-3 w-3 mr-1" />
                        {copied === "html" ? "Copied!" : "Copy"}
                      </Button>
                    </div>
                    <pre className="font-mono text-xs break-words whitespace-pre-wrap">
                      {result.data?.html || "No HTML content"}
                    </pre>
                  </div>
                </TabsContent>
                <TabsContent value="html-preview">
                  <div className="rounded-md border overflow-hidden" style={{ height: "500px" }}>
                    <iframe
                      srcDoc={result.data?.html || ""}
                      className="w-full h-full"
                      sandbox="allow-scripts allow-same-origin"
                      title="HTML Preview"
                    />
                  </div>
                </TabsContent>
              </>
            )}
            <TabsContent value="markdown">
              <div className="bg-muted max-h-[500px] overflow-auto rounded-md p-3">
                <div className="flex justify-end mb-1">
                  <Button
                    variant="noShadow"
                    size="sm"
                    className="h-7 text-xs"
                    onClick={() => copyToClipboard(result.data?.markdown || "", "markdown")}
                  >
                    <Copy className="h-3 w-3 mr-1" />
                    {copied === "markdown" ? "Copied!" : "Copy"}
                  </Button>
                </div>
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
            {showRawHtml && (
              <TabsContent value="rawHtml">
                <div className="bg-muted max-h-[500px] overflow-auto rounded-md p-3">
                  <div className="flex justify-end mb-1">
                    <Button
                      variant="noShadow"
                      size="sm"
                      className="h-7 text-xs"
                      onClick={() => copyToClipboard(result.data?.rawHtml || "", "rawHtml")}
                    >
                      <Copy className="h-3 w-3 mr-1" />
                      {copied === "rawHtml" ? "Copied!" : "Copy"}
                    </Button>
                  </div>
                  <pre className="font-mono text-xs break-words whitespace-pre-wrap">
                    {result.data?.rawHtml || "No content"}
                  </pre>
                </div>
              </TabsContent>
            )}
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
  const [formats, setFormats] = useState<{ markdown: boolean; html: boolean; imageLinks: boolean; rawHtml: boolean }>({
    markdown: true,
    html: true,
    imageLinks: false,
    rawHtml: false,
  })

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
      const formatList = Object.entries(formats)
        .filter(([, enabled]) => enabled)
        .map(([name]) => name)

      const quickcrawlPromise = scrapeWithQuickCrawl(
        normalizedUrl,
        startTime,
        renderJs,
        formatList
      )
      const tinyfishPromise = scrapeWithTinyFish(normalizedUrl, startTime, formatList)

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
          <div className="flex items-center justify-between mb-2">
            <h1 className="text-3xl font-bold tracking-tight">Battle</h1>
            <div className="flex items-center gap-3">
              <a
                href="https://github.com/MabudAlam/quickcrawl"
                target="_blank"
                rel="noopener noreferrer"
              >
                <Button variant="noShadow">
                  <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
                  </svg>
                  GitHub
                </Button>
              </a>
              <a
                href="/playground"
              >
                <Button variant="noShadow">
                  <LayoutDashboard className="h-4 w-4" />
                  Playground
                </Button>
              </a>
            </div>
          </div>
          <p className="text-muted-foreground">
            Compare QuickCrawl and TinyFish scrape results side by side
          </p>
        </div>

          <Card className="mb-8">
          <CardContent className="pt-6">
            <div className="flex flex-col gap-4">
              <div className="flex flex-col sm:flex-row gap-3">
                <Input
                  placeholder="Enter URL to compare (e.g., https://example.com)"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  onKeyDown={handleKeyDown}
                  disabled={isLoading}
                  className="flex-1 min-w-0"
                />
<NeumorphCombobox
                  value={url}
                  onValueChange={(value) => setUrl(value)}
                  placeholder="Examples"
                  className="w-full sm:w-[180px]"
                  disabled={isLoading}
                  options={[
                    { value: "https://www.xda-developers.com/a-self-hosted-llms-is-way-more-powerful-than-a-chat-interface/", label: "XDA Developers" },
                    { value: "https://news.ycombinator.com/item?id=44741682", label: "Hacker News" },
                    { value: "https://substack.com/home/post/p-193786550", label: "Substack" },
                    { value: "https://www.jamesgPT.com/artificial-intelligence", label: "Blog Post" },
                    { value: "https://github.com/MabudAlam/quickcrawl", label: "GitHub" },
                  ]}
                />
                <Button
                  onClick={handleCompare}
                  disabled={isLoading || !url.trim()}
                  className="w-full sm:w-auto"
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

              <div className="flex flex-col sm:flex-row sm:items-center gap-3 sm:gap-6 flex-wrap">
                <div className="flex items-center gap-2">
                  <Switch
                    id="render-js"
                    checked={renderJs}
                    onCheckedChange={setRenderJs}
                    disabled={isLoading}
                  />
                  <Label htmlFor="render-js" className="cursor-pointer text-sm">
                    JS
                  </Label>
                </div>
                <div className="flex items-center gap-2">
                  <Switch
                    id="format-markdown"
                    checked={formats.markdown}
                    onCheckedChange={(checked) =>
                      setFormats((f) => ({ ...f, markdown: checked }))
                    }
                    disabled={isLoading}
                  />
                  <Label htmlFor="format-markdown" className="cursor-pointer text-sm">
                    Markdown
                  </Label>
                </div>
                <div className="flex items-center gap-2">
                  <Switch
                    id="format-html"
                    checked={formats.html}
                    onCheckedChange={(checked) =>
                      setFormats((f) => ({ ...f, html: checked }))
                    }
                    disabled={isLoading}
                  />
                  <Label htmlFor="format-html" className="cursor-pointer text-sm">
                    HTML
                  </Label>
                </div>
                <div className="flex items-center gap-2">
                  <Switch
                    id="format-imageLinks"
                    checked={formats.imageLinks}
                    onCheckedChange={(checked) =>
                      setFormats((f) => ({ ...f, imageLinks: checked }))
                    }
                    disabled={isLoading}
                  />
                  <Label htmlFor="format-imageLinks" className="cursor-pointer text-sm">
                    Image Links
                  </Label>
                </div>
                <div className="flex items-center gap-2">
                  <Switch
                    id="format-rawHtml"
                    checked={formats.rawHtml}
                    onCheckedChange={(checked) =>
                      setFormats((f) => ({ ...f, rawHtml: checked }))
                    }
                    disabled={isLoading}
                  />
                  <Label htmlFor="format-rawHtml" className="cursor-pointer text-sm">
                    Raw HTML
                  </Label>
                </div>
              </div>
            </div>
            {error && (
              <Alert variant="destructive" className="mt-4">
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
                showHtml={formats.html}
                showImageLinks={formats.imageLinks}
                showRawHtml={formats.rawHtml}
              />
              <ResultCard
                title="TinyFish"
                result={results.tinyfish}
                badgeColor="text-blue-500"
                showHtml={formats.html}
                showImageLinks={formats.imageLinks}
              />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
