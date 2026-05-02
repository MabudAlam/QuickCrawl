export interface ScrapeRequest {
  url: string;
  formats?: Format[];
  onlyMainContent?: boolean;
  renderJs?: boolean;
  waitFor?: number;
  headers?: Record<string, string>;
  cssSelector?: string;
  xpath?: string;
  proxy?: string;
  stealth?: boolean;
  extract?: {
    schema: Record<string, unknown>;
    prompt?: string;
    responseFormat?: string;
  };
  chunkStrategy?: { type: ChunkStrategy };
  query?: string;
  filterMode?: FilterMode;
  topK?: number;
  maxMarkdownChars?: number;
  browser?: "lightpanda" | "chrome";
}

export interface CrawlRequest {
  url: string;
  maxDepth?: number;
  maxPages?: number;
  formats?: Format[];
  onlyMainContent?: boolean;
  renderJs?: boolean;
  waitFor?: number;
  browser?: "lightpanda" | "chrome";
  extract?: {
    schema: Record<string, unknown>;
    prompt?: string;
    responseFormat?: string;
  };
  chunkStrategy?: { type: ChunkStrategy };
  query?: string;
  filterMode?: FilterMode;
  topK?: number;
  maxMarkdownChars?: number;
}

export interface MapRequest {
  url: string;
  maxDepth?: number;
  useSitemap?: boolean;
  timeout?: number;
}

export interface SearchRequest {
  query: string;
  region?: string;
  safesearch?: string;
  timelimit?: string;
  renderJs?: boolean;
  formats?: Format[];
}

export interface SearchResult {
  title: string;
  description: string;
  url: string;
  markdown?: string;
  html?: string;
  rawHtml?: string;
  plainText?: string;
  links?: string[];
  json?: string;
}

export interface SearchResponse {
  results: SearchResult[];
}

export type Format = "markdown" | "html" | "rawHtml" | "plainText" | "links" | "json";

export type ChunkStrategy = "sentence" | "regex" | "topic";

export type FilterMode = "bm25" | "cosine";

export interface ChunkResult {
  content: string;
  score: number | null;
  index: number;
}

export interface ScrapeData {
  markdown?: string;
  html?: string;
  rawHtml?: string;
  plainText?: string;
  links?: string[];
  chunks?: ChunkResult[];
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

export interface CrawlState {
  id: string;
  success: boolean;
  status: "scraping" | "completed" | "failed";
  total: number;
  completed: number;
  data: ScrapeData[];
  error: string | null;
}

export interface MapResponse {
  links: string[];
}

export interface APIResponse<T> {
  success: boolean;
  data?: T;
  warning?: string;
  error?: string;
}

export interface HealthResponse {
  status: string;
  version: string;
  renderers: {
    http: boolean;
    chrome: boolean;
  };
  active_crawl_jobs: number;
}

export type Endpoint = "scrape" | "crawl" | "map" | "search";

export interface PlaygroundState {
  endpoint: Endpoint;
  url: string;
  baseUrl: string;
  isLoading: boolean;
  response: APIResponse<unknown> | null;
  crawlId: string | null;
  crawlStatus: CrawlState | null;
  error: string | null;
}

export interface ScrapeOptions {
  formats: Format[];
  onlyMainContent: boolean;
  renderJs: boolean;
  waitFor: number;
  headers: string;
  cssSelector: string;
  browser: "lightpanda" | "chrome" | undefined;
  chunkStrategy: ChunkStrategy | undefined;
  query: string;
  filterMode: FilterMode | undefined;
  topK: number;
  jsonSchema: string;
  extractionPrompt: string;
  extractionResponseFormat: string;
  maxMarkdownChars: number | undefined;
}

export interface CrawlOptions {
  maxDepth: number;
  maxPages: number;
  formats: Format[];
  onlyMainContent: boolean;
  renderJs: boolean;
  waitFor: number;
  browser: "lightpanda" | "chrome" | undefined;
  jsonSchema: string;
  extractionPrompt: string;
  extractionResponseFormat: string;
  chunkStrategy: ChunkStrategy | undefined;
  query: string;
  filterMode: FilterMode | undefined;
  topK: number;
  maxMarkdownChars: number | undefined;
}

export interface MapOptions {
  maxDepth: number;
  useSitemap: boolean;
  timeout: number;
}

export interface SearchOptions {
  query: string;
  region: string;
  kResults: number;
  timelimit: string;
  page: number;
}