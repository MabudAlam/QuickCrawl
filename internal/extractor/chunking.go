package extractor

import (
	"regexp"
	"strings"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

var sentenceBoundaryRe = regexp.MustCompile(`([.!?]+)\s+`)

// ChunkText splits text into chunks according to the specified chunking strategy.
// Supports three strategy types: sentence-based splitting, regex-based splitting,
// and topic-based splitting (by Markdown headers).
func ChunkText(text string, strategy *types.ChunkStrategy) []string {
	if strategy == nil {
		return []string{text}
	}

	switch strategy.Type {
	case types.ChunkSentence:
		return chunkBySentence(text, strategy.MaxChars, strategy.OverlapChars, strategy.Dedupe)
	case types.ChunkRegex:
		return chunkByRegex(text, strategy.Pattern, strategy.MaxChars, strategy.OverlapChars, strategy.Dedupe)
	case types.ChunkTopic:
		return chunkByTopic(text, strategy.MaxChars, strategy.OverlapChars, strategy.Dedupe)
	default:
		return []string{text}
	}
}

// chunkBySentence splits text into chunks by sentence boundaries (.!?)
// and merges chunks that are too small. It respects maxChars and can
// apply deduplication and overlap.
func chunkBySentence(text string, maxChars, overlapChars *int, dedupe *bool) []string {
	max := 1000
	if maxChars != nil {
		max = *maxChars
	}
	minMerge := max / 4

	// Use FindAllStringIndex to preserve punctuation on sentences.
	sentences := make([]string, 0)
	last := 0
	for _, loc := range sentenceBoundaryRe.FindAllStringSubmatchIndex(text, -1) {
		// loc[0]:loc[1] is the full match (.!? + whitespace)
		// loc[2]:loc[3] is capture group 1 (the punctuation only)
		punctEnd := loc[3]
		sentence := strings.TrimSpace(text[last:punctEnd])
		if sentence != "" {
			sentences = append(sentences, sentence)
		}
		last = loc[1]
	}
	if last < len(text) {
		remaining := strings.TrimSpace(text[last:])
		if remaining != "" {
			sentences = append(sentences, remaining)
		}
	}

	chunks := make([]string, 0, (len(sentences)+1)/2)
	var current strings.Builder

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		if current.Len() == 0 {
			current.WriteString(sentence)
		} else if current.Len()+len(sentence)+1 <= max {
			current.WriteString(" ")
			current.WriteString(sentence)
		} else {
			chunk := strings.TrimSpace(current.String())
			if chunk != "" {
				chunks = append(chunks, chunk)
			}
			current.Reset()
			current.WriteString(sentence)
		}
	}

	if current.Len() > 0 {
		chunk := strings.TrimSpace(current.String())
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
	}

	// FIX: guard the merge so it never pushes chunks[len-2] past maxChars.
	if len(chunks) > 1 {
		last := chunks[len(chunks)-1]
		prev := chunks[len(chunks)-2]
		if len(last) < minMerge && len(prev)+1+len(last) <= max {
			chunks[len(chunks)-2] += " " + last
			chunks = chunks[:len(chunks)-1]
		}
	}

	// FIX: split oversized chunks first, then dedupe — so split twins can be caught.
	if maxChars != nil && *maxChars > 0 {
		chunks = splitOversizedChunks(chunks, *maxChars, overlapChars)
	}

	if dedupe == nil || *dedupe {
		chunks = removeDuplicateChunks(chunks)
	}

	return chunks
}

// chunkByRegex splits text using a custom regex pattern and optionally
// deduplicates the resulting chunks.
func chunkByRegex(text string, pattern string, maxChars, overlapChars *int, dedupe *bool) []string {
	if pattern == "" {
		return []string{text}
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return []string{text}
	}

	parts := re.Split(text, -1)
	chunks := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			chunks = append(chunks, trimmed)
		}
	}

	if dedupe == nil || *dedupe {
		chunks = removeDuplicateChunks(chunks)
	}

	return chunks
}

// chunkByTopic splits text by Markdown headers (lines starting with #)
// and merges very small chunks. Each header starts a new topic chunk.
func chunkByTopic(text string, maxChars, overlapChars *int, dedupe *bool) []string {
	// FIX: removed dead `lines` variable; reuse the single Split result directly.
	chunks := make([]string, 0)
	var current strings.Builder

	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") && current.Len() > 0 {
			chunk := strings.TrimSpace(current.String())
			if chunk != "" {
				chunks = append(chunks, chunk)
			}
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}

	if current.Len() > 0 {
		chunk := strings.TrimSpace(current.String())
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
	}

	chunks = mergeSmallChunks(chunks, 50)

	if dedupe == nil || *dedupe {
		chunks = removeDuplicateChunks(chunks)
	}

	return chunks
}

// mergeSmallChunks combines consecutive chunks that are below minChars
// threshold by merging them with preceding chunks until the threshold
// is met or all chunks are consumed.
func mergeSmallChunks(chunks []string, minChars int) []string {
	if len(chunks) == 0 {
		return chunks
	}

	result := make([]string, 0, len(chunks))
	var carry string

	for _, chunk := range chunks {
		if carry != "" {
			combined := carry + "\n\n" + chunk
			if len(combined) >= minChars {
				result = append(result, combined)
				carry = ""
			} else {
				carry = combined
			}
		} else if len(chunk) < minChars {
			carry = chunk
		} else {
			result = append(result, chunk)
		}
	}

	if carry != "" {
		if len(result) > 0 {
			result[len(result)-1] += "\n\n" + carry
		} else {
			result = append(result, carry)
		}
	}

	return result
}

// removeDuplicateChunks deduplicates chunks by filtering out content
// that appears visually identical when whitespace-normalized.
func removeDuplicateChunks(chunks []string) []string {
	seen := make(map[string]bool, len(chunks))
	result := make([]string, 0, len(chunks))

	for _, chunk := range chunks {
		normalized := canonicalizeChunk(chunk)
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, chunk)
		}
	}

	return result
}

// canonicalizeChunk normalizes chunk text by lowercasing and removing
// extra whitespace for comparison purposes.
func canonicalizeChunk(chunk string) string {
	tokens := strings.Fields(chunk)
	return strings.ToLower(strings.Join(tokens, " "))
}

// splitOversizedChunks splits chunks that exceed maxChars into smaller
// pieces, optionally overlapping with the previous chunk using overlapChars.
func splitOversizedChunks(chunks []string, maxChars int, overlapChars *int) []string {
	overlap := 0
	if overlapChars != nil {
		overlap = *overlapChars
	}

	var result []string
	for _, chunk := range chunks {
		if len(chunk) <= maxChars {
			result = append(result, chunk)
			continue
		}

		words := strings.Fields(chunk)
		var current strings.Builder

		for _, word := range words {
			if current.Len()+len(word)+1 > maxChars {
				if current.Len() > 0 {
					// FIX: capture content BEFORE reset so overlap can reference it.
					prev := current.String()
					result = append(result, strings.TrimSpace(prev))
					current.Reset()

					if overlap > 0 {
						// FIX: use `prev` (not the now-empty builder) and no variable shadowing.
						prevWords := strings.Fields(prev)
						start := len(prevWords) - overlap/10
						if start < 0 {
							start = 0
						}
						overlapText := strings.Join(prevWords[start:], " ")
						current.WriteString(overlapText)
						current.WriteString(" ")
					}
				}
			}
			current.WriteString(word)
			current.WriteString(" ")
		}

		if current.Len() > 0 {
			result = append(result, strings.TrimSpace(current.String()))
		}
	}

	return result
}
