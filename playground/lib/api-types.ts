export interface ScrapeRequest {
  url: string;
  formats?: Format[];
  renderMode?: RenderMode | null;
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
  renderMode?: RenderMode | null;
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

export type SearchTimeRange = "" | "day" | "week" | "month" | "year";

export interface SearchRequest {
  query: string;
  region?: string;
  timeRange?: SearchTimeRange;
  language?: string;
  categories?: string;
  page?: number;
  use_bm25?: boolean;
  renderMode?: RenderMode | null;
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

export type RenderMode = "auto" | "browser" | "http";

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

export type Endpoint = "scrape" | "crawl" | "map" | "search" | "brand";

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
  renderMode: RenderMode | null; // null = inherit server default, "auto"/"browser"/"http" override it
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
  renderMode: RenderMode | null;
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
  timeRange: SearchTimeRange;
  page: number;
}

export interface BrandRequest {
  url: string;
}

export interface BrandResponse {
  success: boolean;
  domain: string;
  brand?: BrandData;
  screenshot?: string; // base64-encoded full-page PNG/JPEG
}

export interface BrandData {
  domain?: string;
  title?: string;
  name?: string;
  tagline?: string;
  description?: string;
  colors?: BrandColor[];
  logos?: BrandLogo[];
  backdrops?: BrandBackdrop[];
  address?: BrandAddress;
  socials?: SocialLink[];
  links?: BrandLinks;
  primary_language?: string;
  fonts?: BrandFonts;
  styleguide?: BrandStyleguide;
}

export interface BrandFonts {
  fonts: BrandFont[];
  fontLinks: Record<string, BrandFontLink>;
}

export interface BrandFont {
  font: string;
  uses: string[];
  fallbacks: string[];
  num_elements: number;
  num_words: number;
  percent_elements: number;
  percent_words: number;
}

export interface BrandFontLink {
  type: "google" | "custom" | "adobe" | "system";
  files: Record<string, string>;
  displayName?: string;
  category?: string;
}

export interface BrandStyleguide {
  mode: "light" | "dark";
  colors: BrandStyleguideColors;
  typography: BrandStyleguideTypography;
  elementSpacing: Record<string, string>;
  shadows: Record<string, string>;
  components: BrandStyleguideComponents;
  fontLinks: Record<string, BrandFontLink>;
}

export interface BrandStyleguideColors {
  accent: string;
  background: string;
  text: string;
}

export interface BrandStyleguideTypography {
  headings: Record<string, BrandTextStyle>;
  p: BrandTextStyle;
}

export interface BrandTextStyle {
  fontFamily: string;
  fontSize: string;
  fontWeight: number;
  lineHeight: string;
  letterSpacing: string;
  fontFallbacks: string[];
}

export interface BrandStyleguideComponents {
  button: BrandButtonVariants;
  card: BrandCardStyle;
}

export interface BrandButtonVariants {
  primary: BrandButtonStyle;
  secondary: BrandButtonStyle;
  link: BrandButtonStyle;
}

export interface BrandButtonStyle {
  backgroundColor: string;
  color: string;
  borderColor: string;
  borderRadius: string;
  borderWidth: string;
  borderStyle: string;
  padding: string;
  fontSize: string;
  fontWeight: number;
  minWidth: string;
  minHeight: string;
  textDecoration: string;
  boxShadow: string;
  fontFallbacks: string[];
  fontFamily: string;
  css: string;
}

export interface BrandCardStyle {
  backgroundColor: string;
  borderColor: string;
  borderRadius: string;
  borderWidth: string;
  borderStyle: string;
  padding: string;
  boxShadow: string;
  textColor: string;
  css: string;
}

export interface BrandColor {
  hex: string;
  name: string;
}

export interface BrandLogo {
  url: string;
  format?: string;
  sizes?: number[];
  mode?: string;
  colors?: BrandColor[];
  resolution?: ImageResolution;
}

export interface BrandBackdrop {
  url: string;
  colors?: BrandColor[];
  resolution?: ImageResolution;
}

export interface ImageResolution {
  width: number;
  height: number;
  aspect_ratio: number;
}

export interface BrandAddress {
  city?: string;
  country?: string;
  country_code?: string;
  state_province?: string;
  state_code?: string;
  postal_code?: string;
}

export interface SocialLink {
  type: string;
  url: string;
}

export interface BrandLinks {
  careers?: string;
  contact?: string;
  pricing?: string;
  terms?: string;
  privacy?: string;
  blog?: string;
  login?: string;
  signup?: string;
}