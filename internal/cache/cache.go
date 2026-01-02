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

// CacheEntry represents a cached LLM response with its embedding.
type CacheEntry struct {
	ID          string          `json:"id"`
	QueryHash   string          `json:"query_hash"`
	QueryText   string          `json:"user_query"`
	Embedding   []float32       `json:"embedding"`
	LLMResponse json.RawMessage `json:"llm_response"`
	CreatedAt   int64           `json:"created_at"`
}

// CacheService defines the interface for cache operations.
type CacheService interface {
	// CheckExactMatch returns cached response if SHA-256 hash matches.
	CheckExactMatch(ctx context.Context, queryHash string) (*CacheEntry, error)

	// SearchSimilar performs vector KNN search, returns entry if similarity > threshold.
	SearchSimilar(ctx context.Context, embedding []float32, threshold float64) (*CacheEntry, float64, error)

	// StoreAsync saves a new cache entry asynchronously.
	StoreAsync(entry *CacheEntry)

	// Close releases any resources held by the cache service.
	Close() error
}

// CacheServiceImpl implements CacheService using Redis Stack.
type CacheServiceImpl struct {
	redis     *RedisClient
	logger    *logger.Logger
	indexName string
}

// CacheServiceConfig holds configuration for the cache service.
type CacheServiceConfig struct {
	IndexName  string
	Dimensions int
}


// DefaultCacheServiceConfig returns default configuration.
func DefaultCacheServiceConfig() *CacheServiceConfig {
	return &CacheServiceConfig{
		IndexName:  "cache_idx",
		Dimensions: 1536,
	}
}

// NewCacheService creates a new CacheService with the given Redis client.
func NewCacheService(redis *RedisClient, log *logger.Logger, cfg *CacheServiceConfig) (*CacheServiceImpl, error) {
	if cfg == nil {
		cfg = DefaultCacheServiceConfig()
	}

	svc := &CacheServiceImpl{
		redis:     redis,
		logger:    log,
		indexName: cfg.IndexName,
	}

	// Create vector index if it doesn't exist
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := redis.CreateVectorIndex(ctx, cfg.IndexName, cfg.Dimensions); err != nil {
		return nil, fmt.Errorf("failed to create vector index: %w", err)
	}

	return svc, nil
}

// CacheKeyFromHash generates a cache key from a query hash.
// The key format is: cache:{hash_id}
// where hash_id is the hash value without the "sha256:" prefix.
func CacheKeyFromHash(queryHash string) string {
	// Remove "sha256:" prefix if present
	hashID := strings.TrimPrefix(queryHash, "sha256:")
	return fmt.Sprintf("cache:%s", hashID)
}

// CheckExactMatch looks up a cache entry by its query hash.
// Returns the cached entry if found, nil if not found.
func (c *CacheServiceImpl) CheckExactMatch(ctx context.Context, queryHash string) (*CacheEntry, error) {
	key := CacheKeyFromHash(queryHash)

	// Check if key exists
	exists, err := c.redis.Exists(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to check key existence: %w", err)
	}

	if !exists {
		return nil, nil
	}

	// Get the JSON document
	data, err := c.redis.JSONGet(ctx, key, "$")
	if err != nil {
		return nil, fmt.Errorf("failed to get cache entry: %w", err)
	}

	if data == nil {
		return nil, nil
	}

	// JSON.GET with $ path returns an array
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
// Returns the best matching entry if similarity exceeds the threshold, nil otherwise.
// The similarity score is returned as the second value (0.0 to 1.0).
func (c *CacheServiceImpl) SearchSimilar(ctx context.Context, embedding []float32, threshold float64) (*CacheEntry, float64, error) {
	if len(embedding) == 0 {
		return nil, 0, fmt.Errorf("embedding cannot be empty")
	}

	// Convert embedding to bytes for the query
	embeddingBytes := float32SliceToBytes(embedding)

	// Build KNN query for vector similarity search
	// Using HNSW index with cosine similarity
	// Query format: *=>[KNN 1 @embedding $vec AS __vector_score]
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

	// Get the best match
	bestMatch := results[0]
	similarity := bestMatch.Score

	// Check if similarity meets threshold
	if similarity <= threshold {
		c.logger.Info("vector search below threshold",
			"similarity", similarity,
			"threshold", threshold,
		)
		return nil, similarity, nil
	}

	// Parse the document
	if bestMatch.Document == nil {
		return nil, similarity, nil
	}

	var entry CacheEntry
	if err := json.Unmarshal(bestMatch.Document, &entry); err != nil {
		return nil, similarity, fmt.Errorf("failed to unmarshal cache entry: %w", err)
	}

	c.logger.Info("vector search hit",
		"similarity", similarity,
		"threshold", threshold,
		"cache_key", bestMatch.Key,
	)

	return &entry, similarity, nil
}

// float32SliceToBytes converts a float32 slice to a byte slice for Redis vector queries.
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


// StoreAsync saves a new cache entry asynchronously using a goroutine.
// This implements write-behind caching to avoid impacting response latency.
// Any errors during storage are logged but do not affect the caller.
func (c *CacheServiceImpl) StoreAsync(entry *CacheEntry) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := c.store(ctx, entry); err != nil {
			c.logger.Error("async cache write failed",
				"error", err.Error(),
				"cache_key", entry.ID,
				"query_hash", entry.QueryHash,
			)
		} else {
			c.logger.Info("cache entry stored",
				"cache_key", entry.ID,
				"query_hash", entry.QueryHash,
			)
		}
	}()
}

// store performs the actual cache storage operation.
func (c *CacheServiceImpl) store(ctx context.Context, entry *CacheEntry) error {
	// Validate entry has all required fields
	if err := validateCacheEntry(entry); err != nil {
		return fmt.Errorf("invalid cache entry: %w", err)
	}

	// Generate cache key if not set
	if entry.ID == "" {
		entry.ID = CacheKeyFromHash(entry.QueryHash)
	}

	// Set timestamp if not set
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().Unix()
	}

	// Store the entry as JSON
	if err := c.redis.JSONSet(ctx, entry.ID, "$", entry); err != nil {
		return fmt.Errorf("failed to store cache entry: %w", err)
	}

	return nil
}

// validateCacheEntry checks that all required fields are present.
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
	if len(entry.LLMResponse) == 0 {
		return fmt.Errorf("llm_response is required")
	}
	return nil
}

// Store performs synchronous cache storage (for testing purposes).
func (c *CacheServiceImpl) Store(ctx context.Context, entry *CacheEntry) error {
	return c.store(ctx, entry)
}
