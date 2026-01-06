package middleware

import "context"

type contextKey string

const bufferedBodyKey contextKey = "bufferedBody"

// SetBufferedBody stores the request body bytes in the context.
func SetBufferedBody(ctx context.Context, body []byte) context.Context {
	return context.WithValue(ctx, bufferedBodyKey, body)
}

// GetBufferedBody retrieves the buffered body bytes from the context.
func GetBufferedBody(ctx context.Context) []byte {
	body, ok := ctx.Value(bufferedBodyKey).([]byte)
	if !ok {
		return nil
	}
	return body
}
