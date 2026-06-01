// Package core provides the LLM-based structured extraction used by /v1/scrape-core.
//
// The implementation is a self-contained port of the OpenAI Structured Outputs
// flow from internal/crawler/scrape.go. It supports the same request shape
// (jsonSchema, extract, llmExtractionPrompt, llmResponseFormat) and produces
// the same shape of response: the LLM-extracted JSON is stored in
// ScrapeData.JSON, ready to be serialized to the client.
//
// Errors are returned as *core.QuickCrawlError so the rest of this package
// can keep its own error type without conversion.
package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

// llmHTTPClient is shared by all LLM calls. It has a 60-second timeout and
// connection pooling so back-to-back requests reuse TCP connections.
var llmHTTPClient = &http.Client{
	Timeout: 60 * time.Second,
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	},
}

// llmExtractor owns the LLM extraction flow. It is stateless beyond the
// shared HTTP client; callers pass the *types.LLMConfig on every call so
// per-request overrides (prompt, response format) work.
type llmExtractor struct{}

// newLLMExtractor returns a new llmExtractor.
func newLLMExtractor() *llmExtractor {
	return &llmExtractor{}
}

// run drives the full extraction pipeline:
//  1. Validate inputs (LLM configured, schema present, API key set).
//  2. Call the LLM with the user-provided JSON schema.
//  3. Validate the response against the schema.
//  4. Return the marshaled JSON for storage in ScrapeData.JSON.
func (e *llmExtractor) run(markdown string, schema json.RawMessage, llm *types.LLMConfig) (json.RawMessage, *QuickCrawlError) {
	if llm == nil {
		return nil, ErrExtraction.New("json extraction requested but no LLM configured. Set [extraction.llm] in server config.")
	}
	if llm.APIKey == "" {
		return nil, ErrExtraction.New("LLM API key is empty. Set [extraction.llm.api_key] or EXTRACTION__LLM__API_KEY.")
	}
	if len(schema) == 0 {
		return nil, ErrInvalidRequest.New("Structured extraction (formats: json/extract) requires a 'jsonSchema' field. Provide a JSON Schema object.")
	}

	var schemaValue any
	if err := json.Unmarshal(schema, &schemaValue); err != nil {
		return nil, ErrExtraction.New(fmt.Sprintf("invalid JSON schema: %v", err))
	}

	result, callErr := callOpenAI(markdown, schemaValue, llm)
	if callErr != nil {
		return nil, callErr
	}
	if vErr := validateAgainstSchema(result, schemaValue); vErr != nil {
		return nil, vErr
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, ErrExtraction.New(fmt.Sprintf("failed to serialize structured extraction result: %v", err))
	}
	return encoded, nil
}

// callOpenAI POSTs to the OpenAI Chat Completions endpoint with the user's
// JSON schema wrapped in a response_format block. It returns the parsed
// content as a generic any (map[string]any, []any, string, float64, bool, nil).
func callOpenAI(markdown string, schema any, llm *types.LLMConfig) (any, *QuickCrawlError) {
	baseURL := "https://api.openai.com"
	if llm.BaseURL != nil && strings.TrimSpace(*llm.BaseURL) != "" {
		baseURL = strings.TrimSpace(*llm.BaseURL)
	}

	systemPrompt := llm.ExtractionPrompt
	if systemPrompt == "" {
		systemPrompt = "You are a data extraction assistant. Extract structured data from the user's content according to the provided JSON schema. Return ONLY the JSON object that matches the schema."
	}
	responseFormat := llm.ResponseFormat
	if responseFormat == "" {
		responseFormat = "extracted_data"
	}

	reqBody := map[string]any{
		"model":      llm.Model,
		"max_tokens": llm.MaxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": "Extract structured data from the following content.\n\n## Content\n" + markdown},
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

	status, text, err := postJSON(baseURL+"/v1/chat/completions", map[string]string{
		"authorization": "Bearer " + llm.APIKey,
		"content-type":  "application/json",
	}, reqBody)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, ErrExtraction.New(fmt.Sprintf("OpenAI API error (%s): %s", http.StatusText(status), truncate(text, 200)))
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
		return nil, ErrExtraction.New(fmt.Sprintf("failed to parse OpenAI response: %v", err))
	}
	if len(parsed.Choices) == 0 {
		return nil, ErrExtraction.New("OpenAI returned no choices")
	}
	msg := parsed.Choices[0].Message
	if msg.Refusal != nil && *msg.Refusal != "" {
		return nil, ErrExtraction.New(fmt.Sprintf("OpenAI refused to extract: %s", *msg.Refusal))
	}
	if msg.Content == nil || *msg.Content == "" {
		return nil, ErrExtraction.New("OpenAI returned no content")
	}
	return parseJSONContent(*msg.Content)
}

// postJSON performs an HTTP POST with a JSON body and returns the status
// code and response body text.
func postJSON(rawURL string, headers map[string]string, body any) (int, string, *QuickCrawlError) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return 0, "", ErrExtraction.New(fmt.Sprintf("failed to marshal LLM request: %v", err))
	}
	req, err := http.NewRequest(http.MethodPost, rawURL, bytes.NewReader(encoded))
	if err != nil {
		return 0, "", ErrExtraction.New(fmt.Sprintf("failed to create LLM request: %v", err))
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := llmHTTPClient.Do(req)
	if err != nil {
		return 0, "", ErrExtraction.New(fmt.Sprintf("LLM API request failed: %v", err))
	}
	defer resp.Body.Close()
	text, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", ErrExtraction.New(fmt.Sprintf("failed to read LLM response: %v", err))
	}
	return resp.StatusCode, string(text), nil
}

// parseJSONContent strips ```json ... ``` fences and unmarshals the result.
// Models occasionally wrap JSON in code blocks even with strict response_format.
func parseJSONContent(text string) (any, *QuickCrawlError) {
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
		return nil, ErrExtraction.New(fmt.Sprintf("LLM returned invalid JSON: %v\nResponse preview: %s", err, truncate(text, 200)))
	}
	return out, nil
}

// validateAgainstSchema is the entry point for schema validation. If schema
// is not an object (e.g. just a bool), it is a no-op.
func validateAgainstSchema(value any, schema any) *QuickCrawlError {
	schemaMap, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	if err := validateSchemaObject(value, schemaMap); err != nil {
		return ErrExtraction.New(fmt.Sprintf("LLM output failed schema validation: %v", err))
	}
	return nil
}

// validateSchemaObject recursively validates a parsed JSON value against a
// JSON schema object. Supports type, enum, required, properties,
// additionalProperties, and items. Other keywords are ignored.
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

// validateValueType accepts a JSON schema type field, which can be a single
// string ("string") or a list of strings (["string","null"] for nullable).
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

// validateSimpleType checks a single JSON schema primitive type. JSON numbers
// are always decoded as float64 in Go, so "integer" requires the value to be
// a float64 that equals its int64 truncation.
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

// valueInEnum returns true if target is one of the enum values. Comparison
// uses fmt.Sprintf("%#v", ...) so primitive types (string, float64, bool)
// compare correctly.
func valueInEnum(values []any, target any) bool {
	for _, candidate := range values {
		if fmt.Sprintf("%#v", candidate) == fmt.Sprintf("%#v", target) {
			return true
		}
	}
	return false
}

// truncate returns the first n characters of s.
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// resolveLLMInputs is a small helper that picks the effective schema, prompt,
// and response-format from the various request fields, mirroring the original
// /v1/scrape precedence (extract.* is preferred, then top-level fallbacks).
// It also mutates llm.ExtractionPrompt / llm.ResponseFormat when an override
// is provided, so callers can pass the result straight to the extractor.
func resolveLLMInputs(req *ScrapeRequest, llm *types.LLMConfig) (json.RawMessage, *types.LLMConfig) {
	schema := req.JSONSchema
	if schema == nil && req.Extract != nil {
		s := req.Extract.Schema
		schema = &s
	}

	var prompt, responseFormat string
	if req.LLMExtractionPrompt != nil {
		prompt = *req.LLMExtractionPrompt
	} else if req.Extract != nil && req.Extract.Prompt != "" {
		prompt = req.Extract.Prompt
	}
	if req.LLMResponseFormat != nil {
		responseFormat = *req.LLMResponseFormat
	} else if req.Extract != nil && req.Extract.ResponseFormat != "" {
		responseFormat = req.Extract.ResponseFormat
	}

	if llm != nil {
		if prompt != "" {
			llm.ExtractionPrompt = prompt
		}
		if responseFormat != "" {
			llm.ResponseFormat = responseFormat
		}
	}

	return derefSchema(schema), llm
}

// derefSchema returns the raw bytes of a schema pointer, or nil if it is nil.
func derefSchema(s *json.RawMessage) json.RawMessage {
	if s == nil {
		return nil
	}
	return *s
}
