"use client";

import { useState, useCallback, useEffect } from "react";
import { Loader2, Play, Copy, Check, X, ChevronDown, ChevronUp, Minus, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
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
  const [baseUrl, setBaseUrl] = useState(initialBaseUrl);

  const [isLoading, setIsLoading] = useState(false);
  const [response, setResponse] = useState<APIResponse<unknown> | null>(null);
  const [crawlId, setCrawlId] = useState<string | null>(null);
  const [crawlStatus, setCrawlStatus] = useState<CrawlState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [copiedSnippet, setCopiedSnippet] = useState<string | null>(null);
  const [codeLanguage, setCodeLanguage] = useState<"curl" | "fetch" | "python">("curl");
  const [codeSnippetExpanded, setCodeSnippetExpanded] = useState(false);
  const [advancedExpanded, setAdvancedExpanded] = useState(false);
  const [schemaBuilderOpen, setSchemaBuilderOpen] = useState(false);
  const [schemaFields, setSchemaFields] = useState<{ name: string; type: string; description: string; itemType?: string }[]>([
    { name: "title", type: "string", description: "" },
  ]);

  const [scrapeOptions, setScrapeOptions] = useState<ScrapeOptions>({
    formats: ["markdown"] as Format[],
    renderJs: false,
    waitFor: 0,
    headers: "",
    cssSelector: "",
    includeTags: "",
    excludeTags: "",
    jsonSchema: "",
    extractionPrompt: "",
    extractionResponseFormat: "",
    maxMarkdownChars: undefined,
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
    renderJs: false,
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
  const [searchRenderJs, setSearchRenderJs] = useState(false);
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
            // Invalid JSON schema, skip extract
          }
        }
        return {
          url,
          formats: scrapeOptions.formats.length ? scrapeOptions.formats : ["markdown"],
          renderJs: scrapeOptions.renderJs,
          waitFor: scrapeOptions.waitFor || undefined,
          headers: scrapeOptions.headers || undefined,
          cssSelector: scrapeOptions.cssSelector || undefined,
          includeTags: scrapeOptions.includeTags ? scrapeOptions.includeTags.split(",").map(s => s.trim()) : undefined,
          excludeTags: scrapeOptions.excludeTags ? scrapeOptions.excludeTags.split(",").map(s => s.trim()) : undefined,
          extract,
          maxMarkdownChars: scrapeOptions.maxMarkdownChars || undefined,
        } as ScrapeRequest;
      }
      case "crawl": {
        return {
          url,
          maxDepth: crawlOptions.maxDepth,
          maxPages: crawlOptions.maxPages,
          formats: crawlOptions.formats.length ? crawlOptions.formats : ["markdown"],
          renderJs: crawlOptions.renderJs,
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
          renderJs: searchRenderJs,
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

    try {
      switch (endpoint) {
        case "scrape": {
          const res = await scrape(request as ScrapeRequest);
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
      renderJs: false,
      waitFor: 0,
      headers: "",
      cssSelector: "",
      includeTags: "",
      excludeTags: "",
      jsonSchema: "",
      extractionPrompt: "",
      extractionResponseFormat: "",
      maxMarkdownChars: undefined,
    });
    setCrawlOptions({
      maxDepth: 2,
      maxPages: 100,
      formats: ["markdown"] as Format[],
      renderJs: false,
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
          <MapResponseViewer data={response.data as MapResponse} rawResponse={response} />
        ) : isSearchResponse ? (
          <SearchResponseViewer data={response.data as SearchResponse} rawResponse={response} />
        ) : response.success && response.data ? (
          <ResponseViewer data={response.data as ScrapeData} rawResponse={response} />
        ) : (
          <pre className="bg-white p-4 rounded-base border-2 border-border overflow-auto max-h-[400px] text-sm font-mono">
            {JSON.stringify(response, null, 2)}
          </pre>
        )}
      </div>
    );
  };

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b-2 border-border bg-background sticky top-0 z-50">
        <div className="container mx-auto px-4 py-3 flex items-center justify-between">
          <div className="flex items-center gap-4">
            <h1 className="text-xl font-heading font-bold">quickcrawl Playground</h1>
            {health ? (
              <Badge variant="default" className="bg-green-500">Server OK</Badge>
            ) : (
              <Badge variant="neutral">Server Offline</Badge>
            )}
          </div>

          <div className="flex items-center gap-3">
            <a href="/battle">
              <Button variant="noShadow" size="sm">
                QuickCrawl vs TinyFish
              </Button>
            </a>
            <a
              href="https://github.com/MabudAlam/quickcrawl"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex"
            >
              <Button variant="noShadow" size="sm">
                <svg className="w-4 h-4 mr-2" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
                </svg>
                GitHub
              </Button>
            </a>
          </div>
        </div>
      </header>

      

      <main className="container mx-auto px-4 py-6 space-y-6">
        <Card className="w-full">
            <CardHeader>
              <CardTitle>API Endpoint</CardTitle>
            </CardHeader>
            <CardContent className="space-y-5">
              <Tabs
                value={endpoint}
                onValueChange={(v) => handleEndpointChange(v as Endpoint)}
              >
                <TabsList className="flex w-full overflow-x-auto flex-nowrap">
                  <TabsTrigger value="scrape">Scrape</TabsTrigger>
                  <TabsTrigger value="crawl">Crawl</TabsTrigger>
                  <TabsTrigger value="map">Map</TabsTrigger>
                  <TabsTrigger value="search">Search</TabsTrigger>
                </TabsList>

                <div className="mt-4 space-y-4">
                  {endpoint !== "search" && (
                    <div className="space-y-2">
                      <Label htmlFor="url">URL</Label>
                      <Input
                        id="url"
                        value={url}
                        onChange={(e) => setUrl(e.target.value)}
                        placeholder="https://example.com"
                        onKeyDown={(e) => {
                          if (e.key === "Enter" && !isLoading && url) {
                            handleSubmit();
                          }
                        }}
                      />
                    </div>
                  )}
                </div>

                <TabsContent value="scrape" className="space-y-4">
                  <div className="space-y-2">
                    <Label>Output Formats</Label>
                    <div className="flex flex-wrap gap-3">
                      {(["markdown", "html", "rawHtml", "plainText", "links", "json"] as Format[]).map(
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
                      <Switch
                        id="scrape-render"
                        checked={scrapeOptions.renderJs}
                        onCheckedChange={(checked) =>
                          setScrapeOptions({ ...scrapeOptions, renderJs: checked })
                        }
                      />
                      <Label htmlFor="scrape-render" className="text-sm">Render JS</Label>
                    </div>
                  </div>

                  {scrapeOptions.renderJs && (
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
                </TabsContent>

                <TabsContent value="crawl" className="space-y-4">
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
                      {(["markdown", "html", "rawHtml", "plainText", "links"] as Format[]).map(
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
                      <Switch
                        id="crawl-render"
                        checked={crawlOptions.renderJs}
                        onCheckedChange={(checked) =>
                          setCrawlOptions({ ...crawlOptions, renderJs: checked })
                        }
                      />
                      <Label htmlFor="crawl-render" className="text-sm">Render JS</Label>
                    </div>
                  </div>

                  {crawlOptions.renderJs && (
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
                </TabsContent>

                <TabsContent value="map" className="space-y-4">
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
                </TabsContent>

                <TabsContent value="search" className="space-y-4">
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
                        <Switch
                          id="search-render-js"
                          checked={searchRenderJs}
                          onCheckedChange={(checked) => setSearchRenderJs(checked)}
                        />
                        <Label htmlFor="search-render-js" className="text-sm">Render JS</Label>
                      </div>

                      <div className="space-y-2">
                        <Label>Output Formats</Label>
                        <div className="flex flex-wrap gap-3">
                          {(["markdown", "html", "rawHtml", "plainText", "links"] as Format[]).map(
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
                </TabsContent>
              </Tabs>
            </CardContent>
          </Card>

        <div>
          {!response && !crawlStatus && !error && !isLoading && (
            <div className="text-center py-16 border-2 border-dashed border-border rounded-lg text-gray-400">
              <Play className="w-12 h-12 mx-auto mb-4 opacity-20" />
              <p className="text-lg font-medium">Run an endpoint to see the response</p>
              <p className="text-sm mt-1">Select Scrape, Crawl, or Map and enter a URL</p>
            </div>
          )}

          {isLoading && !crawlStatus && (
            <Card>
              <CardContent className="py-12">
                <div className="text-center">
                  <Loader2 className="w-12 h-12 mx-auto mb-4 animate-spin text-main" />
                  <p className="text-lg font-medium">Processing request...</p>
                  <p className="text-sm text-gray-500 mt-1">This may take a moment</p>
                </div>
              </CardContent>
            </Card>
          )}

          {crawlId && !crawlStatus && (
            <Card>
              <CardContent className="py-6">
                <div className="flex items-center gap-3">
                  <Loader2 className="w-5 h-5 animate-spin text-main" />
                  <span className="font-medium">Polling for crawl status...</span>
                </div>
                <div className="mt-3 text-sm text-gray-500">
                  Job ID: <code className="bg-gray-100 px-2 py-1 rounded">{crawlId}</code>
                </div>
                <p className="mt-2 text-xs text-gray-400">Checking status every 2 seconds</p>
              </CardContent>
            </Card>
          )}

          {(response || crawlStatus || error) && renderResponse()}
        </div>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between cursor-pointer" onClick={() => setCodeSnippetExpanded(!codeSnippetExpanded)}>
            <div className="flex items-center gap-2">
              <CardTitle>Code Snippet</CardTitle>
              <Button variant="neutral" size="sm" className="h-6 px-2">
                {codeSnippetExpanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
              </Button>
            </div>
            {codeSnippetExpanded && (
              <div className="flex gap-1">
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
                <pre className="bg-secondary-background p-4 rounded-base border-2 border-border overflow-auto max-h-[200px] text-sm font-mono">
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
      </main>

      <footer className="border-t-2 border-border bg-background py-4 mt-auto">
        <div className="container mx-auto px-4 flex items-center justify-between text-sm text-gray-500">
          <div className="flex items-center gap-2">
            <span className="font-heading font-semibold">quickcrawl</span>
            <span>•</span>
            <span>Open source web scraping API</span>
          </div>
          <div className="flex items-center gap-4">
            <a href="https://github.com/MabudAlam/quickcrawl" target="_blank" rel="noopener noreferrer" className="hover:text-foreground transition-colors">
              GitHub
            </a>
            <a href="https://github.com/MabudAlam/quickcrawl/issues" target="_blank" rel="noopener noreferrer" className="hover:text-foreground transition-colors">
              Issues
            </a>
            <a href="https://github.com/MabudAlam/quickcrawl#readme" target="_blank" rel="noopener noreferrer" className="hover:text-foreground transition-colors">
              Docs
            </a>
          </div>
        </div>
      </footer>
    </div>
  );
}