// Package models contains property-based tests for request handling.
package models

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// **Feature: semantic-cache-gateway, Property 1: Query Text Extraction Preserves User Messages**
// **Validates: Requirements 1.2**
//
// For any valid ChatCompletionRequest with one or more user messages,
// extracting the query text SHALL produce a string containing all user
// message content in order.
func TestExtractQueryText_PreservesUserMessages(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random user messages (at least 1)
		numUserMessages := rapid.IntRange(1, 5).Draw(t, "numUserMessages")
		userContents := make([]string, numUserMessages)
		for i := 0; i < numUserMessages; i++ {
			userContents[i] = rapid.StringMatching(`[a-zA-Z0-9 ]{1,50}`).Draw(t, "userContent")
		}

		// Generate random non-user messages to intersperse
		numOtherMessages := rapid.IntRange(0, 3).Draw(t, "numOtherMessages")
		otherRoles := []string{"system", "assistant"}

		// Build the messages slice with user and other messages
		var messages []Message
		userIdx := 0

		// Add some non-user messages at the start
		for i := 0; i < numOtherMessages/2; i++ {
			role := otherRoles[rapid.IntRange(0, len(otherRoles)-1).Draw(t, "roleIdx")]
			content := rapid.StringMatching(`[a-zA-Z0-9 ]{1,30}`).Draw(t, "otherContent")
			messages = append(messages, Message{Role: role, Content: content})
		}

		// Add user messages interspersed with other messages
		for userIdx < numUserMessages {
			messages = append(messages, Message{Role: "user", Content: userContents[userIdx]})
			userIdx++

			// Maybe add a non-user message after
			if rapid.Bool().Draw(t, "addOther") && numOtherMessages > 0 {
				role := otherRoles[rapid.IntRange(0, len(otherRoles)-1).Draw(t, "roleIdx2")]
				content := rapid.StringMatching(`[a-zA-Z0-9 ]{1,30}`).Draw(t, "otherContent2")
				messages = append(messages, Message{Role: role, Content: content})
			}
		}

		req := &ChatCompletionRequest{
			Model:    "gpt-4",
			Messages: messages,
		}

		// Extract query text
		result := ExtractQueryText(req)

		// Property 1: All user message contents must appear in the result
		for _, content := range userContents {
			if !strings.Contains(result, content) {
				t.Fatalf("User message content %q not found in extracted text %q", content, result)
			}
		}

		// Property 2: User messages must appear in order (the core property)
		// This is the definitive check - the result should be exactly the user messages joined by spaces
		// This implicitly validates that non-user messages are excluded since the result must match exactly
		expectedJoined := strings.Join(userContents, " ")
		if result != expectedJoined {
			t.Fatalf("Extracted text does not match expected.\nExpected: %q\nGot: %q", expectedJoined, result)
		}
	})
}

// Test edge case: empty request
func TestExtractQueryText_NilRequest(t *testing.T) {
	result := ExtractQueryText(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil request, got %q", result)
	}
}

// Test edge case: no user messages
func TestExtractQueryText_NoUserMessages(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numMessages := rapid.IntRange(0, 5).Draw(t, "numMessages")
		nonUserRoles := []string{"system", "assistant"}

		var messages []Message
		for i := 0; i < numMessages; i++ {
			role := nonUserRoles[rapid.IntRange(0, len(nonUserRoles)-1).Draw(t, "roleIdx")]
			content := rapid.StringMatching(`[a-zA-Z0-9 ]{1,30}`).Draw(t, "content")
			messages = append(messages, Message{Role: role, Content: content})
		}

		req := &ChatCompletionRequest{
			Model:    "gpt-4",
			Messages: messages,
		}

		result := ExtractQueryText(req)
		if result != "" {
			t.Fatalf("Expected empty string when no user messages, got %q", result)
		}
	})
}


// **Feature: semantic-cache-gateway, Property 3: SHA-256 Hash Determinism**
// **Validates: Requirements 3.1**
//
// For any query text string, computing the SHA-256 hash multiple times
// SHALL always produce the same hash value.
func TestComputeQueryHash_Determinism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random query text
		queryText := rapid.String().Draw(t, "queryText")

		// Compute hash multiple times
		hash1 := ComputeQueryHash(queryText)
		hash2 := ComputeQueryHash(queryText)
		hash3 := ComputeQueryHash(queryText)

		// Property: All hash computations must produce identical results
		if hash1 != hash2 {
			t.Fatalf("Hash not deterministic: first=%q, second=%q", hash1, hash2)
		}
		if hash2 != hash3 {
			t.Fatalf("Hash not deterministic: second=%q, third=%q", hash2, hash3)
		}

		// Property: Hash must have the expected prefix
		if !strings.HasPrefix(hash1, "sha256:") {
			t.Fatalf("Hash missing expected prefix 'sha256:', got %q", hash1)
		}

		// Property: Hash hex portion must be 64 characters (256 bits = 32 bytes = 64 hex chars)
		hexPart := strings.TrimPrefix(hash1, "sha256:")
		if len(hexPart) != 64 {
			t.Fatalf("Hash hex portion should be 64 characters, got %d: %q", len(hexPart), hexPart)
		}
	})
}
