package search

import (
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Hello World", []string{"hello", "world"}},
		{"Go-lang is GREAT!", []string{"go", "lang", "is", "great"}},
		{"  multiple   spaces  ", []string{"multiple", "spaces"}},
		{"simple text", []string{"simple", "text"}},
		{"", nil},
	}

	for _, tt := range tests {
		got := tokenize(tt.input)
		if len(got) == 0 && len(tt.expected) == 0 {
			continue
		}
		if !sliceEqual(got, tt.expected) {
			t.Errorf("tokenize(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTermFrequency(t *testing.T) {
	tests := []struct {
		term     string
		document string
		expected int
	}{
		{"golang", "learn golang programming", 1},
		{"python", "learn golang programming", 0},
		{"go", "go go go", 3},
		{"", "hello", 0},
		{"GOLANG", "Learn Golang", 0},
	}

	for _, tt := range tests {
		got := termFrequency(tt.term, tt.document)
		if got != tt.expected {
			t.Errorf("termFrequency(%q, %q) = %d, want %d", tt.term, tt.document, got, tt.expected)
		}
	}
}

func TestComputeBM25FScoresRareTerm(t *testing.T) {
	query := "kubernetes"

	titles := []string{
		"Beginner Kubernetes Guide",
		"Python Tutorial",
		"Web Development Basics",
	}
	snippets := []string{
		"Learn kubernetes orchestration",
		"Learn python from scratch",
		"Build websites with HTML",
	}

	weights := DefaultBM25FWeights()
	scores := ComputeBM25FScores(query, titles, snippets, weights)

	if scores == nil {
		t.Fatal("ComputeBM25FScores returned nil")
	}
	if len(scores) != 3 {
		t.Errorf("expected 3 scores, got %d", len(scores))
	}

	if scores[0] <= 0 {
		t.Errorf("D1 (kubernetes in both fields) should have positive score, got %.4f", scores[0])
	}
	if scores[1] != 0 {
		t.Errorf("D2 (no kubernetes) should have zero score, got %.4f", scores[1])
	}
	if scores[2] != 0 {
		t.Errorf("D3 (no kubernetes) should have zero score, got %.4f", scores[2])
	}
}

func TestComputeBM25FScoresFieldWeighting(t *testing.T) {
	query := "tutorial"

	titles := []string{
		"X Y Z",
		"Python Tutorial for Beginners",
	}
	snippets := []string{
		"Python Tutorial Guide Here",
		"Learn python basics",
	}

	weightsTitleHeavy := BM25FWeights{Title: 5.0, Snippet: 1.0}
	weightsEqual := BM25FWeights{Title: 1.0, Snippet: 1.0}

	scoresTitleHeavy := ComputeBM25FScores(query, titles, snippets, weightsTitleHeavy)
	scoresEqual := ComputeBM25FScores(query, titles, snippets, weightsEqual)

	scoreD1Equal := scoresEqual[0]
	scoreD2TitleHeavy := scoresTitleHeavy[1]
	scoreD2Equal := scoresEqual[1]

	if scoreD1Equal <= 0 {
		t.Errorf("D1 should have positive score, got %.4f", scoreD1Equal)
	}
	if scoreD2Equal <= 0 {
		t.Errorf("D2 (tutorial in title) should have positive score, got %.4f", scoreD2Equal)
	}

	if scoreD2TitleHeavy <= scoreD2Equal {
		t.Errorf("title-heavy weights should give D2 (tutorial in title) higher score: titleHeavy=%.4f, equal=%.4f",
			scoreD2TitleHeavy, scoreD2Equal)
	}
}

func TestComputeBM25FScoresEmptyInput(t *testing.T) {
	query := "golang"
	var titles, snippets []string

	weights := DefaultBM25FWeights()
	scores := ComputeBM25FScores(query, titles, snippets, weights)
	if scores != nil {
		t.Errorf("expected nil for empty input, got %v", scores)
	}
}

func TestComputeBM25FScoresMismatchedLength(t *testing.T) {
	query := "golang"
	titles := []string{"Golang Tutorial"}
	snippets := []string{"Learn golang", "Python is easy"}

	weights := DefaultBM25FWeights()
	scores := ComputeBM25FScores(query, titles, snippets, weights)
	if scores != nil {
		t.Errorf("expected nil for mismatched lengths, got %v", scores)
	}
}

func TestComputeBM25FScoresInvalidWeights(t *testing.T) {
	query := "golang"
	titles := []string{"Golang Tutorial", "Python Tutorial"}
	snippets := []string{"Learn golang", "Learn python"}

	invalidWeights := BM25FWeights{Title: 0, Snippet: 0}
	scores := ComputeBM25FScores(query, titles, snippets, invalidWeights)

	if scores == nil || len(scores) != 2 {
		t.Fatalf("expected 2 valid scores with default fallback weights, got %v", scores)
	}
}

func TestRerankByBM25(t *testing.T) {
	type doc struct {
		title string
		score float64
	}

	results := []doc{
		{title: "doc3", score: 1.0},
		{title: "doc1", score: 3.0},
		{title: "doc2", score: 2.0},
	}

	sorted := RerankByBM25(results, func(d doc) float64 {
		return d.score
	})

	if sorted[0].title != "doc1" {
		t.Errorf("expected doc1 first, got %s", sorted[0].title)
	}
	if sorted[1].title != "doc2" {
		t.Errorf("expected doc2 second, got %s", sorted[1].title)
	}
	if sorted[2].title != "doc3" {
		t.Errorf("expected doc3 third, got %s", sorted[2].title)
	}
}

func TestRerankByBM25Empty(t *testing.T) {
	type doc struct {
		title string
	}
	sorted := RerankByBM25([]doc{}, func(d doc) float64 { return 0 })
	if len(sorted) != 0 {
		t.Errorf("expected empty result, got %d", len(sorted))
	}
}

func TestRerankByBM25SingleDoc(t *testing.T) {
	type doc struct {
		title string
	}

	results := []doc{{title: "only"}}
	sorted := RerankByBM25(results, func(d doc) float64 {
		return 1.0
	})

	if len(sorted) != 1 || sorted[0].title != "only" {
		t.Errorf("single doc should remain unchanged")
	}
}

func TestBM25FWeightsDefaults(t *testing.T) {
	weights := DefaultBM25FWeights()
	if weights.Title != 2.0 {
		t.Errorf("default title weight should be 2.0, got %.2f", weights.Title)
	}
	if weights.Snippet != 1.0 {
		t.Errorf("default snippet weight should be 1.0, got %.2f", weights.Snippet)
	}
}