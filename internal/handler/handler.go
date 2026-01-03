// Package handler provides the main cache handler that orchestrates all components.
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"semantic-cache-gateway/internal/cache"
	"semantic-cache-gateway/internal/embedding"
	"semantic-cache-gateway/internal/logger"
	"semantic-cache-gateway/internal/middleware"
	"semantic-cache-gateway/internal/models"
	"semantic-cache-gateway/internal/proxy"
)

// CacheHandler orchestrates the caching pipeline for LLM requests.
type CacheHandler struct {
	cache       cache.CacheService
	embedding   embedding.EmbeddingService
	proxy       proxy.UpstreamProxy
	logger      *logger.Logger
	threshold   float64
}

// Config holds configuration for the cache handler.
type Config struct {
	SimilarityThreshold float64
}

// New creates a new CacheHandler with the given dependencies.
func New(
	cacheService cache.CacheService,
	embeddingService embedding.EmbeddingService,
	upstreamProxy proxy.UpstreamProxy,
	log *logger.Logger,
	cfg *Config,
) *CacheHandler {
	threshold := 0.95
	if cfg != nil && cfg.SimilarityThreshold > 0 {
		threshold = cfg.SimilarityThreshold
	}

	return &CacheHandler{
		cache:     cacheService,
		embedding: embeddingService,
		proxy:     upstreamProxy,
		logger:    log,
		threshold: threshold,
	}
}


// ServeHTTP handles incoming chat completion requests through the caching pipeline.
// Flow: body buffer → hash check → embedding → vector search → upstream
func (h *CacheHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	requestID := logger.GenerateRequestID()
	ctx := logger.ContextWithRequestID(r.Context(), requestID)
	r = r.WithContext(ctx)

	log := h.logger.WithRequestID(requestID)
	log.Info("processing request", "path", r.URL.Path, "method", r.Method)

	// Get buffered body from context (set by middleware)
	bodyBytes := middleware.GetBufferedBody(r.Context())
	if bodyBytes == nil {
		h.writeError(w, http.StatusBadRequest, "Request body not available", "invalid_request_error")
		h.logError(log, requestID, startTime, "request body not available")
		return
	}

	// Parse the request to extract query text
	var chatReq models.ChatCompletionRequest
	if err := json.Unmarshal(bodyBytes, &chatReq); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid request format", "invalid_request_error")
		h.logError(log, requestID, startTime, "failed to parse request: "+err.Error())
		return
	}

	// Extract query text from user messages
	queryText := models.ExtractQueryText(&chatReq)
	if queryText == "" {
		h.writeError(w, http.StatusBadRequest, "No user messages found in request", "invalid_request_error")
		h.logError(log, requestID, startTime, "no user messages in request")
		return
	}

	// Compute SHA-256 hash for exact match lookup
	queryHash := models.ComputeQueryHash(queryText)
	log.Info("query extracted", "query_hash", queryHash, "query_length", len(queryText))

	// Step 1: Check for exact hash match
	exactMatch, err := h.cache.CheckExactMatch(ctx, queryHash)
	if err != nil {
		log.Error("exact match check failed", "error", err.Error())
		// Continue to embedding on cache error (graceful degradation)
	} else if exactMatch != nil {
		// Cache hit on exact match
		h.serveCachedResponse(w, exactMatch, log, requestID, startTime, 1.0)
		return
	}

	log.Info("no exact match, generating embedding")

	// Step 2: Generate embedding for vector search
	embedStart := time.Now()
	embeddingVec, err := h.embedding.Generate(ctx, queryText)
	embedLatency := time.Since(embedStart).Seconds() * 1000

	if err != nil {
		log.Error("embedding generation failed", "error", err.Error(), "embed_latency_ms", embedLatency)
		// Forward to upstream on embedding failure (graceful degradation)
		h.forwardToUpstream(w, r, bodyBytes, log, requestID, startTime, queryHash, queryText, nil)
		return
	}

	log.Info("embedding generated", "embed_latency_ms", embedLatency, "dimensions", len(embeddingVec))

	// Step 3: Perform vector similarity search
	searchStart := time.Now()
	similarEntry, similarity, err := h.cache.SearchSimilar(ctx, embeddingVec, h.threshold)
	searchLatency := time.Since(searchStart).Seconds() * 1000

	if err != nil {
		log.Error("vector search failed", "error", err.Error(), "search_latency_ms", searchLatency)
		// Forward to upstream on search failure (graceful degradation)
		h.forwardToUpstream(w, r, bodyBytes, log, requestID, startTime, queryHash, queryText, embeddingVec)
		return
	}

	log.Info("vector search completed", "search_latency_ms", searchLatency, "similarity", similarity)

	if similarEntry != nil {
		// Cache hit on semantic match
		h.serveCachedResponse(w, similarEntry, log, requestID, startTime, similarity)
		return
	}

	// Step 4: Cache miss - forward to upstream
	log.Info("cache miss, forwarding to upstream")
	h.forwardToUpstream(w, r, bodyBytes, log, requestID, startTime, queryHash, queryText, embeddingVec)
}


// serveCachedResponse writes a cached response to the client.
func (h *CacheHandler) serveCachedResponse(
	w http.ResponseWriter,
	entry *cache.CacheEntry,
	log *logger.Logger,
	requestID string,
	startTime time.Time,
	similarity float64,
) {
	totalLatency := time.Since(startTime).Seconds() * 1000

	// Record stats
	RecordHit(int64(totalLatency))

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache-Status", "HIT")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(entry.LLMResponse)) // Convert string back to bytes

	log.LogRequest(logger.RequestLog{
		RequestID:       requestID,
		Status:          "cache_hit",
		TotalLatencyMs:  totalLatency,
		SimilarityScore: similarity,
	})
}

// forwardToUpstream forwards the request to the upstream LLM and caches the response.
func (h *CacheHandler) forwardToUpstream(
	w http.ResponseWriter,
	r *http.Request,
	bodyBytes []byte,
	log *logger.Logger,
	requestID string,
	startTime time.Time,
	queryHash string,
	queryText string,
	embeddingVec []float32,
) {
	// Restore the request body for forwarding
	middleware.RestoreBody(r)

	// Forward to upstream
	resp, err := h.proxy.Forward(r.Context(), r)
	if err != nil {
		totalLatency := time.Since(startTime).Seconds() * 1000
		log.Error("upstream request failed", "error", err.Error())
		h.writeError(w, http.StatusBadGateway, "Upstream request failed", "upstream_error")
		log.LogRequest(logger.RequestLog{
			RequestID:      requestID,
			Status:         "error",
			TotalLatencyMs: totalLatency,
			Error:          err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Read upstream response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		totalLatency := time.Since(startTime).Seconds() * 1000
		log.Error("failed to read upstream response", "error", err.Error())
		h.writeError(w, http.StatusBadGateway, "Failed to read upstream response", "upstream_error")
		log.LogRequest(logger.RequestLog{
			RequestID:      requestID,
			Status:         "error",
			TotalLatencyMs: totalLatency,
			Error:          err.Error(),
		})
		return
	}

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("X-Cache-Status", "MISS")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)

	totalLatency := time.Since(startTime).Seconds() * 1000

	// Store in cache asynchronously (only if we have embedding and response is successful)
	if embeddingVec != nil && resp.StatusCode == http.StatusOK {
		entry := &cache.CacheEntry{
			QueryHash:   queryHash,
			QueryText:   queryText,
			Embedding:   embeddingVec,
			LLMResponse: string(respBody), // Store as string
			CreatedAt:   time.Now().Unix(),
		}
		h.cache.StoreAsync(entry)
		log.Info("cache entry queued for storage", "query_hash", queryHash)
	}

	log.LogRequest(logger.RequestLog{
		RequestID:      requestID,
		Status:         "cache_miss",
		TotalLatencyMs: totalLatency,
	})

	// Record stats
	RecordMiss(int64(totalLatency))
}


// writeError writes an OpenAI-compatible error response.
func (h *CacheHandler) writeError(w http.ResponseWriter, statusCode int, message, errType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errResp := struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}{}
	errResp.Error.Message = message
	errResp.Error.Type = errType

	json.NewEncoder(w).Encode(errResp)
}

// logError logs an error with request context.
func (h *CacheHandler) logError(log *logger.Logger, requestID string, startTime time.Time, errMsg string) {
	totalLatency := time.Since(startTime).Seconds() * 1000

	// Record stats
	RecordError()

	log.LogRequest(logger.RequestLog{
		RequestID:      requestID,
		Status:         "error",
		TotalLatencyMs: totalLatency,
		Error:          errMsg,
	})
}

// HealthHandler returns a simple health check handler.
func HealthHandler(redisClient interface{ IsHealthy(context.Context) bool }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		status := struct {
			Status string `json:"status"`
			Redis  string `json:"redis"`
		}{
			Status: "healthy",
			Redis:  "connected",
		}

		if redisClient != nil && !redisClient.IsHealthy(ctx) {
			status.Status = "degraded"
			status.Redis = "disconnected"
		}

		w.Header().Set("Content-Type", "application/json")
		if status.Status == "healthy" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(status)
	}
}
