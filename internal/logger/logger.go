// Package logger provides structured JSON logging using slog.
package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	// RequestIDKey is the context key for request ID.
	RequestIDKey contextKey = "request_id"
)

// RequestLog contains structured fields for request logging.
type RequestLog struct {
	RequestID       string  `json:"request_id"`
	Status          string  `json:"status"` // cache_hit, cache_miss, error
	TotalLatencyMs  float64 `json:"total_latency_ms"`
	EmbedLatencyMs  float64 `json:"embed_latency_ms,omitempty"`
	SearchLatencyMs float64 `json:"search_latency_ms,omitempty"`
	SimilarityScore float64 `json:"similarity_score,omitempty"`
	Error           string  `json:"error,omitempty"`
}

// Logger wraps slog.Logger with additional functionality.
type Logger struct {
	*slog.Logger
}

// New creates a new Logger with JSON output to stdout.
func New() *Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return &Logger{
		Logger: slog.New(handler),
	}
}

// NewWithLevel creates a new Logger with the specified log level.
func NewWithLevel(level slog.Level) *Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	return &Logger{
		Logger: slog.New(handler),
	}
}

// With returns a new Logger with the given attributes.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		Logger: l.Logger.With(args...),
	}
}

// WithRequestID returns a new Logger with the request ID attached.
func (l *Logger) WithRequestID(requestID string) *Logger {
	return l.With("request_id", requestID)
}

// GenerateRequestID creates a new unique request ID.
func GenerateRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a simple counter-based ID if random fails
		return "req-fallback"
	}
	return "req-" + hex.EncodeToString(bytes)
}

// ContextWithRequestID adds a request ID to the context.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// RequestIDFromContext extracts the request ID from context.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// LogRequest logs a complete request with all fields from RequestLog.
func (l *Logger) LogRequest(log RequestLog) {
	attrs := []any{
		"request_id", log.RequestID,
		"status", log.Status,
		"total_latency_ms", log.TotalLatencyMs,
	}

	if log.EmbedLatencyMs > 0 {
		attrs = append(attrs, "embed_latency_ms", log.EmbedLatencyMs)
	}
	if log.SearchLatencyMs > 0 {
		attrs = append(attrs, "search_latency_ms", log.SearchLatencyMs)
	}
	if log.SimilarityScore > 0 {
		attrs = append(attrs, "similarity_score", log.SimilarityScore)
	}
	if log.Error != "" {
		attrs = append(attrs, "error", log.Error)
	}

	l.Info("request completed", attrs...)
}

// LogEmbeddingLatency logs embedding generation latency.
func (l *Logger) LogEmbeddingLatency(requestID string, latencyMs float64) {
	l.Info("embedding generated",
		"request_id", requestID,
		"embed_latency_ms", latencyMs,
	)
}

// LogSearchLatency logs vector search latency and similarity score.
func (l *Logger) LogSearchLatency(requestID string, latencyMs float64, similarityScore float64) {
	l.Info("vector search completed",
		"request_id", requestID,
		"search_latency_ms", latencyMs,
		"similarity_score", similarityScore,
	)
}

// LogCacheHit logs a cache hit event.
func (l *Logger) LogCacheHit(requestID string, latencyMs float64, similarityScore float64) {
	l.Info("cache hit",
		"request_id", requestID,
		"status", "cache_hit",
		"total_latency_ms", latencyMs,
		"similarity_score", similarityScore,
	)
}

// LogCacheMiss logs a cache miss event.
func (l *Logger) LogCacheMiss(requestID string, latencyMs float64) {
	l.Info("cache miss",
		"request_id", requestID,
		"status", "cache_miss",
		"total_latency_ms", latencyMs,
	)
}

// LogError logs an error with context.
func (l *Logger) LogError(requestID string, err error, msg string) {
	l.Error(msg,
		"request_id", requestID,
		"error", err.Error(),
	)
}
