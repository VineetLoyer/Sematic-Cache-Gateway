package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"semantic-cache-gateway/internal/logger"
)

type CacheEntry struct {
	ID          string    `json:"id"`
	QueryHash   string    `json:"query_hash"`
	QueryText   string    `json:"user_query"`
	Embedding   []float32 `json:"embedding"`
	LLMResponse string    `json:"llm_response"`
	CreatedAt   int64     `json:"created_at"`
}

type CacheService interface {
	CheckExactMatch(ctx context.Context, queryHash string) (*CacheEntry, error)
	SearchSimilar(ctx context.Context, embedding []float32, threshold float64) (*CacheEntry, float64, error)
	StoreAsync(entry *CacheEntry)
	Clear(ctx context.Context) error
	Close() error
}

type CacheServiceImpl struct {
	redis     *RedisClient
	logger    *logger.Logger
	indexName string
}

type CacheServiceConfig struct {
	IndexName  string
	Dimensions int
}

// DefaultCacheServiceConfig returns default configuration.
func DefaultCacheServiceConfig() *CacheServiceConfig {
	return &CacheServiceConfig{IndexName: "cache_idx", Dimensions: 1536}
}

// NewCacheService creates a new CacheService with the given Redis client.
func NewCacheService(redis *RedisClient, log *logger.Logger, cfg *CacheServiceConfig) (*CacheServiceImpl, error) {
	if cfg == nil {
		cfg = DefaultCacheServiceConfig()
	}
	svc := &CacheServiceImpl{redis: redis, logger: log, indexName: cfg.IndexName}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := redis.CreateVectorIndex(ctx, cfg.IndexName, cfg.Dimensions); err != nil {
		return nil, fmt.Errorf("failed to create vector index: %w", err)
	}
	return svc, nil
}

// CacheKeyFromHash generates a cache key from a query hash.
func CacheKeyFromHash(queryHash string) string {
	hashID := strings.TrimPrefix(queryHash, "sha256:")
	return fmt.Sprintf("cache:%s", hashID)
}

// CheckExactMatch looks up a cache entry by its query hash.
func (c *CacheServiceImpl) CheckExactMatch(ctx context.Context, queryHash string) (*CacheEntry, error) {
	key := CacheKeyFromHash(queryHash)

	exists, err := c.redis.Exists(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to check key existence: %w", err)
	}
	if !exists {
		return nil, nil
	}

	data, err := c.redis.JSONGet(ctx, key, "$")
	if err != nil {
		return nil, fmt.Errorf("failed to get cache entry: %w", err)
	}
	if data == nil {
		return nil, nil
	}

	var entries []CacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache entry: %w", err)
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return &entries[0], nil
}

// Close releases resources held by the cache service.
func (c *CacheServiceImpl) Close() error {
	return c.redis.Close()
}

// SearchSimilar performs a KNN vector search to find semantically similar cached entries.
func (c *CacheServiceImpl) SearchSimilar(ctx context.Context, embedding []float32, threshold float64) (*CacheEntry, float64, error) {
	if len(embedding) == 0 {
		return nil, 0, fmt.Errorf("embedding cannot be empty")
	}

	embeddingBytes := float32SliceToBytes(embedding)
	query := "*=>[KNN 1 @embedding $vec AS __vector_score]"

	results, err := c.redis.FTSearch(ctx, c.indexName, query,
		"PARAMS", "2", "vec", embeddingBytes,
		"RETURN", "1", "$",
		"SORTBY", "__vector_score",
		"DIALECT", "2",
	)
	if err != nil {
		return nil, 0, fmt.Errorf("vector search failed: %w", err)
	}
	if len(results) == 0 {
		return nil, 0, nil
	}

	bestMatch := results[0]
	similarity := bestMatch.Score

	if similarity <= threshold {
		c.logger.Info("vector search below threshold", "similarity", similarity, "threshold", threshold)
		return nil, similarity, nil
	}

	if bestMatch.Document == nil {
		return nil, similarity, nil
	}

	var entry CacheEntry
	if err := json.Unmarshal(bestMatch.Document, &entry); err != nil {
		return nil, similarity, fmt.Errorf("failed to unmarshal cache entry: %w", err)
	}

	c.logger.Info("vector search hit", "similarity", similarity, "threshold", threshold, "cache_key", bestMatch.Key)
	return &entry, similarity, nil
}

func float32SliceToBytes(floats []float32) []byte {
	bytes := make([]byte, len(floats)*4)
	for i, f := range floats {
		bits := *(*uint32)(unsafe.Pointer(&f))
		bytes[i*4] = byte(bits)
		bytes[i*4+1] = byte(bits >> 8)
		bytes[i*4+2] = byte(bits >> 16)
		bytes[i*4+3] = byte(bits >> 24)
	}
	return bytes
}

// StoreAsync saves a new cache entry asynchronously.
func (c *CacheServiceImpl) StoreAsync(entry *CacheEntry) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := c.store(ctx, entry); err != nil {
			c.logger.Error("async cache write failed", "error", err.Error(), "cache_key", entry.ID, "query_hash", entry.QueryHash)
		} else {
			c.logger.Info("cache entry stored", "cache_key", entry.ID, "query_hash", entry.QueryHash)
		}
	}()
}

func (c *CacheServiceImpl) store(ctx context.Context, entry *CacheEntry) error {
	if err := validateCacheEntry(entry); err != nil {
		return fmt.Errorf("invalid cache entry: %w", err)
	}
	if entry.ID == "" {
		entry.ID = CacheKeyFromHash(entry.QueryHash)
	}
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().Unix()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	if err := c.redis.JSONSetRaw(ctx, entry.ID, "$", string(data)); err != nil {
		return fmt.Errorf("failed to store cache entry: %w", err)
	}
	return nil
}

func validateCacheEntry(entry *CacheEntry) error {
	if entry == nil {
		return fmt.Errorf("entry cannot be nil")
	}
	if entry.QueryHash == "" {
		return fmt.Errorf("query_hash is required")
	}
	if entry.QueryText == "" {
		return fmt.Errorf("user_query is required")
	}
	if len(entry.Embedding) == 0 {
		return fmt.Errorf("embedding is required")
	}
	if entry.LLMResponse == "" {
		return fmt.Errorf("llm_response is required")
	}
	return nil
}

// Store performs synchronous cache storage (for testing).
func (c *CacheServiceImpl) Store(ctx context.Context, entry *CacheEntry) error {
	return c.store(ctx, entry)
}

// Clear removes all cache entries from Redis.
func (c *CacheServiceImpl) Clear(ctx context.Context) error {
	client := c.redis.Client()
	
	// Delete all keys with cache: prefix
	var cursor uint64
	var deleted int64
	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, "cache:*", 100).Result()
		if err != nil {
			return fmt.Errorf("failed to scan keys: %w", err)
		}
		
		if len(keys) > 0 {
			count, err := client.Del(ctx, keys...).Result()
			if err != nil {
				return fmt.Errorf("failed to delete keys: %w", err)
			}
			deleted += count
		}
		
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	
	c.logger.Info("cache cleared", "deleted_keys", deleted)
	return nil
}
