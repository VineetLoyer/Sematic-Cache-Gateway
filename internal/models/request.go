package models

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

// ExtractQueryText concatenates all user messages from the request into a single string.
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

// ComputeQueryHash returns a SHA-256 hash of the query text with "sha256:" prefix.
func ComputeQueryHash(queryText string) string {
	hash := sha256.Sum256([]byte(queryText))
	return "sha256:" + hex.EncodeToString(hash[:])
}
