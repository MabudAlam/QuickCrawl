package search

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Standard BM25 parameters.
//
// k1 controls term-frequency saturation:
//   - larger values reward repeated terms more
//   - typical range: 1.2 - 2.0
//
// b controls document-length normalization:
//   - 0.0 disables length normalization
//   - 1.0 applies full normalization
//   - 0.75 is the common default
const (
	k1 = 1.5
	b  = 0.75
)

// tokenize splits text into lowercase tokens by removing whitespace
// and punctuation.
func tokenize(text string) []string {
	raw := strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})

	var tokens []string

	for _, token := range raw {
		token = strings.ToLower(token)

		if token != "" {
			tokens = append(tokens, token)
		}
	}

	return tokens
}

// documentLength returns the number of tokens in a document.
func documentLength(document string) int {
	return len(tokenize(document))
}

// averageDocumentLength computes the average token count across all documents in the corpus.
func averageDocumentLength(corpus []string) float64 {
	if len(corpus) == 0 {
		return 0
	}
	total := 0
	for _, doc := range corpus {
		total += documentLength(doc)
	}
	return float64(total) / float64(len(corpus))
}

// termFrequency returns the number of occurrences of a term inside a document.
func termFrequency(term string, document string) int {
	tf := 0
	for _, token := range tokenize(document) {
		if token == term {
			tf++
		}
	}
	return tf
}

// documentFrequency returns the number of documents in the corpus that contain the term.
func documentFrequency(term string, corpus []string) int {
	df := 0
	for _, doc := range corpus {
		if termFrequency(term, doc) > 0 {
			df++
		}
	}
	return df
}

// idf computes the BM25 inverse document frequency.
func idf(term string, corpus []string) float64 {
	N := float64(len(corpus))
	if N == 0 {
		return 0
	}
	df := float64(documentFrequency(term, corpus))
	if df == 0 {
		return 0
	}
	return math.Log(((N - df + 0.5) / (df + 0.5)) + 1.0)
}

// bm25TermScore computes the BM25 contribution of a single term for a specific document.
func bm25TermScore(term string, document string, corpus []string) float64 {
	tf := float64(termFrequency(term, document))
	if tf == 0 {
		return 0
	}

	dl := float64(documentLength(document))
	avgdl := averageDocumentLength(corpus)

	numerator := tf * (k1 + 1)
	denominator := tf + k1*(1-b+b*(dl/avgdl))

	return idf(term, corpus) * (numerator / denominator)
}

// bm25 computes the total BM25 score for a document against the query.
func bm25(query string, document string, corpus []string) float64 {
	score := 0.0
	for _, term := range tokenize(query) {
		score += bm25TermScore(term, document, corpus)
	}
	return score
}

// BM25FWeights holds the per-field weights used by BM25F.
type BM25FWeights struct {
	Title   float64
	Snippet float64
}

// DefaultBM25FWeights returns the default field weights.
func DefaultBM25FWeights() BM25FWeights {
	return BM25FWeights{
		Title:   2.0,
		Snippet: 1.0,
	}
}

// ComputeBM25FScores computes BM25F scores for documents with separate
// title and snippet fields, using the provided per-field weights.
func ComputeBM25FScores(
	query string,
	titles []string,
	snippets []string,
	weights BM25FWeights,
) map[int]float64 {
	if len(titles) != len(snippets) || len(titles) == 0 {
		return nil
	}
	if weights.Title <= 0 {
		weights.Title = 1.0
	}
	if weights.Snippet <= 0 {
		weights.Snippet = 1.0
	}

	// Build per-field corpora for length normalization.
	titleDocs := make([]string, len(titles))
	snippetDocs := make([]string, len(snippets))
	for i := range titles {
		titleDocs[i] = titles[i]
		snippetDocs[i] = snippets[i]
	}

	scores := make(map[int]float64, len(titles))
	for i := range titles {
		scores[i] = bm25f(
			query,
			[]bm25fField{
				{text: titleDocs[i], weight: weights.Title, corpus: titleDocs},
				{text: snippetDocs[i], weight: weights.Snippet, corpus: snippetDocs},
			},
		)
	}
	return scores
}

// bm25fField is one weighted field in a BM25F document.
type bm25fField struct {
	text    string
	weight  float64
	corpus  []string
}

// bm25f computes the BM25F score for a document with multiple weighted
// fields against the query.
//
// For each query term, the score is:
//
//	score = sum_over_fields( weight_f * (tf_f * (k1 + 1)) /
//	                         (tf_f + k1 * (1 - b + b * dl_f / avgdl_f)) ) * idf
//
// where idf is computed from the union of documents containing the term
// in any field (so a term that appears in *some* document's title still
// contributes a positive idf even if it never appears in any snippet).
func bm25f(query string, fields []bm25fField) float64 {
	score := 0.0
	for _, term := range tokenize(query) {
		// Compute idf over the union of field corpora: a term counts as
		// "in the corpus" if it appears in any field of any document.
		N := float64(len(fields[0].corpus))
		if N == 0 {
			continue
		}
		df := 0
		for _, f := range fields {
			for _, doc := range f.corpus {
				if termFrequency(term, doc) > 0 {
					df++
					break
				}
			}
		}
		if df == 0 {
			continue
		}
		termIDF := math.Log(((N - float64(df) + 0.5) / (float64(df) + 0.5)) + 1.0)

		// Sum weighted, length-normalized term frequency across all fields.
		termScore := 0.0
		for _, f := range fields {
			tf := float64(termFrequency(term, f.text))
			if tf == 0 {
				continue
			}
			dl := float64(documentLength(f.text))
			avgdl := averageDocumentLength(f.corpus)
			if avgdl == 0 {
				continue
			}
			numerator := tf * (k1 + 1)
			denominator := tf + k1*(1-b+b*(dl/avgdl))
			termScore += f.weight * (numerator / denominator)
		}
		score += termIDF * termScore
	}
	return score
}

// RerankByBM25 re-ranks results by BM25 score (descending) and updates positions accordingly.
// Returns a new slice with results sorted by bm25Score, with positions re-assigned starting from 1.
func RerankByBM25[T any](results []T, getBM25Score func(T) float64) []T {
	if len(results) <= 1 {
		return results
	}

	type indexedResult struct {
		index    int
		result   T
		bm25Score float64
	}

	indexed := make([]indexedResult, len(results))
	for i, r := range results {
		indexed[i] = indexedResult{
			index:      i,
			result:     r,
			bm25Score: getBM25Score(r),
		}
	}

	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].bm25Score > indexed[j].bm25Score
	})

	sorted := make([]T, len(results))
	for newPos, ir := range indexed {
		sorted[newPos] = ir.result
	}

	return sorted
}
