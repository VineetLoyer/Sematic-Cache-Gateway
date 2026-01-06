package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

type RequestLog struct {
	RequestID       string  `json:"request_id"`
	Status          string  `json:"status"`
	TotalLatencyMs  float64 `json:"total_latency_ms"`
	EmbedLatencyMs  float64 `json:"embed_latency_ms,omitempty"`
	SearchLatencyMs float64 `json:"search_latency_ms,omitempty"`
	SimilarityScore float64 `json:"similarity_score,omitempty"`
	Error           string  `json:"error,omitempty"`
}

type Logger struct {
	*slog.Logger
}

// New creates a JSON logger writing to stdout.
func New() *Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &Logger{Logger: slog.New(handler)}
}

// NewWithLevel creates a logger with the specified log level.
func NewWithLevel(level slog.Level) *Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return &Logger{Logger: slog.New(handler)}
}

// With returns a new Logger with additional attributes.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{Logger: l.Logger.With(args...)}
}

// WithRequestID returns a logger with the request ID attached.
func (l *Logger) WithRequestID(requestID string) *Logger {
	return l.With("request_id", requestID)
}

// GenerateRequestID creates a unique request ID.
func GenerateRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
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

// LogRequest logs a completed request with all relevant fields.
func (l *Logger) LogRequest(log RequestLog) {
	attrs := []any{"request_id", log.RequestID, "status", log.Status, "total_latency_ms", log.TotalLatencyMs}
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

// LogCacheHit logs a cache hit event.
func (l *Logger) LogCacheHit(requestID string, latencyMs float64, similarityScore float64) {
	l.Info("cache hit", "request_id", requestID, "status", "cache_hit", "total_latency_ms", latencyMs, "similarity_score", similarityScore)
}

// LogCacheMiss logs a cache miss event.
func (l *Logger) LogCacheMiss(requestID string, latencyMs float64) {
	l.Info("cache miss", "request_id", requestID, "status", "cache_miss", "total_latency_ms", latencyMs)
}

// LogError logs an error with request context.
func (l *Logger) LogError(requestID string, err error, msg string) {
	l.Error(msg, "request_id", requestID, "error", err.Error())
}
