package crawler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/extractor"
	"github.com/MabudAlam/quickcrawl/internal/renderer"
	"github.com/MabudAlam/quickcrawl/internal/types"
)

const defaultMaxTokens = 4096

// llmHTTPClient is a shared HTTP client for making requests to LLM APIs.
// It has a 60-second timeout and connection pooling configured.
var llmHTTPClient = &http.Client{
	Timeout: 60 * time.Second,
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	},
}

// ScrapeURL fetches a single URL and extracts content in the requested formats.
// It handles proxy configuration, stealth mode, JavaScript rendering decisions,
// content extraction, and optional LLM-based structured data extraction.
//
// The function returns a ScrapeData containing all requested output formats
// (markdown, html, rawHtml, links, plainText, json) along with page metadata.
func ScrapeURL(
	req *types.ScrapeRequest,
	rend *renderer.FallbackRenderer,
	llmConfig *types.LLMConfig,
	userAgent string,
	defaultStealth bool,
	renderJSDefault *bool,
) (*types.ScrapeData, *types.QuickCrawlError) {
	totalStart := time.Now()

	if req.Actions != nil {
		return nil, types.ErrInvalidRequest.New("The 'actions' parameter is not yet supported. Use cssSelector or xpath for element targeting.")
	}

	injectStealth := defaultStealth
	if req.Stealth != nil {
		injectStealth = *req.Stealth
	}

	effectiveRenderJS := effectiveRenderJSSetting(req.RenderJS, renderJSDefault)

	needsTempFetcher := req.Proxy != nil || (req.Stealth != nil && *req.Stealth != defaultStealth)

	var fetchResult *types.FetchResult

	if needsTempFetcher {
		proxy := ""
		if req.Proxy != nil {
			proxy = *req.Proxy
		}

		effectiveUA := userAgent
		if injectStealth {
			pool := types.GetBuiltinUAPool()
			effectiveUA = pool[rand.Intn(len(pool))]
		}

		if effectiveRenderJS != nil && !*effectiveRenderJS {
			proxyPtr := (*string)(nil)
			if proxy != "" {
				proxyPtr = &proxy
			}
			tempHTTP := renderer.NewHTTPFetcher(effectiveUA, proxyPtr, injectStealth)
			result, err := tempHTTP.Fetch(req.URL, req.Headers, req.WaitFor)
			if err != nil {
				return nil, err
			}
			if result == nil {
				return nil, types.ErrHttp.New("fetch returned nil")
			}
			fetchResult = result
		} else {
			mergedHeaders := req.Headers
			if injectStealth {
				if mergedHeaders == nil {
					mergedHeaders = make(map[string]string)
				}
				if _, ok := mergedHeaders["User-Agent"]; !ok {
					mergedHeaders["User-Agent"] = effectiveUA
				}
			}
			result, err := rend.Fetch(req.URL, mergedHeaders, req.RenderJS, req.WaitFor, req.Browser)
			if err != nil {
				return nil, err
			}
			fetchResult = result
		}
	} else {
		result, err := rend.Fetch(req.URL, req.Headers, req.RenderJS, req.WaitFor, req.Browser)
		if err != nil {
			return nil, err
		}
		fetchResult = result
	}

	fetchElapsed := time.Since(totalStart)
	log.Printf("[scrape] fetch completed: url=%s duration=%v status=%d", req.URL, fetchElapsed, fetchResult.StatusCode)

	warning := buildFetchWarning(fetchResult)
	extractStart := time.Now()
	data := extractor.Extract(extractor.ExtractOptions{
		RawHTML:         fetchResult.HTML,
		RawBytes:        fetchResult.RawBytes,
		SourceURL:       fetchResult.URL,
		StatusCode:      int(fetchResult.StatusCode),
		RenderedMode:    fetchResult.RenderedWith,
		TimeTaken:       fetchResult.TimeTaken,
		Formats:         req.Formats,
		OnlyMainContent: req.OnlyMainContent,
		IncludeTags:     req.IncludeTags,
		ExcludeTags:     req.ExcludeTags,
		CSSSelector:     req.CSSSelector,
		XPath:           req.XPath,
		ChunkStrategy:   req.ChunkStrategy,
		Query:           req.Query,
		FilterMode:      req.FilterMode,
		TopK:            req.TopK,
	})

	extractElapsed := time.Since(extractStart)
	hasMarkdown := data.Markdown != nil
	markdownLen := 0
	if data.Markdown != nil {
		markdownLen = len(*data.Markdown)
	}
	log.Printf("[scrape] extract completed: url=%s duration=%v has_markdown=%v markdown_len=%d",
		req.URL, extractElapsed, hasMarkdown, markdownLen)

	if data.Warning != nil && warning != nil {
		combined := *data.Warning + "; " + *warning
		data.Warning = &combined
	} else if warning != nil {
		data.Warning = warning
	}

	if includesJSONFormat(req.Formats) {
		effectiveSchema := req.JSONSchema
		if effectiveSchema == nil && req.Extract != nil {
			effectiveSchema = &req.Extract.Schema
		}

		extractionPrompt := ""
		if req.LLMExtractionPrompt != nil {
			extractionPrompt = *req.LLMExtractionPrompt
		} else if req.Extract != nil && req.Extract.Prompt != "" {
			extractionPrompt = req.Extract.Prompt
		}

		responseFormat := ""
		if req.LLMResponseFormat != nil {
			responseFormat = *req.LLMResponseFormat
		} else if req.Extract != nil && req.Extract.ResponseFormat != "" {
			responseFormat = req.Extract.ResponseFormat
		}

		var byokConfig *types.LLMConfig
		if req.LLMAPIKey != nil {
			provider := "openai"
			if req.LLMProvider != nil {
				provider = *req.LLMProvider
			}
			model := "gpt-4o-mini"
			if req.LLMModel != nil {
				model = *req.LLMModel
			}
			byokConfig = &types.LLMConfig{
				APIKey:           *req.LLMAPIKey,
				Provider:         provider,
				Model:            model,
				MaxTokens:        defaultMaxTokens,
				ExtractionPrompt: extractionPrompt,
				ResponseFormat:   responseFormat,
			}
		}

		effectiveLLM := byokConfig
		if effectiveLLM == nil && llmConfig != nil {
			effectiveLLM = llmConfig
			if extractionPrompt != "" {
				effectiveLLM.ExtractionPrompt = extractionPrompt
			}
			if responseFormat != "" {
				effectiveLLM.ResponseFormat = responseFormat
			}
		}

		if effectiveSchema != nil && effectiveLLM != nil {
			if data.Markdown != nil {
				llmInput := buildLLMInput(data, req.ChunkStrategy, req.Query, req.FilterMode, req.TopK, req.MaxMarkdownChars)
				jsonResult, err := extractStructured(llmInput, *effectiveSchema, effectiveLLM)
				if err != nil {
					return nil, err
				}
				if jsonResult != "" {
					data.JSON = []byte(jsonResult)
				}
			}
		} else if effectiveSchema != nil && effectiveLLM == nil {
			return nil, types.ErrExtraction.New("json extraction requested but no LLM configured. Either set [extraction.llm] in server config, or pass 'llmApiKey' in the request body.")
		} else if effectiveSchema == nil {
			return nil, types.ErrInvalidRequest.New("Structured extraction (formats: json/extract) requires a 'jsonSchema' field. Provide a JSON Schema object.")
		}
	}

	totalElapsed := time.Since(totalStart)
	log.Printf("[scrape] scrape completed: url=%s total_duration=%v", req.URL, totalElapsed)

	return data, nil
}

// buildLLMInput builds the content to send to LLM based on chunking settings.
// If chunking is enabled with query, uses filtered chunks.
// Otherwise, uses full markdown or truncates by MaxMarkdownChars.
func buildLLMInput(data *types.ScrapeData, chunkStrategy *types.ChunkStrategy, query *string, filterMode *types.FilterMode, topK *int, maxMarkdownChars *int) string {
	if data.Markdown == nil {
		return ""
	}

	markdown := *data.Markdown

	// If chunking is enabled with query, use filtered chunks
	if chunkStrategy != nil && query != nil && len(*query) > 0 && data.Chunks != nil && len(data.Chunks) > 0 {
		var filtered []types.ChunkResult
		if filterMode != nil {
			chunkTexts := make([]string, len(data.Chunks))
			for i, c := range data.Chunks {
				chunkTexts[i] = c.Content
			}
			filtered = extractor.FilterChunksScored(chunkTexts, *query, filterMode, 5)
		} else {
			filtered = data.Chunks
		}

		// Apply TopK limit
		if topK != nil && *topK < len(filtered) {
			filtered = filtered[:*topK]
		}

		// Join filtered chunks
		var sb strings.Builder
		for i, chunk := range filtered {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(chunk.Content)
		}
		return sb.String()
	}

	// If MaxMarkdownChars is set, truncate
	if maxMarkdownChars != nil && *maxMarkdownChars > 0 && len(markdown) > *maxMarkdownChars {
		return markdown[:*maxMarkdownChars]
	}

	return markdown
}

// effectiveRenderJSSetting determines the effective JavaScript rendering setting
// by respecting user request while falling back to the default configuration.
func effectiveRenderJSSetting(request *bool, defaultVal *bool) *bool {
	if request != nil {
		return request
	}
	return defaultVal
}

// buildFetchWarning generates a warning message based on fetch result issues.
// It checks for non-success status codes and anti-bot challenge pages.
func buildFetchWarning(fetchResult *types.FetchResult) *string {
	if fetchResult.Warning != nil {
		return fetchResult.Warning
	}

	if fetchResult.StatusCode >= 400 {
		warning := "Target returned " + formatHTTPStatusCode(int(fetchResult.StatusCode)) + " " + humanReadableStatusText(fetchResult.StatusCode)
		return &warning
	}

	return detectBlockInterstitial(fetchResult.HTML)
}

// formatHTTPStatusCode converts a numeric status code to its 3-digit string representation.
func formatHTTPStatusCode(code int) string {
	if code < 100 || code > 999 {
		return "Error"
	}
	return string(rune('0'+code/100%10)) + string(rune('0'+code/10%10)) + string(rune('0'+code%10))
}

// humanReadableStatusText returns a human-readable description for common HTTP status codes.
func humanReadableStatusText(statusCode uint16) string {
	switch statusCode {
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 405:
		return "Method Not Allowed"
	case 408:
		return "Request Timeout"
	case 410:
		return "Gone"
	case 429:
		return "Too Many Requests"
	case 451:
		return "Unavailable For Legal Reasons"
	case 500:
		return "Internal Server Error"
	case 502:
		return "Bad Gateway"
	case 503:
		return "Service Unavailable"
	case 504:
		return "Gateway Timeout"
	default:
		return "Error"
	}
}

// detectBlockInterstitial checks if the HTML content indicates an anti-bot challenge page
// such as Cloudflare, CAPTCHA, or other access restriction pages.
func detectBlockInterstitial(html string) *string {
	if len(html) > 50000 {
		return nil
	}

	scanLimit := 128 * 1024
	end := len(html)
	if end > scanLimit {
		end = scanLimit
	}

	lower := strings.ToLower(html[:end])
	markers := []string{
		"just a moment",
		"attention required",
		"cf-browser-verification",
		"cf-challenge",
		"captcha",
		"access denied",
	}

	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			msg := "Blocked by anti-bot protection"
			return &msg
		}
	}

	return nil
}

// includesJSONFormat checks if the formats list contains JSON format.
func includesJSONFormat(formats []types.OutputFormat) bool {
	for _, f := range formats {
		if f == types.FormatJson {
			return true
		}
	}
	return false
}

// extractStructured uses an LLM to extract structured data from markdown content
// according to a JSON schema. Supports OpenAI with Structured Outputs.
func extractStructured(markdown string, schema json.RawMessage, llm *types.LLMConfig) (string, *types.QuickCrawlError) {
	if llm == nil {
		return "", types.ErrExtraction.New("JSON extraction requested but no LLM configured. Either set [extraction.llm] in server config, or pass 'llmApiKey' in the request body.")
	}
	if llm.APIKey == "" {
		return "", types.ErrExtraction.New("LLM API key is empty. Set [extraction.llm.api_key] or EXTRACTION__LLM__API_KEY.")
	}

	if len(schema) == 0 {
		return "", types.ErrInvalidRequest.New("Structured extraction (formats: json/extract) requires a 'jsonSchema' field. Provide a JSON Schema object.")
	}

	var schemaValue any
	if err := json.Unmarshal(schema, &schemaValue); err != nil {
		return "", types.ErrExtraction.New(fmt.Sprintf("Invalid JSON schema: %v", err))
	}

	result, err := callOpenAIAPI(markdown, schemaValue, llm)
	if err != nil {
		return "", err
	}

	if vErr := validateDataAgainstSchema(result, schemaValue); vErr != nil {
		return "", vErr
	}

	encoded, err2 := json.Marshal(result)
	if err2 != nil {
		return "", types.ErrExtraction.New(fmt.Sprintf("Failed to serialize structured extraction result: %v", err2))
	}
	return string(encoded), nil
}

// callOpenAIAPI sends a request to the OpenAI Chat API to extract structured data.
// Uses Structured Outputs with response_format for reliable schema adherence.
func callOpenAIAPI(markdown string, schema any, llm *types.LLMConfig) (any, *types.QuickCrawlError) {
	baseURL := "https://api.openai.com"
	if llm.BaseURL != nil && strings.TrimSpace(*llm.BaseURL) != "" {
		baseURL = strings.TrimSpace(*llm.BaseURL)
	}

	extractionPrompt := llm.ExtractionPrompt
	if extractionPrompt == "" {
		extractionPrompt = "You are a data extraction assistant. Extract structured data from the user's content according to the provided JSON schema. Return ONLY the JSON object that matches the schema."
	}

	responseFormat := llm.ResponseFormat
	if responseFormat == "" {
		responseFormat = "extracted_data"
	}

	reqBody := map[string]any{
		"model":      llm.Model,
		"max_tokens": llm.MaxTokens,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": extractionPrompt,
			},
			{
				"role":    "user",
				"content": "Extract structured data from the following content.\n\n## Content\n" + markdown,
			},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   responseFormat,
				"strict": true,
				"schema": schema,
			},
		},
	}
	status, text, err := sendJSONRequest(baseURL+"/v1/chat/completions", map[string]string{
		"authorization": "Bearer " + llm.APIKey,
		"content-type":  "application/json",
	}, reqBody)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, types.ErrExtraction.New(fmt.Sprintf("OpenAI API error (%s): %s", http.StatusText(status), truncateText(text)))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Refusal *string `json:"refusal"`
				Content *string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, types.ErrExtraction.New(fmt.Sprintf("Failed to parse OpenAI response: %v", err))
	}
	if len(parsed.Choices) == 0 {
		return nil, types.ErrExtraction.New("OpenAI returned no choices")
	}
	msg := parsed.Choices[0].Message

	if msg.Refusal != nil && *msg.Refusal != "" {
		return nil, types.ErrExtraction.New(fmt.Sprintf("OpenAI refused to extract: %s", *msg.Refusal))
	}

	if msg.Content == nil || *msg.Content == "" {
		return nil, types.ErrExtraction.New("OpenAI returned no content")
	}

	var result any
	if err := json.Unmarshal([]byte(*msg.Content), &result); err != nil {
		return nil, types.ErrExtraction.New(fmt.Sprintf("Failed to parse OpenAI JSON output: %v", err))
	}
	return result, nil
}

// sendJSONRequest makes an HTTP POST request with JSON body and returns the response.
func sendJSONRequest(rawURL string, headers map[string]string, body any) (int, string, *types.QuickCrawlError) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return 0, "", types.ErrExtraction.New(fmt.Sprintf("Failed to marshal LLM request: %v", err))
	}
	req, err := http.NewRequest(http.MethodPost, rawURL, bytes.NewReader(encoded))
	if err != nil {
		return 0, "", types.ErrExtraction.New(fmt.Sprintf("Failed to create LLM request: %v", err))
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := llmHTTPClient.Do(req)
	if err != nil {
		return 0, "", types.ErrExtraction.New(fmt.Sprintf("LLM API request failed: %v", err))
	}
	defer resp.Body.Close()
	text, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", types.ErrExtraction.New(fmt.Sprintf("Failed to read LLM response: %v", err))
	}
	return resp.StatusCode, string(text), nil
}

// parseJSONFromLLM extracts JSON from LLM response text, handling markdown code blocks.
func parseJSONFromLLM(text string) (any, *types.QuickCrawlError) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") {
		inner := strings.TrimPrefix(trimmed, "```json")
		if inner == trimmed {
			inner = strings.TrimPrefix(trimmed, "```")
		}
		trimmed = strings.TrimSuffix(inner, "```")
		trimmed = strings.TrimSpace(trimmed)
	}

	var out any
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		preview := truncateText(text)
		return nil, types.ErrExtraction.New(fmt.Sprintf("LLM returned invalid JSON: %v\nResponse preview: %s", err, preview))
	}
	return out, nil
}

// truncateText truncates text to 200 characters for safe error messages.
func truncateText(text string) string {
	if len(text) > 200 {
		return text[:200]
	}
	return text
}

// validateDataAgainstSchema validates extracted data against a JSON schema.
// Returns an error if validation fails.
func validateDataAgainstSchema(value any, schema any) *types.QuickCrawlError {
	schemaMap, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	if err := validateSchemaObject(value, schemaMap); err != nil {
		return types.ErrExtraction.New(fmt.Sprintf("LLM output failed schema validation: %v", err))
	}
	return nil
}

// validateSchemaObject recursively validates a value against a JSON schema object.
func validateSchemaObject(value any, schema map[string]any) error {
	if typ, ok := schema["type"]; ok {
		if err := validateValueType(value, typ); err != nil {
			return err
		}
	}
	if enumVals, ok := schema["enum"].([]any); ok {
		if !valueInEnum(enumVals, value) {
			return fmt.Errorf("value %#v not in enum", value)
		}
	}

	switch v := value.(type) {
	case map[string]any:
		if req, ok := schema["required"].([]any); ok {
			for _, item := range req {
				key, _ := item.(string)
				if key == "" {
					continue
				}
				if _, exists := v[key]; !exists {
					return fmt.Errorf("missing required field %q", key)
				}
			}
		}
		props, _ := schema["properties"].(map[string]any)
		for key, propSchemaRaw := range props {
			propVal, exists := v[key]
			if !exists {
				continue
			}
			propSchema, _ := propSchemaRaw.(map[string]any)
			if propSchema == nil {
				continue
			}
			if err := validateSchemaObject(propVal, propSchema); err != nil {
				return fmt.Errorf("field %q: %w", key, err)
			}
		}
		if addl, ok := schema["additionalProperties"].(bool); ok && !addl {
			for key := range v {
				if props != nil {
					if _, exists := props[key]; exists {
						continue
					}
				}
				return fmt.Errorf("unexpected additional property %q", key)
			}
		}
	case []any:
		if itemsSchema, ok := schema["items"].(map[string]any); ok {
			for i, item := range v {
				if err := validateSchemaObject(item, itemsSchema); err != nil {
					return fmt.Errorf("item %d: %w", i, err)
				}
			}
		}
	}
	return nil
}

// validateValueType checks if a value matches one of the allowed types in the schema.
func validateValueType(value any, typ any) error {
	switch t := typ.(type) {
	case string:
		return validateSimpleType(value, t)
	case []any:
		for _, candidate := range t {
			if cand, ok := candidate.(string); ok && validateSimpleType(value, cand) == nil {
				return nil
			}
		}
		return fmt.Errorf("value %T does not match allowed types", value)
	default:
		return nil
	}
}

// validateSimpleType checks if a value matches a specific JSON schema type.
func validateSimpleType(value any, typ string) error {
	switch typ {
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("expected object, got %T", value)
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("expected array, got %T", value)
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("expected number, got %T", value)
		}
	case "integer":
		if n, ok := value.(float64); !ok || n != float64(int64(n)) {
			return fmt.Errorf("expected integer, got %T", value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", value)
		}
	case "null":
		if value != nil {
			return fmt.Errorf("expected null, got %T", value)
		}
	}
	return nil
}

// valueInEnum checks if a value exists in the list of allowed enum values.
func valueInEnum(values []any, target any) bool {
	for _, candidate := range values {
		if fmt.Sprintf("%#v", candidate) == fmt.Sprintf("%#v", target) {
			return true
		}
	}
	return false
}
