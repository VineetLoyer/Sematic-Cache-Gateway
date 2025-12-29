package middleware

import "context"

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// bufferedBodyKey is the context key for storing buffered request body
	bufferedBodyKey contextKey = "bufferedBody"
)

// SetBufferedBody stores the buffered body bytes in the context
func SetBufferedBody(ctx context.Context, body []byte) context.Context {
	return context.WithValue(ctx, bufferedBodyKey, body)
}

// GetBufferedBody retrieves the buffered body bytes from the context
func GetBufferedBody(ctx context.Context) []byte {
	body, ok := ctx.Value(bufferedBodyKey).([]byte)
	if !ok {
		return nil
	}
	return body
}
