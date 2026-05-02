package quickcrawl

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestErrorResult(t *testing.T) {
	result := errorResult("test error")
	if result == nil {
		t.Fatal("errorResult returned nil")
	}
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Content))
	}
}

func TestJsonString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", `"hello"`},
		{"hello world", `"hello world"`},
		{"hello\nworld", `"hello\nworld"`},
	}

	for _, tt := range tests {
		result := jsonString(tt.input)
		if result != tt.expected {
			t.Errorf("jsonString(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFormatScrapeDataNil(t *testing.T) {
	result := formatScrapeData(nil)
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("formatScrapeData(nil) returned invalid JSON: %v", err)
	}
	if _, ok := data["error"]; !ok {
		t.Error("expected error field in result")
	}
}

func TestNormalizePinnedRenderer(t *testing.T) {
	chrome := "chrome"
	auto := "auto"
	lightpanda := "lightpanda"
	tests := []struct {
		name      string
		renderer  *string
		browser   *string
		want      *string
		expectErr bool
	}{
		{name: "renderer only", renderer: &chrome, want: &chrome},
		{name: "browser only", browser: &chrome, want: &chrome},
		{name: "auto clears pin", renderer: &auto, want: nil},
		{name: "matching values", renderer: &chrome, browser: &chrome, want: &chrome},
		{name: "conflicting values", renderer: &chrome, browser: &lightpanda, expectErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizePinnedRenderer(tt.renderer, tt.browser)
			if tt.expectErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalizePinnedRenderer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScrapeInputSchemaAdvertisesRenderer(t *testing.T) {
	schema := scrapeInputSchema()
	props := schema["properties"].(map[string]any)
	renderer := props["renderer"].(map[string]any)

	if renderer["type"] != "string" {
		t.Fatalf("renderer type = %v, want string", renderer["type"])
	}

	enumVals := renderer["enum"].([]string)
	want := []string{"auto", "lightpanda", "chrome"}
	if !reflect.DeepEqual(enumVals, want) {
		t.Fatalf("renderer enum = %v, want %v", enumVals, want)
	}
}

func TestCrawlInputSchemaAdvertisesRenderer(t *testing.T) {
	schema := crawlInputSchema()
	props := schema["properties"].(map[string]any)
	renderer := props["renderer"].(map[string]any)

	if renderer["type"] != "string" {
		t.Fatalf("renderer type = %v, want string", renderer["type"])
	}

	enumVals := renderer["enum"].([]string)
	want := []string{"auto", "lightpanda", "chrome"}
	if !reflect.DeepEqual(enumVals, want) {
		t.Fatalf("renderer enum = %v, want %v", enumVals, want)
	}
}
