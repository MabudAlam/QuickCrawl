package extractor

import (
	"strings"
	"testing"

	quickcrawlcore "github.com/MabudAlam/quickcrawl/internal/types"
)

func sampleChunks() []string {
	return []string{
		"The quick brown fox jumps over the lazy dog.",
		"Machine learning is a subset of artificial intelligence.",
		"Rust is a systems programming language focused on safety.",
		"Natural language processing enables computers to understand text.",
	}
}

func TestFilterChunksBm25ReturnsTopK(t *testing.T) {
	chunks := sampleChunks()
	bm25Mode := quickcrawlcore.FilterBm25
	result := FilterChunksScored(chunks, "machine learning AI", &bm25Mode, 2)
	if len(result) != 2 {
		t.Errorf("Expected 2 chunks, got %d", len(result))
	}
	found := false
	for _, c := range result {
		if containsIgnoreCase(c.Content, "machine learning") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected ML chunk to be ranked high")
	}
}

func TestFilterChunksCosineReturnsTopK(t *testing.T) {
	chunks := sampleChunks()
	cosineMode := quickcrawlcore.FilterCosine
	result := FilterChunksScored(chunks, "programming language Rust", &cosineMode, 2)
	if len(result) != 2 {
		t.Errorf("Expected 2 chunks, got %d", len(result))
	}
	found := false
	for _, c := range result {
		if containsIgnoreCase(c.Content, "rust") || containsIgnoreCase(c.Content, "language") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected Rust/language chunk to be ranked high")
	}
}

func TestFilterChunksEmptyQueryReturnsAll(t *testing.T) {
	chunks := sampleChunks()
	bm25Mode := quickcrawlcore.FilterBm25
	result := FilterChunksScored(chunks, "", &bm25Mode, 2)
	if len(result) != len(chunks) {
		t.Errorf("Expected all %d chunks, got %d", len(chunks), len(result))
	}
}

func TestFilterChunksTopKCappedAtChunkCount(t *testing.T) {
	chunks := []string{"a", "b"}
	bm25Mode := quickcrawlcore.FilterBm25
	result := FilterChunksScored(chunks, "a", &bm25Mode, 100)
	if len(result) != 2 {
		t.Errorf("Expected 2 chunks, got %d", len(result))
	}
}

func TestFilterChunksRankingOrderIsPreserved(t *testing.T) {
	chunks := []string{
		"irrelevant background",
		"rust programming language ownership borrow checker",
		"rust",
	}
	bm25Mode := quickcrawlcore.FilterBm25
	result := FilterChunksScored(chunks, "rust programming language", &bm25Mode, 2)
	if result[0].Content != chunks[1] {
		t.Errorf("Expected highest ranked to be '%s', got '%s'", chunks[1], result[0].Content)
	}
}

func TestFilterChunksBm25AndCosineCanDiverge(t *testing.T) {
	chunks := []string{
		"token token token token token token token token",
		"token related semantic context",
		"unrelated content",
	}
	bm25Mode := quickcrawlcore.FilterBm25
	cosineMode := quickcrawlcore.FilterCosine

	bm25Result := FilterChunksScored(chunks, "token semantic", &bm25Mode, 2)
	cosineResult := FilterChunksScored(chunks, "token semantic", &cosineMode, 2)

	if len(bm25Result) != 2 || len(cosineResult) != 2 {
		t.Error("Expected 2 results each")
	}
}

func TestFilterChunksCosineNormalizesScores(t *testing.T) {
	chunks := []string{
		"apple apple apple",
		"banana banana banana",
		"apple banana",
	}
	cosineMode := quickcrawlcore.FilterCosine
	result := FilterChunksScored(chunks, "apple", &cosineMode, 3)
	if result[0].Score != nil && (*result[0].Score > 1.0 || *result[0].Score < 0) {
		t.Errorf("Cosine similarity should be between 0 and 1, got %f", *result[0].Score)
	}
}

func TestFilterChunksWithNilModeUsesBm25(t *testing.T) {
	chunks := []string{"test chunk", "another chunk"}
	result := FilterChunksScored(chunks, "test", nil, 2)
	if len(result) != 2 {
		t.Errorf("Expected 2 chunks, got %d", len(result))
	}
}

func TestFilterChunksEmptyChunks(t *testing.T) {
	chunks := []string{}
	bm25Mode := quickcrawlcore.FilterBm25
	result := FilterChunksScored(chunks, "query", &bm25Mode, 5)
	if len(result) != 0 {
		t.Errorf("Expected 0 chunks, got %d", len(result))
	}
}

func TestFilterChunksWithScores(t *testing.T) {
	chunks := []string{
		"apple banana cherry",
		"dog elephant fox",
		"green apple red apple",
	}
	bm25Mode := quickcrawlcore.FilterBm25
	result := FilterChunksScored(chunks, "apple", &bm25Mode, 3)
	if len(result) != 3 {
		t.Errorf("Expected 3 chunks, got %d", len(result))
	}
	if result[0].Score == nil {
		t.Error("Expected score to be set")
	}
	if result[0].Index != 0 && result[0].Index != 2 {
		t.Error("Expected first result to be about apples")
	}
}

func TestTokenize(t *testing.T) {
	tokens := tokenizeText("Hello, World! 123")
	if len(tokens) < 2 {
		t.Errorf("Expected tokens, got %v", tokens)
	}
}

func TestTokenizeLowercases(t *testing.T) {
	tokens := tokenizeText("HELLO world")
	found := false
	for _, t := range tokens {
		if t == "hello" || t == "world" {
			found = true
		}
	}
	if !found {
		t.Error("Expected lowercase tokens")
	}
}

func TestTokenizeIgnoresSingleChars(t *testing.T) {
	tokens := tokenizeText("a b c")
	if len(tokens) != 0 {
		t.Errorf("Expected no tokens for single chars, got %v", tokens)
	}
}

func TestTokenizeMixed(t *testing.T) {
	tokens := tokenizeText("Hello-world 123 test!")
	if len(tokens) < 2 {
		t.Errorf("Expected tokens, got %v", tokens)
	}
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && containsLower(s, strings.ToLower(substr))
}

func containsLower(s, lower string) bool {
	for i := 0; i <= len(s)-len(lower); i++ {
		if strings.ToLower(s[i:i+len(lower)]) == lower {
			return true
		}
	}
	return false
}
