"use client"

import { useState, useEffect } from "react"
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
  ChevronDown,
  SunIcon,
  MoonIcon,
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioItem,
  DropdownMenuRadioGroup,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import ReactMarkdown from "react-markdown"
import { useTheme } from "next-themes"

interface ScrapeResult {
  success: boolean
  data?: {
    markdown?: string
    html?: string
    plainText?: string
    links?: string[]
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
  markdownTimeTaken?: number
  htmlTimeTaken?: number
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
  renderJs: boolean | null = false,
  ttl: number | undefined = undefined,
  formatList: string[] = ["markdown"]
): Promise<ScrapeResult> {
  const baseUrl = getBaseUrl()

  try {
    const requestBody: Record<string, unknown> = {
      url,
      formats: formatList,
    }
    if (renderJs !== null) {
      requestBody.renderJs = renderJs
    }
    if (ttl !== undefined) {
      requestBody.ttl = ttl
    }

    const response = await fetchWithTimeout(`${baseUrl}/v1/scrape`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(requestBody),
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
        links: data.data?.links,
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
    let markdown: string | undefined
    let html: string | undefined
    let links: string[] | undefined
    let imageLinks: string[] | undefined
    let markdownTimeTaken: number | undefined
    let htmlTimeTaken: number | undefined

    const includeLinks = formatList.includes("links")
    const includeImageLinks = formatList.includes("imageLinks")
    const includeMarkdown = formatList.includes("markdown")
    const includePlainText = formatList.includes("plainText")
    const includeHtml = formatList.includes("html")

    const fetchOptions = {
      method: "POST",
      headers: {
        "X-API-Key": apiKey,
        "Content-Type": "application/json",
      },
    }

    if (includeMarkdown || includePlainText) {
      const format = includeMarkdown ? "markdown" : "html"
      const mdStartTime = Date.now()
      const mdResponse = await fetch("https://api.fetch.tinyfish.ai", {
        ...fetchOptions,
        body: JSON.stringify({
          urls: [url],
          format,
          links: includeLinks,
          image_links: includeImageLinks,
        }),
      })
      markdownTimeTaken = Date.now() - mdStartTime

      const mdData = await mdResponse.json()
      const mdResult = mdData.results?.[0]
      if (mdResult && !mdResult.error) {
        if (includeMarkdown) markdown = mdResult.text
        if (includePlainText) html = mdResult.text
        if (includeLinks && mdResult.links) links = mdResult.links
        if (includeImageLinks && mdResult.image_links) imageLinks = mdResult.image_links
      }
    }

    if (includeHtml) {
      const htmlStartTime = Date.now()
      const htmlResponse = await fetch("https://api.fetch.tinyfish.ai", {
        ...fetchOptions,
        body: JSON.stringify({
          urls: [url],
          format: "html",
          links: includeLinks,
          image_links: includeImageLinks,
        }),
      })
      htmlTimeTaken = Date.now() - htmlStartTime

      const htmlData = await htmlResponse.json()
      const htmlResult = htmlData.results?.[0]
      if (htmlResult && !htmlResult.error) {
        html = htmlResult.text
        if (includeLinks && htmlResult.links) links = htmlResult.links
        if (includeImageLinks && htmlResult.image_links) imageLinks = htmlResult.image_links
      }
    }

    const timeTaken = Date.now() - sharedStartTime

    if (!markdown && !html && !links && !imageLinks) {
      return {
        success: false,
        error: "No content returned from TinyFish",
        timeTaken,
      }
    }

    return {
      success: true,
      data: {
        markdown,
        plainText: markdown || html,
        html,
        links,
        imageLinks,
        metadata: {
          title: undefined,
          description: undefined,
          timeTaken,
        },
      },
      timeTaken,
      markdownTimeTaken,
      htmlTimeTaken,
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
  showPlainText,
  showLinks,
  showImageLinks,
  showRawHtml,
  theme,
}: {
  title: string
  result: ScrapeResult
  badgeColor: string
  showHtml?: boolean
  showPlainText?: boolean
  showLinks?: boolean
  showImageLinks?: boolean
  showRawHtml?: boolean
  theme?: "light" | "dark"
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
            {result.markdownTimeTaken !== undefined && (
              <Badge variant="neutral" className="flex items-center gap-1">
                <Clock className="h-3 w-3" />
                MD: {result.markdownTimeTaken}ms
              </Badge>
            )}
            {result.htmlTimeTaken !== undefined && (
              <Badge variant="neutral" className="flex items-center gap-1">
                <Clock className="h-3 w-3" />
                HTML: {result.htmlTimeTaken}ms
              </Badge>
            )}
            {result.timeTaken && result.markdownTimeTaken === undefined && result.htmlTimeTaken === undefined && (
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
<TabsList className="mb-3 flex flex-wrap">
              {showImageLinks && <TabsTrigger value="imageGrid">Images</TabsTrigger>}
              {showImageLinks && <TabsTrigger value="imageLinks">URLs</TabsTrigger>}
              {showHtml && <TabsTrigger value="html">HTML</TabsTrigger>}
              {showHtml && <TabsTrigger value="html-preview">Preview</TabsTrigger>}
              <TabsTrigger value="markdown">Markdown</TabsTrigger>
              {showPlainText && <TabsTrigger value="plainText">Plain Text</TabsTrigger>}
              {showLinks && <TabsTrigger value="links">Links</TabsTrigger>}
              {showRawHtml && <TabsTrigger value="rawHtml">Raw HTML</TabsTrigger>}
            </TabsList>
            {showImageLinks && (
              <TabsContent value="imageGrid">
                <div className="bg-secondary-background max-h-[500px] overflow-auto rounded-md p-3">
                  {result.data?.imageLinks && result.data.imageLinks.length > 0 ? (
                    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3">
                      {result.data.imageLinks.map((link, i) => (
                        <div key={i} className="group relative aspect-square bg-background rounded-lg overflow-hidden border border-border">
                          <img
                            src={link}
                            alt={`Image ${i + 1}`}
                            className="w-full h-full object-cover object-center"
                            loading="lazy"
                            onError={(e) => {
                              const img = e.target as HTMLImageElement
                              img.style.display = 'none'
                              const errorDiv = img.nextElementSibling as HTMLElement | null
                              if (errorDiv) errorDiv.classList.remove('hidden')
                            }}
                          />
                          <div className="hidden absolute inset-0 flex items-center justify-center bg-muted text-muted-foreground text-xs p-2 text-center">
                            Failed to load
                          </div>
                          <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                            <a
                              href={link}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="p-2 bg-background rounded-full hover:bg-background/80"
                              onClick={(e) => e.stopPropagation()}
                            >
                              <ExternalLink className="w-5 h-5" />
                            </a>
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-muted-foreground text-sm">No image links found</p>
                  )}
                </div>
              </TabsContent>
            )}
            {showImageLinks && (
              <TabsContent value="imageLinks">
                <div className="bg-secondary-background max-h-[500px] overflow-auto rounded-md p-3">
                  {result.data?.imageLinks && result.data.imageLinks.length > 0 ? (
                    <ul className="space-y-2">
                      {result.data.imageLinks.map((link, i) => (
                        <li key={i} className="text-xs break-all">
                          <a href={link} target="_blank" rel="noopener noreferrer" className="text-main hover:text-main/80">
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
              <TabsContent value="html">
                <div className="bg-secondary-background max-h-[500px] overflow-auto rounded-md p-3">
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
                  <pre className="font-mono text-xs break-words whitespace-pre-wrap text-foreground">
                    {result.data?.html || "No HTML content"}
                  </pre>
                </div>
              </TabsContent>
            )}
            {showHtml && (
              <TabsContent value="html-preview">
                <div className="rounded-md border overflow-hidden bg-background" style={{ height: "500px" }}>
                  <iframe
                    srcDoc={`<!DOCTYPE html>
<html>
<head>
  <style>
    :root { color-scheme: light dark; }
    body.dark { background-color: #1a1a1a; color: #f5f5f5; }
    body.dark a { color: #8b9fd9; }
    body.dark pre, body.dark code { background-color: #2a2a2a; color: #f5f5f5; }
    body.light { background-color: #ffffff; color: #000000; }
    body.light a { color: #0066cc; }
    body.light pre, body.light code { background-color: #f5f5f5; color: #000000; }
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; padding: 12px; margin: 0; line-height: 1.5; }
    img { max-width: 100%; height: auto; }
    pre { overflow-x: auto; padding: 8px; border-radius: 4px; }
    code { padding: 2px 4px; border-radius: 3px; }
  </style>
</head>
<body class="${theme || 'light'}">${result.data?.html || "<p>No HTML content</p>"}</body>
</html>`}
                    className="w-full h-full bg-background"
                    sandbox="allow-scripts allow-same-origin"
                    title="HTML Preview"
                  />
                </div>
              </TabsContent>
            )}
            <TabsContent value="markdown">
              <div className="bg-secondary-background max-h-[500px] overflow-auto rounded-md p-3">
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
                <pre className="font-mono text-xs break-words whitespace-pre-wrap text-foreground">
                  {result.data?.markdown || "No content"}
                </pre>
              </div>
            </TabsContent>
            {showPlainText && (
              <TabsContent value="plainText">
                <div className="bg-secondary-background max-h-[500px] overflow-auto rounded-md p-3">
                  <div className="flex justify-end mb-1">
                    <Button
                      variant="noShadow"
                      size="sm"
                      className="h-7 text-xs"
                      onClick={() => copyToClipboard(result.data?.plainText || "", "plainText")}
                    >
                      <Copy className="h-3 w-3 mr-1" />
                      {copied === "plainText" ? "Copied!" : "Copy"}
                    </Button>
                  </div>
                  <pre className="font-mono text-xs break-words whitespace-pre-wrap text-foreground">
                    {result.data?.plainText || "No content"}
                  </pre>
                </div>
              </TabsContent>
            )}
            {showLinks && (
              <TabsContent value="links">
                <div className="bg-secondary-background max-h-[500px] overflow-auto rounded-md p-3">
                  {result.data?.links && result.data.links.length > 0 ? (
                    <ul className="space-y-2">
                      {result.data.links.map((link, i) => (
                        <li key={i} className="text-xs break-all">
                          <a href={link} target="_blank" rel="noopener noreferrer" className="text-main hover:text-main/80">
                            {link}
                          </a>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="text-muted-foreground text-sm">No links found</p>
                  )}
                </div>
              </TabsContent>
            )}
            {showRawHtml && (
              <TabsContent value="rawHtml">
                <div className="bg-secondary-background max-h-[500px] overflow-auto rounded-md p-3">
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
                  <pre className="font-mono text-xs break-words whitespace-pre-wrap text-foreground">
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
  const [renderJs, setRenderJs] = useState<boolean | null>(null)
  const [ttl, setTtl] = useState<number | undefined>(undefined)
  const [formats, setFormats] = useState<{ markdown: boolean; html: boolean; plainText: boolean; links: boolean; imageLinks: boolean; rawHtml: boolean }>({
    markdown: true,
    html: true,
    plainText: false,
    links: false,
    imageLinks: false,
    rawHtml: false,
  })
  const { resolvedTheme, setTheme } = useTheme()
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    setMounted(true)
  }, [])

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
        ttl,
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
              {mounted && (
                <Button
                  variant="noShadow"
                  onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
                >
                  {resolvedTheme === "dark" ? (
                    <SunIcon className="h-4 w-4" />
                  ) : (
                    <MoonIcon className="h-4 w-4" />
                  )}
                </Button>
              )}
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
                  <Label className="text-sm">Renderer</Label>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="default" size="sm" className="h-8 w-[140px] justify-start">
                        {renderJs === null ? "Auto" : renderJs ? "Browser" : "HTTP"}
                        <ChevronDown className="ml-auto h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="start">
                      <DropdownMenuRadioGroup
                        value={renderJs === null ? "auto" : renderJs ? "browser" : "http"}
                        onValueChange={(v) => setRenderJs(v === "auto" ? null : v === "browser")}
                      >
                        <DropdownMenuRadioItem value="auto">Auto</DropdownMenuRadioItem>
                        <DropdownMenuRadioItem value="http">HTTP</DropdownMenuRadioItem>
                        <DropdownMenuRadioItem value="browser">Browser</DropdownMenuRadioItem>
                      </DropdownMenuRadioGroup>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
                <div className="flex items-center gap-2">
                  <Label className="text-sm">Cache TTL</Label>
                  <Input
                    type="number"
                    placeholder="3600"
                    value={ttl ?? ""}
                    onChange={(e) => setTtl(e.target.value ? parseInt(e.target.value) : undefined)}
                    className="h-8 w-[100px]"
                    min={0}
                  />
                  <span className="text-xs text-gray-500">seconds</span>
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
                    id="format-plainText"
                    checked={formats.plainText}
                    onCheckedChange={(checked) =>
                      setFormats((f) => ({ ...f, plainText: checked }))
                    }
                    disabled={isLoading}
                  />
                  <Label htmlFor="format-plainText" className="cursor-pointer text-sm">
                    Plain Text
                  </Label>
                </div>
                <div className="flex items-center gap-2">
                  <Switch
                    id="format-links"
                    checked={formats.links}
                    onCheckedChange={(checked) =>
                      setFormats((f) => ({ ...f, links: checked }))
                    }
                    disabled={isLoading}
                  />
                  <Label htmlFor="format-links" className="cursor-pointer text-sm">
                    Links
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
                showPlainText={formats.plainText}
                showLinks={formats.links}
                showImageLinks={formats.imageLinks}
                showRawHtml={formats.rawHtml}
                theme={mounted ? (resolvedTheme === "dark" ? "dark" : "light") : "light"}
              />
              <ResultCard
                title="TinyFish"
                result={results.tinyfish}
                badgeColor="text-blue-500"
                showHtml={formats.html}
                showPlainText={formats.plainText}
                showLinks={formats.links}
                showImageLinks={formats.imageLinks}
                showRawHtml={formats.rawHtml}
                theme={mounted ? (resolvedTheme === "dark" ? "dark" : "light") : "light"}
              />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
