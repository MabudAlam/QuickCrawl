"use client";

import { useState, useCallback, useEffect } from "react";
import { Loader2, Play, Copy, Check, X, ChevronDown, ChevronUp, Minus, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Slider } from "@/components/ui/slider";
import { Checkbox } from "@/components/ui/checkbox";
import { Badge } from "@/components/ui/badge";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from "@/components/ui/dropdown-menu";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { AppSidebar } from "./sidebar";
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
} from "@/lib/api-client";
import type {
  Endpoint,
  ScrapeRequest,
  CrawlRequest,
  MapRequest,
  SearchRequest,
  APIResponse,
  CrawlState,
  HealthResponse,
  Format,
  ScrapeData,
  MapResponse,
  SearchResponse,
  ScrapeOptions,
  CrawlOptions,
} from "@/lib/api-types";
import { ResponseViewer, MapResponseViewer, SearchResponseViewer } from "@/components/response-viewer";

interface PlaygroundPageProps {
  initialBaseUrl?: string;
}

export default function PlaygroundPage({ initialBaseUrl }: PlaygroundPageProps) {
  const [endpoint, setEndpoint] = useState<Endpoint>("scrape");
  const [url, setUrl] = useState("");
  const [baseUrl] = useState(initialBaseUrl);

  const [isLoading, setIsLoading] = useState(false);
  const [response, setResponse] = useState<APIResponse<unknown> | null>(null);
  const [crawlId, setCrawlId] = useState<string | null>(null);
  const [crawlStatus, setCrawlStatus] = useState<CrawlState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [copiedSnippet, setCopiedSnippet] = useState<string | null>(null);
  const [timeTakenMs, setTimeTakenMs] = useState<number | null>(null)
  const [codeLanguage, setCodeLanguage] = useState<"curl" | "fetch" | "python">("curl");
  const [codeSnippetExpanded, setCodeSnippetExpanded] = useState(false);
  const [advancedExpanded, setAdvancedExpanded] = useState(false);
  const [schemaBuilderOpen, setSchemaBuilderOpen] = useState(false);
  const [schemaFields, setSchemaFields] = useState<{ name: string; type: string; description: string; itemType?: string }[]>([
    { name: "title", type: "string", description: "" },
  ]);

  const [scrapeOptions, setScrapeOptions] = useState<ScrapeOptions>({
    formats: ["markdown"] as Format[],
    renderJs: null,
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
  });

  const generateSchema = () => {
    const properties: Record<string, { type: string; description?: string; items?: { type: string } }> = {};
    const required: string[] = [];

    schemaFields.forEach((field) => {
      if (field.name.trim()) {
        if (field.type === "array") {
          properties[field.name] = {
            type: "array",
            items: { type: field.itemType || "string" },
            ...(field.description ? { description: field.description } : {}),
          };
        } else {
          properties[field.name] = {
            type: field.type,
            ...(field.description ? { description: field.description } : {}),
          };
        }
        required.push(field.name);
      }
    });

    return JSON.stringify(
      {
        type: "object",
        properties,
        required,
        additionalProperties: false,
      },
      null,
      2
    );
  };

  const [crawlOptions, setCrawlOptions] = useState<CrawlOptions>({
    maxDepth: 2,
    maxPages: 100,
    formats: ["markdown"] as Format[],
    renderJs: null,
    waitFor: 0,
    includeTags: "",
    excludeTags: "",
    maxMarkdownChars: undefined,
  });

  const [mapOptions, setMapOptions] = useState({
    maxDepth: 2,
    useSitemap: true,
    timeout: 30000,
  });

  const [searchQuery, setSearchQuery] = useState("");
  const [searchRegion, setSearchRegion] = useState("us-en");
  const [searchTimeLimit, setSearchTimeLimit] = useState("");
  const [searchFormats, setSearchFormats] = useState<Format[]>(["markdown"]);
  const [searchRenderJs, setSearchRenderJs] = useState<boolean | null>(false);
  const [searchScrape, setSearchScrape] = useState(false);

  useEffect(() => {
    const fetchHealth = async () => {
      try {
        const h = await checkHealth(baseUrl);
        setHealth(h);
        setError(null);
      } catch {
        setHealth(null);
        setError("Cannot connect to server");
      }
    };
    fetchHealth();
    const interval = setInterval(fetchHealth, 10000);
    return () => clearInterval(interval);
  }, [baseUrl]);

  const handleFormatChange = useCallback(
    (type: "scrape" | "crawl", format: Format, checked: boolean) => {
      if (type === "scrape") {
        setScrapeOptions((prev) => ({
          ...prev,
          formats: checked
            ? [...prev.formats, format]
            : prev.formats.filter((f) => f !== format),
        }));
      } else {
        setCrawlOptions((prev) => ({
          ...prev,
          formats: checked
            ? [...prev.formats, format]
            : prev.formats.filter((f) => f !== format),
        }));
      }
    },
    []
  );

  const buildRequest = useCallback(() => {
    if (!url && endpoint !== "search") return null;
    switch (endpoint) {
      case "scrape": {
        let extract: ScrapeRequest["extract"] = undefined;
        if (scrapeOptions.formats.includes("json") && scrapeOptions.jsonSchema.trim()) {
          try {
            extract = {
              schema: JSON.parse(scrapeOptions.jsonSchema),
              prompt: scrapeOptions.extractionPrompt.trim() || undefined,
              responseFormat: scrapeOptions.extractionResponseFormat.trim() || undefined,
            };
          } catch {
          }
        }
        return {
          url,
          formats: scrapeOptions.formats.length ? scrapeOptions.formats : ["markdown"],
          ...(scrapeOptions.renderJs !== null && { renderJs: scrapeOptions.renderJs }),
          waitFor: scrapeOptions.waitFor || undefined,
          headers: scrapeOptions.headers || undefined,
          cssSelector: scrapeOptions.cssSelector || undefined,
          includeTags: scrapeOptions.includeTags ? scrapeOptions.includeTags.split(",").map(s => s.trim()) : undefined,
          excludeTags: scrapeOptions.excludeTags ? scrapeOptions.excludeTags.split(",").map(s => s.trim()) : undefined,
          extract,
          maxMarkdownChars: scrapeOptions.maxMarkdownChars || undefined,
          ttl: scrapeOptions.ttl,
        } as ScrapeRequest;
      }
      case "crawl": {
        return {
          url,
          maxDepth: crawlOptions.maxDepth,
          maxPages: crawlOptions.maxPages,
          formats: crawlOptions.formats.length ? crawlOptions.formats : ["markdown"],
          ...(crawlOptions.renderJs !== null && { renderJs: crawlOptions.renderJs }),
          waitFor: crawlOptions.waitFor || undefined,
          maxMarkdownChars: crawlOptions.maxMarkdownChars || undefined,
        } as CrawlRequest;
      }
      case "map":
        return {
          url,
          maxDepth: mapOptions.maxDepth,
          useSitemap: mapOptions.useSitemap,
          timeout: mapOptions.timeout,
        } as MapRequest;
      case "search":
        return {
          query: searchQuery,
          region: searchRegion,
          timelimit: searchTimeLimit || undefined,
          ...(searchRenderJs !== null && { renderJs: searchRenderJs }),
          formats: searchFormats,
          scrape: searchScrape,
        } as SearchRequest;
    }
  }, [endpoint, url, scrapeOptions, crawlOptions, mapOptions, searchQuery, searchRegion, searchTimeLimit, searchRenderJs, searchFormats, searchScrape]);

  const handleSubmit = async () => {
    const request = buildRequest();
    if (!request) {
      setError("Please enter a URL");
      return;
    }

    setIsLoading(true);
    setError(null);
    setResponse(null);
    setCrawlStatus(null);
    const startTime = Date.now();

    try {
      switch (endpoint) {
        case "scrape": {
          const res = await scrape(request as ScrapeRequest);
          setTimeTakenMs(Date.now() - startTime);
          setResponse(res);
          if (!res.success && res.error) {
            setError(res.error);
          }
          break;
        }
        case "crawl": {
          const res = await startCrawl(request as CrawlRequest);
          const crawlId = (res as { success: boolean; id?: string; data?: { id?: string } }).id || res.data?.id;
          if (res.success && crawlId) {
            setCrawlId(crawlId);
            pollCrawlStatus(crawlId);
          } else {
            setResponse(res);
            if (res.error) setError(res.error);
          }
          break;
        }
        case "map": {
          const res = await map(request as MapRequest);
          setTimeTakenMs(Date.now() - startTime);
          setResponse(res);
          if (!res.success && res.error) {
            setError(res.error);
          }
          break;
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "An error occurred");
    } finally {
      setIsLoading(false);
    }
  };

  const pollCrawlStatus = async (id: string) => {
    const poll = async () => {
      try {
        const status = await getCrawlStatus(id);
        setCrawlStatus(status);
        if (status.status === "completed" || status.status === "failed") {
          if (status.status === "failed") {
            setError(status.error || "Crawl failed");
          }
          return;
        }
        setTimeout(poll, 2000);
      } catch {
        setError("Failed to get crawl status");
      }
    };
    poll();
  };

  const handleCancelCrawl = async () => {
    if (crawlId) {
      await cancelCrawl(crawlId);
      setCrawlStatus((prev) => (prev ? { ...prev, status: "failed", error: "Cancelled" } : null));
    }
  };

  const handleSearchSubmit = async () => {
    if (!searchQuery) return;

    setIsLoading(true);
    setError(null);
    setResponse(null);

    try {
      const request: SearchRequest = {
        query: searchQuery,
        region: searchRegion,
        timelimit: searchTimeLimit || undefined,
        renderJs: searchRenderJs,
        formats: searchFormats,
        scrape: searchScrape,
      };

      const res = await search(request);
      setResponse(res);
      if (!res.success && res.error) {
        setError(res.error);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "An error occurred");
    } finally {
      setIsLoading(false);
    }
  };

  const copySnippet = (code: string, type: string) => {
    navigator.clipboard.writeText(code);
    setCopiedSnippet(type);
    setTimeout(() => setCopiedSnippet(null), 2000);
  };

  const getCodeSnippet = () => {
    const request = buildRequest();
    if (!request) return "";
    switch (codeLanguage) {
      case "curl":
        return generateCurlCommand(endpoint, request, baseUrl);
      case "fetch":
        return generateFetchCode(endpoint, request, baseUrl);
      case "python":
        return generatePythonCode(endpoint, request, baseUrl);
    }
  };

  const clearAll = () => {
    setResponse(null);
    setCrawlStatus(null);
    setError(null);
    setCrawlId(null);
    setUrl("");
    setSearchQuery("");
    setSearchRegion("us-en");
    setSearchTimeLimit("");
    setSearchFormats(["markdown"]);
    setSearchRenderJs(false);
    setSearchScrape(false);
    setAdvancedExpanded(false);
    setSchemaBuilderOpen(false);
    setSchemaFields([{ name: "title", type: "string", description: "" }]);
    setScrapeOptions({
      formats: ["markdown"] as Format[],
      renderJs: null,
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
    });
    setCrawlOptions({
      maxDepth: 2,
      maxPages: 100,
      formats: ["markdown"] as Format[],
      renderJs: null,
      waitFor: 0,
      includeTags: "",
      excludeTags: "",
      maxMarkdownChars: undefined,
    });
    setMapOptions({
      maxDepth: 2,
      useSitemap: true,
      timeout: 30000,
    });
  };

  const handleEndpointChange = (newEndpoint: Endpoint) => {
    setEndpoint(newEndpoint);
    clearAll();
  };

  const renderResponse = () => {
    if (crawlStatus) {
      const progressPercent = crawlStatus.total > 0
        ? Math.round((crawlStatus.completed / crawlStatus.total) * 100)
        : 0;

      return (
        <div className="space-y-4">
          <div className="flex items-center justify-between flex-wrap gap-3">
            <div className="flex items-center gap-3">
              {crawlStatus.status === "scraping" && (
                <Loader2 className="w-5 h-5 animate-spin text-main" />
              )}
              <Badge variant={crawlStatus.status === "completed" ? "default" : crawlStatus.status === "failed" ? "neutral" : "neutral"}>
                {crawlStatus.status === "scraping" ? "SCRAPING" : crawlStatus.status.toUpperCase()}
              </Badge>
              <span className="text-sm text-gray-500">
                {crawlStatus.completed} / {crawlStatus.total} pages ({progressPercent}%)
              </span>
            </div>
            <div className="flex gap-2">
              {crawlStatus.status === "scraping" && (
                <Button variant="neutral" size="sm" onClick={handleCancelCrawl}>
                  Cancel
                </Button>
              )}
              <Button variant="neutral" size="sm" onClick={clearAll}>
                Clear
              </Button>
            </div>
          </div>

          {crawlStatus.status === "scraping" && (
            <div className="space-y-2">
              <div className="flex justify-between text-xs text-gray-400">
                <span>Progress</span>
                <span>{crawlStatus.completed} of {crawlStatus.total}</span>
              </div>
              <div className="h-2 bg-secondary-background rounded-full overflow-hidden border border-border">
                <div
                  className="h-full bg-main transition-all duration-500 ease-out"
                  style={{ width: `${progressPercent}%` }}
                />
              </div>
              <div className="flex justify-between items-center text-xs text-gray-400">
                <span>Job ID: <code className="bg-gray-100 px-1 rounded">{crawlStatus.id}</code></span>
                <span>Polling every 2s...</span>
              </div>
            </div>
          )}

          {crawlStatus.error && (
            <div className="p-4 bg-red-50 border-2 border-red-200 rounded-base text-red-700 text-sm font-medium">
              {crawlStatus.error}
            </div>
          )}

          {(crawlStatus.status === "completed" || crawlStatus.status === "scraping") && crawlStatus.data && crawlStatus.data.length > 0 && (
            <ResponseViewer data={crawlStatus.data as ScrapeData[]} rawResponse={crawlStatus} />
          )}
        </div>
      );
    }

    if (!response) return null;

    const isMapResponse = endpoint === "map" && response.success && (response.data as MapResponse)?.links !== undefined;
    const isSearchResponse = response.success && (response.data as SearchResponse)?.results !== undefined;

    return (
      <div className="space-y-4">
        {error && (
          <div className="p-4 bg-red-50 border-2 border-red-200 rounded-base text-red-700 text-sm font-medium">
            {error}
          </div>
        )}
        {isMapResponse ? (
          <MapResponseViewer data={response.data as MapResponse} rawResponse={response} timeTakenMs={timeTakenMs} />
        ) : isSearchResponse ? (
          <SearchResponseViewer data={response.data as SearchResponse} rawResponse={response} timeTakenMs={timeTakenMs} />
        ) : response.success && response.data ? (
          <ResponseViewer data={response.data as ScrapeData} rawResponse={response} timeTakenMs={timeTakenMs} />
        ) : (
          <pre className="bg-white p-4 rounded-base border-2 border-border overflow-auto max-h-[400px] text-sm font-mono">
            {JSON.stringify(response, null, 2)}
          </pre>
        )}
      </div>
    );
  };

  const renderScrapeForm = () => (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Output Formats</Label>
        <div className="flex flex-wrap gap-3">
          {(["markdown", "html", "rawHtml", "plainText", "links", "json", "imageLinks"] as Format[]).map(
            (format) => (
              <div key={format} className="flex items-center gap-2">
                <Checkbox
                  id={`scrape-${format}`}
                  checked={scrapeOptions.formats.includes(format)}
                  onCheckedChange={(checked) =>
                    handleFormatChange("scrape", format, checked as boolean)
                  }
                />
                <Label htmlFor={`scrape-${format}`} className="text-sm font-normal">
                  {format}
                </Label>
              </div>
            )
          )}
        </div>
      </div>

      {scrapeOptions.formats.includes("json") && (
        <div className="p-3 bg-muted rounded-lg space-y-3">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium">JSON Extraction (requires OpenAI API key)</p>
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
            <div className="p-3 bg-background rounded border space-y-3">
              <p className="text-xs text-muted-foreground">Define fields to generate JSON schema</p>
              <div className="space-y-2 max-h-48 overflow-y-auto">
                {schemaFields.map((field, index) => (
                  <div key={index} className="flex gap-2 items-center">
                    <Input
                      placeholder="Field name"
                      value={field.name}
                      onChange={(e) => {
                        const updated = [...schemaFields];
                        updated[index].name = e.target.value;
                        setSchemaFields(updated);
                      }}
                      className="text-sm h-8 flex-1"
                    />
                    <select
                      value={field.type}
                      onChange={(e) => {
                        const updated = [...schemaFields];
                        updated[index].type = e.target.value;
                        if (e.target.value !== "array") {
                          delete updated[index].itemType;
                        }
                        setSchemaFields(updated);
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
                          const updated = [...schemaFields];
                          updated[index].itemType = e.target.value;
                          setSchemaFields(updated);
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
                        const updated = [...schemaFields];
                        updated[index].description = e.target.value;
                        setSchemaFields(updated);
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
                      <X className="w-4 h-4" />
                    </Button>
                  </div>
                ))}
              </div>
              <div className="flex gap-2">
                <Button
                  variant="noShadow"
                  size="sm"
                  onClick={() =>
                    setSchemaFields([...schemaFields, { name: "", type: "string", description: "" }])
                  }
                  className="h-7 text-xs"
                >
                  Add Field
                </Button>
                <Button
                  variant="default"
                  size="sm"
                  onClick={() => {
                    setScrapeOptions({ ...scrapeOptions, jsonSchema: generateSchema() });
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
                setScrapeOptions({ ...scrapeOptions, jsonSchema: e.target.value })
              }
              placeholder='{"type": "object", "properties": {"title": {"type": "string"}}}'
              className="font-mono text-sm"
              rows={4}
            />
          </div>
          <div className="grid grid-cols-2 gap-2">
            <div className="space-y-1">
              <Label htmlFor="extractionPrompt" className="text-xs">Prompt</Label>
              <Input
                id="extractionPrompt"
                value={scrapeOptions.extractionPrompt}
                onChange={(e) =>
                  setScrapeOptions({ ...scrapeOptions, extractionPrompt: e.target.value })
                }
                placeholder="Extraction prompt..."
                className="text-sm h-8"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="extractionResponseFormat" className="text-xs">Response Format</Label>
              <Input
                id="extractionResponseFormat"
                value={scrapeOptions.extractionResponseFormat}
                onChange={(e) =>
                  setScrapeOptions({ ...scrapeOptions, extractionResponseFormat: e.target.value })
                }
                placeholder="format_name"
                className="text-sm h-8"
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
              <Button variant="default" size="sm" className="h-8 w-[140px] justify-start">
                {scrapeOptions.renderJs === null ? "Auto" : scrapeOptions.renderJs ? "Browser" : "HTTP"}
                <ChevronDown className="ml-auto h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuRadioGroup
                value={scrapeOptions.renderJs === null ? "auto" : scrapeOptions.renderJs ? "browser" : "http"}
                onValueChange={(v) =>
                  setScrapeOptions({
                    ...scrapeOptions,
                    renderJs: v === "auto" ? null : v === "browser",
                  })
                }
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
            value={scrapeOptions.ttl ?? ""}
            onChange={(e) =>
              setScrapeOptions({
                ...scrapeOptions,
                ttl: e.target.value ? parseInt(e.target.value) : undefined,
              })}
            className="h-8 w-[100px]"
            min={0}
          />
          <span className="text-xs text-gray-500">seconds (0=bypass)</span>
        </div>
      </div>

      {scrapeOptions.renderJs !== false && (
        <div className="space-y-2">
          <Label>Wait: {scrapeOptions.waitFor}ms</Label>
          <Slider
            value={[scrapeOptions.waitFor]}
            onValueChange={([v]) => setScrapeOptions({ ...scrapeOptions, waitFor: v })}
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
        {advancedExpanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
        {advancedExpanded ? "Hide" : "Show"} advanced options
      </button>

      {advancedExpanded && (
        <div className="space-y-4 pt-2 border-t border-border">
          <div className="space-y-2">
            <Label htmlFor="scrape-css">CSS Selector</Label>
            <Input
              id="scrape-css"
              value={scrapeOptions.cssSelector}
              onChange={(e) =>
                setScrapeOptions({ ...scrapeOptions, cssSelector: e.target.value })
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
            <Label htmlFor="scrape-include-tags">Include Tags (comma-separated)</Label>
            <Input
              id="scrape-include-tags"
              value={scrapeOptions.includeTags}
              onChange={(e) =>
                setScrapeOptions({ ...scrapeOptions, includeTags: e.target.value })
              }
              placeholder="article, main"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="scrape-exclude-tags">Exclude Tags (comma-separated)</Label>
            <Input
              id="scrape-exclude-tags"
              value={scrapeOptions.excludeTags}
              onChange={(e) =>
                setScrapeOptions({ ...scrapeOptions, excludeTags: e.target.value })
              }
              placeholder="nav, footer"
            />
          </div>
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
            <Loader2 className="w-4 h-4 mr-2 animate-spin" />
            Scraping...
          </>
        ) : (
          <>
            <Play className="w-4 h-4 mr-2" />
            Run
          </>
        )}
      </Button>
    </div>
  );

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
          {(["markdown", "html", "rawHtml", "plainText", "links", "imageLinks"] as Format[]).map(
            (format) => (
              <div key={format} className="flex items-center gap-2">
                <Checkbox
                  id={`crawl-${format}`}
                  checked={crawlOptions.formats.includes(format)}
                  onCheckedChange={(checked) =>
                    handleFormatChange("crawl", format, checked as boolean)
                  }
                />
                <Label htmlFor={`crawl-${format}`} className="text-sm font-normal">
                  {format}
                </Label>
              </div>
            )
          )}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2 sm:gap-6">
        <div className="flex items-center gap-2">
          <Label className="text-sm">Renderer</Label>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="default" size="sm" className="h-8 w-[140px] justify-start">
                {crawlOptions.renderJs === null ? "Auto" : crawlOptions.renderJs ? "Browser" : "HTTP"}
                <ChevronDown className="ml-auto h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuRadioGroup
                value={crawlOptions.renderJs === null ? "auto" : crawlOptions.renderJs ? "browser" : "http"}
                onValueChange={(v) =>
                  setCrawlOptions({
                    ...crawlOptions,
                    renderJs: v === "auto" ? null : v === "browser",
                  })
                }
              >
                  <DropdownMenuRadioItem value="auto">Auto</DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="http">HTTP</DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="browser">Browser</DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {crawlOptions.renderJs !== false && (
        <div className="space-y-2">
          <Label>Wait: {crawlOptions.waitFor}ms</Label>
          <Slider
            value={[crawlOptions.waitFor]}
            onValueChange={([v]) => setCrawlOptions({ ...crawlOptions, waitFor: v })}
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
            <Loader2 className="w-4 h-4 mr-2 animate-spin" />
            Crawling...
          </>
        ) : (
          <>
            <Play className="w-4 h-4 mr-2" />
            Run
          </>
        )}
      </Button>
    </div>
  );

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
        <Label htmlFor="map-sitemap" className="text-sm">Use Sitemap</Label>
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
            <Loader2 className="w-4 h-4 mr-2 animate-spin" />
            Mapping...
          </>
        ) : (
          <>
            <Play className="w-4 h-4 mr-2" />
            Run
          </>
        )}
      </Button>
    </div>
  );

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
              handleSearchSubmit();
            }
          }}
        />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>Region</Label>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="default" size="sm" className="h-10 w-full justify-start">
                {searchRegion}
                <ChevronDown className="ml-auto h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuRadioGroup value={searchRegion} onValueChange={setSearchRegion}>
                <DropdownMenuRadioItem value="us-en">US (English)</DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="uk-en">UK (English)</DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="au-en">Australia</DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="ca-en">Canada</DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="de-de">Germany</DropdownMenuRadioItem>
                <DropdownMenuRadioItem value="fr-fr">France</DropdownMenuRadioItem>
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <div className="space-y-2">
          <Label htmlFor="search-timelimit">Time Limit</Label>
          <select
            id="search-timelimit"
            value={searchTimeLimit}
            onChange={(e) => setSearchTimeLimit(e.target.value)}
            className="h-10 w-full px-3 py-2 text-sm border rounded-md bg-background"
          >
            <option value="">Any time</option>
            <option value="d">Past day</option>
            <option value="w">Past week</option>
            <option value="m">Past month</option>
            <option value="y">Past year</option>
          </select>
        </div>
      </div>

      <div className="flex items-center gap-2">
        <Switch
          id="search-scrape"
          checked={searchScrape}
          onCheckedChange={(checked) => setSearchScrape(checked)}
        />
        <Label htmlFor="search-scrape" className="text-sm">Scrape each result</Label>
      </div>

      {searchScrape && (
        <>
          <div className="flex items-center gap-2">
            <Label className="text-sm">Renderer</Label>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="default" size="sm" className="h-8 w-[140px] justify-start">
                  {searchRenderJs === null ? "Auto" : searchRenderJs ? "Browser" : "HTTP"}
                  <ChevronDown className="ml-auto h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start">
                <DropdownMenuRadioGroup
                  value={searchRenderJs === null ? "auto" : searchRenderJs ? "browser" : "http"}
                  onValueChange={(v) =>
                    setSearchRenderJs(v === "auto" ? null : v === "browser")
                  }
                >
                  <DropdownMenuRadioItem value="auto">Auto</DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="http">HTTP</DropdownMenuRadioItem>
                  <DropdownMenuRadioItem value="browser">Browser</DropdownMenuRadioItem>
                </DropdownMenuRadioGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>

          <div className="space-y-2">
            <Label>Output Formats</Label>
            <div className="flex flex-wrap gap-3">
              {(["markdown", "html", "rawHtml", "plainText", "links", "imageLinks"] as Format[]).map(
                (format) => (
                  <div key={format} className="flex items-center gap-2">
                    <Checkbox
                      id={`search-${format}`}
                      checked={searchFormats.includes(format)}
                      onCheckedChange={(checked) =>
                        setSearchFormats(checked
                          ? [...searchFormats, format]
                          : searchFormats.filter((f) => f !== format))
                      }
                    />
                    <Label htmlFor={`search-${format}`} className="text-sm font-normal">
                      {format}
                    </Label>
                  </div>
                )
              )}
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
            <Loader2 className="w-4 h-4 mr-2 animate-spin" />
            Searching...
          </>
        ) : (
          <>
            <Play className="w-4 h-4 mr-2" />
            Run
          </>
        )}
      </Button>
    </div>
  );

  const renderActiveForm = () => {
    switch (endpoint) {
      case "scrape":
        return renderScrapeForm();
      case "crawl":
        return renderCrawlForm();
      case "map":
        return renderMapForm();
      case "search":
        return renderSearchForm();
    }
  };

  return (
    <SidebarProvider>
      <AppSidebar activeEndpoint={endpoint} onEndpointChange={handleEndpointChange} health={health} />
      <SidebarInset>
        <header className="flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear group-has-[[data-collapsible=icon]]/sidebar-wrapper:h-12">
          <div className="flex items-center gap-2 px-4">
            <SidebarTrigger className="-ml-1" />
            <Breadcrumb>
              <BreadcrumbList>
                <BreadcrumbItem className="hidden md:block">
                  <BreadcrumbLink href="/playground">
                    Playground
                  </BreadcrumbLink>
                </BreadcrumbItem>
                <BreadcrumbSeparator className="hidden md:block" />
                <BreadcrumbItem>
                  <BreadcrumbPage className="capitalize">{endpoint}</BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>
          </div>
          <div className="ml-auto flex items-center gap-2 px-4">
            <a href="/battle">
              <Button variant="noShadow" size="sm">
                QuickCrawl vs TinyFish
              </Button>
            </a>
          </div>
        </header>

        <div className="flex flex-col gap-4 p-4 flex-1 min-h-0 overflow-auto">
          <div className="flex flex-col lg:flex-row gap-4 flex-1 min-h-0 items-stretch">
            <div className="w-full lg:w-[500px] xl:w-[600px] shrink-0">
              <Card className="h-[500px] sm:h-[600px] flex flex-col">
                <CardHeader className="pb-3 shrink-0">
                  <CardTitle className="text-base">Request & Options</CardTitle>
                </CardHeader>
                <CardContent className="pt-0 flex-1 min-h-0 overflow-auto">
                  {endpoint !== "search" && (
                    <div className="mb-4">
                      <Input
                        id="url"
                        value={url}
                        onChange={(e) => setUrl(e.target.value)}
                        placeholder="https://example.com"
                        className="flex-1"
                        onKeyDown={(e) => {
                          if (e.key === "Enter" && !isLoading && url) {
                            handleSubmit();
                          }
                        }}
                      />
                    </div>
                  )}
                  {renderActiveForm()}
                </CardContent>
              </Card>
            </div>

            <div className="flex-1 min-w-0 flex flex-col gap-4 lg:h-[600px] lg:min-h-0 min-h-[350px] max-h-full">
              {!response && !crawlStatus && !error && !isLoading && (
                <div className="flex-1 flex items-center justify-center border-2 border-dashed border-border rounded-lg text-gray-400">
                  <div className="text-center">
                    <Play className="w-12 h-12 mx-auto mb-4 opacity-20" />
                    <p className="text-lg font-medium">Run an endpoint to see the response</p>
                    <p className="text-sm mt-1">Select Scrape, Crawl, or Map and enter a URL</p>
                  </div>
                </div>
              )}

              {isLoading && !crawlStatus && (
                <Card className="flex-1">
                  <CardContent className="py-12 flex items-center justify-center h-full">
                    <div className="text-center">
                      <Loader2 className="w-12 h-12 mx-auto mb-4 animate-spin text-main" />
                      <p className="text-lg font-medium">Processing request...</p>
                      <p className="text-sm text-gray-500 mt-1">This may take a moment</p>
                    </div>
                  </CardContent>
                </Card>
              )}

              {crawlId && !crawlStatus && (
                <Card className="flex-1">
                  <CardContent className="py-6 flex items-center justify-center h-full">
                    <div className="text-center">
                      <Loader2 className="w-5 h-5 mx-auto mb-3 animate-spin text-main" />
                      <span className="font-medium block">Polling for crawl status...</span>
                      <div className="mt-3 text-sm text-gray-500">
                        Job ID: <code className="bg-gray-100 px-2 py-1 rounded">{crawlId}</code>
                      </div>
                      <p className="mt-2 text-xs text-gray-400">Checking every 2s</p>
                    </div>
                  </CardContent>
                </Card>
              )}

              {(response || crawlStatus || error) && (
                <div className="flex-1 min-h-0 min-w-0 flex flex-col">
                  {renderResponse()}
                </div>
              )}
            </div>
          </div>

          <Card>
            <CardHeader className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 cursor-pointer" onClick={() => setCodeSnippetExpanded(!codeSnippetExpanded)}>
              <div className="flex items-center gap-2">
                <CardTitle>Code Snippet</CardTitle>
                <Button variant="neutral" size="sm" className="h-6 px-2">
                  {codeSnippetExpanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
                </Button>
              </div>
              {codeSnippetExpanded && (
                <div className="flex flex-wrap gap-1">
                  <Button
                    variant={codeLanguage === "curl" ? "default" : "noShadow"}
                    size="sm"
                    onClick={(e) => { e.stopPropagation(); setCodeLanguage("curl"); }}
                  >
                    cURL
                  </Button>
                  <Button
                    variant={codeLanguage === "fetch" ? "default" : "noShadow"}
                    size="sm"
                    onClick={(e) => { e.stopPropagation(); setCodeLanguage("fetch"); }}
                  >
                    Fetch
                  </Button>
                  <Button
                    variant={codeLanguage === "python" ? "default" : "noShadow"}
                    size="sm"
                    onClick={(e) => { e.stopPropagation(); setCodeLanguage("python"); }}
                  >
                    Python
                  </Button>
                </div>
              )}
            </CardHeader>
            {codeSnippetExpanded && (
              <CardContent>
                <div className="relative">
                  <pre className="bg-secondary-background p-4 rounded-base border-2 border-border overflow-auto max-h-[200px] text-sm font-mono break-all whitespace-pre-wrap">
                    {getCodeSnippet() || "// Configure options to generate code"}
                  </pre>
                  {getCodeSnippet() && (
                    <Button
                      variant="noShadow"
                      size="icon"
                      className="absolute top-2 right-2"
                      onClick={() => copySnippet(getCodeSnippet(), codeLanguage)}
                    >
                      {copiedSnippet === codeLanguage ? (
                        <Check className="w-4 h-4" />
                      ) : (
                        <Copy className="w-4 h-4" />
                      )}
                    </Button>
                  )}
                </div>
              </CardContent>
            )}
          </Card>
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}
