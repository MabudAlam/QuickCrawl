export interface ScrapeRequest {
  url: string;
  formats?: Format[];
  renderJs?: boolean | null;
  waitFor?: number;
  headers?: Record<string, string>;
  cssSelector?: string;
  includeTags?: string[];
  excludeTags?: string[];
  extract?: {
    schema: Record<string, unknown>;
    prompt?: string;
    responseFormat?: string;
  };
  maxMarkdownChars?: number;
  ttl?: number;
}

export interface CrawlRequest {
  url: string;
  maxDepth?: number;
  maxPages?: number;
  formats?: Format[];
  renderJs?: boolean | null;
  waitFor?: number;
  includeTags?: string[];
  excludeTags?: string[];
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
  timeRange?: string;
  language?: string;
  categories?: string;
  page?: number;
  timelimit?: string;
  use_bm25?: boolean;
  renderJs?: boolean | null;
  formats?: Format[];
  scrape?: boolean;
}

export interface SearchResult {
  position: number;
  score: number;
  bm25_score?: number;
  title: string;
  url: string;
  site_name?: string;
  snippet: string;
  engine?: string;
  published?: string;
  markdown?: string;
  html?: string;
  rawHtml?: string;
  plainText?: string;
  links?: string[];
  json?: string;
}

export interface SearchResponse {
  query: string;
  results: SearchResult[];
  total_results: number;
  page: number;
  engine: string;
  took_ms: number;
}

export type Format = "markdown" | "html" | "rawHtml" | "plainText" | "links" | "json" | "imageLinks";

export interface ScrapeData {
  markdown?: string;
  html?: string;
  rawHtml?: string;
  plainText?: string;
  links?: string[];
  imageLinks?: string[];
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
  timeTakenMs?: number;
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
  renderJs: boolean | null; // null = auto, true = always browser, false = always http
  waitFor: number;
  headers: string;
  cssSelector: string;
  includeTags: string;
  excludeTags: string;
  jsonSchema: string;
  extractionPrompt: string;
  extractionResponseFormat: string;
  maxMarkdownChars: number | undefined;
  ttl: number | undefined;
}

export interface CrawlOptions {
  maxDepth: number;
  maxPages: number;
  formats: Format[];
  renderJs: boolean | null;
  waitFor: number;
  includeTags: string;
  excludeTags: string;
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