package loader

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createCustomTitleLine creates a custom-title JSONL line
func createCustomTitleLine(sessionID, title string) string {
	entry := map[string]interface{}{
		"type":        "custom-title",
		"customTitle": title,
		"sessionId":   sessionID,
	}
	data, _ := json.Marshal(entry)
	return string(data)
}

// createAgentNameLine creates an agent-name JSONL line
func createAgentNameLine(sessionID, name string) string {
	entry := map[string]interface{}{
		"type":      "agent-name",
		"agentName": name,
		"sessionId": sessionID,
	}
	data, _ := json.Marshal(entry)
	return string(data)
}

// createTestJSONLEntryWithSessionID creates a JSONL usage entry with a session ID
func createTestJSONLEntryWithSessionID(ts time.Time, model string, inputTokens, outputTokens int, messageID, requestID, sessionID string) string {
	entry := map[string]interface{}{
		"timestamp":    ts.Format(time.RFC3339),
		"model":        model,
		"requestId":    requestID,
		"sessionId":    sessionID,
		"project_path": "/test/project",
		"message": map[string]interface{}{
			"id":    messageID,
			"model": model,
			"usage": map[string]interface{}{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
			},
		},
	}
	data, _ := json.Marshal(entry)
	return string(data)
}

func TestSessionNameFromCustomTitle(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	sessionID := "ca81db6e-cb9b-4b53-995b-f5d58b0e52f1"
	ts := time.Now()

	lines := []string{
		createCustomTitleLine(sessionID, "topamo-blacklist-feature"),
		createTestJSONLEntryWithSessionID(ts, "claude-sonnet-4-5-20250514", 100, 50, "msg1", "req1", sessionID),
		createTestJSONLEntryWithSessionID(ts.Add(time.Minute), "claude-sonnet-4-5-20250514", 200, 100, "msg2", "req2", sessionID),
	}

	addProjectFile(t, basePath, "test-project", sessionID+".jsonl", lines)

	l := New()
	entries, err := l.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	for _, entry := range entries {
		assert.Equal(t, "topamo-blacklist-feature", entry.SessionName, "SessionName should be populated from custom-title")
		assert.Equal(t, sessionID, entry.SessionID, "SessionID should be preserved")
	}
}

func TestSessionNameFromAgentName(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	sessionID := "ab12cd34-ef56-7890-abcd-ef1234567890"
	ts := time.Now()

	lines := []string{
		createAgentNameLine(sessionID, "my-agent-task"),
		createTestJSONLEntryWithSessionID(ts, "claude-sonnet-4-5-20250514", 100, 50, "msg1", "req1", sessionID),
	}

	addProjectFile(t, basePath, "test-project", sessionID+".jsonl", lines)

	l := New()
	entries, err := l.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	assert.Equal(t, "my-agent-task", entries[0].SessionName, "SessionName should fallback to agent-name")
}

func TestCustomTitlePriorityOverAgentName(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	sessionID := "11111111-2222-3333-4444-555555555555"
	ts := time.Now()

	lines := []string{
		createAgentNameLine(sessionID, "agent-fallback-name"),
		createCustomTitleLine(sessionID, "preferred-custom-title"),
		createTestJSONLEntryWithSessionID(ts, "claude-sonnet-4-5-20250514", 100, 50, "msg1", "req1", sessionID),
	}

	addProjectFile(t, basePath, "test-project", sessionID+".jsonl", lines)

	l := New()
	entries, err := l.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	assert.Equal(t, "preferred-custom-title", entries[0].SessionName, "custom-title should take priority over agent-name")
}

func TestNoSessionName(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	sessionID := "99999999-8888-7777-6666-555544443333"
	ts := time.Now()

	lines := []string{
		createTestJSONLEntryWithSessionID(ts, "claude-sonnet-4-5-20250514", 100, 50, "msg1", "req1", sessionID),
	}

	addProjectFile(t, basePath, "test-project", sessionID+".jsonl", lines)

	l := New()
	entries, err := l.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	assert.Empty(t, entries[0].SessionName, "SessionName should be empty when no custom-title or agent-name exists")
}

func TestSourceFilePopulated(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	sessionID := "ffffffff-eeee-dddd-cccc-bbbbbbbbbbbb"
	ts := time.Now()

	lines := []string{
		createTestJSONLEntryWithSessionID(ts, "claude-sonnet-4-5-20250514", 100, 50, "msg1", "req1", sessionID),
	}

	addProjectFile(t, basePath, "test-project", sessionID+".jsonl", lines)

	l := New()
	entries, err := l.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	assert.NotEmpty(t, entries[0].SourceFile, "SourceFile should be populated with the JSONL file path")
	assert.Contains(t, entries[0].SourceFile, sessionID+".jsonl", "SourceFile should contain the filename")
}

func TestSessionNameAcrossFiles(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	ts := time.Now()

	// 主檔：含 custom-title + usage entry
	mainLines := []string{
		createCustomTitleLine(sessionID, "cross-file-feature"),
		createTestJSONLEntryWithSessionID(ts, "claude-sonnet-4-5-20250514", 100, 50, "msg1", "req1", sessionID),
	}
	addProjectFile(t, basePath, "test-project", sessionID+".jsonl", mainLines)

	// Subagent 檔：只有 usage entry，無 custom-title，但 sessionId 相同
	subLines := []string{
		createTestJSONLEntryWithSessionID(ts.Add(time.Minute), "claude-sonnet-4-5-20250514", 200, 100, "msg2", "req2", sessionID),
	}
	addProjectFile(t, basePath, "test-project/"+sessionID+"/subagents", "agent-abc123.jsonl", subLines)

	l := New()
	entries, err := l.LoadFromPath(context.Background(), basePath)
	require.NoError(t, err)
	require.Len(t, entries, 2, "Should load entries from both main file and subagent file")

	for _, entry := range entries {
		assert.Equal(t, "cross-file-feature", entry.SessionName,
			"All entries with same sessionID should have SessionName, even from subagent files")
		assert.Equal(t, sessionID, entry.SessionID)
	}
}
