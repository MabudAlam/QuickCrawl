"use client"

import { useState, useCallback, useEffect } from "react"
import {
  Loader2,
  Play,
  Copy,
  Check,
  X,
  FileCode,
  ChevronDown,
  ChevronUp,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Slider } from "@/components/ui/slider"
import { Checkbox } from "@/components/ui/checkbox"
import { Badge } from "@/components/ui/badge"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from "@/components/ui/dropdown-menu"
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { AppSidebar } from "./sidebar"
import {
  scrape,
  startCrawl,
  getCrawlStatus,
  cancelCrawl,
  map,
  search,
  checkHealth,
  generateCurlCommand,
  generateFetchCode,
  generatePythonCode,
} from "@/lib/api-client"
import type {
  Endpoint,
  ScrapeRequest,
  CrawlRequest,
  MapRequest,
  SearchRequest,
  SearchTimeRange,
  APIResponse,
  CrawlState,
  HealthResponse,
  Format,
  RenderMode,
  ScrapeData,
  MapResponse,
  SearchResponse,
  ScrapeOptions,
  CrawlOptions,
} from "@/lib/api-types"
import {
  ResponseViewer,
  MapResponseViewer,
  SearchResponseViewer,
} from "@/components/response-viewer"

interface PlaygroundPageProps {
  initialBaseUrl?: string
}

const nowMs = () => performance.now()

export default function PlaygroundPage({
  initialBaseUrl,
}: PlaygroundPageProps) {
  const [endpoint, setEndpoint] = useState<Endpoint>("scrape")
  const [url, setUrl] = useState("")
  const [baseUrl] = useState(initialBaseUrl)

  const [isLoading, setIsLoading] = useState(false)
  const [response, setResponse] = useState<APIResponse<unknown> | null>(null)
  const [crawlId, setCrawlId] = useState<string | null>(null)
  const [crawlStatus, setCrawlStatus] = useState<CrawlState | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [health, setHealth] = useState<HealthResponse | null>(null)
  const [copiedSnippet, setCopiedSnippet] = useState<string | null>(null)
  const [timeTakenMs, setTimeTakenMs] = useState<number | null>(null)
  const [codeLanguage, setCodeLanguage] = useState<"curl" | "fetch" | "python">(
    "curl"
  )
  const [codeSheetOpen, setCodeSheetOpen] = useState(false)
  const [advancedExpanded, setAdvancedExpanded] = useState(false)
  const [schemaBuilderOpen, setSchemaBuilderOpen] = useState(false)
  const [schemaFields, setSchemaFields] = useState<
    { name: string; type: string; description: string; itemType?: string }[]
  >([{ name: "title", type: "string", description: "" }])

  const [scrapeOptions, setScrapeOptions] = useState<ScrapeOptions>({
    formats: ["markdown"] as Format[],
    renderMode: "auto",
    waitFor: 0,
    headers: "",
    cssSelector: "",
    includeTags: "",
    excludeTags: "",
    jsonSchema: "",
    extractionPrompt: "",
    extractionResponseFormat: "",
    maxMarkdownChars: undefined,
    ttl: undefined,
  })

  const generateSchema = () => {
    const properties: Record<
      string,
      { type: string; description?: string; items?: { type: string } }
    > = {}
    const required: string[] = []

    schemaFields.forEach((field) => {
      if (field.name.trim()) {
        if (field.type === "array") {
          properties[field.name] = {
            type: "array",
            items: { type: field.itemType || "string" },
            ...(field.description ? { description: field.description } : {}),
          }
        } else {
          properties[field.name] = {
            type: field.type,
            ...(field.description ? { description: field.description } : {}),
          }
        }
        required.push(field.name)
      }
    })

    return JSON.stringify(
      {
        type: "object",
        properties,
        required,
        additionalProperties: false,
      },
      null,
      2
    )
  }

  const [crawlOptions, setCrawlOptions] = useState<CrawlOptions>({
    maxDepth: 2,
    maxPages: 100,
    formats: ["markdown"] as Format[],
    renderMode: "auto",
    waitFor: 0,
    includeTags: "",
    excludeTags: "",
    maxMarkdownChars: undefined,
  })

  const [mapOptions, setMapOptions] = useState({
    maxDepth: 2,
    useSitemap: true,
    timeout: 30000,
  })

  const [searchQuery, setSearchQuery] = useState("")
  const [searchRegion, setSearchRegion] = useState("us-en")
  const [searchTimeRange, setSearchTimeRange] = useState<SearchTimeRange>("")
  const [searchFormats, setSearchFormats] = useState<Format[]>(["markdown"])
  const [searchRenderMode, setSearchRenderMode] = useState<RenderMode>("auto")
  const [searchScrape, setSearchScrape] = useState(false)
  const [searchUseBM25, setSearchUseBM25] = useState(false)
  const [searchPage, setSearchPage] = useState(1)

  useEffect(() => {
    const fetchHealth = async () => {
      try {
        const h = await checkHealth(baseUrl)
        setHealth(h)
        setError(null)
      } catch {
        setHealth(null)
        setError("Cannot connect to server")
      }
    }
    fetchHealth()
    const interval = setInterval(fetchHealth, 10000)
    return () => clearInterval(interval)
  }, [baseUrl])

  const handleFormatChange = useCallback(
    (type: "scrape" | "crawl", format: Format, checked: boolean) => {
      if (type === "scrape") {
        setScrapeOptions((prev) => ({
          ...prev,
          formats: checked
            ? [...prev.formats, format]
            : prev.formats.filter((f) => f !== format),
        }))
      } else {
        setCrawlOptions((prev) => ({
          ...prev,
          formats: checked
            ? [...prev.formats, format]
            : prev.formats.filter((f) => f !== format),
        }))
      }
    },
    []
  )

  const buildRequest = useCallback(() => {
    if (!url && endpoint !== "search") return null
    switch (endpoint) {
      case "scrape": {
        let extract: ScrapeRequest["extract"] = undefined
        if (
          scrapeOptions.formats.includes("json") &&
          scrapeOptions.jsonSchema.trim()
        ) {
          try {
            extract = {
              schema: JSON.parse(scrapeOptions.jsonSchema),
              prompt: scrapeOptions.extractionPrompt.trim() || undefined,
              responseFormat:
                scrapeOptions.extractionResponseFormat.trim() || undefined,
            }
          } catch {}
        }
        return {
          url,
          formats: scrapeOptions.formats.length
            ? scrapeOptions.formats
            : ["markdown"],
          renderMode: scrapeOptions.renderMode,
          waitFor: scrapeOptions.waitFor || undefined,
          headers: scrapeOptions.headers || undefined,
          cssSelector: scrapeOptions.cssSelector || undefined,
          includeTags: scrapeOptions.includeTags
            ? scrapeOptions.includeTags.split(",").map((s) => s.trim())
            : undefined,
          excludeTags: scrapeOptions.excludeTags
            ? scrapeOptions.excludeTags.split(",").map((s) => s.trim())
            : undefined,
          extract,
          maxMarkdownChars: scrapeOptions.maxMarkdownChars || undefined,
          ttl: scrapeOptions.ttl,
        } as ScrapeRequest
      }
      case "crawl": {
        return {
          url,
          maxDepth: crawlOptions.maxDepth,
          maxPages: crawlOptions.maxPages,
          formats: crawlOptions.formats.length
            ? crawlOptions.formats
            : ["markdown"],
          renderMode: crawlOptions.renderMode,
          waitFor: crawlOptions.waitFor || undefined,
          maxMarkdownChars: crawlOptions.maxMarkdownChars || undefined,
        } as CrawlRequest
      }
      case "map":
        return {
          url,
          maxDepth: mapOptions.maxDepth,
          useSitemap: mapOptions.useSitemap,
          timeout: mapOptions.timeout,
        } as MapRequest
      case "search":
        return {
          query: searchQuery,
          region: searchRegion,
          ...(searchTimeRange && { timeRange: searchTimeRange }),
          renderMode: searchRenderMode,
          formats: searchFormats,
          scrape: searchScrape,
        } as SearchRequest
    }
  }, [
    endpoint,
    url,
    scrapeOptions,
    crawlOptions,
    mapOptions,
    searchQuery,
    searchRegion,
    searchTimeRange,
    searchRenderMode,
    searchFormats,
    searchScrape,
  ])

  const handleSubmit = async () => {
    const request = buildRequest()
    if (!request) {
      setError("Please enter a URL")
      return
    }

    setIsLoading(true)
    setError(null)
    setResponse(null)
    setCrawlStatus(null)
    const startTime = nowMs()

    try {
      switch (endpoint) {
        case "scrape": {
          const res = await scrape(request as ScrapeRequest)
          setTimeTakenMs(Math.round(nowMs() - startTime))
          setResponse(res)
          if (!res.success && res.error) {
            setError(res.error)
          }
          break
        }
        case "crawl": {
          const res = await startCrawl(request as CrawlRequest)
          const crawlId =
            (res as { success: boolean; id?: string; data?: { id?: string } })
              .id || res.data?.id
          if (res.success && crawlId) {
            setCrawlId(crawlId)
            pollCrawlStatus(crawlId)
          } else {
            setResponse(res)
            if (res.error) setError(res.error)
          }
          break
        }
        case "map": {
          const res = await map(request as MapRequest)
          setTimeTakenMs(Math.round(nowMs() - startTime))
          setResponse(res)
          if (!res.success && res.error) {
            setError(res.error)
          }
          break
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "An error occurred")
    } finally {
      setIsLoading(false)
    }
  }

  const pollCrawlStatus = async (id: string) => {
    const poll = async () => {
      try {
        const status = await getCrawlStatus(id)
        setCrawlStatus(status)
        if (status.status === "completed" || status.status === "failed") {
          if (status.status === "failed") {
            setError(status.error || "Crawl failed")
          }
          return
        }
        setTimeout(poll, 2000)
      } catch {
        setError("Failed to get crawl status")
      }
    }
    poll()
  }

  const handleCancelCrawl = async () => {
    if (crawlId) {
      await cancelCrawl(crawlId)
      setCrawlStatus((prev) =>
        prev ? { ...prev, status: "failed", error: "Cancelled" } : null
      )
    }
  }

  const handleSearchSubmit = async () => {
    if (!searchQuery) return

    setIsLoading(true)
    setError(null)
    setResponse(null)

    try {
      const request: SearchRequest = {
        query: searchQuery,
        region: searchRegion,
        ...(searchTimeRange && { timeRange: searchTimeRange }),
        page: searchPage,
        use_bm25: searchUseBM25,
        renderMode: searchRenderMode,
        formats: searchFormats,
        scrape: searchScrape,
      }

      const res = await search(request)
      setResponse(res)
      if (!res.success && res.error) {
        setError(res.error)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "An error occurred")
    } finally {
      setIsLoading(false)
    }
  }

  const copySnippet = (code: string, type: string) => {
    navigator.clipboard.writeText(code)
    setCopiedSnippet(type)
    setTimeout(() => setCopiedSnippet(null), 2000)
  }

  const getCodeSnippet = () => {
    const request = buildRequest()
    if (!request) return ""
    switch (codeLanguage) {
      case "curl":
        return generateCurlCommand(endpoint, request, baseUrl)
      case "fetch":
        return generateFetchCode(endpoint, request, baseUrl)
      case "python":
        return generatePythonCode(endpoint, request, baseUrl)
    }
  }

  const clearAll = () => {
    setResponse(null)
    setCrawlStatus(null)
    setError(null)
    setCrawlId(null)
    setUrl("")
    setSearchQuery("")
    setSearchRegion("us-en")
    setSearchTimeRange("")
    setSearchFormats(["markdown"])
    setSearchRenderMode("auto")
    setSearchScrape(false)
    setSearchUseBM25(false)
    setSearchPage(1)
    setAdvancedExpanded(false)
    setSchemaBuilderOpen(false)
    setSchemaFields([{ name: "title", type: "string", description: "" }])
    setScrapeOptions({
      formats: ["markdown"] as Format[],
      renderMode: "auto",
      waitFor: 0,
      headers: "",
      cssSelector: "",
      includeTags: "",
      excludeTags: "",
      jsonSchema: "",
      extractionPrompt: "",
      extractionResponseFormat: "",
      maxMarkdownChars: undefined,
      ttl: undefined,
    })
    setCrawlOptions({
      maxDepth: 2,
      maxPages: 100,
      formats: ["markdown"] as Format[],
      renderMode: "auto",
      waitFor: 0,
      includeTags: "",
      excludeTags: "",
      maxMarkdownChars: undefined,
    })
    setMapOptions({
      maxDepth: 2,
      useSitemap: true,
      timeout: 30000,
    })
  }

  const handleEndpointChange = (newEndpoint: Endpoint) => {
    setEndpoint(newEndpoint)
    clearAll()
  }

  const renderResponse = () => {
    if (crawlStatus) {
      const progressPercent =
        crawlStatus.total > 0
          ? Math.round((crawlStatus.completed / crawlStatus.total) * 100)
          : 0

      return (
        <div className="flex h-full min-h-0 flex-col gap-4 overflow-hidden">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-3">
              {crawlStatus.status === "scraping" && (
                <Loader2 className="h-5 w-5 animate-spin text-main" />
              )}
              <Badge
                variant={
                  crawlStatus.status === "completed"
                    ? "default"
                    : crawlStatus.status === "failed"
                      ? "neutral"
                      : "neutral"
                }
              >
                {crawlStatus.status === "scraping"
                  ? "SCRAPING"
                  : crawlStatus.status.toUpperCase()}
              </Badge>
              <span className="text-sm text-gray-500">
                {crawlStatus.completed} / {crawlStatus.total} pages (
                {progressPercent}%)
              </span>
            </div>
            <div className="flex gap-2">
              {crawlStatus.status === "scraping" && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="neutral"
                      size="sm"
                      onClick={handleCancelCrawl}
                    >
                      Cancel
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="top">
                    Stop the current crawl job
                  </TooltipContent>
                </Tooltip>
              )}
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="neutral" size="sm" onClick={clearAll}>
                    Clear
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="top">
                  Clear response and reset form
                </TooltipContent>
              </Tooltip>
            </div>
          </div>

          {crawlStatus.status === "scraping" && (
            <div className="space-y-2">
              <div className="flex justify-between text-xs text-gray-400">
                <span>Progress</span>
                <span>
                  {crawlStatus.completed} of {crawlStatus.total}
                </span>
              </div>
              <div className="h-2 overflow-hidden rounded-full border border-border bg-secondary-background">
                <div
                  className="h-full bg-main transition-all duration-500 ease-out"
                  style={{ width: `${progressPercent}%` }}
                />
              </div>
              <div className="flex items-center justify-between text-xs text-gray-400">
                <span>
                  Job ID:{" "}
                  <code className="rounded bg-gray-100 px-1">
                    {crawlStatus.id}
                  </code>
                </span>
                <span>Polling every 2s...</span>
              </div>
            </div>
          )}

          {crawlStatus.error && (
            <div className="rounded-base border-2 border-red-200 bg-red-50 p-4 text-sm font-medium text-red-700">
              {crawlStatus.error}
            </div>
          )}

          {(crawlStatus.status === "completed" ||
            crawlStatus.status === "scraping") &&
            crawlStatus.data &&
            crawlStatus.data.length > 0 && (
              <div className="min-h-0 flex-1">
                <ResponseViewer
                  data={crawlStatus.data as ScrapeData[]}
                  rawResponse={crawlStatus}
                />
              </div>
            )}
        </div>
      )
    }

    if (!response) return null

    const isMapResponse =
      endpoint === "map" &&
      response.success &&
      (response.data as MapResponse)?.links !== undefined
    const isSearchResponse =
      response.success &&
      (response.data as SearchResponse)?.results !== undefined

    return (
      <div className="flex h-full min-h-0 flex-col gap-4 overflow-hidden">
        {error && (
          <div className="shrink-0 rounded-base border-2 border-red-200 bg-red-50 p-4 text-sm font-medium text-red-700">
            {error}
          </div>
        )}
        {isMapResponse ? (
          <div className="min-h-0 flex-1">
            <MapResponseViewer
              data={response.data as MapResponse}
              rawResponse={response}
              timeTakenMs={timeTakenMs}
            />
          </div>
        ) : isSearchResponse ? (
          <div className="min-h-0 flex-1">
            <SearchResponseViewer
              data={response.data as SearchResponse}
              rawResponse={response}
              timeTakenMs={timeTakenMs}
            />
          </div>
        ) : response.success && response.data ? (
          <div className="min-h-0 flex-1">
            <ResponseViewer
              data={response.data as ScrapeData}
              rawResponse={response}
              timeTakenMs={timeTakenMs}
              openRenderedInSheet
            />
          </div>
        ) : (
          <pre className="min-h-0 flex-1 overflow-auto rounded-base border-2 border-border bg-white p-4 font-mono text-sm">
            {JSON.stringify(response, null, 2)}
          </pre>
        )}
      </div>
    )
  }

  const renderScrapeForm = () => (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Output Formats</Label>
        <div className="flex flex-wrap gap-3">
          {(
            [
              "markdown",
              "html",
              "rawHtml",
              "plainText",
              "links",
              "json",
              "imageLinks",
            ] as Format[]
          ).map((format) => (
            <div key={format} className="flex items-center gap-2">
              <Checkbox
                id={`scrape-${format}`}
                checked={scrapeOptions.formats.includes(format)}
                onCheckedChange={(checked) =>
                  handleFormatChange("scrape", format, checked as boolean)
                }
              />
              <Label
                htmlFor={`scrape-${format}`}
                className="text-sm font-normal"
              >
                {format}
              </Label>
            </div>
          ))}
        </div>
      </div>

      {scrapeOptions.formats.includes("json") && (
        <div className="bg-muted space-y-3 rounded-lg p-3">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium">
              JSON Extraction (requires OpenAI API key)
            </p>
            <Button
              variant="noShadow"
              size="sm"
              onClick={() => setSchemaBuilderOpen(!schemaBuilderOpen)}
              className="h-7 text-xs"
            >
              {schemaBuilderOpen ? "Hide" : "Build"} Schema
            </Button>
          </div>

          {schemaBuilderOpen && (
            <div className="space-y-3 rounded border bg-background p-3">
              <p className="text-muted-foreground text-xs">
                Define fields to generate JSON schema
              </p>
              <div className="max-h-48 space-y-2 overflow-y-auto">
                {schemaFields.map((field, index) => (
                  <div key={index} className="flex items-center gap-2">
                    <Input
                      placeholder="Field name"
                      value={field.name}
                      onChange={(e) => {
                        const updated = [...schemaFields]
                        updated[index].name = e.target.value
                        setSchemaFields(updated)
                      }}
                      className="h-8 flex-1 text-sm"
                    />
                    <select
                      value={field.type}
                      onChange={(e) => {
                        const updated = [...schemaFields]
                        updated[index].type = e.target.value
                        if (e.target.value !== "array") {
                          delete updated[index].itemType
                        }
                        setSchemaFields(updated)
                      }}
                      className="h-8 rounded border bg-background px-2 text-sm"
                    >
                      <option value="string">string</option>
                      <option value="number">number</option>
                      <option value="boolean">boolean</option>
                      <option value="array">array</option>
                      <option value="object">object</option>
                    </select>
                    {field.type === "array" && (
                      <select
                        value={field.itemType || "string"}
                        onChange={(e) => {
                          const updated = [...schemaFields]
                          updated[index].itemType = e.target.value
                          setSchemaFields(updated)
                        }}
                        className="h-8 rounded border bg-background px-2 text-sm"
                      >
                        <option value="string">items: string</option>
                        <option value="number">items: number</option>
                        <option value="object">items: object</option>
                      </select>
                    )}
                    <Input
                      placeholder="Description (optional)"
                      value={field.description}
                      onChange={(e) => {
                        const updated = [...schemaFields]
                        updated[index].description = e.target.value
                        setSchemaFields(updated)
                      }}
                      className="h-8 flex-1 text-sm"
                    />
                    <Button
                      variant="noShadow"
                      size="icon"
                      className="h-8 w-8"
                      onClick={() =>
                        setSchemaFields(
                          schemaFields.filter((_, i) => i !== index)
                        )
                      }
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  </div>
                ))}
              </div>
              <div className="flex gap-2">
                <Button
                  variant="noShadow"
                  size="sm"
                  onClick={() =>
                    setSchemaFields([
                      ...schemaFields,
                      { name: "", type: "string", description: "" },
                    ])
                  }
                  className="h-7 text-xs"
                >
                  Add Field
                </Button>
                <Button
                  variant="default"
                  size="sm"
                  onClick={() => {
                    setScrapeOptions({
                      ...scrapeOptions,
                      jsonSchema: generateSchema(),
                    })
                  }}
                  className="h-7 text-xs"
                >
                  Generate Schema
                </Button>
              </div>
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="jsonSchema">JSON Schema</Label>
            <Textarea
              id="jsonSchema"
              value={scrapeOptions.jsonSchema}
              onChange={(e) =>
                setScrapeOptions({
                  ...scrapeOptions,
                  jsonSchema: e.target.value,
                })
              }
              placeholder='{"type": "object", "properties": {"title": {"type": "string"}}}'
              className="font-mono text-sm"
              rows={4}
            />
          </div>
          <div className="grid grid-cols-2 gap-2">
            <div className="space-y-1">
              <Label htmlFor="extractionPrompt" className="text-xs">
                Prompt
              </Label>
              <Input
                id="extractionPrompt"
                value={scrapeOptions.extractionPrompt}
                onChange={(e) =>
                  setScrapeOptions({
                    ...scrapeOptions,
                    extractionPrompt: e.target.value,
                  })
                }
                placeholder="Extraction prompt..."
                className="h-8 text-sm"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="extractionResponseFormat" className="text-xs">
                Response Format
              </Label>
              <Input
                id="extractionResponseFormat"
                value={scrapeOptions.extractionResponseFormat}
                onChange={(e) =>
                  setScrapeOptions({
                    ...scrapeOptions,
                    extractionResponseFormat: e.target.value,
                  })
                }
                placeholder="format_name"
                className="h-8 text-sm"
              />
            </div>
          </div>
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2 sm:gap-6">
        <div className="flex items-center gap-2">
          <Label className="text-sm">Renderer</Label>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="default"
                size="sm"
                className="h-8 w-[140px] justify-start"
              >
                {scrapeOptions.renderMode === "auto"
                  ? "Auto"
                  : scrapeOptions.renderMode === "browser"
                    ? "Browser"
                    : "HTTP"}
                <ChevronDown className="ml-auto h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuRadioGroup
                value={scrapeOptions.renderMode}
                onValueChange={(v) =>
                  setScrapeOptions({
                    ...scrapeOptions,
                    renderMode: v as RenderMode,
                  })
                }
              >
                <DropdownMenuRadioItem value="auto">Auto</DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="http">HTTP</DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="browser">
                  Browser
                </DropdownMenuRadioItem>
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        <div className="flex items-center gap-2">
          <Label className="text-sm">Cache TTL</Label>
          <Input
            type="number"
            placeholder="3600"
            value={scrapeOptions.ttl ?? ""}
            onChange={(e) =>
              setScrapeOptions({
                ...scrapeOptions,
                ttl: e.target.value ? parseInt(e.target.value) : undefined,
              })
            }
            className="h-8 w-[100px]"
            min={0}
          />
          <span className="text-xs text-gray-500">seconds (0=bypass)</span>
        </div>
      </div>

      {scrapeOptions.renderMode !== "http" && (
        <div className="space-y-2">
          <Label>Wait: {scrapeOptions.waitFor}ms</Label>
          <Slider
            value={[scrapeOptions.waitFor]}
            onValueChange={([v]) =>
              setScrapeOptions({ ...scrapeOptions, waitFor: v })
            }
            min={0}
            max={10000}
            step={500}
          />
        </div>
      )}

      <button
        type="button"
        onClick={() => setAdvancedExpanded(!advancedExpanded)}
        className="flex items-center gap-1 text-sm text-gray-500 hover:text-gray-700"
      >
        {advancedExpanded ? (
          <ChevronUp className="h-4 w-4" />
        ) : (
          <ChevronDown className="h-4 w-4" />
        )}
        {advancedExpanded ? "Hide" : "Show"} advanced options
      </button>

      {advancedExpanded && (
        <div className="space-y-4 border-t border-border pt-2">
          <div className="space-y-2">
            <Label htmlFor="scrape-css">CSS Selector</Label>
            <Input
              id="scrape-css"
              value={scrapeOptions.cssSelector}
              onChange={(e) =>
                setScrapeOptions({
                  ...scrapeOptions,
                  cssSelector: e.target.value,
                })
              }
              placeholder=".main-content"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="scrape-headers">Headers (JSON)</Label>
            <Input
              id="scrape-headers"
              value={scrapeOptions.headers}
              onChange={(e) =>
                setScrapeOptions({ ...scrapeOptions, headers: e.target.value })
              }
              placeholder='{"User-Agent": "..."}'
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="scrape-include-tags">
              Include Tags (comma-separated)
            </Label>
            <Input
              id="scrape-include-tags"
              value={scrapeOptions.includeTags}
              onChange={(e) =>
                setScrapeOptions({
                  ...scrapeOptions,
                  includeTags: e.target.value,
                })
              }
              placeholder="article, main"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="scrape-exclude-tags">
              Exclude Tags (comma-separated)
            </Label>
            <Input
              id="scrape-exclude-tags"
              value={scrapeOptions.excludeTags}
              onChange={(e) =>
                setScrapeOptions({
                  ...scrapeOptions,
                  excludeTags: e.target.value,
                })
              }
              placeholder="nav, footer"
            />
          </div>
        </div>
      )}

      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            className="w-full"
            size="lg"
            onClick={handleSubmit}
            disabled={isLoading || !url}
          >
            {isLoading ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Scraping...
              </>
            ) : (
              <>
                <Play className="mr-2 h-4 w-4" />
                Run
              </>
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">
          {isLoading ? "Request in progress..." : "Send request to API"}
        </TooltipContent>
      </Tooltip>
    </div>
  )

  const renderCrawlForm = () => (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>Max Depth: {crawlOptions.maxDepth}</Label>
          <Slider
            value={[crawlOptions.maxDepth]}
            onValueChange={([v]) =>
              setCrawlOptions({ ...crawlOptions, maxDepth: v })
            }
            min={1}
            max={10}
          />
        </div>
        <div className="space-y-2">
          <Label>Max Pages: {crawlOptions.maxPages}</Label>
          <Slider
            value={[crawlOptions.maxPages]}
            onValueChange={([v]) =>
              setCrawlOptions({ ...crawlOptions, maxPages: v })
            }
            min={1}
            max={1000}
            step={10}
          />
        </div>
      </div>

      <div className="space-y-2">
        <Label>Output Formats</Label>
        <div className="flex flex-wrap gap-3">
          {(
            [
              "markdown",
              "html",
              "rawHtml",
              "plainText",
              "links",
              "imageLinks",
            ] as Format[]
          ).map((format) => (
            <div key={format} className="flex items-center gap-2">
              <Checkbox
                id={`crawl-${format}`}
                checked={crawlOptions.formats.includes(format)}
                onCheckedChange={(checked) =>
                  handleFormatChange("crawl", format, checked as boolean)
                }
              />
              <Label
                htmlFor={`crawl-${format}`}
                className="text-sm font-normal"
              >
                {format}
              </Label>
            </div>
          ))}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2 sm:gap-6">
        <div className="flex items-center gap-2">
          <Label className="text-sm">Renderer</Label>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="default"
                size="sm"
                className="h-8 w-[140px] justify-start"
              >
                {crawlOptions.renderMode === "auto"
                  ? "Auto"
                  : crawlOptions.renderMode === "browser"
                    ? "Browser"
                    : "HTTP"}
                <ChevronDown className="ml-auto h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuRadioGroup
                value={crawlOptions.renderMode}
                onValueChange={(v) =>
                  setCrawlOptions({
                    ...crawlOptions,
                    renderMode: v as RenderMode,
                  })
                }
              >
                <DropdownMenuRadioItem value="auto">Auto</DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="http">HTTP</DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="browser">
                  Browser
                </DropdownMenuRadioItem>
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {crawlOptions.renderMode !== "http" && (
        <div className="space-y-2">
          <Label>Wait: {crawlOptions.waitFor}ms</Label>
          <Slider
            value={[crawlOptions.waitFor]}
            onValueChange={([v]) =>
              setCrawlOptions({ ...crawlOptions, waitFor: v })
            }
            min={0}
            max={10000}
            step={500}
          />
        </div>
      )}

      <Button
        className="w-full"
        size="lg"
        onClick={handleSubmit}
        disabled={isLoading || !url}
      >
        {isLoading ? (
          <>
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            Crawling...
          </>
        ) : (
          <>
            <Play className="mr-2 h-4 w-4" />
            Run
          </>
        )}
      </Button>
    </div>
  )

  const renderMapForm = () => (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Max Depth: {mapOptions.maxDepth}</Label>
        <Slider
          value={[mapOptions.maxDepth]}
          onValueChange={([v]) => setMapOptions({ ...mapOptions, maxDepth: v })}
          min={1}
          max={10}
        />
      </div>

      <div className="flex items-center gap-2">
        <Switch
          id="map-sitemap"
          checked={mapOptions.useSitemap}
          onCheckedChange={(checked) =>
            setMapOptions({ ...mapOptions, useSitemap: checked })
          }
        />
        <Label htmlFor="map-sitemap" className="text-sm">
          Use Sitemap
        </Label>
      </div>

      <div className="space-y-2">
        <Label>Timeout: {mapOptions.timeout}ms</Label>
        <Slider
          value={[mapOptions.timeout]}
          onValueChange={([v]) => setMapOptions({ ...mapOptions, timeout: v })}
          min={5000}
          max={120000}
          step={5000}
        />
      </div>

      <Button
        className="w-full"
        size="lg"
        onClick={handleSubmit}
        disabled={isLoading || !url}
      >
        {isLoading ? (
          <>
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            Mapping...
          </>
        ) : (
          <>
            <Play className="mr-2 h-4 w-4" />
            Run
          </>
        )}
      </Button>
    </div>
  )

  const renderSearchForm = () => (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="search-query">Search Query</Label>
        <Input
          id="search-query"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder="Enter your search query..."
          onKeyDown={(e) => {
            if (e.key === "Enter" && searchQuery && !isLoading) {
              handleSearchSubmit()
            }
          }}
        />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>Region</Label>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="default"
                size="sm"
                className="h-10 w-full justify-start"
              >
                {searchRegion}
                <ChevronDown className="ml-auto h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuRadioGroup
                value={searchRegion}
                onValueChange={setSearchRegion}
              >
                <DropdownMenuRadioItem value="us-en">
                  US (English)
                </DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="uk-en">
                  UK (English)
                </DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="au-en">
                  Australia
                </DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="ca-en">
                  Canada
                </DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="de-de">
                  Germany
                </DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="fr-fr">
                  France
                </DropdownMenuRadioItem>
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <div className="space-y-2">
          <Label htmlFor="search-timeRange">Time Range</Label>
          <select
            id="search-timeRange"
            value={searchTimeRange}
            onChange={(e) => setSearchTimeRange(e.target.value as SearchTimeRange)}
            className="h-10 w-full rounded-md border bg-background px-3 py-2 text-sm"
          >
            <option value="">Any time</option>
            <option value="day">Past day</option>
            <option value="week">Past week</option>
            <option value="month">Past month</option>
            <option value="year">Past year</option>
          </select>
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="search-page">Page</Label>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="default"
            size="sm"
            className="h-10 w-10 shrink-0 p-0"
            disabled={searchPage <= 1}
            onClick={() => setSearchPage(Math.max(1, searchPage - 1))}
          >
            −
          </Button>
          <Input
            id="search-page"
            type="number"
            min={1}
            max={1000}
            value={searchPage}
            onChange={(e) => {
              const v = parseInt(e.target.value, 10)
              if (Number.isNaN(v) || v < 1) {
                setSearchPage(1)
              } else {
                setSearchPage(Math.min(1000, v))
              }
            }}
            className="h-10 text-center"
          />
          <Button
            type="button"
            variant="default"
            size="sm"
            className="h-10 w-10 shrink-0 p-0"
            disabled={searchPage >= 1000}
            onClick={() => setSearchPage(Math.min(1000, searchPage + 1))}
          >
            +
          </Button>
          <span className="text-xs text-muted-foreground">
            (1-indexed)
          </span>
        </div>
      </div>

      <div className="flex items-center gap-2">
        <Switch
          id="search-scrape"
          checked={searchScrape}
          onCheckedChange={(checked) => setSearchScrape(checked)}
        />
        <Label htmlFor="search-scrape" className="text-sm">
          Scrape each result
        </Label>
      </div>

      <div className="flex items-center gap-2">
        <Switch
          id="search-bm25"
          checked={searchUseBM25}
          onCheckedChange={(checked) => setSearchUseBM25(checked)}
        />
        <Label htmlFor="search-bm25" className="text-sm">
          Use BM25 scoring
        </Label>
      </div>

      {searchScrape && (
        <>
          <div className="flex items-center gap-2">
            <Label className="text-sm">Renderer</Label>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="default"
                  size="sm"
                  className="h-8 w-[140px] justify-start"
                >
                  {searchRenderMode === "auto"
                    ? "Auto"
                    : searchRenderMode === "browser"
                      ? "Browser"
                      : "HTTP"}
                  <ChevronDown className="ml-auto h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start">
                <DropdownMenuRadioGroup
                  value={searchRenderMode}
                  onValueChange={(v) =>
                    setSearchRenderMode(v as RenderMode)
                  }
                >
                  <DropdownMenuRadioItem value="auto">
                    Auto
                  </DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="http">
                    HTTP
                  </DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="browser">
                    Browser
                  </DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>

          <div className="space-y-2">
            <Label>Output Formats</Label>
            <div className="flex flex-wrap gap-3">
              {(
                [
                  "markdown",
                  "html",
                  "rawHtml",
                  "plainText",
                  "links",
                  "imageLinks",
                ] as Format[]
              ).map((format) => (
                <div key={format} className="flex items-center gap-2">
                  <Checkbox
                    id={`search-${format}`}
                    checked={searchFormats.includes(format)}
                    onCheckedChange={(checked) =>
                      setSearchFormats(
                        checked
                          ? [...searchFormats, format]
                          : searchFormats.filter((f) => f !== format)
                      )
                    }
                  />
                  <Label
                    htmlFor={`search-${format}`}
                    className="text-sm font-normal"
                  >
                    {format}
                  </Label>
                </div>
              ))}
            </div>
          </div>
        </>
      )}

      <Button
        className="w-full"
        size="lg"
        onClick={handleSearchSubmit}
        disabled={isLoading || !searchQuery}
      >
        {isLoading ? (
          <>
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            Searching...
          </>
        ) : (
          <>
            <Play className="mr-2 h-4 w-4" />
            Run
          </>
        )}
      </Button>
    </div>
  )

  const renderActiveForm = () => {
    switch (endpoint) {
      case "scrape":
        return renderScrapeForm()
      case "crawl":
        return renderCrawlForm()
      case "map":
        return renderMapForm()
      case "search":
        return renderSearchForm()
    }
  }

  return (
    <SidebarProvider>
      <AppSidebar
        activeEndpoint={endpoint}
        onEndpointChange={handleEndpointChange}
        health={health}
      />
      <SidebarInset>
        <header className="flex h-16 shrink-0 items-center gap-2 border-b-2 border-border bg-background transition-[width,height] ease-linear group-has-[[data-collapsible=icon]]/sidebar-wrapper:h-12">
          <div className="flex min-w-0 items-center gap-2 px-4">
            <SidebarTrigger className="-ml-1" />
            <Breadcrumb>
              <BreadcrumbList>
                <BreadcrumbItem className="hidden md:block">
                  <BreadcrumbLink href="/playground">Playground</BreadcrumbLink>
                </BreadcrumbItem>
                <BreadcrumbSeparator className="hidden md:block" />
                <BreadcrumbItem>
                  <BreadcrumbPage className="capitalize">
                    {endpoint}
                  </BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>
          </div>
          <div className="ml-auto hidden items-center gap-2 px-4 sm:flex">
            <Tooltip>
              <TooltipTrigger asChild>
                <a href="/battle">
                  <Button variant="noShadow" size="sm">
                    QuickCrawl vs TinyFish
                  </Button>
                </a>
              </TooltipTrigger>
              <TooltipContent side="bottom">
                Compare QuickCrawl and TinyFish scrape results side by side
              </TooltipContent>
            </Tooltip>
            {/* <Tooltip>
              <TooltipTrigger asChild>
                <a href="/battle-articles">
                  <Button variant="noShadow" size="sm">
                    Battle Articles
                  </Button>
                </a>
              </TooltipTrigger>
              <TooltipContent side="bottom">
                Compare across 15 news articles
              </TooltipContent>
            </Tooltip> */}
          </div>
        </header>

        <main className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto [background-color:var(--secondary-background)] bg-[linear-gradient(90deg,var(--border)_1px,transparent_1px),linear-gradient(var(--border)_1px,transparent_1px)] bg-[size:28px_28px] bg-fixed p-3 sm:p-4">
          <div className="grid min-h-0 flex-1 gap-4 xl:grid-cols-[minmax(320px,420px)_minmax(0,1fr)]">
            <Card className="h-[520px] min-w-0 gap-0 overflow-hidden bg-background/95 py-0 sm:h-[620px] xl:h-[calc(100vh-6.5rem)]">
              <CardHeader className="shrink-0 border-b-2 border-border bg-main px-4 py-4 text-main-foreground sm:px-5">
                <div className="flex items-center justify-between gap-3">
                  <CardTitle className="text-base">Request</CardTitle>
                  <Badge variant="neutral" className="shrink-0 capitalize">
                    {endpoint}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent className="min-h-0 flex-1 overflow-auto px-4 py-4 sm:px-5">
                {endpoint !== "search" && (
                  <div className="mb-4">
                    <Label htmlFor="url" className="mb-2 block text-sm">
                      URL
                    </Label>
                    <Input
                      id="url"
                      value={url}
                      onChange={(e) => setUrl(e.target.value)}
                      placeholder="https://example.com"
                      className="w-full"
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && !isLoading && url) {
                          handleSubmit()
                        }
                      }}
                    />
                  </div>
                )}
                {renderActiveForm()}
              </CardContent>
            </Card>

            <div className="h-[520px] min-w-0 sm:h-[620px] xl:h-[calc(100vh-6.5rem)]">
              {!response && !crawlStatus && !error && !isLoading && (
                <Card className="h-full min-w-0 gap-0 overflow-hidden bg-background/95 py-0">
                  <CardHeader className="shrink-0 border-b-2 border-border px-4 py-4 sm:px-5">
                    <div className="flex items-center justify-between gap-3">
                      <CardTitle className="text-base">Response</CardTitle>
                      <Badge variant="neutral">Idle</Badge>
                    </div>
                  </CardHeader>
                  <CardContent className="flex min-h-0 flex-1 items-center justify-center px-4 py-10 sm:px-5">
                    <div className="max-w-sm text-center">
                      <div className="mx-auto mb-4 flex size-14 items-center justify-center rounded-base border-2 border-border bg-secondary-background shadow-shadow">
                        <Play className="size-6 text-main" />
                      </div>
                      <p className="text-lg font-bold font-heading">
                        No response yet
                      </p>
                      <p className="mt-2 text-sm text-foreground/60">
                        Configure the request and run it to inspect the API
                        payload.
                      </p>
                    </div>
                  </CardContent>
                </Card>
              )}

              {isLoading && !crawlStatus && (
                <Card className="h-full min-w-0 gap-0 overflow-hidden bg-background/95 py-0">
                  <CardHeader className="shrink-0 border-b-2 border-border px-4 py-4 sm:px-5">
                    <div className="flex items-center justify-between gap-3">
                      <CardTitle className="text-base">Response</CardTitle>
                      <Badge variant="neutral">Running</Badge>
                    </div>
                  </CardHeader>
                  <CardContent className="flex min-h-0 flex-1 items-center justify-center px-4 py-10 sm:px-5">
                    <div className="text-center">
                      <Loader2 className="mx-auto mb-4 size-12 animate-spin text-main" />
                      <p className="text-lg font-bold font-heading">
                        Processing request
                      </p>
                      <p className="mt-2 text-sm text-foreground/60">
                        This may take a moment.
                      </p>
                    </div>
                  </CardContent>
                </Card>
              )}

              {crawlId && !crawlStatus && (
                <Card className="h-full min-w-0 gap-0 overflow-hidden bg-background/95 py-0">
                  <CardHeader className="shrink-0 border-b-2 border-border px-4 py-4 sm:px-5">
                    <div className="flex items-center justify-between gap-3">
                      <CardTitle className="text-base">Response</CardTitle>
                      <Badge variant="neutral">Polling</Badge>
                    </div>
                  </CardHeader>
                  <CardContent className="flex min-h-0 flex-1 items-center justify-center px-4 py-10 sm:px-5">
                    <div className="max-w-full text-center">
                      <Loader2 className="mx-auto mb-3 size-5 animate-spin text-main" />
                      <span className="block font-bold font-heading">
                        Polling for crawl status
                      </span>
                      <div className="mt-3 text-sm text-foreground/60">
                        Job ID:{" "}
                        <code className="rounded-base border border-border bg-secondary-background px-2 py-1 break-all">
                          {crawlId}
                        </code>
                      </div>
                      <p className="mt-2 text-xs text-foreground/50">
                        Checking every 2s
                      </p>
                    </div>
                  </CardContent>
                </Card>
              )}

              {(response || crawlStatus || error) && (
                <div className="flex h-full min-h-0 min-w-0 flex-col">
                  {renderResponse()}
                </div>
              )}
            </div>
          </div>

          <div className="float-in-air fixed right-4 bottom-4 z-40 flex flex-col items-end gap-2 sm:right-6 sm:bottom-6">
            <span className="rounded-base border-2 border-border bg-background px-2.5 py-1 text-xs font-heading shadow-shadow">
              Code snippet
            </span>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="default"
                  size="icon"
                  className="size-11"
                  onClick={() => setCodeSheetOpen(true)}
                  aria-label="Open code snippet"
                >
                  <FileCode className="h-5 w-5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="left">
                View code snippets (cURL, Fetch, Python)
              </TooltipContent>
            </Tooltip>
          </div>

          <Sheet open={codeSheetOpen} onOpenChange={setCodeSheetOpen}>
            <SheetContent
              side="right"
              className="w-[92vw] gap-0 overflow-hidden p-0 sm:max-w-[560px]"
            >
              <SheetHeader className="border-b-2 border-border bg-main p-4 pr-12 text-main-foreground sm:p-5">
                <SheetTitle className="text-main-foreground">
                  Code Snippet
                </SheetTitle>
              </SheetHeader>
              <div className="flex min-h-0 flex-1 flex-col gap-4 p-4 sm:p-5">
                <div className="flex flex-wrap gap-2">
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant={
                          codeLanguage === "curl" ? "default" : "noShadow"
                        }
                        size="sm"
                        onClick={() => setCodeLanguage("curl")}
                      >
                        cURL
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side="top">
                      Generate cURL command
                    </TooltipContent>
                  </Tooltip>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant={
                          codeLanguage === "fetch" ? "default" : "noShadow"
                        }
                        size="sm"
                        onClick={() => setCodeLanguage("fetch")}
                      >
                        Fetch
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side="top">
                      Generate JavaScript Fetch code
                    </TooltipContent>
                  </Tooltip>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant={
                          codeLanguage === "python" ? "default" : "noShadow"
                        }
                        size="sm"
                        onClick={() => setCodeLanguage("python")}
                      >
                        Python
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side="top">
                      Generate Python requests code
                    </TooltipContent>
                  </Tooltip>
                </div>

                <div className="relative min-h-0 flex-1">
                  <pre className="h-full min-h-[360px] overflow-auto rounded-base border-2 border-border bg-secondary-background p-4 pr-14 font-mono text-sm break-words whitespace-pre-wrap">
                    {getCodeSnippet() ||
                      "// Configure options to generate code"}
                  </pre>
                  {getCodeSnippet() && (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="noShadow"
                          size="icon"
                          className="absolute top-2 right-2"
                          onClick={() =>
                            copySnippet(getCodeSnippet(), codeLanguage)
                          }
                          aria-label="Copy code snippet"
                        >
                          {copiedSnippet === codeLanguage ? (
                            <Check className="h-4 w-4" />
                          ) : (
                            <Copy className="h-4 w-4" />
                          )}
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent side="left">
                        {copiedSnippet === codeLanguage
                          ? "Copied!"
                          : "Copy code snippet"}
                      </TooltipContent>
                    </Tooltip>
                  )}
                </div>
              </div>
            </SheetContent>
          </Sheet>
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
