package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBodyBufferMiddleware_ValidJSON(t *testing.T) {
	// Create a handler that reads the body and verifies it can be read
	var capturedBody []byte
	var capturedContextBody []byte
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read body from request
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read body: %v", err)
			return
		}
		capturedBody = body

		// Get body from context
		capturedContextBody = GetBodyBytes(r)

		w.WriteHeader(http.StatusOK)
	})

	// Wrap with middleware
	wrapped := BodyBufferMiddleware(handler)

	// Create request with valid JSON body
	originalBody := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", bytes.NewBufferString(originalBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	// Verify status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Verify body was passed through correctly
	if string(capturedBody) != originalBody {
		t.Errorf("Body mismatch.\nExpected: %s\nGot: %s", originalBody, string(capturedBody))
	}

	// Verify context body matches
	if string(capturedContextBody) != originalBody {
		t.Errorf("Context body mismatch.\nExpected: %s\nGot: %s", originalBody, string(capturedContextBody))
	}
}

func TestBodyBufferMiddleware_MalformedJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for malformed JSON")
	})

	wrapped := BodyBufferMiddleware(handler)

	// Create request with malformed JSON
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", bytes.NewBufferString(`{"invalid json`))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	// Verify 400 status code
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rr.Code)
	}

	// Verify error response format
	var errResp ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
		t.Errorf("Failed to decode error response: %v", err)
	}

	if errResp.Error.Type != "invalid_request_error" {
		t.Errorf("Expected error type 'invalid_request_error', got '%s'", errResp.Error.Type)
	}
}

func TestBodyBufferMiddleware_EmptyBody(t *testing.T) {
	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	wrapped := BodyBufferMiddleware(handler)

	// Create request with empty body
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", bytes.NewBufferString(""))
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	// Empty body should pass through
	if !handlerCalled {
		t.Error("Handler should be called for empty body")
	}

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}

func TestBodyBufferMiddleware_GETRequest(t *testing.T) {
	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	wrapped := BodyBufferMiddleware(handler)

	// GET requests should pass through without body processing
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("Handler should be called for GET request")
	}

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}

func TestRestoreBody(t *testing.T) {
	originalBody := `{"test":"data"}`
	var firstRead, secondRead []byte

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First read
		body, _ := io.ReadAll(r.Body)
		firstRead = body

		// Restore and read again
		RestoreBody(r)
		body, _ = io.ReadAll(r.Body)
		secondRead = body

		w.WriteHeader(http.StatusOK)
	})

	wrapped := BodyBufferMiddleware(handler)

	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(originalBody))
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if string(firstRead) != originalBody {
		t.Errorf("First read mismatch.\nExpected: %s\nGot: %s", originalBody, string(firstRead))
	}

	if string(secondRead) != originalBody {
		t.Errorf("Second read mismatch.\nExpected: %s\nGot: %s", originalBody, string(secondRead))
	}
}

func TestGetBodyBytes_NoBuffer(t *testing.T) {
	// Create a request without going through middleware
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString("test"))

	body := GetBodyBytes(req)
	if body != nil {
		t.Errorf("Expected nil body for unbuffered request, got %v", body)
	}
}
