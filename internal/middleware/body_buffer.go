package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

type BufferedRequest struct {
	*http.Request
	BodyBytes []byte
}

type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}

// BodyBufferMiddleware reads and buffers the request body for reuse by downstream handlers.
func BodyBufferMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			writeErrorResponse(w, http.StatusBadRequest, "Failed to read request body", "invalid_request_error")
			return
		}
		r.Body.Close()

		if len(bodyBytes) > 0 && !json.Valid(bodyBytes) {
			writeErrorResponse(w, http.StatusBadRequest, "Request body is not valid JSON", "invalid_request_error")
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		ctx := SetBufferedBody(r.Context(), bodyBytes)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

func writeErrorResponse(w http.ResponseWriter, statusCode int, message, errType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	errResp := ErrorResponse{}
	errResp.Error.Message = message
	errResp.Error.Type = errType
	json.NewEncoder(w).Encode(errResp)
}

// GetBodyBytes retrieves the buffered body bytes from the request context.
func GetBodyBytes(r *http.Request) []byte {
	return GetBufferedBody(r.Context())
}

// RestoreBody resets the request body so it can be read again.
func RestoreBody(r *http.Request) {
	bodyBytes := GetBodyBytes(r)
	if bodyBytes != nil {
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
}
