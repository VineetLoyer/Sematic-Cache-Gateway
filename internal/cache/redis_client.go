// Package cache provides caching functionality using Redis Stack.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"semantic-cache-gateway/internal/logger"
)

// RedisClient wraps the go-redis client with additional functionality
// for JSON operations and vector search queries.
type RedisClient struct {
	client *redis.Client
	logger *logger.Logger
}

// RedisConfig holds configuration for the Redis connection.
type RedisConfig struct {
	URL            string
	MaxRetries     int
	DialTimeout    time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	PoolSize       int
	MinIdleConns   int
}

// DefaultRedisConfig returns a RedisConfig with sensible defaults.
func DefaultRedisConfig(url string) *RedisConfig {
	return &RedisConfig{
		URL:          url,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	}
}

// NewRedisClient creates a new Redis client with the given configuration.
func NewRedisClient(cfg *RedisConfig, log *logger.Logger) (*RedisClient, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	opts.MaxRetries = cfg.MaxRetries
	opts.DialTimeout = cfg.DialTimeout
	opts.ReadTimeout = cfg.ReadTimeout
	opts.WriteTimeout = cfg.WriteTimeout
	opts.PoolSize = cfg.PoolSize
	opts.MinIdleConns = cfg.MinIdleConns

	client := redis.NewClient(opts)

	return &RedisClient{
		client: client,
		logger: log,
	}, nil
}


// Ping checks the Redis connection health.
func (r *RedisClient) Ping(ctx context.Context) error {
	_, err := r.client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	return nil
}

// Close closes the Redis connection.
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// IsHealthy performs a health check on the Redis connection.
func (r *RedisClient) IsHealthy(ctx context.Context) bool {
	err := r.Ping(ctx)
	return err == nil
}

// JSONSet stores a JSON value at the specified key and path.
func (r *RedisClient) JSONSet(ctx context.Context, key string, path string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Use JSON.SET command from RedisJSON module
	cmd := r.client.Do(ctx, "JSON.SET", key, path, string(data))
	if cmd.Err() != nil {
		return fmt.Errorf("JSON.SET failed: %w", cmd.Err())
	}
	return nil
}

// JSONSetRaw stores a raw JSON string at the specified key and path.
func (r *RedisClient) JSONSetRaw(ctx context.Context, key string, path string, jsonStr string) error {
	cmd := r.client.Do(ctx, "JSON.SET", key, path, jsonStr)
	if cmd.Err() != nil {
		return fmt.Errorf("JSON.SET failed: %w", cmd.Err())
	}
	return nil
}

// JSONGet retrieves a JSON value from the specified key and path.
func (r *RedisClient) JSONGet(ctx context.Context, key string, path string) ([]byte, error) {
	cmd := r.client.Do(ctx, "JSON.GET", key, path)
	if cmd.Err() != nil {
		if cmd.Err() == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("JSON.GET failed: %w", cmd.Err())
	}

	result, err := cmd.Text()
	if err != nil {
		return nil, fmt.Errorf("failed to get JSON result: %w", err)
	}

	return []byte(result), nil
}

// Exists checks if a key exists in Redis.
func (r *RedisClient) Exists(ctx context.Context, key string) (bool, error) {
	result, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("EXISTS failed: %w", err)
	}
	return result > 0, nil
}


// SearchResult represents a single result from a vector search.
type SearchResult struct {
	Key        string
	Score      float64
	Document   []byte
}

// FTSearch performs a vector similarity search using RediSearch.
// The query should be a properly formatted RediSearch query string.
func (r *RedisClient) FTSearch(ctx context.Context, index string, query string, args ...interface{}) ([]SearchResult, error) {
	// Build the FT.SEARCH command arguments
	cmdArgs := []interface{}{"FT.SEARCH", index, query}
	cmdArgs = append(cmdArgs, args...)

	cmd := r.client.Do(ctx, cmdArgs...)
	if cmd.Err() != nil {
		return nil, fmt.Errorf("FT.SEARCH failed: %w", cmd.Err())
	}

	// Parse the search results
	return r.parseSearchResults(cmd)
}

// parseSearchResults parses the raw FT.SEARCH response into SearchResult structs.
func (r *RedisClient) parseSearchResults(cmd *redis.Cmd) ([]SearchResult, error) {
	raw, err := cmd.Slice()
	if err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	if len(raw) == 0 {
		return nil, nil
	}

	// First element is the total count
	totalCount, ok := raw[0].(int64)
	if !ok {
		return nil, fmt.Errorf("unexpected result format: first element is not count")
	}

	if totalCount == 0 {
		return nil, nil
	}

	var results []SearchResult

	// Results come in pairs: key, [field, value, field, value, ...]
	i := 1
	for i < len(raw) {
		if i >= len(raw) {
			break
		}

		// Get the key
		key, ok := raw[i].(string)
		if !ok {
			i++
			continue
		}
		i++

		result := SearchResult{Key: key}

		// Get the fields array
		if i < len(raw) {
			if fields, ok := raw[i].([]interface{}); ok {
				result.Score, result.Document = r.parseSearchFields(fields)
			}
			i++
		}

		results = append(results, result)
	}

	return results, nil
}


// parseSearchFields extracts score and document from search result fields.
func (r *RedisClient) parseSearchFields(fields []interface{}) (float64, []byte) {
	var score float64
	var document []byte

	for i := 0; i < len(fields)-1; i += 2 {
		fieldName, ok := fields[i].(string)
		if !ok {
			continue
		}

		switch fieldName {
		case "__vector_score":
			if scoreStr, ok := fields[i+1].(string); ok {
				fmt.Sscanf(scoreStr, "%f", &score)
				// Convert distance to similarity (1 - distance for cosine)
				score = 1 - score
			}
		case "$":
			if docStr, ok := fields[i+1].(string); ok {
				document = []byte(docStr)
			}
		}
	}

	return score, document
}

// CreateVectorIndex creates an HNSW vector index for cache entries.
func (r *RedisClient) CreateVectorIndex(ctx context.Context, indexName string, dimensions int) error {
	// Check if index already exists
	cmd := r.client.Do(ctx, "FT.INFO", indexName)
	if cmd.Err() == nil {
		// Index already exists
		r.logger.Info("vector index already exists", "index", indexName)
		return nil
	}

	// Create the index with HNSW algorithm
	createCmd := r.client.Do(ctx,
		"FT.CREATE", indexName,
		"ON", "JSON",
		"PREFIX", "1", "cache:",
		"SCHEMA",
		"$.query_hash", "AS", "query_hash", "TAG",
		"$.embedding", "AS", "embedding", "VECTOR", "HNSW", "6",
		"TYPE", "FLOAT32",
		"DIM", dimensions,
		"DISTANCE_METRIC", "COSINE",
	)

	if createCmd.Err() != nil {
		// Check if error is because index already exists
		if strings.Contains(createCmd.Err().Error(), "Index already exists") {
			r.logger.Info("vector index already exists", "index", indexName)
			return nil
		}
		return fmt.Errorf("FT.CREATE failed: %w", createCmd.Err())
	}

	r.logger.Info("created vector index", "index", indexName, "dimensions", dimensions)
	return nil
}

// Client returns the underlying redis.Client for advanced operations.
func (r *RedisClient) Client() *redis.Client {
	return r.client
}
