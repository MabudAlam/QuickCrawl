package quickcrawl

import (
	"encoding/json"
	"testing"
)

func TestErrorResult(t *testing.T) {
	result, err := errorResult("test error")
	if err != nil {
		t.Fatalf("errorResult returned error: %v", err)
	}
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

func TestFormatCoreScrapeDataNil(t *testing.T) {
	result := formatCoreScrapeData(nil)
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("formatCoreScrapeData(nil) returned invalid JSON: %v", err)
	}
	if _, ok := data["error"]; !ok {
		t.Error("expected error field in result")
	}
}
