// Package main is the entry point for the Semantic Cache Gateway.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"semantic-cache-gateway/internal/cache"
	"semantic-cache-gateway/internal/config"
	"semantic-cache-gateway/internal/embedding"
	"semantic-cache-gateway/internal/handler"
	"semantic-cache-gateway/internal/logger"
	"semantic-cache-gateway/internal/middleware"
	"semantic-cache-gateway/internal/proxy"
)

func main() {
	// Initialize logger
	log := logger.New()
	log.Info("starting semantic cache gateway")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Error("failed to load configuration", "error", err.Error())
		os.Exit(1)
	}

	log.Info("configuration loaded",
		"port", cfg.Port,
		"upstream_url", cfg.UpstreamURL,
		"similarity_threshold", cfg.SimilarityThreshold,
	)

	// Initialize Redis client
	redisConfig := cache.DefaultRedisConfig(cfg.RedisURL)
	redisClient, err := cache.NewRedisClient(redisConfig, log)
	if err != nil {
		log.Error("failed to create redis client", "error", err.Error())
		os.Exit(1)
	}
	defer redisClient.Close()

	// Check Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := redisClient.Ping(ctx); err != nil {
		log.Error("failed to connect to redis", "error", err.Error())
		cancel()
		os.Exit(1)
	}
	cancel()
	log.Info("connected to redis", "url", cfg.RedisURL)


	// Initialize cache service
	cacheService, err := cache.NewCacheService(redisClient, log, nil)
	if err != nil {
		log.Error("failed to create cache service", "error", err.Error())
		os.Exit(1)
	}
	defer cacheService.Close()
	log.Info("cache service initialized")

	// Initialize embedding service
	embeddingConfig := embedding.DefaultConfig(cfg.EmbeddingAPIKey)
	embeddingService := embedding.NewService(embeddingConfig)
	log.Info("embedding service initialized", "model", embeddingConfig.ModelName)

	// Initialize upstream proxy
	proxyConfig := proxy.ProxyConfig{
		UpstreamURL: cfg.UpstreamURL,
		Timeout:     60 * time.Second,
	}
	upstreamProxy, err := proxy.New(proxyConfig)
	if err != nil {
		log.Error("failed to create upstream proxy", "error", err.Error())
		os.Exit(1)
	}
	log.Info("upstream proxy initialized", "upstream_url", cfg.UpstreamURL)

	// Initialize cache handler
	handlerConfig := &handler.Config{
		SimilarityThreshold: cfg.SimilarityThreshold,
	}
	cacheHandler := handler.New(cacheService, embeddingService, upstreamProxy, log, handlerConfig)

	// Set up HTTP router
	mux := http.NewServeMux()

	// Apply middleware chain to cache handler
	chatHandler := middleware.BodyBufferMiddleware(cacheHandler)
	mux.Handle("/chat/completions", chatHandler)
	mux.Handle("/v1/chat/completions", chatHandler)

	// Health check endpoint
	mux.HandleFunc("/health", handler.HealthHandler(redisClient))

	// Stats endpoints
	mux.HandleFunc("/stats", handler.StatsDashboard)
	mux.HandleFunc("/stats/json", handler.StatsJSON)

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Info("server listening", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err.Error())
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("server forced to shutdown", "error", err.Error())
	}

	log.Info("server stopped")
}
