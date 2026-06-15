package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/MabudAlam/quickcrawl/internal/types"
	"github.com/MabudAlam/quickcrawl/internal/utils"
	"github.com/redis/go-redis/v9"
	"github.com/klauspost/compress/zstd"
)

const (
	keyPrefix    = "qc:"
	cacheVersion = "v3" // bumped: switched from renderMode *bool to renderMode *RenderMode
)

type RedisCache struct {
	client *redis.Client
	cfg    types.CacheConfig
}

type cachedEntry struct {
	CachedAt   int64           `json:"cached_at"`
	URL        string          `json:"url"`
	Formats    string          `json:"formats"`
	RenderMode string          `json:"render_mode"`
	Data       json.RawMessage `json:"data"`
}

func NewRedisCache(cfg types.CacheConfig) (*RedisCache, error) {
	if !cfg.Enabled {
		return &RedisCache{cfg: cfg}, nil
	}

	if cfg.RedisURL == "" {
		return &RedisCache{cfg: cfg}, nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisURL,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &RedisCache{
		client: client,
		cfg:    cfg,
	}, nil
}

func (c *RedisCache) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

func (c *RedisCache) Enabled() bool {
	return c.cfg.Enabled && c.client != nil
}

func (c *RedisCache) generateKey(url string, formats []string, mode *types.RenderMode) string {
	formatsStr := strings.Join(formats, ",")
	modeStr := "unset"
	if mode != nil && *mode != "" {
		modeStr = string(*mode)
	}

	data := fmt.Sprintf("%s|%s|%s|%s", url, formatsStr, modeStr, cacheVersion)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%sscrape:%s", keyPrefix, hex.EncodeToString(hash[:]))
}

func (c *RedisCache) compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := zstd.NewWriter(&buf, zstd.WithWindowSize(256*1024))
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *RedisCache) decompress(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)
	decoder, err := zstd.NewReader(reader)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	return io.ReadAll(decoder)
}

func (c *RedisCache) Get(ctx context.Context, url string, formats []string, mode *types.RenderMode, ttl int64) (json.RawMessage, bool, error) {
	if !c.Enabled() {
		return nil, false, nil
	}

	key := c.generateKey(url, formats, mode)

	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		utils.Log.Warn("redis get failed", "key", key, "error", err)
		return nil, false, nil
	}

	decompressed, err := c.decompress(data)
	if err != nil {
		utils.Log.Warn("decompress failed", "key", key, "error", err)
		return nil, false, nil
	}

	var entry cachedEntry
	if err := json.Unmarshal(decompressed, &entry); err != nil {
		utils.Log.Warn("cache unmarshal failed", "key", key, "error", err)
		return nil, false, nil
	}

	cachedAt := entry.CachedAt
	age := time.Now().Unix() - cachedAt

	if ttl == 0 {
		return nil, false, nil
	}

	if ttl > 0 && age > ttl {
		_ = c.client.Del(ctx, key)
		return nil, false, nil
	}

	return entry.Data, true, nil
}

func (c *RedisCache) Set(ctx context.Context, url string, formats []string, mode *types.RenderMode, data json.RawMessage) error {
	if !c.Enabled() {
		return nil
	}

	key := c.generateKey(url, formats, mode)

	formatsStr := strings.Join(formats, ",")
	modeStr := "unset"
	if mode != nil && *mode != "" {
		modeStr = string(*mode)
	}

	entry := cachedEntry{
		CachedAt:   time.Now().Unix(),
		URL:        url,
		Formats:    formatsStr,
		RenderMode: modeStr,
		Data:       data,
	}

	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal cached result: %w", err)
	}

	compressed, err := c.compress(encoded)
	if err != nil {
		utils.Log.Warn("compression failed, storing uncompressed", "error", err)
		compressed = encoded
	}

	defaultTTL := time.Duration(c.cfg.TTLDefaultSecs) * time.Second
	if defaultTTL <= 0 {
		defaultTTL = 24 * time.Hour
	}

	if err := c.client.Set(ctx, key, compressed, defaultTTL).Err(); err != nil {
		utils.Log.Warn("redis set failed", "key", key, "error", err)
		return err
	}

	return nil
}

func (c *RedisCache) Delete(ctx context.Context, url string, formats []string, mode *types.RenderMode) error {
	if !c.Enabled() {
		return nil
	}

	key := c.generateKey(url, formats, mode)
	return c.client.Del(ctx, key).Err()
}

func (c *RedisCache) Flush(url string) error {
	if !c.Enabled() {
		return nil
	}

	ctx := context.Background()
	pattern := fmt.Sprintf("%sscrape:*", keyPrefix)

	var cursor uint64
	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		for _, key := range keys {
			data, err := c.client.Get(ctx, key).Bytes()
			if err != nil {
				continue
			}

			decompressed, err := c.decompress(data)
			if err != nil {
				continue
			}

			var entry cachedEntry
			if err := json.Unmarshal(decompressed, &entry); err != nil {
				continue
			}

			if entry.URL == url {
				_ = c.client.Del(ctx, key)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return nil
}

func (c *RedisCache) Stats(ctx context.Context) (map[string]interface{}, error) {
	if !c.Enabled() {
		return map[string]interface{}{
			"enabled": false,
		}, nil
	}

	info, err := c.client.Info(ctx, "memory").Result()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(info, "\n")
	stats := make(map[string]interface{})
	for _, line := range lines {
		if strings.HasPrefix(line, "used_memory_human") {
			stats["memory_used"] = strings.TrimSpace(strings.Split(line, ":")[1])
		}
	}

	var cursor uint64
	var keyCount int64
	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, keyPrefix+"*", 100).Result()
		if err != nil {
			return nil, err
		}
		keyCount += int64(len(keys))
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	stats["enabled"] = true
	stats["keys"] = keyCount
	stats["redis_url"] = c.cfg.RedisURL

	return stats, nil
}

func NormalizeFormats(formats []string) []string {
	if len(formats) == 0 {
		return []string{"markdown"}
	}
	sorted := make([]string, len(formats))
	copy(sorted, formats)
	sort.Strings(sorted)
	return sorted
}