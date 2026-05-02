"use client";

import { useState } from "react";
import { ExternalLink, Copy, Check } from "lucide-react";
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
          <div className="bg-white p-6 rounded-base border-2 border-border h-full overflow-auto markdown-content relative">
            <ReactMarkdown>{page.markdown}</ReactMarkdown>
            <CopyButton />
          </div>
        ) : (
          <div className="text-gray-500 text-sm p-6 bg-white rounded-base border-2 border-border relative">
            No markdown content available
            <CopyButton />
          </div>
        );
      case "html":
        return page.html ? (
          <div className="bg-white rounded-base border-2 border-border h-full overflow-auto relative">
            <pre className="p-6 text-sm font-mono whitespace-pre-wrap">{page.html}</pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-gray-500 text-sm p-6 bg-white rounded-base border-2 border-border relative">
            No HTML content available
            <CopyButton />
          </div>
        );
      case "rawHtml":
        return page.rawHtml ? (
          <div className="bg-white rounded-base border-2 border-border h-full overflow-auto relative">
            <pre className="p-6 text-sm font-mono whitespace-pre-wrap">{page.rawHtml}</pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-gray-500 text-sm p-6 bg-white rounded-base border-2 border-border relative">
            No raw HTML content available
            <CopyButton />
          </div>
        );
      case "plainText":
        return page.plainText ? (
          <div className="bg-white rounded-base border-2 border-border h-full overflow-auto relative">
            <pre className="p-6 text-sm font-mono whitespace-pre-wrap">{page.plainText}</pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-gray-500 text-sm p-6 bg-white rounded-base border-2 border-border relative">
            No plain text content available
            <CopyButton />
          </div>
        );
      case "links":
        return page.links && page.links.length > 0 ? (
          <div className="bg-white rounded-base border-2 border-border h-full overflow-auto relative">
            <ul className="divide-y divide-border">
              {page.links.map((link, i) => (
                <li key={i} className="p-3">
                  <a href={link} target="_blank" rel="noopener noreferrer" className="flex items-center gap-2 text-blue-600 hover:text-blue-800 text-sm break-all">
                    <ExternalLink className="w-4 h-4 flex-shrink-0" />
                    {link}
                  </a>
                </li>
              ))}
            </ul>
            <CopyButton />
          </div>
        ) : (
          <div className="text-gray-500 text-sm p-6 bg-white rounded-base border-2 border-border relative">
            No links available
            <CopyButton />
          </div>
        );
      case "json":
        return page.json ? (
          <div className="bg-white rounded-base border-2 border-border h-full overflow-auto relative">
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
          <div className="text-gray-500 text-sm p-6 bg-white rounded-base border-2 border-border relative">
            No JSON extraction available
            <CopyButton />
          </div>
        );
      case "metadata":
        return page.metadata ? (
          <div className="bg-white rounded-base border-2 border-border h-full overflow-auto relative">
            <pre className="p-6 text-sm font-mono whitespace-pre-wrap">{JSON.stringify(page.metadata, null, 2)}</pre>
            <CopyButton />
          </div>
        ) : (
          <div className="text-gray-500 text-sm p-6 bg-white rounded-base border-2 border-border relative">
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
            <p className="text-sm text-gray-600 line-clamp-2 mb-3">
              {String(page.metadata.description)}
            </p>
          )}
          {page.metadata?.sourceURL && (
            <a
              href={String(page.metadata.sourceURL)}
              target="_blank"
              rel="noopener noreferrer"
              className="text-sm text-blue-600 hover:text-blue-800 flex items-center gap-2 truncate block mb-2"
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
              <span className="text-sm text-gray-500">
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
}

export function ResponseViewer({ data, rawResponse }: ResponseViewerProps) {
  const [copied, setCopied] = useState(false);
  const [selectedPage, setSelectedPage] = useState<PageItem | null>(null);

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
    json: (item as Record<string, unknown>).json as string | undefined,
    metadata: item.metadata as PageItem["metadata"],
  }));

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-4">
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
        <Button variant="neutral" size="sm" onClick={copyToClipboard}>
          {copied ? <Check className="w-4 h-4 mr-1" /> : <Copy className="w-4 h-4 mr-1" />}
          {copied ? "Copied" : "Copy"}
        </Button>
      </CardHeader>
      <CardContent className="pt-0">
        <Tabs defaultValue="rendered" className="w-full">
          <TabsList className="mb-4 overflow-x-auto flex-nowrap">
            <TabsTrigger value="api">API</TabsTrigger>
            <TabsTrigger value="rendered">Rendered</TabsTrigger>
          </TabsList>

          <TabsContent value="api">
            <pre className="bg-white p-4 rounded-base border-2 border-border text-sm overflow-auto max-h-[450px] font-mono">
              {JSON.stringify(rawResponse || data, null, 2)}
            </pre>
          </TabsContent>

          <TabsContent value="rendered">
            <div className="border-2 border-border rounded-base overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-12">#</TableHead>
                    <TableHead className="min-w-[150px]">Source URL</TableHead>
                    <TableHead className="min-w-[200px]">Preview</TableHead>
                    <TableHead className="w-20">Status</TableHead>
                    <TableHead className="w-24">Time</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {pages.map((page) => (
                    <TableRow
                      key={page.index}
                      onClick={() => setSelectedPage(page)}
                      className="cursor-pointer"
                    >
                      <TableCell className="text-muted-foreground">{page.index + 1}</TableCell>
                      <TableCell className="max-w-[200px]">
                        <a
                          href={String(page.metadata?.sourceURL || page.metadata?.url || page.url || "")}
                          target="_blank"
                          rel="noopener noreferrer"
                          onClick={(e) => e.stopPropagation()}
                          className="text-blue-600 hover:text-blue-800 hover:underline line-clamp-2"
                        >
                          {String(page.metadata?.sourceURL || page.metadata?.url || page.url || "N/A")}
                        </a>
                      </TableCell>
                      <TableCell className="max-w-[300px] text-muted-foreground line-clamp-3">
                        {page.markdown || page.plainText || "-"}
                      </TableCell>
                      <TableCell>
                        <Badge variant="neutral">
                          {page.metadata?.statusCode || "N/A"}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {page.metadata?.titmeTaken ? `${page.metadata.titmeTaken}ms` : "-"}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </TabsContent>
        </Tabs>
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

export function MapResponseViewer({ data, rawResponse }: { data: MapResponse | null; rawResponse?: unknown }) {
  const [copied, setCopied] = useState(false);

  if (!data) return null;

  const copyToClipboard = () => {
    navigator.clipboard.writeText(JSON.stringify(rawResponse || data, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-4">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold">Discovered URLs</h2>
          <Badge variant="neutral">{data.links?.length || 0}</Badge>
        </div>
        <Button variant="neutral" size="sm" onClick={copyToClipboard}>
          {copied ? <Check className="w-4 h-4 mr-1" /> : <Copy className="w-4 h-4 mr-1" />}
          {copied ? "Copied" : "Copy"}
        </Button>
      </CardHeader>
      <CardContent className="pt-0">
        <Tabs defaultValue="rendered" className="w-full">
          <TabsList className="mb-4 overflow-x-auto flex-nowrap">
            <TabsTrigger value="api">API</TabsTrigger>
            <TabsTrigger value="rendered">Rendered</TabsTrigger>
          </TabsList>

          <TabsContent value="api">
            <pre className="bg-white p-4 rounded-base border-2 border-border text-sm overflow-auto max-h-[400px] font-mono">
              {JSON.stringify(rawResponse || data, null, 2)}
            </pre>
          </TabsContent>

          <TabsContent value="rendered">
            <div className="bg-white rounded-base border-2 border-border max-h-[400px] overflow-auto">
              <ul className="divide-y divide-border">
                {data.links?.map((link, i) => (
                  <li key={i} className="p-3">
                    <a href={link} target="_blank" rel="noopener noreferrer" className="flex items-center gap-2 text-blue-600 hover:text-blue-800 text-sm break-all">
                      <ExternalLink className="w-4 h-4 flex-shrink-0" />
                      {link}
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}

export function SearchResponseViewer({ data, rawResponse }: { data: SearchResponse | null; rawResponse?: unknown }) {
  const [copied, setCopied] = useState(false);
  const [expandedResult, setExpandedResult] = useState<number | null>(null);

  if (!data) return null;

  const copyToClipboard = () => {
    navigator.clipboard.writeText(JSON.stringify(rawResponse || data, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-4">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold">Search Results</h2>
          <Badge variant="neutral">{data.results?.length || 0}</Badge>
        </div>
        <Button variant="neutral" size="sm" onClick={copyToClipboard}>
          {copied ? <Check className="w-4 h-4 mr-1" /> : <Copy className="w-4 h-4 mr-1" />}
          {copied ? "Copied" : "Copy"}
        </Button>
      </CardHeader>
      <CardContent className="pt-0">
        <Tabs defaultValue="rendered" className="w-full">
          <TabsList className="mb-4 overflow-x-auto flex-nowrap">
            <TabsTrigger value="api">API</TabsTrigger>
            <TabsTrigger value="rendered">Rendered</TabsTrigger>
          </TabsList>

          <TabsContent value="api">
            <pre className="bg-white p-4 rounded-base border-2 border-border text-sm overflow-auto max-h-[400px] font-mono">
              {JSON.stringify(rawResponse || data, null, 2)}
            </pre>
          </TabsContent>

          <TabsContent value="rendered">
            <div className="bg-white rounded-base border-2 border-border max-h-[600px] overflow-auto">
              <ul className="divide-y divide-border">
                {data.results?.map((result, i) => (
                  <li key={i} className="p-4">
                    <div className="flex items-start gap-2">
                      <ExternalLink className="w-4 h-4 flex-shrink-0 mt-0.5 text-blue-600" />
                      <div className="flex-1 min-w-0">
                        <a href={result.url} target="_blank" rel="noopener noreferrer" className="font-medium text-blue-600 hover:text-blue-800">
                          {result.title}
                        </a>
                        {result.description && (
                          <p className="text-sm text-gray-600 mt-1 line-clamp-2">{result.description}</p>
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
                                <Label className="text-xs font-semibold text-gray-500 uppercase mb-1 block">Markdown</Label>
                                <pre className="bg-gray-50 p-3 rounded border text-xs overflow-auto max-h-[200px]">{result.markdown}</pre>
                              </div>
                            )}
                            {result.html && (
                              <div>
                                <Label className="text-xs font-semibold text-gray-500 uppercase mb-1 block">HTML</Label>
                                <pre className="bg-gray-50 p-3 rounded border text-xs overflow-auto max-h-[200px]" dangerouslySetInnerHTML={{ __html: result.html.slice(0, 500) + (result.html.length > 500 ? "..." : "") }} />
                              </div>
                            )}
                            {result.plainText && (
                              <div>
                                <Label className="text-xs font-semibold text-gray-500 uppercase mb-1 block">Plain Text</Label>
                                <pre className="bg-gray-50 p-3 rounded border text-xs overflow-auto max-h-[200px]">{result.plainText}</pre>
                              </div>
                            )}
                            {result.links && result.links.length > 0 && (
                              <div>
                                <Label className="text-xs font-semibold text-gray-500 uppercase mb-1 block">Links ({result.links.length})</Label>
                                <ul className="bg-gray-50 p-3 rounded border text-xs overflow-auto max-h-[200px] space-y-1">
                                  {result.links.slice(0, 20).map((link, j) => (
                                    <li key={j} className="truncate">
                                      <a href={link} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:text-blue-800">{link}</a>
                                    </li>
                                  ))}
                                  {result.links.length > 20 && (
                                    <li className="text-gray-500">...and {result.links.length - 20} more</li>
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
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}