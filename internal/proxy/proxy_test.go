// Package proxy provides upstream LLM forwarding functionality.
package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestProxy_Forward_TimeoutHandling tests that the proxy correctly handles
// timeout scenarios when the upstream server takes too long to respond.
// Requirements: 1.4
func TestProxy_Forward_TimeoutHandling(t *testing.T) {
	// Create a slow upstream server that delays longer than the timeout
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // Delay longer than timeout
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"response": "delayed"}`))
	}))
	defer slowServer.Close()

	// Create proxy with a short timeout
	proxy, err := New(ProxyConfig{
		UpstreamURL: slowServer.URL,
		Timeout:     50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// Create a test request
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"test": "data"}`))
	req.Header.Set("Content-Type", "application/json")

	// Forward the request - should timeout
	_, err = proxy.Forward(context.Background(), req)

	// Verify timeout error occurred
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Check that the error message indicates upstream failure
	if !strings.Contains(err.Error(), "upstream request failed") {
		t.Errorf("expected error to contain 'upstream request failed', got: %v", err)
	}
}

// TestProxy_Forward_UpstreamErrorPropagation tests that upstream HTTP errors
// are correctly propagated back to the caller.
// Requirements: 1.4
func TestProxy_Forward_UpstreamErrorPropagation(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedStatus int
	}{
		{
			name:           "500 Internal Server Error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   `{"error": {"message": "Internal server error", "type": "server_error"}}`,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "503 Service Unavailable",
			statusCode:     http.StatusServiceUnavailable,
			responseBody:   `{"error": {"message": "Service unavailable", "type": "server_error"}}`,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:           "429 Rate Limited",
			statusCode:     http.StatusTooManyRequests,
			responseBody:   `{"error": {"message": "Rate limit exceeded", "type": "rate_limit_error"}}`,
			expectedStatus: http.StatusTooManyRequests,
		},
		{
			name:           "400 Bad Request",
			statusCode:     http.StatusBadRequest,
			responseBody:   `{"error": {"message": "Invalid request", "type": "invalid_request_error"}}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create upstream server that returns the error
			errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer errorServer.Close()

			// Create proxy
			proxy, err := New(ProxyConfig{
				UpstreamURL: errorServer.URL,
				Timeout:     5 * time.Second,
			})
			if err != nil {
				t.Fatalf("failed to create proxy: %v", err)
			}

			// Create test request
			req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"model": "gpt-4"}`))
			req.Header.Set("Content-Type", "application/json")

			// Forward the request
			resp, err := proxy.Forward(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer resp.Body.Close()

			// Verify status code is propagated
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			// Verify response body is propagated
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read response body: %v", err)
			}

			if string(body) != tt.responseBody {
				t.Errorf("expected body %q, got %q", tt.responseBody, string(body))
			}
		})
	}
}

// TestProxy_Forward_ConnectionRefused tests handling when upstream is unreachable.
// Requirements: 1.4
func TestProxy_Forward_ConnectionRefused(t *testing.T) {
	// Create proxy pointing to a non-existent server
	proxy, err := New(ProxyConfig{
		UpstreamURL: "http://localhost:59999", // Unlikely to be in use
		Timeout:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// Create test request
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"test": "data"}`))
	req.Header.Set("Content-Type", "application/json")

	// Forward the request - should fail with connection error
	_, err = proxy.Forward(context.Background(), req)

	// Verify error occurred
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}

	// Check that the error indicates upstream failure
	if !strings.Contains(err.Error(), "upstream request failed") {
		t.Errorf("expected error to contain 'upstream request failed', got: %v", err)
	}
}

// TestProxy_Forward_ContextCancellation tests that context cancellation is respected.
// Requirements: 1.4
func TestProxy_Forward_ContextCancellation(t *testing.T) {
	// Create a slow upstream server
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	// Create proxy with long timeout
	proxy, err := New(ProxyConfig{
		UpstreamURL: slowServer.URL,
		Timeout:     10 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// Create a context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())

	// Create test request
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"test": "data"}`))
	req.Header.Set("Content-Type", "application/json")

	// Cancel context after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Forward the request - should be cancelled
	_, err = proxy.Forward(ctx, req)

	// Verify error occurred due to cancellation
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}

	if !strings.Contains(err.Error(), "upstream request failed") {
		t.Errorf("expected error to contain 'upstream request failed', got: %v", err)
	}
}
