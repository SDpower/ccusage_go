package loader

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nestedCacheLine builds an assistant entry whose usage carries ONLY the
// nested cache_creation structure (Anthropic's newer schema), without the
// flat cache_creation_input_tokens field.
func nestedCacheLine(ts time.Time, ephemeral1h, ephemeral5m, cacheRead int) string {
	usage := map[string]any{
		"input_tokens":             4,
		"output_tokens":            10,
		"cache_read_input_tokens":  cacheRead,
		"cache_creation": map[string]any{
			"ephemeral_1h_input_tokens": ephemeral1h,
			"ephemeral_5m_input_tokens": ephemeral5m,
		},
	}
	entry := map[string]any{
		"type":      "assistant",
		"timestamp": ts.Format(time.RFC3339),
		"requestId": "req-nested",
		"sessionId": "sess-nested",
		"message": map[string]any{
			"id":    "msg-nested",
			"model": "claude-opus-4-20250514",
			"usage": usage,
		},
	}
	data, _ := json.Marshal(entry)
	return string(data)
}

// TestNestedCacheCreationFallback verifies P1: when only nested
// cache_creation.ephemeral_*_input_tokens are present, the loader sums them
// into the entry's cache creation token count.
func TestNestedCacheCreationFallback(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	addProjectFile(t, basePath, "proj-nested", "session.jsonl", []string{
		nestedCacheLine(now, 2916, 100, 124915),
	})

	ldr := New()
	entries, err := ldr.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	e := entries[0]
	expectedCacheCreation := 2916 + 100
	assert.Equal(t, expectedCacheCreation, e.CacheCreationInputTokens,
		"nested ephemeral_1h + ephemeral_5m must populate CacheCreationInputTokens")
	assert.Equal(t, 124915, e.CacheReadInputTokens)
	// TotalTokens = input + output + cache_creation + cache_read
	assert.Equal(t, 4+10+expectedCacheCreation+124915, e.TotalTokens)
}

// TestFlatCacheStillWorks ensures the existing flat cache_creation_input_tokens
// path remains correct (no regression from the nested fallback).
func TestFlatCacheStillWorks(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	usage := map[string]any{
		"input_tokens":                4,
		"output_tokens":               10,
		"cache_creation_input_tokens": 500,
		"cache_read_input_tokens":     1000,
	}
	entry := map[string]any{
		"type":      "assistant",
		"timestamp": now.Format(time.RFC3339),
		"requestId": "req-flat",
		"sessionId": "sess-flat",
		"message": map[string]any{
			"id":    "msg-flat",
			"model": "claude-sonnet-4-20250514",
			"usage": usage,
		},
	}
	data, _ := json.Marshal(entry)

	addProjectFile(t, basePath, "proj-flat", "session.jsonl", []string{string(data)})

	ldr := New()
	entries, err := ldr.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	assert.Equal(t, 500, entries[0].CacheCreationInputTokens)
	assert.Equal(t, 1000, entries[0].CacheReadInputTokens)
}

// TestFlatPrecedenceOverNested ensures flat takes precedence when both exist.
// Real Anthropic data currently emits both with identical values; flat should win.
func TestFlatPrecedenceOverNested(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	usage := map[string]any{
		"input_tokens":                4,
		"output_tokens":               10,
		"cache_creation_input_tokens": 7777,
		"cache_creation": map[string]any{
			"ephemeral_1h_input_tokens": 1,
			"ephemeral_5m_input_tokens": 2,
		},
	}
	entry := map[string]any{
		"type":      "assistant",
		"timestamp": now.Format(time.RFC3339),
		"requestId": "req-both",
		"sessionId": "sess-both",
		"message": map[string]any{
			"id":    "msg-both",
			"model": "claude-sonnet-4-20250514",
			"usage": usage,
		},
	}
	data, _ := json.Marshal(entry)

	addProjectFile(t, basePath, "proj-both", "session.jsonl", []string{string(data)})

	ldr := New()
	entries, err := ldr.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, 7777, entries[0].CacheCreationInputTokens, "flat must take precedence")
}

// silence unused import warning helper if any future test needs strings
var _ = strings.Contains

// TestSessionName_AITitleAsThirdSource verifies P3: ai-title fills SessionName
// when neither custom-title nor agent-name supplies one.
func TestSessionName_AITitleAsThirdSource(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	sessionID := "sess-ai-only"
	aiTitleLine := func(sid, title string) string {
		raw := map[string]any{
			"type":      "ai-title",
			"aiTitle":   title,
			"sessionId": sid,
		}
		b, _ := json.Marshal(raw)
		return string(b)
	}

	addProjectFile(t, basePath, "proj-ai", "session.jsonl", []string{
		aiTitleLine(sessionID, "AI generated title"),
		createTestJSONLEntryWithSessionID(now, "claude-sonnet-4-20250514", 10, 5, "m-ai", "r-ai", sessionID),
	})

	ldr := New()
	entries, err := ldr.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "AI generated title", entries[0].SessionName)
}

// TestSessionName_PriorityOrder ensures custom-title > agent-name > ai-title.
func TestSessionName_PriorityOrder(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	sessionID := "sess-prio"

	customLine := func(sid, t string) string {
		b, _ := json.Marshal(map[string]any{"type": "custom-title", "customTitle": t, "sessionId": sid})
		return string(b)
	}
	agentLine := func(sid, n string) string {
		b, _ := json.Marshal(map[string]any{"type": "agent-name", "agentName": n, "sessionId": sid})
		return string(b)
	}
	aiLine := func(sid, t string) string {
		b, _ := json.Marshal(map[string]any{"type": "ai-title", "aiTitle": t, "sessionId": sid})
		return string(b)
	}

	addProjectFile(t, basePath, "proj-prio", "session.jsonl", []string{
		aiLine(sessionID, "ai-name"),
		agentLine(sessionID, "agent-name"),
		customLine(sessionID, "custom-name"),
		createTestJSONLEntryWithSessionID(now, "claude-sonnet-4-20250514", 10, 5, "m-prio", "r-prio", sessionID),
	})

	ldr := New()
	entries, err := ldr.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "custom-name", entries[0].SessionName, "custom-title must win")
}

// TestNonStreamPath_PreservesUsageLimitResetTime verifies the non-stream path
// also retains usage_limit_reset_time on entry.Raw.
func TestNonStreamPath_PreservesUsageLimitResetTime(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	resetT := now.Add(2 * time.Hour).Format(time.RFC3339)
	raw := map[string]any{
		"type":                   "assistant",
		"timestamp":              now.Format(time.RFC3339),
		"requestId":              "req-reset",
		"sessionId":              "sess-reset",
		"usage_limit_reset_time": resetT,
		"message": map[string]any{
			"id":    "msg-reset",
			"model": "claude-opus-4-20250514",
			"usage": map[string]any{
				"input_tokens":  1,
				"output_tokens": 1,
			},
		},
	}
	data, _ := json.Marshal(raw)
	addProjectFile(t, basePath, "proj-reset", "session.jsonl", []string{string(data)})

	ldr := New()
	// Non-stream path: do NOT set StreamProcessing.
	entries, err := ldr.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].Raw, "Raw must retain usage_limit_reset_time")
	assert.Equal(t, resetT, entries[0].Raw["usage_limit_reset_time"])
}

// TestServerToolUse_PopulatesRequestCounts verifies P2: usage.server_tool_use
// fields are extracted into first-class WebSearchRequests / WebFetchRequests.
func TestServerToolUse_PopulatesRequestCounts(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	raw := map[string]any{
		"type":      "assistant",
		"timestamp": now.Format(time.RFC3339),
		"requestId": "req-stu",
		"sessionId": "sess-stu",
		"message": map[string]any{
			"id":    "msg-stu",
			"model": "claude-opus-4-20250514",
			"usage": map[string]any{
				"input_tokens":  1,
				"output_tokens": 1,
				"server_tool_use": map[string]any{
					"web_search_requests": 3,
					"web_fetch_requests":  2,
				},
			},
		},
	}
	data, _ := json.Marshal(raw)
	addProjectFile(t, basePath, "proj-stu", "session.jsonl", []string{string(data)})

	ldr := New()
	entries, err := ldr.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, 3, entries[0].WebSearchRequests)
	assert.Equal(t, 2, entries[0].WebFetchRequests)
}

// TestIsSidechain_PreservedAndNotFiltered ensures sub-agent entries are loaded
// (not filtered) and IsSidechain reflects the JSONL field.
func TestIsSidechain_PreservedAndNotFiltered(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	raw := map[string]any{
		"type":        "assistant",
		"timestamp":   now.Format(time.RFC3339),
		"requestId":   "req-side",
		"sessionId":   "sess-side",
		"isSidechain": true,
		"message": map[string]any{
			"id":    "msg-side",
			"model": "claude-opus-4-20250514",
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		},
	}
	data, _ := json.Marshal(raw)
	addProjectFile(t, basePath, "proj-side", "session.jsonl", []string{string(data)})

	ldr := New()
	entries, err := ldr.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1, "sidechain entries must NOT be filtered")
	assert.True(t, entries[0].IsSidechain)
}

// TestShouldCountAsParseError_AllowlistedTypes ensures legitimate non-usage
// types (attachment, system, last-prompt, file-history-snapshot,
// permission-mode, agent-setting, queue-operation, ai-title, user, summary)
// do NOT inflate the parse-error count.
func TestShouldCountAsParseError_AllowlistedTypes(t *testing.T) {
	ldr := New()
	missingUsage := assertError("missing required message.usage object")
	missingMsg := assertError("missing required message object")

	allowed := []string{
		"user", "summary", "attachment", "system", "last-prompt",
		"file-history-snapshot", "permission-mode", "agent-setting",
		"queue-operation", "ai-title",
	}
	for _, ty := range allowed {
		raw := map[string]interface{}{"type": ty}
		assert.False(t, ldr.shouldCountAsParseError(missingUsage, raw),
			"%s should not be counted as parse error (missing usage)", ty)
		assert.False(t, ldr.shouldCountAsParseError(missingMsg, raw),
			"%s should not be counted as parse error (missing message)", ty)
	}

	// Unknown / assistant types still count as parse error if usage missing.
	assert.True(t, ldr.shouldCountAsParseError(missingUsage,
		map[string]interface{}{"type": "assistant"}))
	assert.True(t, ldr.shouldCountAsParseError(missingUsage,
		map[string]interface{}{"type": "unknown-future-type"}))
}

// assertError is a tiny helper to build an error with a given message
// for parse-error allowlist tests.
func assertError(msg string) error { return errString(msg) }

type errString string

func (e errString) Error() string { return string(e) }

// TestCacheMissReason_PopulatedFromDiagnostics covers diagnostics extraction.
func TestCacheMissReason_PopulatedFromDiagnostics(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	raw := map[string]any{
		"type":      "assistant",
		"timestamp": now.Format(time.RFC3339),
		"requestId": "req-diag",
		"sessionId": "sess-diag",
		"message": map[string]any{
			"id":    "msg-diag",
			"model": "claude-opus-4-20250514",
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
			"diagnostics": map[string]any{
				"cache_miss_reason": map[string]any{
					"type":                      "tool_call_change",
					"cache_missed_input_tokens": 12345,
				},
			},
		},
	}
	data, _ := json.Marshal(raw)
	addProjectFile(t, basePath, "proj-diag", "session.jsonl", []string{string(data)})

	ldr := New()
	entries, err := ldr.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].CacheMissReason)
	assert.Equal(t, "tool_call_change", entries[0].CacheMissReason.Type)
	assert.Equal(t, 12345, entries[0].CacheMissReason.CacheMissedInputTokens)
}
