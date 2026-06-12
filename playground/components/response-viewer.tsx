"use client"

import { useState } from "react"
import { ExternalLink, Copy, Check, Eye } from "lucide-react"
import ReactMarkdown from "react-markdown"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet"
import { Alert, AlertTitle } from "@/components/ui/alert"
import { Label } from "@/components/ui/label"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { ScrapeData, MapResponse, SearchResponse } from "@/lib/api-types"

interface PageItem {
  index: number
  url?: string
  title?: string
  description?: string
  markdown?: string
  html?: string
  rawHtml?: string
  plainText?: string
  links?: string[]
  imageLinks?: string[]
  json?: string
  metadata?: {
    title?: string
    description?: string
    ogpTitle?: string
    ogpDescription?: string
    ogpImage?: string
    canonicalUrl?: string
    sourceURL?: string
    url?: string
    language?: string
    statusCode?: number
    renderedMode?: string
    timeTaken?: number
    [key: string]: unknown
  }
}

interface PageSheetProps {
  page: PageItem
  open: boolean
  onClose: () => void
}

function PageSheet({ page, open, onClose }: PageSheetProps) {
  const [activeTab, setActiveTab] = useState("")
  const [showCopiedAlert, setShowCopiedAlert] = useState(false)

  const availableTabs = [
    { id: "markdown", label: "Markdown", hasContent: !!page.markdown },
    { id: "html", label: "HTML", hasContent: !!page.html },
    { id: "rawHtml", label: "Raw HTML", hasContent: !!page.rawHtml },
    { id: "plainText", label: "Plain Text", hasContent: !!page.plainText },
    {
      id: "links",
      label: "Links",
      hasContent: !!(page.links && page.links.length > 0),
    },
    {
      id: "imageLinks",
      label: "Images",
      hasContent: !!(page.imageLinks && page.imageLinks.length > 0),
    },
    { id: "json", label: "JSON", hasContent: !!page.json },
    { id: "metadata", label: "Metadata", hasContent: !!page.metadata },
  ].filter((tab) => tab.hasContent)

  useState(() => {
    if (availableTabs.length > 0 && !activeTab) {
      setActiveTab(availableTabs[0].id)
    }
  })

  const getContent = () => {
    switch (activeTab) {
      case "markdown":
        return page.markdown || ""
      case "html":
        return page.html || ""
      case "rawHtml":
        return page.rawHtml || ""
      case "plainText":
        return page.plainText || ""
      case "links":
        return page.links ? page.links.join("\n") : ""
      case "imageLinks":
        return page.imageLinks ? page.imageLinks.join("\n") : ""
      case "json":
        return typeof page.json === "string"
          ? page.json
          : JSON.stringify(page.json, null, 2)
      case "metadata":
        return page.metadata ? JSON.stringify(page.metadata, null, 2) : ""
      default:
        return ""
    }
  }

  const copyContent = () => {
    const content = getContent()
    if (content) {
      navigator.clipboard.writeText(content)
      setShowCopiedAlert(true)
      setTimeout(() => setShowCopiedAlert(false), 2000)
    }
  }

  const hasContent = () => {
    return availableTabs.some((tab) => tab.id === activeTab && tab.hasContent)
  }

  const CopyButton = () => (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="neutral"
          size="sm"
          onClick={copyContent}
          disabled={!hasContent()}
          className="absolute top-3 right-3"
        >
          {showCopiedAlert ? (
            <Check className="h-4 w-4" />
          ) : (
            <Copy className="h-4 w-4" />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="left">
        {showCopiedAlert ? "Copied!" : "Copy to clipboard"}
      </TooltipContent>
    </Tooltip>
  )

  const renderContent = () => {
    switch (activeTab) {
      case "markdown":
        return page.markdown ? (
          <div className="markdown-content relative h-full overflow-auto rounded-base border-2 border-border bg-secondary-background p-6">
            <ReactMarkdown>{page.markdown}</ReactMarkdown>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground relative rounded-base border-2 border-border bg-secondary-background p-6 text-sm">
            No markdown content available
            <CopyButton />
          </div>
        )
      case "html":
        return page.html ? (
          <div className="relative h-full overflow-auto rounded-base border-2 border-border bg-secondary-background">
            <pre className="p-6 font-mono text-sm whitespace-pre-wrap">
              {page.html}
            </pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground relative rounded-base border-2 border-border bg-secondary-background p-6 text-sm">
            No HTML content available
            <CopyButton />
          </div>
        )
      case "rawHtml":
        return page.rawHtml ? (
          <div className="relative h-full overflow-auto rounded-base border-2 border-border bg-secondary-background">
            <pre className="p-6 font-mono text-sm whitespace-pre-wrap">
              {page.rawHtml}
            </pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground relative rounded-base border-2 border-border bg-secondary-background p-6 text-sm">
            No raw HTML content available
            <CopyButton />
          </div>
        )
      case "plainText":
        return page.plainText ? (
          <div className="relative h-full overflow-auto rounded-base border-2 border-border bg-secondary-background">
            <pre className="p-6 font-mono text-sm whitespace-pre-wrap">
              {page.plainText}
            </pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground relative rounded-base border-2 border-border bg-secondary-background p-6 text-sm">
            No plain text content available
            <CopyButton />
          </div>
        )
      case "links":
        return page.links && page.links.length > 0 ? (
          <div className="relative h-full overflow-auto rounded-base border-2 border-border bg-secondary-background">
            <ul className="divide-y divide-border">
              {page.links.map((link, i) => (
                <li key={i} className="p-3">
                  <a
                    href={link}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center gap-2 text-sm break-all text-main hover:text-main/80"
                  >
                    <ExternalLink className="h-4 w-4 flex-shrink-0" />
                    {link}
                  </a>
                </li>
              ))}
            </ul>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground relative rounded-base border-2 border-border bg-secondary-background p-6 text-sm">
            No links available
            <CopyButton />
          </div>
        )
      case "imageLinks":
        return page.imageLinks && page.imageLinks.length > 0 ? (
          <div className="relative h-full overflow-auto rounded-base border-2 border-border bg-secondary-background p-4">
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4">
              {page.imageLinks.map((img, i) => (
                <div
                  key={i}
                  className="group relative aspect-square overflow-hidden rounded-lg border border-border bg-background"
                >
                  <img
                    src={img}
                    alt={`Image ${i + 1}`}
                    className="h-full w-full object-cover object-center"
                    loading="lazy"
                  />
                  <div className="absolute inset-0 flex items-center justify-center bg-black/50 opacity-0 transition-opacity group-hover:opacity-100">
                    <a
                      href={img}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="rounded-full bg-background p-2"
                    >
                      <ExternalLink className="h-5 w-5 text-foreground" />
                    </a>
                  </div>
                </div>
              ))}
            </div>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground relative rounded-base border-2 border-border bg-secondary-background p-6 text-sm">
            No images available
            <CopyButton />
          </div>
        )
      case "json":
        return page.json ? (
          <div className="relative h-full overflow-auto rounded-base border-2 border-border bg-secondary-background">
            <pre className="p-6 font-mono text-sm whitespace-pre-wrap">
              {typeof page.json === "string"
                ? (() => {
                    try {
                      return JSON.stringify(JSON.parse(page.json), null, 2)
                    } catch {
                      return page.json
                    }
                  })()
                : JSON.stringify(page.json, null, 2)}
            </pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground relative rounded-base border-2 border-border bg-secondary-background p-6 text-sm">
            No JSON extraction available
            <CopyButton />
          </div>
        )
      case "metadata":
        return page.metadata ? (
          <div className="relative h-full overflow-auto rounded-base border-2 border-border bg-secondary-background">
            <pre className="p-6 font-mono text-sm whitespace-pre-wrap">
              {JSON.stringify(page.metadata, null, 2)}
            </pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground relative rounded-base border-2 border-border bg-secondary-background p-6 text-sm">
            No metadata available
            <CopyButton />
          </div>
        )
      default:
        return null
    }
  }

  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()}>
      <SheetContent
        side="right"
        className="flex w-[95vw] flex-col overflow-hidden p-0 sm:max-w-[95vw] md:max-w-[600px]"
      >
        <div className="flex-shrink-0 border-b-2 border-border p-6">
          <SheetTitle className="mb-2 line-clamp-2 text-xl font-bold">
            {page.metadata?.title || page.title || `Page ${page.index + 1}`}
          </SheetTitle>
          {page.metadata?.description && (
            <p className="text-muted-foreground mb-3 line-clamp-2 text-sm">
              {String(page.metadata.description)}
            </p>
          )}
          {page.metadata?.sourceURL && (
            <a
              href={String(page.metadata.sourceURL)}
              target="_blank"
              rel="noopener noreferrer"
              className="mb-2 block flex items-center gap-2 truncate text-sm text-main hover:text-main/80"
            >
              <ExternalLink className="h-4 w-4 flex-shrink-0" />
              <span className="truncate">
                {String(page.metadata.sourceURL)}
              </span>
            </a>
          )}
          <div className="mt-3 flex items-center gap-3">
            <Badge variant="neutral" className="text-sm">
              {page.metadata?.statusCode || "N/A"}
            </Badge>
            {page.metadata?.timeTaken && (
              <span className="text-muted-foreground text-sm">
                {page.metadata.timeTaken}ms
              </span>
            )}
            {page.metadata?.renderedMode && (
              <Badge variant="neutral" className="text-sm">
                {page.metadata.renderedMode}
              </Badge>
            )}
          </div>
        </div>

        <div className="flex flex-1 flex-col overflow-hidden p-6">
          {showCopiedAlert && (
            <Alert className="mb-4">
              <Check className="size-4" />
              <AlertTitle>Copied to clipboard</AlertTitle>
            </Alert>
          )}
          <Tabs
            value={activeTab}
            onValueChange={setActiveTab}
            className="flex flex-1 flex-col overflow-hidden"
          >
            <TabsList className="mb-4 flex-shrink-0 flex-nowrap overflow-x-auto">
              {availableTabs.map((tab) => (
                <TabsTrigger key={tab.id} value={tab.id}>
                  {tab.label}
                </TabsTrigger>
              ))}
            </TabsList>

            <div className="flex-1 overflow-auto">
              {availableTabs.map((tab) => (
                <TabsContent key={tab.id} value={tab.id} className="m-0 h-full">
                  {renderContent()}
                </TabsContent>
              ))}
            </div>
          </Tabs>
        </div>
      </SheetContent>
    </Sheet>
  )
}

interface ResponseViewerProps {
  data: ScrapeData | ScrapeData[] | null
  rawResponse?: unknown
  timeTakenMs?: number | null
  openRenderedInSheet?: boolean
}

export function ResponseViewer({
  data,
  rawResponse,
  timeTakenMs,
  openRenderedInSheet = false,
}: ResponseViewerProps) {
  const [copied, setCopied] = useState(false)
  const [selectedPage, setSelectedPage] = useState<PageItem | null>(null)
  const [showRendered, setShowRendered] = useState(false)

  if (!data) return null

  const isArray = Array.isArray(data)
  const items = (isArray ? data : [data]) as ScrapeData[]

  const copyToClipboard = () => {
    navigator.clipboard.writeText(JSON.stringify(rawResponse || data, null, 2))
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const pages: PageItem[] = items.map((item, index) => ({
    index,
    sourceURL: item.metadata?.sourceURL as string | undefined,
    url: item.metadata?.url as string | undefined,
    title: item.metadata?.title as string,
    description: item.metadata?.description as string,
    markdown: item.markdown,
    html: item.html,
    rawHtml: item.rawHtml,
    plainText: item.plainText,
    links: item.links,
    imageLinks: (item as Record<string, unknown>).imageLinks as
      | string[]
      | undefined,
    json: (item as Record<string, unknown>).json as string | undefined,
    metadata: item.metadata as PageItem["metadata"],
  }))

  const handleRenderedClick = () => {
    if (openRenderedInSheet && pages.length > 0) {
      setSelectedPage(pages[0])
      return
    }

    setShowRendered(!showRendered)
  }

  return (
    <Card className="relative flex h-full min-w-0 flex-col gap-0 overflow-hidden bg-background/95 py-0">
      <CardHeader className="shrink-0 border-b-2 border-border px-4 py-4 sm:px-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-center gap-3">
            <h2 className="text-lg font-semibold">
              {isArray ? `${items.length} Results` : "Response"}
            </h2>
            {items[0]?.metadata && (
              <Badge variant="neutral">
                {String(items[0].metadata.statusCode || "N/A")}
              </Badge>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {timeTakenMs !== null && timeTakenMs !== undefined && (
              <Badge variant="neutral" className="text-xs">
                {timeTakenMs}ms
              </Badge>
            )}
            {showRendered && (
              <Badge variant="neutral" className="text-xs">
                {pages.length} {pages.length === 1 ? "page" : "pages"}
              </Badge>
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="neutral"
                  size="sm"
                  onClick={handleRenderedClick}
                >
                  <Eye className="mr-1 h-4 w-4" />
                  {showRendered ? "API" : "Rendered"}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">
                {showRendered
                  ? "View raw API response"
                  : "View rendered content"}
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="neutral" size="sm" onClick={copyToClipboard}>
                  {copied ? (
                    <Check className="mr-1 h-4 w-4" />
                  ) : (
                    <Copy className="mr-1 h-4 w-4" />
                  )}
                  {copied ? "Copied" : "Copy"}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">
                {copied ? "Copied!" : "Copy response to clipboard"}
              </TooltipContent>
            </Tooltip>
          </div>
        </div>
      </CardHeader>
      <CardContent className="min-h-0 flex-1 overflow-hidden px-4 py-4 sm:px-5">
        {showRendered ? (
          <div className="h-full overflow-hidden rounded-base border-2 border-border bg-secondary-background">
            <ul className="h-full divide-y divide-border overflow-auto">
              {pages.map((page, idx) => (
                <li
                  key={idx}
                  className="cursor-pointer p-3 transition-colors hover:bg-background"
                  onClick={() => setSelectedPage(page)}
                >
                  <div className="flex items-start gap-3">
                    <span className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-main/10 text-xs font-medium text-main">
                      {idx + 1}
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="line-clamp-1 text-sm font-medium text-foreground">
                        {page.metadata?.title ||
                          page.title ||
                          `Page ${idx + 1}`}
                      </p>
                      {page.metadata?.sourceURL && (
                        <p className="text-muted-foreground mt-0.5 truncate text-xs">
                          {String(page.metadata.sourceURL)}
                        </p>
                      )}
                      <div className="mt-2 flex flex-wrap gap-2">
                        {page.markdown && (
                          <Badge variant="neutral" className="text-xs">
                            markdown
                          </Badge>
                        )}
                        {page.html && (
                          <Badge variant="neutral" className="text-xs">
                            html
                          </Badge>
                        )}
                        {page.rawHtml && (
                          <Badge variant="neutral" className="text-xs">
                            rawHtml
                          </Badge>
                        )}
                        {page.plainText && (
                          <Badge variant="neutral" className="text-xs">
                            plainText
                          </Badge>
                        )}
                        {page.links && page.links.length > 0 && (
                          <Badge variant="neutral" className="text-xs">
                            {page.links.length} links
                          </Badge>
                        )}
                        {page.imageLinks && page.imageLinks.length > 0 && (
                          <Badge variant="neutral" className="text-xs">
                            {page.imageLinks.length} images
                          </Badge>
                        )}
                      </div>
                    </div>
                    <div className="flex flex-shrink-0 items-center gap-2">
                      {page.metadata?.statusCode && (
                        <Badge
                          variant={
                            page.metadata.statusCode === 200
                              ? "default"
                              : "neutral"
                          }
                          className="text-xs"
                        >
                          {page.metadata.statusCode}
                        </Badge>
                      )}
                      {page.metadata?.renderedMode && (
                        <Badge variant="neutral" className="text-xs">
                          {page.metadata.renderedMode}
                        </Badge>
                      )}
                      <Eye className="text-muted-foreground h-4 w-4" />
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          </div>
        ) : (
          <pre className="h-full max-w-full overflow-auto rounded-base border-2 border-border bg-secondary-background p-4 font-mono text-sm break-words whitespace-pre-wrap">
            {JSON.stringify(rawResponse || data, null, 2)}
          </pre>
        )}
      </CardContent>

      {selectedPage && (
        <PageSheet
          page={selectedPage}
          open={!!selectedPage}
          onClose={() => setSelectedPage(null)}
        />
      )}
    </Card>
  )
}

export function MapResponseViewer({
  data,
  rawResponse,
  timeTakenMs,
}: {
  data: MapResponse | null
  rawResponse?: unknown
  timeTakenMs?: number | null
}) {
  const [copied, setCopied] = useState(false)
  const [showRendered, setShowRendered] = useState(false)

  if (!data) return null

  const copyToClipboard = () => {
    navigator.clipboard.writeText(JSON.stringify(rawResponse || data, null, 2))
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <Card className="flex h-full min-w-0 flex-col gap-0 overflow-hidden bg-background/95 py-0">
      <CardHeader className="shrink-0 border-b-2 border-border px-4 py-4 sm:px-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <h2 className="text-lg font-semibold">URLs</h2>
            <Badge variant="neutral">{data.links?.length || 0}</Badge>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {timeTakenMs !== null && timeTakenMs !== undefined && (
              <Badge variant="neutral" className="text-xs">
                {timeTakenMs}ms
              </Badge>
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="neutral"
                  size="sm"
                  onClick={() => setShowRendered(!showRendered)}
                >
                  <Eye className="mr-1 h-4 w-4" />
                  {showRendered ? "API" : "Rendered"}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">
                {showRendered ? "View raw API response" : "View rendered URLs"}
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="neutral" size="sm" onClick={copyToClipboard}>
                  {copied ? (
                    <Check className="mr-1 h-4 w-4" />
                  ) : (
                    <Copy className="mr-1 h-4 w-4" />
                  )}
                  {copied ? "Copied" : "Copy"}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">
                {copied ? "Copied!" : "Copy response to clipboard"}
              </TooltipContent>
            </Tooltip>
          </div>
        </div>
      </CardHeader>
      <CardContent className="min-h-0 flex-1 overflow-hidden px-4 py-4 sm:px-5">
        {showRendered ? (
          <div className="h-full overflow-auto rounded-base border-2 border-border bg-secondary-background">
            <ul className="divide-y divide-border">
              {data.links?.map((link, i) => (
                <li key={i} className="p-3">
                  <a
                    href={link}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center gap-2 text-sm break-all text-main hover:text-main/80"
                  >
                    <ExternalLink className="h-4 w-4 flex-shrink-0" />
                    {link}
                  </a>
                </li>
              ))}
            </ul>
          </div>
        ) : (
          <pre className="h-full max-w-full overflow-auto rounded-base border-2 border-border bg-secondary-background p-4 font-mono text-sm break-words whitespace-pre-wrap">
            {JSON.stringify(rawResponse || data, null, 2)}
          </pre>
        )}
      </CardContent>
    </Card>
  )
}

export function SearchResponseViewer({
  data,
  rawResponse,
  timeTakenMs,
}: {
  data: SearchResponse | null
  rawResponse?: unknown
  timeTakenMs?: number | null
}) {
  const [copied, setCopied] = useState(false)
  const [expandedResult, setExpandedResult] = useState<number | null>(null)
  const [showRendered, setShowRendered] = useState(false)

  if (!data) return null

  const copyToClipboard = () => {
    navigator.clipboard.writeText(JSON.stringify(rawResponse || data, null, 2))
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <Card className="flex h-full min-w-0 flex-col gap-0 overflow-hidden bg-background/95 py-0">
      <CardHeader className="shrink-0 border-b-2 border-border px-4 py-4 sm:px-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <h2 className="text-lg font-semibold">Results</h2>
            <Badge variant="neutral">{data.results?.length || 0}</Badge>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {timeTakenMs !== null && timeTakenMs !== undefined && (
              <Badge variant="neutral" className="text-xs">
                {timeTakenMs}ms
              </Badge>
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="neutral"
                  size="sm"
                  onClick={() => setShowRendered(!showRendered)}
                >
                  <Eye className="mr-1 h-4 w-4" />
                  {showRendered ? "API" : "Rendered"}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">
                {showRendered
                  ? "View raw API response"
                  : "View rendered results"}
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="neutral" size="sm" onClick={copyToClipboard}>
                  {copied ? (
                    <Check className="mr-1 h-4 w-4" />
                  ) : (
                    <Copy className="mr-1 h-4 w-4" />
                  )}
                  {copied ? "Copied" : "Copy"}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="top">
                {copied ? "Copied!" : "Copy response to clipboard"}
              </TooltipContent>
            </Tooltip>
          </div>
        </div>
      </CardHeader>
      <CardContent className="min-h-0 flex-1 overflow-hidden px-4 py-4 sm:px-5">
        {showRendered ? (
          <div className="h-full overflow-auto rounded-base border-2 border-border bg-secondary-background">
            <ul className="divide-y divide-border">
              {data.results?.map((result, i) => (
                <li key={i} className="p-4">
                  <div className="flex items-start gap-2">
                    <ExternalLink className="mt-0.5 h-4 w-4 flex-shrink-0 text-main" />
                    <div className="min-w-0 flex-1">
                      <a
                        href={result.url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="font-medium text-main hover:text-main/80"
                      >
                        {result.title}
                      </a>
                      {result.snippet && (
                        <p className="text-muted-foreground mt-1 line-clamp-2 text-sm">
                          {result.snippet}
                        </p>
                      )}
                      <div className="mt-2 flex flex-wrap gap-2">
                        {result.markdown && (
                          <Badge variant="neutral">markdown</Badge>
                        )}
                        {result.html && <Badge variant="neutral">html</Badge>}
                        {result.rawHtml && (
                          <Badge variant="neutral">rawHtml</Badge>
                        )}
                        {result.plainText && (
                          <Badge variant="neutral">plainText</Badge>
                        )}
                        {result.links && result.links.length > 0 && (
                          <Badge variant="neutral">
                            {result.links.length} links
                          </Badge>
                        )}
                      </div>

                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="noShadow"
                            size="sm"
                            className="mt-2"
                            onClick={() =>
                              setExpandedResult(expandedResult === i ? null : i)
                            }
                          >
                            {expandedResult === i
                              ? "Hide Content"
                              : "Show Content"}
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent side="top">
                          {expandedResult === i
                            ? "Hide result content"
                            : "Show result content"}
                        </TooltipContent>
                      </Tooltip>

                      {expandedResult === i && (
                        <div className="mt-4 space-y-4">
                          {result.markdown && (
                            <div>
                              <Label className="text-muted-foreground mb-1 block text-xs font-semibold uppercase">
                                Markdown
                              </Label>
                              <pre className="max-h-[200px] overflow-auto rounded border bg-background p-3 text-xs">
                                {result.markdown}
                              </pre>
                            </div>
                          )}
                          {result.html && (
                            <div>
                              <Label className="text-muted-foreground mb-1 block text-xs font-semibold uppercase">
                                HTML
                              </Label>
                              <pre
                                className="max-h-[200px] overflow-auto rounded border bg-background p-3 text-xs"
                                dangerouslySetInnerHTML={{
                                  __html:
                                    result.html.slice(0, 500) +
                                    (result.html.length > 500 ? "..." : ""),
                                }}
                              />
                            </div>
                          )}
                          {result.plainText && (
                            <div>
                              <Label className="text-muted-foreground mb-1 block text-xs font-semibold uppercase">
                                Plain Text
                              </Label>
                              <pre className="max-h-[200px] overflow-auto rounded border bg-background p-3 text-xs">
                                {result.plainText}
                              </pre>
                            </div>
                          )}
                          {result.links && result.links.length > 0 && (
                            <div>
                              <Label className="text-muted-foreground mb-1 block text-xs font-semibold uppercase">
                                Links ({result.links.length})
                              </Label>
                              <ul className="max-h-[200px] space-y-1 overflow-auto rounded border bg-background p-3 text-xs">
                                {result.links.slice(0, 20).map((link, j) => (
                                  <li key={j} className="truncate">
                                    <a
                                      href={link}
                                      target="_blank"
                                      rel="noopener noreferrer"
                                      className="text-main hover:text-main/80"
                                    >
                                      {link}
                                    </a>
                                  </li>
                                ))}
                                {result.links.length > 20 && (
                                  <li className="text-muted-foreground">
                                    ...and {result.links.length - 20} more
                                  </li>
                                )}
                              </ul>
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          </div>
        ) : (
          <pre className="h-full max-w-full overflow-auto rounded-base border-2 border-border bg-secondary-background p-4 font-mono text-sm break-words whitespace-pre-wrap">
            {JSON.stringify(rawResponse || data, null, 2)}
          </pre>
        )}
      </CardContent>
    </Card>
  )
}
