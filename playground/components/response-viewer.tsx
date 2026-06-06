"use client";

import { useState } from "react";
import { ExternalLink, Copy, Check, Eye, Image } from "lucide-react";
import ReactMarkdown from "react-markdown";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from "@/components/ui/sheet";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { ScrapeData, MapResponse, SearchResponse, SearchResult } from "@/lib/api-types";

interface PageItem {
  index: number;
  url?: string;
  title?: string;
  description?: string;
  markdown?: string;
  html?: string;
  rawHtml?: string;
  plainText?: string;
  links?: string[];
  imageLinks?: string[];
  json?: string;
  metadata?: {
    title?: string;
    description?: string;
    ogpTitle?: string;
    ogpDescription?: string;
    ogpImage?: string;
    canonicalUrl?: string;
    sourceURL?: string;
    url?: string;
    language?: string;
    statusCode?: number;
    renderedMode?: string;
    timeTaken?: number;
    [key: string]: unknown;
  };
}

interface PageSheetProps {
  page: PageItem;
  open: boolean;
  onClose: () => void;
}

function PageSheet({ page, open, onClose }: PageSheetProps) {
  const [activeTab, setActiveTab] = useState("");
  const [showCopiedAlert, setShowCopiedAlert] = useState(false);

  const availableTabs = [
    { id: "markdown", label: "Markdown", hasContent: !!page.markdown },
    { id: "html", label: "HTML", hasContent: !!page.html },
    { id: "rawHtml", label: "Raw HTML", hasContent: !!page.rawHtml },
    { id: "plainText", label: "Plain Text", hasContent: !!page.plainText },
    { id: "links", label: "Links", hasContent: !!(page.links && page.links.length > 0) },
    { id: "imageLinks", label: "Images", hasContent: !!(page.imageLinks && page.imageLinks.length > 0) },
    { id: "json", label: "JSON", hasContent: !!page.json },
    { id: "metadata", label: "Metadata", hasContent: !!page.metadata },
  ].filter((tab) => tab.hasContent);

  useState(() => {
    if (availableTabs.length > 0 && !activeTab) {
      setActiveTab(availableTabs[0].id);
    }
  });

  const getContent = () => {
    switch (activeTab) {
      case "markdown":
        return page.markdown || "";
      case "html":
        return page.html || "";
      case "rawHtml":
        return page.rawHtml || "";
      case "plainText":
        return page.plainText || "";
      case "links":
        return page.links ? page.links.join("\n") : "";
      case "imageLinks":
        return page.imageLinks ? page.imageLinks.join("\n") : "";
      case "json":
        return typeof page.json === "string" ? page.json : JSON.stringify(page.json, null, 2);
      case "metadata":
        return page.metadata ? JSON.stringify(page.metadata, null, 2) : "";
      default:
        return "";
    }
  };

  const copyContent = () => {
    const content = getContent();
    if (content) {
      navigator.clipboard.writeText(content);
      setShowCopiedAlert(true);
      setTimeout(() => setShowCopiedAlert(false), 2000);
    }
  };

  const hasContent = () => {
    return availableTabs.some((tab) => tab.id === activeTab && tab.hasContent);
  };

  const CopyButton = () => (
    <Button variant="neutral" size="sm" onClick={copyContent} disabled={!hasContent()} className="absolute top-3 right-3">
      {showCopiedAlert ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
    </Button>
  );

  const renderContent = () => {
    switch (activeTab) {
      case "markdown":
        return page.markdown ? (
          <div className="bg-secondary-background p-6 rounded-base border-2 border-border h-full overflow-auto markdown-content relative">
            <ReactMarkdown>{page.markdown}</ReactMarkdown>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground text-sm p-6 bg-secondary-background rounded-base border-2 border-border relative">
            No markdown content available
            <CopyButton />
          </div>
        );
      case "html":
        return page.html ? (
          <div className="bg-secondary-background rounded-base border-2 border-border h-full overflow-auto relative">
            <pre className="p-6 text-sm font-mono whitespace-pre-wrap">{page.html}</pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground text-sm p-6 bg-secondary-background rounded-base border-2 border-border relative">
            No HTML content available
            <CopyButton />
          </div>
        );
      case "rawHtml":
        return page.rawHtml ? (
          <div className="bg-secondary-background rounded-base border-2 border-border h-full overflow-auto relative">
            <pre className="p-6 text-sm font-mono whitespace-pre-wrap">{page.rawHtml}</pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground text-sm p-6 bg-secondary-background rounded-base border-2 border-border relative">
            No raw HTML content available
            <CopyButton />
          </div>
        );
      case "plainText":
        return page.plainText ? (
          <div className="bg-secondary-background rounded-base border-2 border-border h-full overflow-auto relative">
            <pre className="p-6 text-sm font-mono whitespace-pre-wrap">{page.plainText}</pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground text-sm p-6 bg-secondary-background rounded-base border-2 border-border relative">
            No plain text content available
            <CopyButton />
          </div>
        );
      case "links":
        return page.links && page.links.length > 0 ? (
          <div className="bg-secondary-background rounded-base border-2 border-border h-full overflow-auto relative">
            <ul className="divide-y divide-border">
              {page.links.map((link, i) => (
                <li key={i} className="p-3">
                  <a href={link} target="_blank" rel="noopener noreferrer" className="flex items-center gap-2 text-main hover:text-main/80 text-sm break-all">
                    <ExternalLink className="w-4 h-4 flex-shrink-0" />
                    {link}
                  </a>
                </li>
              ))}
            </ul>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground text-sm p-6 bg-secondary-background rounded-base border-2 border-border relative">
            No links available
            <CopyButton />
          </div>
        );
      case "imageLinks":
        return page.imageLinks && page.imageLinks.length > 0 ? (
          <div className="bg-secondary-background rounded-base border-2 border-border h-full overflow-auto relative p-4">
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-4">
              {page.imageLinks.map((img, i) => (
                <div key={i} className="group relative aspect-square bg-background rounded-lg overflow-hidden border border-border">
                  <img
                    src={img}
                    alt={`Image ${i + 1}`}
                    className="w-full h-full object-cover object-center"
                    loading="lazy"
                  />
                  <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                    <a
                      href={img}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="p-2 bg-background rounded-full"
                    >
                      <ExternalLink className="w-5 h-5 text-foreground" />
                    </a>
                  </div>
                </div>
              ))}
            </div>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground text-sm p-6 bg-secondary-background rounded-base border-2 border-border relative">
            No images available
            <CopyButton />
          </div>
        );
      case "json":
        return page.json ? (
          <div className="bg-secondary-background rounded-base border-2 border-border h-full overflow-auto relative">
            <pre className="p-6 text-sm font-mono whitespace-pre-wrap">
              {typeof page.json === "string"
                ? (() => {
                    try {
                      return JSON.stringify(JSON.parse(page.json), null, 2);
                    } catch {
                      return page.json;
                    }
                  })()
                : JSON.stringify(page.json, null, 2)}
            </pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground text-sm p-6 bg-secondary-background rounded-base border-2 border-border relative">
            No JSON extraction available
            <CopyButton />
          </div>
        );
      case "metadata":
        return page.metadata ? (
          <div className="bg-secondary-background rounded-base border-2 border-border h-full overflow-auto relative">
            <pre className="p-6 text-sm font-mono whitespace-pre-wrap">{JSON.stringify(page.metadata, null, 2)}</pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-muted-foreground text-sm p-6 bg-secondary-background rounded-base border-2 border-border relative">
            No metadata available
            <CopyButton />
          </div>
        );
      default:
        return null;
    }
  };

  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()}>
      <SheetContent side="right" className="w-[95vw] sm:max-w-[95vw] md:max-w-[600px] overflow-hidden flex flex-col p-0">
        <div className="flex-shrink-0 p-6 border-b-2 border-border">
          <SheetTitle className="text-xl font-bold line-clamp-2 mb-2">
            {page.metadata?.title || page.title || `Page ${page.index + 1}`}
          </SheetTitle>
          {page.metadata?.description && (
            <p className="text-sm text-muted-foreground line-clamp-2 mb-3">
              {String(page.metadata.description)}
            </p>
          )}
          {page.metadata?.sourceURL && (
            <a
              href={String(page.metadata.sourceURL)}
              target="_blank"
              rel="noopener noreferrer"
              className="text-sm text-main hover:text-main/80 flex items-center gap-2 truncate block mb-2"
            >
              <ExternalLink className="w-4 h-4 flex-shrink-0" />
              <span className="truncate">{String(page.metadata.sourceURL)}</span>
            </a>
          )}
          <div className="flex items-center gap-3 mt-3">
            <Badge variant="neutral" className="text-sm">
              {page.metadata?.statusCode || "N/A"}
            </Badge>
            {page.metadata?.timeTaken && (
              <span className="text-sm text-muted-foreground">
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

        <div className="flex-1 overflow-hidden flex flex-col p-6">
          {showCopiedAlert && (
            <Alert className="mb-4">
              <Check className="size-4" />
              <AlertTitle>Copied to clipboard</AlertTitle>
            </Alert>
          )}
          <Tabs value={activeTab} onValueChange={setActiveTab} className="flex-1 flex flex-col overflow-hidden">
            <TabsList className="flex-shrink-0 flex-nowrap mb-4 overflow-x-auto">
              {availableTabs.map((tab) => (
                <TabsTrigger key={tab.id} value={tab.id}>
                  {tab.label}
                </TabsTrigger>
              ))}
            </TabsList>

            <div className="flex-1 overflow-auto">
              {availableTabs.map((tab) => (
                <TabsContent key={tab.id} value={tab.id} className="h-full m-0">
                  {renderContent()}
                </TabsContent>
              ))}
            </div>
          </Tabs>
        </div>
      </SheetContent>
    </Sheet>
  );
}

interface ResponseViewerProps {
  data: ScrapeData | ScrapeData[] | null;
  rawResponse?: unknown;
  timeTakenMs?: number | null;
}

export function ResponseViewer({ data, rawResponse, timeTakenMs }: ResponseViewerProps) {
  const [copied, setCopied] = useState(false);
  const [selectedPage, setSelectedPage] = useState<PageItem | null>(null);
  const [showRendered, setShowRendered] = useState(false);

  if (!data) return null;

  const isArray = Array.isArray(data);
  const items = (isArray ? data : [data]) as ScrapeData[];

  const copyToClipboard = () => {
    navigator.clipboard.writeText(JSON.stringify(rawResponse || data, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

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
    imageLinks: (item as Record<string, unknown>).imageLinks as string[] | undefined,
    json: (item as Record<string, unknown>).json as string | undefined,
    metadata: item.metadata as PageItem["metadata"],
  }));

  return (
    <Card className="flex-1 flex flex-col overflow-hidden relative">
      <CardHeader className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-4 shrink-0">
        <div className="flex items-center gap-3">
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
          <Button variant="neutral" size="sm" onClick={() => setShowRendered(!showRendered)}>
            <Eye className="w-4 h-4 mr-1" />
            {showRendered ? "API" : "Rendered"}
          </Button>
          <Button variant="neutral" size="sm" onClick={copyToClipboard}>
            {copied ? <Check className="w-4 h-4 mr-1" /> : <Copy className="w-4 h-4 mr-1" />}
            {copied ? "Copied" : "Copy"}
          </Button>
        </div>
      </CardHeader>
<CardContent className="pt-0 flex-1 min-h-0 overflow-auto">
        {showRendered ? (
          <div className="bg-secondary-background rounded-base border-2 border-border overflow-hidden">
            <ul className="divide-y divide-border max-h-[500px] overflow-auto">
              {pages.map((page, idx) => (
                <li key={idx} className="p-3 hover:bg-background cursor-pointer transition-colors" onClick={() => setSelectedPage(page)}>
                  <div className="flex items-start gap-3">
                    <span className="flex-shrink-0 w-6 h-6 rounded-full bg-main/10 text-main text-xs flex items-center justify-center font-medium">
                      {idx + 1}
                    </span>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-foreground line-clamp-1">
                        {page.metadata?.title || page.title || `Page ${idx + 1}`}
                      </p>
                      {page.metadata?.sourceURL && (
                        <p className="text-xs text-muted-foreground truncate mt-0.5">{String(page.metadata.sourceURL)}</p>
                      )}
                      <div className="flex flex-wrap gap-2 mt-2">
                        {page.markdown && <Badge variant="neutral" className="text-xs">markdown</Badge>}
                        {page.html && <Badge variant="neutral" className="text-xs">html</Badge>}
                        {page.rawHtml && <Badge variant="neutral" className="text-xs">rawHtml</Badge>}
                        {page.plainText && <Badge variant="neutral" className="text-xs">plainText</Badge>}
                        {page.links && page.links.length > 0 && <Badge variant="neutral" className="text-xs">{page.links.length} links</Badge>}
                        {page.imageLinks && page.imageLinks.length > 0 && <Badge variant="neutral" className="text-xs">{page.imageLinks.length} images</Badge>}
                      </div>
                    </div>
                    <div className="flex-shrink-0 flex items-center gap-2">
                      {page.metadata?.statusCode && (
                        <Badge variant={page.metadata.statusCode === 200 ? "default" : "neutral"} className="text-xs">
                          {page.metadata.statusCode}
                        </Badge>
                      )}
                      {page.metadata?.renderedMode && (
                        <Badge variant="neutral" className="text-xs">
                          {page.metadata.renderedMode}
                        </Badge>
                      )}
                      <Eye className="w-4 h-4 text-muted-foreground" />
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          </div>
        ) : (
          <pre className="bg-secondary-background p-4 rounded-base border-2 border-border text-sm max-w-full h-[500px] overflow-auto font-mono whitespace-pre-wrap break-all">
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
  );
}

export function MapResponseViewer({ data, rawResponse, timeTakenMs }: { data: MapResponse | null; rawResponse?: unknown; timeTakenMs?: number | null }) {
  const [copied, setCopied] = useState(false);
  const [showRendered, setShowRendered] = useState(false);

  if (!data) return null;

  const copyToClipboard = () => {
    navigator.clipboard.writeText(JSON.stringify(rawResponse || data, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Card className="h-full flex flex-col overflow-hidden">
      <CardHeader className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-4 shrink-0">
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
          <Button variant="neutral" size="sm" onClick={() => setShowRendered(!showRendered)}>
            <Eye className="w-4 h-4 mr-1" />
            {showRendered ? "API" : "Rendered"}
          </Button>
          <Button variant="neutral" size="sm" onClick={copyToClipboard}>
            {copied ? <Check className="w-4 h-4 mr-1" /> : <Copy className="w-4 h-4 mr-1" />}
            {copied ? "Copied" : "Copy"}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="pt-0 flex-1 min-h-0 overflow-auto">
        {showRendered ? (
          <div className="bg-secondary-background rounded-base border-2 border-border max-h-[500px] overflow-auto">
            <ul className="divide-y divide-border">
              {data.links?.map((link, i) => (
                <li key={i} className="p-3">
                  <a href={link} target="_blank" rel="noopener noreferrer" className="flex items-center gap-2 text-main hover:text-main/80 text-sm break-all">
                    <ExternalLink className="w-4 h-4 flex-shrink-0" />
                    {link}
                  </a>
                </li>
              ))}
            </ul>
          </div>
        ) : (
          <pre className="bg-secondary-background p-4 rounded-base border-2 border-border text-sm max-w-full h-[500px] overflow-auto font-mono whitespace-pre-wrap break-all">
            {JSON.stringify(rawResponse || data, null, 2)}
          </pre>
        )}
      </CardContent>
    </Card>
  );
}

export function SearchResponseViewer({ data, rawResponse, timeTakenMs }: { data: SearchResponse | null; rawResponse?: unknown; timeTakenMs?: number | null }) {
  const [copied, setCopied] = useState(false);
  const [expandedResult, setExpandedResult] = useState<number | null>(null);
  const [showRendered, setShowRendered] = useState(false);

  if (!data) return null;

  const copyToClipboard = () => {
    navigator.clipboard.writeText(JSON.stringify(rawResponse || data, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Card className="h-full flex flex-col overflow-hidden">
      <CardHeader className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-4 shrink-0">
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
          <Button variant="neutral" size="sm" onClick={() => setShowRendered(!showRendered)}>
            <Eye className="w-4 h-4 mr-1" />
            {showRendered ? "API" : "Rendered"}
          </Button>
          <Button variant="neutral" size="sm" onClick={copyToClipboard}>
            {copied ? <Check className="w-4 h-4 mr-1" /> : <Copy className="w-4 h-4 mr-1" />}
            {copied ? "Copied" : "Copy"}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="pt-0 flex-1 min-h-0 overflow-auto">
        {showRendered ? (
          <div className="bg-secondary-background rounded-base border-2 border-border max-h-[500px] overflow-auto">
            <ul className="divide-y divide-border">
              {data.results?.map((result, i) => (
                <li key={i} className="p-4">
                  <div className="flex items-start gap-2">
                    <ExternalLink className="w-4 h-4 flex-shrink-0 mt-0.5 text-main" />
                    <div className="flex-1 min-w-0">
                      <a href={result.url} target="_blank" rel="noopener noreferrer" className="font-medium text-main hover:text-main/80">
                        {result.title}
                      </a>
                      {result.description && (
                        <p className="text-sm text-muted-foreground mt-1 line-clamp-2">{result.description}</p>
                      )}
                      <div className="flex flex-wrap gap-2 mt-2">
                        {result.markdown && <Badge variant="neutral">markdown</Badge>}
                        {result.html && <Badge variant="neutral">html</Badge>}
                        {result.rawHtml && <Badge variant="neutral">rawHtml</Badge>}
                        {result.plainText && <Badge variant="neutral">plainText</Badge>}
                        {result.links && result.links.length > 0 && <Badge variant="neutral">{result.links.length} links</Badge>}
                      </div>

                      <Button
                        variant="noShadow"
                        size="sm"
                        className="mt-2"
                        onClick={() => setExpandedResult(expandedResult === i ? null : i)}
                      >
                        {expandedResult === i ? "Hide Content" : "Show Content"}
                      </Button>

                      {expandedResult === i && (
                        <div className="mt-4 space-y-4">
                          {result.markdown && (
                            <div>
                              <Label className="text-xs font-semibold text-muted-foreground uppercase mb-1 block">Markdown</Label>
                              <pre className="bg-background p-3 rounded border text-xs overflow-auto max-h-[200px]">{result.markdown}</pre>
                            </div>
                          )}
                          {result.html && (
                            <div>
                              <Label className="text-xs font-semibold text-muted-foreground uppercase mb-1 block">HTML</Label>
                              <pre className="bg-background p-3 rounded border text-xs overflow-auto max-h-[200px]" dangerouslySetInnerHTML={{ __html: result.html.slice(0, 500) + (result.html.length > 500 ? "..." : "") }} />
                            </div>
                          )}
                          {result.plainText && (
                            <div>
                              <Label className="text-xs font-semibold text-muted-foreground uppercase mb-1 block">Plain Text</Label>
                              <pre className="bg-background p-3 rounded border text-xs overflow-auto max-h-[200px]">{result.plainText}</pre>
                            </div>
                          )}
                          {result.links && result.links.length > 0 && (
                            <div>
                              <Label className="text-xs font-semibold text-muted-foreground uppercase mb-1 block">Links ({result.links.length})</Label>
                              <ul className="bg-background p-3 rounded border text-xs overflow-auto max-h-[200px] space-y-1">
                                {result.links.slice(0, 20).map((link, j) => (
                                  <li key={j} className="truncate">
                                    <a href={link} target="_blank" rel="noopener noreferrer" className="text-main hover:text-main/80">{link}</a>
                                  </li>
                                ))}
                                {result.links.length > 20 && (
                                  <li className="text-muted-foreground">...and {result.links.length - 20} more</li>
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
          <pre className="bg-secondary-background p-4 rounded-base border-2 border-border text-sm max-w-full h-[500px] overflow-auto font-mono whitespace-pre-wrap break-all">
            {JSON.stringify(rawResponse || data, null, 2)}
          </pre>
        )}
      </CardContent>
    </Card>
  );
}