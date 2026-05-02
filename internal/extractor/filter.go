package extractor

import (
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/MabudAlam/quickcrawl/internal/types"
)

// ScoredChunk holds a chunk with its relevance score and original index.
type ScoredChunk struct {
	Content string
	Score   float64
	Index   int
}

var tokenizeTextPool = sync.Pool{
	New: func() any {
		b := &strings.Builder{}
		return b
	},
}

// FilterChunksScored filters and ranks text chunks based on their relevance
// to a query using either BM25 or cosine similarity algorithms. Returns
// up to topK scored chunks.
func FilterChunksScored(chunks []string, query string, mode *types.FilterMode, topK int) []types.ChunkResult {
	if len(chunks) == 0 || query == "" {
		results := make([]types.ChunkResult, len(chunks))
		for i, c := range chunks {
			results[i] = types.ChunkResult{
				Content: c,
				Score:   nil,
				Index:   i,
			}
		}
		return results
	}

	if topK <= 0 {
		topK = 5
	}
	if topK > len(chunks) {
		topK = len(chunks)
	}

	var scored []ScoredChunk
	if mode != nil && *mode == types.FilterCosine {
		scored = filterUsingCosine(chunks, query, topK)
	} else {
		scored = filterUsingBM25(chunks, query, topK)
	}

	results := make([]types.ChunkResult, len(scored))
	for i, s := range scored {
		results[i] = types.ChunkResult{
			Content: s.Content,
			Score:   &s.Score,
			Index:   s.Index,
		}
	}

	return results
}

// filterUsingBM25 ranks chunks using the Okapi BM25 algorithm with
// standard parameters (k1=1.2, b=0.75). BM25 is a probabilistic
// relevance model that handles term frequency saturation.
func filterUsingBM25(chunks []string, query string, topK int) []ScoredChunk {
	k1 := 1.2
	b := 0.75

	queryTokens := tokenizeText(query)
	tokenized := make([][]string, len(chunks))
	for i, c := range chunks {
		tokenized[i] = tokenizeText(c)
	}

	n := float64(len(chunks))
	totalTokens := 0
	for _, tokens := range tokenized {
		totalTokens += len(tokens)
	}

	// FIX: guard against divide-by-zero when all chunks are empty.
	avgDL := float64(totalTokens) / n
	if avgDL == 0 {
		avgDL = 1
	}

	df := make(map[string]int, len(tokenized)*2)
	for _, tokens := range tokenized {
		seen := make(map[string]bool, len(tokens))
		for _, t := range tokens {
			if !seen[t] {
				seen[t] = true
				df[t]++
			}
		}
	}

	scored := make([]ScoredChunk, 0, len(chunks))
	for i, tokens := range tokenized {
		dl := float64(len(tokens))
		tfMap := make(map[string]int, len(tokens))
		for _, t := range tokens {
			tfMap[t]++
		}

		var score float64
		for _, term := range queryTokens {
			tf := float64(tfMap[term])
			dfTerm := float64(df[term])
			idf := math.Log((n-dfTerm+0.5)/(dfTerm+0.5) + 1)
			tfNorm := (tf * (k1 + 1)) / (tf + k1*(1-b+b*dl/avgDL))
			score += idf * tfNorm
		}

		scored = append(scored, ScoredChunk{
			Content: chunks[i],
			Score:   score,
			Index:   i,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	if len(scored) > topK {
		scored = scored[:topK]
	}

	return scored
}

// filterUsingCosine ranks chunks using TF-IDF vector cosine similarity
// against the query. Each chunk is converted to a TF-IDF vector and
// similarity is computed against the query vector.
func filterUsingCosine(chunks []string, query string, topK int) []ScoredChunk {
	queryTokens := tokenizeText(query)

	tokenized := make([][]string, len(chunks))
	for i, c := range chunks {
		tokenized[i] = tokenizeText(c)
	}

	vocabSet := make(map[string]bool, len(queryTokens)*2)
	for _, tokens := range tokenized {
		for _, t := range tokens {
			vocabSet[t] = true
		}
	}
	for _, t := range queryTokens {
		vocabSet[t] = true
	}

	vocab := make([]string, 0, len(vocabSet))
	for t := range vocabSet {
		vocab = append(vocab, t)
	}
	sort.Strings(vocab)

	nDocs := float64(1 + len(chunks))

	// FIX: build a set of all query tokens so every query term gets +1 DF,
	// not just queryTokens[0] as the original code incorrectly did.
	queryTokenSet := make(map[string]bool, len(queryTokens))
	for _, t := range queryTokens {
		queryTokenSet[t] = true
	}

	idf := make(map[string]float64, len(vocab))
	for _, term := range vocab {
		df := 0
		// FIX: check membership in the full query token set.
		if queryTokenSet[term] {
			df++
		}
		for _, tokens := range tokenized {
			for _, t := range tokens {
				if t == term {
					df++
					break
				}
			}
		}
		idf[term] = math.Log((nDocs-float64(df)+0.5)/(float64(df)+0.5) + 1)
	}

	qVec := tfidf(queryTokens, vocab, idf)
	qNorm := norm(qVec)

	scored := make([]ScoredChunk, 0, len(chunks))
	for i, tokens := range tokenized {
		dVec := tfidf(tokens, vocab, idf)
		dNorm := norm(dVec)

		sim := 0.0
		if qNorm > 0 && dNorm > 0 {
			sim = dot(qVec, dVec) / (qNorm * dNorm)
		}

		scored = append(scored, ScoredChunk{
			Content: chunks[i],
			Score:   sim,
			Index:   i,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	if len(scored) > topK {
		scored = scored[:topK]
	}

	return scored
}

// tokenizeText splits text into lowercase alphanumeric tokens,
// filtering out punctuation and other characters. Uses a sync.Pool
// for memory efficiency when called frequently.
func tokenizeText(text string) []string {
	b := tokenizeTextPool.Get().(*strings.Builder)
	b.Reset()
	defer tokenizeTextPool.Put(b)

	var tokens []string
	text = strings.ToLower(text)
	for _, ch := range text {
		if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' {
			b.WriteRune(ch)
		} else {
			if b.Len() > 1 {
				tokens = append(tokens, b.String())
			}
			b.Reset()
		}
	}
	if b.Len() > 1 {
		tokens = append(tokens, b.String())
	}

	return tokens
}

// tfidf computes the TF-IDF vector for a set of tokens given a vocabulary
// and pre-computed IDF scores.
func tfidf(tokens, vocab []string, idf map[string]float64) []float64 {
	tf := make(map[string]int, len(tokens))
	for _, t := range tokens {
		tf[t]++
	}

	vec := make([]float64, len(vocab))
	for i, term := range vocab {
		tfVal := float64(tf[term])
		vec[i] = tfVal / math.Max(1, float64(len(tokens))) * idf[term]
	}
	return vec
}

// dot computes the dot product of two vectors.
func dot(a, b []float64) float64 {
	sum := 0.0
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// norm computes the Euclidean norm (L2 norm) of a vector.
func norm(v []float64) float64 {
	return math.Sqrt(dot(v, v))
}
