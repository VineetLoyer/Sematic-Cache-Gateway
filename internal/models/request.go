// Package models contains data structures for request/response handling.
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Message represents a single message in a chat completion request.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest represents an OpenAI-compatible chat completion request.
type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

// ExtractQueryText concatenates all user messages from the request.
// Returns a single string containing all user message content in order,
// separated by spaces.
func ExtractQueryText(req *ChatCompletionRequest) string {
	if req == nil {
		return ""
	}

	var parts []string
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			parts = append(parts, msg.Content)
		}
	}
	return strings.Join(parts, " ")
}

// ComputeQueryHash computes a SHA-256 hash of the query text.
// Returns the hash as a hex-encoded string with "sha256:" prefix.
func ComputeQueryHash(queryText string) string {
	hash := sha256.Sum256([]byte(queryText))
	return "sha256:" + hex.EncodeToString(hash[:])
}
