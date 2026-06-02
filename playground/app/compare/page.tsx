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
  GitCompare,
  ChevronDown,
  ChevronUp,
  X as XIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
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
    links?: string[]
    json?: unknown
    metadata?: {
      title?: string
      description?: string
      timeTaken?: number
      renderedMode?: string
      warning?: string
      [key: string]: unknown
    }
  }
  error?: string
  errorCode?: string
  timeTaken?: number
}

interface CompareResult {
  original: ScrapeResult
  core: ScrapeResult
}

interface SchemaField {
  name: string
  type: string
  description: string
  itemType?: string
}

function getBaseUrl() {
  return process.env.NEXT_PUBLIC_BASE_URL
}

async function fetchWithTimeout(
  url: string,
  options: RequestInit,
  timeoutMs: number = 120000
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

function buildRequestBody(
  url: string,
  formatList: string[],
  renderJs: boolean,
  jsonSchema: string,
  extractionPrompt: string,
  extractionResponseFormat: string,
  browser: string
): Record<string, unknown> {
  const body: Record<string, unknown> = {
    url,
    formats: formatList,
    renderJs,
    onlyMain: true,
  }

  if (browser) {
    body.browser = browser
  }

  const includesJson = formatList.includes("json")
  if (includesJson && jsonSchema.trim()) {
    try {
      const extract: Record<string, unknown> = {
        schema: JSON.parse(jsonSchema),
      }
      if (extractionPrompt.trim()) extract.prompt = extractionPrompt.trim()
      if (extractionResponseFormat.trim()) extract.responseFormat = extractionResponseFormat.trim()
      body.extract = extract
    } catch {
      // Invalid JSON schema, omit extract
    }
  }

  return body
}

function parseScrapeResult(data: unknown, timeTaken: number): ScrapeResult {
  const d = data as {
    data?: {
      markdown?: string
      html?: string
      plainText?: string
      imageLinks?: string[]
      links?: string[]
      json?: unknown
      metadata?: {
        title?: string
        description?: string
        timeTaken?: number
        renderedMode?: string
        warning?: string
      }
    }
  }
  return {
    success: true,
    data: {
      markdown: d.data?.markdown,
      html: d.data?.html,
      plainText: d.data?.plainText,
      imageLinks: d.data?.imageLinks,
      links: d.data?.links,
      json: d.data?.json,
      metadata: {
        title: d.data?.metadata?.title,
        description: d.data?.metadata?.description,
        timeTaken: d.data?.metadata?.timeTaken || timeTaken,
        renderedMode: d.data?.metadata?.renderedMode,
        warning: d.data?.metadata?.warning,
      },
    },
    timeTaken,
  }
}

async function scrapeOriginal(
  url: string,
  sharedStartTime: number,
  body: Record<string, unknown>
): Promise<ScrapeResult> {
  const baseUrl = getBaseUrl()

  try {
    const response = await fetchWithTimeout(`${baseUrl}/v1/scrape`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    })

    const timeTaken = Date.now() - sharedStartTime
    const data = await response.json()

    if (!response.ok) {
      return {
        success: false,
        error: data.error || response.statusText,
        errorCode: data.errorCode,
        timeTaken,
      }
    }

    return parseScrapeResult(data, timeTaken)
  } catch (error) {
    const timeTaken = Date.now() - sharedStartTime
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
      timeTaken,
    }
  }
}

async function scrapeCore(
  url: string,
  sharedStartTime: number,
  body: Record<string, unknown>
): Promise<ScrapeResult> {
  const baseUrl = getBaseUrl()

  try {
    const response = await fetchWithTimeout(`${baseUrl}/v1/scrape-core`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    })

    const timeTaken = Date.now() - sharedStartTime
    const data = await response.json()

    if (!response.ok) {
      return {
        success: false,
        error: data.error || response.statusText,
        errorCode: data.errorCode,
        timeTaken,
      }
    }

    return parseScrapeResult(data, timeTaken)
  } catch (error) {
    const timeTaken = Date.now() - sharedStartTime
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
      timeTaken,
    }
  }
}

function generateSchema(fields: SchemaField[]): string {
  const properties: Record<string, { type: string; description?: string; items?: { type: string } }> = {}
  const required: string[] = []

  fields.forEach((field) => {
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

function ResultCard({
  title,
  result,
  badgeColor,
  showHtml,
  showLinks,
  showJson,
}: {
  title: string
  result: ScrapeResult
  badgeColor: string
  showHtml?: boolean
  showLinks?: boolean
  showJson?: boolean
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

  const defaultTab = showJson
    ? "json"
    : showHtml
    ? "html"
    : showLinks
    ? "links"
    : "markdown"

  return (
    <Card className="min-w-0 flex-1">
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <CardTitle className="text-lg">{title}</CardTitle>
            {result.data?.metadata?.renderedMode && (
              <Badge variant="neutral" className="text-xs">
                {result.data.metadata.renderedMode}
              </Badge>
            )}
          </div>
          <div className="flex items-center gap-2">
            {result.timeTaken && (
              <Badge variant="neutral" className="flex items-center gap-1">
                <Clock className="h-3 w-3" />
                {result.timeTaken}ms
              </Badge>
            )}
            {result.data?.metadata?.warning && (
              <Badge variant="neutral" className="text-xs text-yellow-600">
                {result.data.metadata.warning}
              </Badge>
            )}
            {result.success ? (
              <CheckCircle className={`h-5 w-5 ${badgeColor}`} />
            ) : (
              <XCircle className="h-5 w-5 text-red-500" />
            )}
          </div>
        </div>
        {result.data?.metadata?.title && (
          <p className="text-sm text-muted-foreground truncate mt-1">
            {result.data.metadata.title}
          </p>
        )}
      </CardHeader>
      <CardContent>
        {!result.success && result.error !== "Pending" ? (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>
              {result.error}
              {result.errorCode && (
                <span className="ml-2 text-xs opacity-70">({result.errorCode})</span>
              )}
            </AlertDescription>
          </Alert>
        ) : result.error === "Pending" ? (
          <div className="text-muted-foreground flex items-center justify-center py-8">
            <Loader2 className="mr-2 h-6 w-6 animate-spin" />
            <span>Waiting for result...</span>
          </div>
        ) : (
          <Tabs defaultValue={defaultTab}>
            <TabsList className="mb-3">
              {showJson && (
                <TabsTrigger value="json">
                  JSON {result.data?.json ? "✓" : ""}
                </TabsTrigger>
              )}
              {showLinks && <TabsTrigger value="links">Links ({result.data?.links?.length || 0})</TabsTrigger>}
              {showHtml && <TabsTrigger value="html">HTML</TabsTrigger>}
              {showHtml && <TabsTrigger value="html-preview">HTML Preview</TabsTrigger>}
              <TabsTrigger value="markdown">Markdown</TabsTrigger>
              <TabsTrigger value="preview">Preview</TabsTrigger>
            </TabsList>
            {showJson && (
              <TabsContent value="json">
                <div className="bg-muted max-h-[500px] overflow-auto rounded-md p-3">
                  <div className="flex justify-end mb-1">
                    <Button
                      variant="noShadow"
                      size="sm"
                      className="h-7 text-xs"
                      onClick={() =>
                        copyToClipboard(
                          JSON.stringify(result.data?.json ?? null, null, 2),
                          "json"
                        )
                      }
                    >
                      <Copy className="h-3 w-3 mr-1" />
                      {copied === "json" ? "Copied!" : "Copy"}
                    </Button>
                  </div>
                  {result.data?.json ? (
                    <pre className="font-mono text-xs break-words whitespace-pre-wrap">
                      {JSON.stringify(result.data.json, null, 2)}
                    </pre>
                  ) : (
                    <p className="text-muted-foreground text-sm">No JSON output</p>
                  )}
                </div>
              </TabsContent>
            )}
            {showLinks && (
              <TabsContent value="links">
                <div className="bg-muted max-h-[500px] overflow-auto rounded-md p-3">
                  {result.data?.links && result.data.links.length > 0 ? (
                    <ul className="space-y-1">
                      {result.data.links.slice(0, 100).map((link, i) => (
                        <li key={i} className="text-xs break-all">
                          <a href={link} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline">
                            {link}
                          </a>
                        </li>
                      ))}
                      {result.data.links.length > 100 && (
                        <li className="text-xs text-muted-foreground pt-2">
                          ... and {result.data.links.length - 100} more links
                        </li>
                      )}
                    </ul>
                  ) : (
                    <p className="text-muted-foreground text-sm">No links found</p>
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
          </Tabs>
        )}
      </CardContent>
    </Card>
  )
}

export default function ComparePage() {
  const [url, setUrl] = useState("")
  const [isLoading, setIsLoading] = useState(false)
  const [results, setResults] = useState<CompareResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [renderJs, setRenderJs] = useState(false)
  const [formats, setFormats] = useState<{
    markdown: boolean
    html: boolean
    links: boolean
    json: boolean
  }>({
    markdown: true,
    html: true,
    links: true,
    json: false,
  })

  const [jsonSchema, setJsonSchema] = useState("")
  const [extractionPrompt, setExtractionPrompt] = useState("")
  const [extractionResponseFormat, setExtractionResponseFormat] = useState("")
  const [schemaBuilderOpen, setSchemaBuilderOpen] = useState(false)
  const [schemaFields, setSchemaFields] = useState<SchemaField[]>([
    { name: "title", type: "string", description: "" },
  ])

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
      if (typeof window !== "undefined" && "caches" in window) {
        try {
          const cacheNames = await caches.keys()
          await Promise.all(cacheNames.map((name) => caches.delete(name)))
        } catch {
          // Caching APIs not available, continue
        }
      }

      const startTime = Date.now()
      const formatList = Object.entries(formats)
        .filter(([, enabled]) => enabled)
        .map(([name]) => name)

      const body = buildRequestBody(
        normalizedUrl,
        formatList,
        renderJs,
        jsonSchema,
        extractionPrompt,
        extractionResponseFormat,
        renderJs ? "chrome" : ""
      )

      const originalPromise = scrapeOriginal(normalizedUrl, startTime, body)
      const corePromise = scrapeCore(normalizedUrl, startTime, body)

      originalPromise.then((result) => {
        setResults((prev) =>
          prev
            ? { ...prev, original: result }
            : {
                original: result,
                core: { success: false, error: "Pending" },
              }
        )
      })

      corePromise.then((result) => {
        setResults((prev) =>
          prev
            ? { ...prev, core: result }
            : {
                original: { success: false, error: "Pending" },
                core: result,
              }
        )
      })

      await Promise.all([originalPromise, corePromise])
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
            <h1 className="text-3xl font-bold tracking-tight flex items-center gap-2">
              <GitCompare className="h-8 w-8" />
              Compare
            </h1>
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
              <a href="/playground">
                <Button variant="noShadow">
                  <LayoutDashboard className="h-4 w-4" />
                  Playground
                </Button>
              </a>
            </div>
          </div>
          <p className="text-muted-foreground">
            Compare <code className="bg-muted px-1 py-0.5 rounded">/v1/scrape</code> vs{" "}
            <code className="bg-muted px-1 py-0.5 rounded">/v1/scrape-core</code> side by side
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
                    { value: "https://docs.tinyfish.ai", label: "TinyFish Docs" },
                    { value: "https://www.dover.com/tinyfish", label: "Dover TinyFish" },
                    { value: "https://www.xda-developers.com/a-self-hosted-llms-is-way-more-powerful-than-a-chat-interface/", label: "XDA Developers" },
                    { value: "https://news.ycombinator.com/item?id=44741682", label: "Hacker News" },
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
                    JS Rendering
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
                    id="format-json"
                    checked={formats.json}
                    onCheckedChange={(checked) =>
                      setFormats((f) => ({ ...f, json: checked }))
                    }
                    disabled={isLoading}
                  />
                  <Label htmlFor="format-json" className="cursor-pointer text-sm">
                    JSON (LLM)
                  </Label>
                </div>
              </div>

              {formats.json && (
                <div className="p-3 bg-muted rounded-lg space-y-3">
                  <div className="flex items-center justify-between">
                    <p className="text-sm font-medium">
                      JSON Extraction (requires OpenAI API key on server)
                    </p>
                    <Button
                      variant="noShadow"
                      size="sm"
                      onClick={() => setSchemaBuilderOpen(!schemaBuilderOpen)}
                      className="h-7 text-xs"
                    >
                      {schemaBuilderOpen ? (
                        <>
                          <ChevronUp className="h-3 w-3 mr-1" />
                          Hide
                        </>
                      ) : (
                        <>
                          <ChevronDown className="h-3 w-3 mr-1" />
                          Build Schema
                        </>
                      )}
                    </Button>
                  </div>

                  {schemaBuilderOpen && (
                    <div className="p-3 bg-background rounded border space-y-3">
                      <p className="text-xs text-muted-foreground">
                        Define fields to generate a JSON schema
                      </p>
                      <div className="space-y-2 max-h-48 overflow-y-auto">
                        {schemaFields.map((field, index) => (
                          <div key={index} className="flex gap-2 items-center">
                            <Input
                              placeholder="Field name"
                              value={field.name}
                              onChange={(e) => {
                                const updated = [...schemaFields]
                                updated[index] = { ...updated[index], name: e.target.value }
                                setSchemaFields(updated)
                              }}
                              className="text-sm h-8 flex-1"
                            />
                            <select
                              value={field.type}
                              onChange={(e) => {
                                const updated = [...schemaFields]
                                const newType = e.target.value
                                updated[index] = { ...updated[index], type: newType }
                                if (newType !== "array") {
                                  delete updated[index].itemType
                                }
                                setSchemaFields(updated)
                              }}
                              className="h-8 px-2 text-sm border rounded bg-background"
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
                                  updated[index] = { ...updated[index], itemType: e.target.value }
                                  setSchemaFields(updated)
                                }}
                                className="h-8 px-2 text-sm border rounded bg-background"
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
                                updated[index] = { ...updated[index], description: e.target.value }
                                setSchemaFields(updated)
                              }}
                              className="text-sm h-8 flex-1"
                            />
                            <Button
                              variant="noShadow"
                              size="icon"
                              className="h-8 w-8"
                              onClick={() =>
                                setSchemaFields(schemaFields.filter((_, i) => i !== index))
                              }
                            >
                              <XIcon className="w-4 h-4" />
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
                          onClick={() => setJsonSchema(generateSchema(schemaFields))}
                          className="h-7 text-xs"
                        >
                          Generate Schema
                        </Button>
                      </div>
                    </div>
                  )}

                  <div className="space-y-2">
                    <Label htmlFor="jsonSchema" className="text-xs">
                      JSON Schema
                    </Label>
                    <Textarea
                      id="jsonSchema"
                      value={jsonSchema}
                      onChange={(e) => setJsonSchema(e.target.value)}
                      placeholder='{"type": "object", "properties": {"title": {"type": "string"}}}'
                      className="font-mono text-sm"
                      rows={4}
                      disabled={isLoading}
                    />
                  </div>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                    <div className="space-y-1">
                      <Label htmlFor="extractionPrompt" className="text-xs">
                        Prompt (optional)
                      </Label>
                      <Input
                        id="extractionPrompt"
                        value={extractionPrompt}
                        onChange={(e) => setExtractionPrompt(e.target.value)}
                        placeholder="Extraction prompt..."
                        className="text-sm h-8"
                        disabled={isLoading}
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="extractionResponseFormat" className="text-xs">
                        Response Format (optional)
                      </Label>
                      <Input
                        id="extractionResponseFormat"
                        value={extractionResponseFormat}
                        onChange={(e) => setExtractionResponseFormat(e.target.value)}
                        placeholder="format_name"
                        className="text-sm h-8"
                        disabled={isLoading}
                      />
                    </div>
                  </div>
                </div>
              )}
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
                  <span>/v1/scrape</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className="h-3 w-3 rounded-full bg-purple-500" />
                  <span>/v1/scrape-core</span>
                </div>
              </div>
            </div>

            {results.original.data?.metadata?.timeTaken && results.core.data?.metadata?.timeTaken && (
              <Card className="bg-muted/50">
                <CardContent className="pt-4 pb-4">
                  <div className="flex items-center justify-center gap-8 text-sm">
                    <div className="flex items-center gap-2">
                      <span className="text-muted-foreground">/v1/scrape:</span>
                      <span className="font-mono font-semibold">{results.original.data.metadata.timeTaken}ms</span>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-muted-foreground">/v1/scrape-core:</span>
                      <span className="font-mono font-semibold">{results.core.data.metadata.timeTaken}ms</span>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-muted-foreground">Speedup:</span>
                      <Badge variant="neutral" className="font-mono">
                        {(results.original.data.metadata.timeTaken / results.core.data.metadata.timeTaken).toFixed(2)}x
                      </Badge>
                    </div>
                  </div>
                </CardContent>
              </Card>
            )}

            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <ResultCard
                title="/v1/scrape"
                result={results.original}
                badgeColor="text-green-500"
                showHtml={formats.html}
                showLinks={formats.links}
                showJson={formats.json}
              />
              <ResultCard
                title="/v1/scrape-core"
                result={results.core}
                badgeColor="text-purple-500"
                showHtml={formats.html}
                showLinks={formats.links}
                showJson={formats.json}
              />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
