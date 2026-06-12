import type {
  ScrapeRequest,
  CrawlRequest,
  MapRequest,
  SearchRequest,
  APIResponse,
  HealthResponse,
  CrawlState,
  ScrapeData,
  MapResponse,
  SearchResponse,
} from "./api-types";

function getBaseUrl() {
  return process.env.NEXT_PUBLIC_BASE_URL;
}

export { getBaseUrl };

function createHeaders(): HeadersInit {
  return {
    "Content-Type": "application/json",
  };
}

export async function checkHealth(baseUrl?: string): Promise<HealthResponse> {
  const url = baseUrl || getBaseUrl();
  const response = await fetch(`${url}/health`);
  if (!response.ok) {
    throw new Error(`Health check failed: ${response.statusText}`);
  }
  return response.json();
}

export async function scrape(
  request: ScrapeRequest,
): Promise<APIResponse<ScrapeData>> {
  const baseUrl = getBaseUrl();
  const response = await fetch(`${baseUrl}/v1/scrape`, {
    method: "POST",
    headers: createHeaders(),
    body: JSON.stringify(request),
  });
  return response.json();
}

export async function startCrawl(
  request: CrawlRequest,
): Promise<APIResponse<{ id: string }>> {
  const baseUrl = getBaseUrl();
  const response = await fetch(`${baseUrl}/v1/crawl`, {
    method: "POST",
    headers: createHeaders(),
    body: JSON.stringify(request),
  });
  return response.json();
}

export async function getCrawlStatus(
  crawlId: string,
): Promise<CrawlState> {
  const baseUrl = getBaseUrl();
  const response = await fetch(`${baseUrl}/v1/crawl/${crawlId}`, {
    method: "GET",
    headers: createHeaders(),
  });
  return response.json();
}

export async function cancelCrawl(
  crawlId: string,
): Promise<void> {
  const baseUrl = getBaseUrl();
  await fetch(`${baseUrl}/v1/crawl/${crawlId}`, {
    method: "DELETE",
    headers: createHeaders(),
  });
}

export async function map(
  request: MapRequest,
): Promise<APIResponse<MapResponse>> {
  const baseUrl = getBaseUrl();
  const response = await fetch(`${baseUrl}/v1/map`, {
    method: "POST",
    headers: createHeaders(),
    body: JSON.stringify(request),
  });
  return response.json();
}

export async function search(
  request: SearchRequest,
): Promise<APIResponse<SearchResponse>> {
  const baseUrl = getBaseUrl();
  const response = await fetch(`${baseUrl}/v1/search`, {
    method: "POST",
    headers: createHeaders(),
    body: JSON.stringify(request),
  });
  if (!response.ok) {
    const text = await response.text().catch(() => response.statusText);
    return { success: false, error: `HTTP ${response.status}: ${text}` };
  }
  const data: SearchResponse = await response.json();
  return { success: true, data };
}

export function generateCurlCommand(
  endpoint: "scrape" | "crawl" | "map" | "search",
  request: ScrapeRequest | CrawlRequest | MapRequest | SearchRequest,
  baseUrl?: string,
): string {
  const url = baseUrl || getBaseUrl();
  const fullUrl = `${url}/v1/${endpoint}`;

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };

  let curlCmd = `curl -X POST '${fullUrl}'`;

  Object.entries(headers).forEach(([k, v]) => {
    curlCmd += ` \\\n  -H '${k}: ${v}'`;
  });

  const body = typeof request === "object" ? request : {};
  curlCmd += ` \\\n  -d '${JSON.stringify(body, null, 2)}'`;

  return curlCmd;
}

export function generateFetchCode(
  endpoint: "scrape" | "crawl" | "map" | "search",
  request: ScrapeRequest | CrawlRequest | MapRequest | SearchRequest,
  baseUrl?: string,
): string {
  const url = baseUrl || getBaseUrl();
  const fullUrl = `${url}/v1/${endpoint}`;

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };

  return `const response = await fetch('${fullUrl}', {
  method: 'POST',
  headers: ${JSON.stringify(headers, null, 2).replace(/"/g, "'")},
  body: JSON.stringify(${JSON.stringify(request, null, 2)}),
});

const data = await response.json();
console.log(data);`;
}

export function generatePythonCode(
  endpoint: "scrape" | "crawl" | "map" | "search",
  request: ScrapeRequest | CrawlRequest | MapRequest | SearchRequest,
  baseUrl?: string,
): string {
  const url = baseUrl || getBaseUrl();
  const fullUrl = `${url}/v1/${endpoint}`;

  const headers = `    "Content-Type": "application/json",\n`;

  return `import requests
import json

url = "${fullUrl}"
headers = {
${headers}}
data = ${JSON.stringify(request, null, 4)}

response = requests.post(url, headers=headers, json=data)
print(response.json())`;
};
