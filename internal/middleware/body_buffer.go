package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// BufferedRequest wraps http.Request with reusable body
type BufferedRequest struct {
	*http.Request
	BodyBytes []byte
}

// ErrorResponse represents an error response in OpenAI-compatible format
type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}

// BodyBufferMiddleware reads the request body into a buffer and restores it
// for downstream handlers. This solves the read-once problem with http.Request.Body.
func BodyBufferMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only buffer POST requests with a body
		if r.Method != http.MethodPost || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Read the entire body into a buffer
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			writeErrorResponse(w, http.StatusBadRequest, "Failed to read request body", "invalid_request_error")
			return
		}
		r.Body.Close()

		// If body is not empty, validate it's valid JSON
		if len(bodyBytes) > 0 {
			if !json.Valid(bodyBytes) {
				writeErrorResponse(w, http.StatusBadRequest, "Request body is not valid JSON", "invalid_request_error")
				return
			}
		}

		// Restore the body as an io.NopCloser so downstream handlers can read it
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// Store the body bytes in the request context for later use
		bufferedReq := &BufferedRequest{
			Request:   r,
			BodyBytes: bodyBytes,
		}

		// Create a new request with the buffered body stored in context
		ctx := SetBufferedBody(r.Context(), bufferedReq.BodyBytes)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// writeErrorResponse writes an OpenAI-compatible error response
func writeErrorResponse(w http.ResponseWriter, statusCode int, message, errType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errResp := ErrorResponse{}
	errResp.Error.Message = message
	errResp.Error.Type = errType

	json.NewEncoder(w).Encode(errResp)
}

// GetBodyBytes retrieves the buffered body bytes from the request.
// Returns nil if the body was not buffered.
func GetBodyBytes(r *http.Request) []byte {
	return GetBufferedBody(r.Context())
}

// RestoreBody restores the request body from the buffered bytes.
// This allows the body to be read again after it has been consumed.
func RestoreBody(r *http.Request) {
	bodyBytes := GetBodyBytes(r)
	if bodyBytes != nil {
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
}
