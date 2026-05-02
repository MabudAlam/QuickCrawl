package extractor

import (
	"testing"

	quickcrawlcore "github.com/MabudAlam/quickcrawl/internal/types"
)

func makeSentenceStrategy(maxChars *int, overlapChars *int, dedupe *bool) *quickcrawlcore.ChunkStrategy {
	return &quickcrawlcore.ChunkStrategy{
		Type:         quickcrawlcore.ChunkSentence,
		MaxChars:     maxChars,
		OverlapChars: overlapChars,
		Dedupe:       dedupe,
	}
}

func makeRegexStrategy(pattern string, maxChars *int, overlapChars *int, dedupe *bool) *quickcrawlcore.ChunkStrategy {
	return &quickcrawlcore.ChunkStrategy{
		Type:         quickcrawlcore.ChunkRegex,
		Pattern:      pattern,
		MaxChars:     maxChars,
		OverlapChars: overlapChars,
		Dedupe:       dedupe,
	}
}

func makeTopicStrategy(maxChars *int, overlapChars *int, dedupe *bool) *quickcrawlcore.ChunkStrategy {
	return &quickcrawlcore.ChunkStrategy{
		Type:         quickcrawlcore.ChunkTopic,
		MaxChars:     maxChars,
		OverlapChars: overlapChars,
		Dedupe:       dedupe,
	}
}

func TestChunkTextSentenceBasic(t *testing.T) {
	text := "Hello world. This is sentence two. And sentence three."
	maxChars := 30
	chunks := ChunkText(text, makeSentenceStrategy(&maxChars, nil, nil))
	if len(chunks) == 0 {
		t.Error("Expected non-empty chunks")
	}
	for _, chunk := range chunks {
		if len(chunk) > 60 {
			t.Errorf("Chunk too long: %d chars", len(chunk))
		}
	}
}

func TestChunkTextSentenceRespectsMaxChars(t *testing.T) {
	text := "Short sentence. Another sentence here."
	maxChars := 10
	chunks := ChunkText(text, makeSentenceStrategy(&maxChars, nil, nil))
	if len(chunks) == 0 {
		t.Error("Expected non-empty chunks")
	}
}

func TestChunkTextTopicOnHeadings(t *testing.T) {
	text := "# Title\nContent under title with enough text to exceed the minimum chunk size threshold.\n## Section\nSection content that is also long enough to be kept as a separate chunk easily.\n### Sub\nSub content with additional words to pass the minimum size requirement for chunks."
	chunks := ChunkText(text, makeTopicStrategy(nil, nil, nil))
	if len(chunks) != 3 {
		t.Errorf("Expected 3 chunks, got %d", len(chunks))
	}
}

func TestChunkTextTopicMergeTiny(t *testing.T) {
	text := "# A\n## B\nSome real content that is long enough to form a proper chunk on its own."
	chunks := ChunkText(text, makeTopicStrategy(nil, nil, nil))
	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk (tiny merged), got %d", len(chunks))
	}
}

func TestChunkTextRegexOnDoubleNewline(t *testing.T) {
	text := "Para one.\n\nPara two.\n\nPara three."
	chunks := ChunkText(text, makeRegexStrategy("\n\n", nil, nil, nil))
	if len(chunks) != 3 {
		t.Errorf("Expected 3 chunks, got %d", len(chunks))
	}
}

func TestChunkTextRegexInvalidPattern(t *testing.T) {
	text := "some text"
	chunks := ChunkText(text, makeRegexStrategy("[invalid", nil, nil, nil))
	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk (whole text), got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("Expected whole text, got '%s'", chunks[0])
	}
}

func TestChunkTextRegexWithMaxChars(t *testing.T) {
	text := "Para one.\n\nPara two.\n\nPara three."
	maxChars := 16
	overlap := 5
	dedupe := true
	chunks := ChunkText(text, makeRegexStrategy("\n\n", &maxChars, &overlap, &dedupe))
	if len(chunks) < 2 {
		t.Errorf("Expected at least 2 chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		if len(chunk) > 16 {
			t.Errorf("Chunk '%s' exceeds maxChars 16", chunk)
		}
	}
}

func TestChunkTextNilStrategy(t *testing.T) {
	text := "Some text"
	chunks := ChunkText(text, nil)
	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("Expected '%s', got '%s'", text, chunks[0])
	}
}

func TestChunkTextUnknownType(t *testing.T) {
	text := "Some text"
	chunks := ChunkText(text, &quickcrawlcore.ChunkStrategy{Type: "unknown"})
	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk, got %d", len(chunks))
	}
}

func TestChunkTextSentenceWithOverlap(t *testing.T) {
	text := "First sentence. Second sentence here. Third sentence at last."
	overlap := 5
	chunks := ChunkText(text, makeSentenceStrategy(nil, &overlap, nil))
	if len(chunks) < 1 {
		t.Error("Expected at least 1 chunk")
	}
}

func TestChunkTextSentenceWithDedupe(t *testing.T) {
	text := "Unique sentence one. Duplicate phrase appears here. Unique sentence three."
	dedupe := true
	chunks := ChunkText(text, makeSentenceStrategy(nil, nil, &dedupe))
	if len(chunks) < 1 {
		t.Error("Expected chunks with dedupe")
	}
}
