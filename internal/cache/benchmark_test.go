package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/klauspost/compress/zstd"
)

func getRedisURL() string {
	if url := os.Getenv("REDIS_URL"); url != "" {
		return url
	}
	return "localhost:6379"
}

func getRedisPassword() string {
	return os.Getenv("REDIS_PASSWORD")
}

func BenchmarkZstdCompress(b *testing.B) {
	largePayload := map[string]interface{}{
		"markdown":   string(make([]byte, 50000)),
		"html":       string(make([]byte, 80000)),
		"plainText":  string(make([]byte, 60000)),
		"links":      generateLinks(500),
		"imageLinks": generateLinks(200),
		"rawHtml":    string(make([]byte, 100000)),
		"metadata": map[string]interface{}{
			"title":       "Test Page Title for Benchmarking",
			"description": "This is a test page used for benchmarking cache performance with realistic payloads",
			"statusCode":  200,
			"url":         "https://example.com/benchmark-test",
		},
	}

	dataBytes, _ := json.Marshal(largePayload)
	b.Logf("Original size: %d bytes", len(dataBytes))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		writer, err := zstd.NewWriter(&buf, zstd.WithWindowSize(256*1024))
		if err != nil {
			b.Fatal(err)
		}
		if _, err := writer.Write(dataBytes); err != nil {
			b.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkZstdDecompress(b *testing.B) {
	largePayload := map[string]interface{}{
		"markdown":   string(make([]byte, 50000)),
		"html":       string(make([]byte, 80000)),
		"plainText":  string(make([]byte, 60000)),
		"links":      generateLinks(500),
		"imageLinks": generateLinks(200),
		"rawHtml":    string(make([]byte, 100000)),
		"metadata": map[string]interface{}{
			"title":       "Test Page Title for Benchmarking",
			"description": "This is a test page used for benchmarking cache performance with realistic payloads",
			"statusCode":  200,
			"url":         "https://example.com/benchmark-test",
		},
	}

	dataBytes, _ := json.Marshal(largePayload)

	var compressed bytes.Buffer
	writer, _ := zstd.NewWriter(&compressed, zstd.WithWindowSize(256*1024))
	writer.Write(dataBytes)
	writer.Close()

	compressedData := compressed.Bytes()
	b.Logf("Compressed size: %d bytes (%.1f%% of original)", len(compressedData), float64(len(compressedData))/float64(len(dataBytes))*100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(compressedData)
		decoder, err := zstd.NewReader(reader)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.ReadAll(decoder); err != nil {
			b.Fatal(err)
		}
		decoder.Close()
	}
}

func BenchmarkCacheGet(b *testing.B) {
	cfg := types.CacheConfig{
		Enabled:       true,
		RedisURL:      getRedisURL(),
		Password:      getRedisPassword(),
		DB:            0,
		TTLDefaultSecs: 300,
	}

	cache, err := NewRedisCache(cfg)
	if err != nil {
		b.Skipf("skipping benchmark: redis not available: %v", err)
	}

	ctx := context.Background()

	largePayload := map[string]interface{}{
		"markdown":   string(make([]byte, 50000)),
		"html":       string(make([]byte, 80000)),
		"plainText":  string(make([]byte, 60000)),
		"links":      generateLinks(500),
		"imageLinks": generateLinks(200),
		"rawHtml":    string(make([]byte, 100000)),
		"metadata": map[string]interface{}{
			"title":       "Test Page Title for Benchmarking",
			"description": "This is a test page used for benchmarking cache performance with realistic payloads",
			"statusCode":  200,
			"url":         "https://example.com/benchmark-test",
		},
	}

	dataBytes, _ := json.Marshal(largePayload)
	b.Logf("Payload size: %d bytes", len(dataBytes))

	testURL := "https://example.com/benchmark-large-payload-" + fmt.Sprintf("%d", time.Now().UnixNano())

	cache.Set(ctx, testURL, []string{"markdown", "html", "plainText", "links", "imageLinks", "rawHtml"}, boolPtr(true), dataBytes)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, found, err := cache.Get(ctx, testURL, []string{"markdown", "html", "plainText", "links", "imageLinks", "rawHtml"}, boolPtr(true), 300)
		if err != nil {
			b.Fatal(err)
		}
		if !found {
			b.Fatal("cache miss")
		}
	}
}

func BenchmarkCacheGetOnlyRedis(b *testing.B) {
	cfg := types.CacheConfig{
		Enabled:       true,
		RedisURL:      getRedisURL(),
		Password:      getRedisPassword(),
		DB:            0,
		TTLDefaultSecs: 300,
	}

	cache, err := NewRedisCache(cfg)
	if err != nil {
		b.Skipf("skipping benchmark: redis not available: %v", err)
	}

	ctx := context.Background()
	testKey := fmt.Sprintf("qc:scrape:benchonly:%d", time.Now().UnixNano())

	largeData := make([]byte, 300000)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	cache.client.Set(ctx, testKey, largeData, 5*time.Minute)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		data, err := cache.client.Get(ctx, testKey).Bytes()
		if err != nil {
			b.Fatal(err)
		}
		if len(data) != 300000 {
			b.Fatal("wrong data size")
		}
	}
}

func BenchmarkCacheGetWithUnmarshal(b *testing.B) {
	cfg := types.CacheConfig{
		Enabled:       true,
		RedisURL:      getRedisURL(),
		Password:      getRedisPassword(),
		DB:            0,
		TTLDefaultSecs: 300,
	}

	cache, err := NewRedisCache(cfg)
	if err != nil {
		b.Skipf("skipping benchmark: redis not available: %v", err)
	}

	ctx := context.Background()

	largePayload := map[string]interface{}{
		"markdown":   string(make([]byte, 50000)),
		"html":       string(make([]byte, 80000)),
		"plainText":  string(make([]byte, 60000)),
		"links":      generateLinks(500),
		"imageLinks": generateLinks(200),
		"rawHtml":    string(make([]byte, 100000)),
		"metadata": map[string]interface{}{
			"title":       "Test Page Title for Benchmarking",
			"description": "This is a test page used for benchmarking cache performance with realistic payloads",
			"statusCode":  200,
			"url":         "https://example.com/benchmark-test",
		},
	}

	dataBytes, _ := json.Marshal(largePayload)
	b.Logf("Payload size: %d bytes", len(dataBytes))

	testURL := "https://example.com/benchmark-unmarshal-" + fmt.Sprintf("%d", time.Now().UnixNano())
	formats := []string{"markdown", "html", "plainText", "links", "imageLinks", "rawHtml"}

	cache.Set(ctx, testURL, formats, boolPtr(true), dataBytes)

	type ScrapeData struct {
		Markdown   *string `json:"markdown,omitempty"`
		HTML       *string `json:"html,omitempty"`
		PlainText  *string `json:"plainText,omitempty"`
		Links      []string `json:"links,omitempty"`
		ImageLinks []string `json:"imageLinks,omitempty"`
		RawHTML    *string `json:"rawHtml,omitempty"`
		Metadata   map[string]interface{} `json:"metadata,omitempty"`
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cachedData, found, _ := cache.Get(ctx, testURL, formats, boolPtr(true), 300)
		if !found {
			b.Fatal("cache miss")
		}

		var data ScrapeData
		if err := json.Unmarshal(cachedData, &data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONUnmarshal(b *testing.B) {
	largePayload := map[string]interface{}{
		"markdown":   string(make([]byte, 50000)),
		"html":       string(make([]byte, 80000)),
		"plainText":  string(make([]byte, 60000)),
		"links":      generateLinks(500),
		"imageLinks": generateLinks(200),
		"rawHtml":    string(make([]byte, 100000)),
		"metadata": map[string]interface{}{
			"title":       "Test Page Title for Benchmarking",
			"description": "This is a test page used for benchmarking cache performance with realistic payloads",
			"statusCode":  200,
			"url":         "https://example.com/benchmark-test",
		},
	}

	dataBytes, _ := json.Marshal(largePayload)
	b.Logf("Payload size: %d bytes", len(dataBytes))

	type ScrapeData struct {
		Markdown   *string `json:"markdown,omitempty"`
		HTML       *string `json:"html,omitempty"`
		PlainText  *string `json:"plainText,omitempty"`
		Links      []string `json:"links,omitempty"`
		ImageLinks []string `json:"imageLinks,omitempty"`
		RawHTML    *string `json:"rawHtml,omitempty"`
		Metadata   map[string]interface{} `json:"metadata,omitempty"`
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var data ScrapeData
		if err := json.Unmarshal(dataBytes, &data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONMarshal(b *testing.B) {
	largePayload := map[string]interface{}{
		"markdown":   string(make([]byte, 50000)),
		"html":       string(make([]byte, 80000)),
		"plainText":  string(make([]byte, 60000)),
		"links":      generateLinks(500),
		"imageLinks": generateLinks(200),
		"rawHtml":    string(make([]byte, 100000)),
		"metadata": map[string]interface{}{
			"title":       "Test Page Title for Benchmarking",
			"description": "This is a test page used for benchmarking cache performance with realistic payloads",
			"statusCode":  200,
			"url":         "https://example.com/benchmark-test",
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(largePayload)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCacheLatencyBreakdown(b *testing.B) {
	cfg := types.CacheConfig{
		Enabled:       true,
		RedisURL:      getRedisURL(),
		Password:      getRedisPassword(),
		DB:            0,
		TTLDefaultSecs: 300,
	}

	cache, err := NewRedisCache(cfg)
	if err != nil {
		b.Skipf("skipping benchmark: redis not available: %v", err)
	}

	ctx := context.Background()

	largePayload := map[string]interface{}{
		"markdown":   string(make([]byte, 50000)),
		"html":       string(make([]byte, 80000)),
		"plainText":  string(make([]byte, 60000)),
		"links":      generateLinks(500),
		"imageLinks": generateLinks(200),
		"rawHtml":    string(make([]byte, 100000)),
		"metadata": map[string]interface{}{
			"title":       "Test Page Title for Benchmarking",
			"description": "This is a test page used for benchmarking cache performance with realistic payloads",
			"statusCode":  200,
			"url":         "https://example.com/benchmark-test",
		},
	}

	dataBytes, _ := json.Marshal(largePayload)
	b.Logf("Payload size: %d bytes", len(dataBytes))

	testURL := "https://example.com/benchmark-breakdown-" + fmt.Sprintf("%d", time.Now().UnixNano())
	formats := []string{"markdown", "html", "plainText", "links", "imageLinks", "rawHtml"}

	cache.Set(ctx, testURL, formats, boolPtr(true), dataBytes)

	type ScrapeData struct {
		Markdown   *string `json:"markdown,omitempty"`
		HTML       *string `json:"html,omitempty"`
		PlainText  *string `json:"plainText,omitempty"`
		Links      []string `json:"links,omitempty"`
		ImageLinks []string `json:"imageLinks,omitempty"`
		RawHTML    *string `json:"rawHtml,omitempty"`
		Metadata   map[string]interface{} `json:"metadata,omitempty"`
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := time.Now()
		cachedData, found, _ := cache.Get(ctx, testURL, formats, boolPtr(true), 300)
		redisTime := time.Since(start)

		if !found {
			b.Fatal("cache miss")
		}

		unmarshalStart := time.Now()
		var data ScrapeData
		if err := json.Unmarshal(cachedData, &data); err != nil {
			b.Fatal(err)
		}
		unmarshalTime := time.Since(unmarshalStart)

		totalTime := time.Since(start)
		b.Logf("Total: %v | Redis: %v | Decompress+Unmarshal: %v", totalTime, redisTime, unmarshalTime)
	}
}

func generateLinks(count int) []string {
	links := make([]string, count)
	for i := 0; i < count; i++ {
		links[i] = fmt.Sprintf("https://example.com/page/%d", i)
	}
	return links
}

func boolPtr(b bool) *bool {
	return &b
}