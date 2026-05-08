package loader

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addSubagentFile writes a JSONL file under projects/<projectName>/subagents/
func addSubagentFile(t *testing.T, basePath, projectName, fileName string, lines []string) string {
	t.Helper()
	dir := filepath.Join(basePath, "projects", projectName, "subagents")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	filePath := filepath.Join(dir, fileName)
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))
	return filePath
}

// TestFilterMode_RecursesSubagentsDir verifies P0 fix: filter mode (ModifiedWithin)
// must include JSONL files inside subagents/ subdirectories. Previously
// collectProjectFiles skipped all subdirectories, missing sub-agent cost.
func TestFilterMode_RecursesSubagentsDir(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	mainLines := []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 100, 50, "msg-main", "req-main"),
	}
	addProjectFile(t, basePath, "proj-a", "session.jsonl", mainLines)

	subLines := []string{
		createTestJSONLEntry(now, "claude-opus-4-20250514", 200, 80, "msg-sub", "req-sub"),
	}
	addSubagentFile(t, basePath, "proj-a", "agent-x.jsonl", subLines)

	loader := New()
	ctx := context.Background()

	// Filter mode (ModifiedWithin) — the path that previously missed subagents
	entries, err := loader.LoadFromPathWithOptions(ctx, basePath, &LoaderOptions{
		ModifiedWithin: 24 * time.Hour,
	})
	require.NoError(t, err)
	assert.Len(t, entries, 2, "filter mode must include subagent file")

	var totalIn, totalOut int
	for _, e := range entries {
		totalIn += e.InputTokens
		totalOut += e.OutputTokens
	}
	assert.Equal(t, 300, totalIn)
	assert.Equal(t, 130, totalOut)
}

// TestFilterMode_MatchesDefaultMode_WithSubagents ensures the filter path
// produces the same entry set as the default Walk path when ModifiedWithin
// covers all files.
func TestFilterMode_MatchesDefaultMode_WithSubagents(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	addProjectFile(t, basePath, "proj-a", "s1.jsonl", []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 10, 5, "m1", "r1"),
	})
	addSubagentFile(t, basePath, "proj-a", "agent-1.jsonl", []string{
		createTestJSONLEntry(now, "claude-opus-4-20250514", 20, 10, "m2", "r2"),
	})
	addSubagentFile(t, basePath, "proj-a", "agent-2.jsonl", []string{
		createTestJSONLEntry(now, "claude-haiku-4-20250514", 30, 15, "m3", "r3"),
	})

	loader := New()
	ctx := context.Background()

	defaultEntries, err := loader.LoadFromPath(ctx, basePath)
	require.NoError(t, err)

	filterEntries, err := loader.LoadFromPathWithOptions(ctx, basePath, &LoaderOptions{
		ModifiedWithin: 24 * time.Hour,
	})
	require.NoError(t, err)

	assert.Equal(t, len(defaultEntries), len(filterEntries),
		"filter mode entry count must match default mode when window covers all files")
}

// TestIncrementalCache_DetectsSubagentChange verifies that modifying a
// subagents/*.jsonl file invalidates the project cache (live mode P0 follow-up).
func TestIncrementalCache_DetectsSubagentChange(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	addProjectFile(t, basePath, "proj-a", "main.jsonl", []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 10, 5, "m1", "r1"),
	})
	subPath := addSubagentFile(t, basePath, "proj-a", "agent-1.jsonl", []string{
		createTestJSONLEntry(now, "claude-opus-4-20250514", 20, 10, "m2", "r2"),
	})

	cache := NewIncrementalCache()
	loader := New()
	calc := &mockCalculator{costPerEntry: 0.01}

	entries, changed, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, entries, 2, "first load must include subagent entry")

	// Re-load: nothing changed
	_, changed, err = cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.False(t, changed, "second load with no changes must report unchanged")

	// Modify subagent file
	time.Sleep(10 * time.Millisecond)
	newSubLines := []string{
		createTestJSONLEntry(now, "claude-opus-4-20250514", 20, 10, "m2", "r2"),
		createTestJSONLEntry(now, "claude-opus-4-20250514", 30, 15, "m3", "r3"),
	}
	content := ""
	for _, line := range newSubLines {
		content += line + "\n"
	}
	require.NoError(t, os.WriteFile(subPath, []byte(content), 0o644))

	entries, changed, err = cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed, "subagent file modification must invalidate cache")
	assert.Len(t, entries, 3, "must include the new subagent entry")
}

// TestShouldSkipProject_DetectsSubagentActivity verifies that a project whose
// only recent activity is inside subagents/ is NOT skipped.
func TestShouldSkipProject_DetectsSubagentActivity(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	// Main JSONL is OLD (2 days ago)
	oldLines := []string{
		createTestJSONLEntry(now.Add(-48*time.Hour), "claude-sonnet-4-20250514", 5, 5, "old-m", "old-r"),
	}
	mainPath := addProjectFile(t, basePath, "proj-a", "old.jsonl", oldLines)
	twoDaysAgo := now.Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(mainPath, twoDaysAgo, twoDaysAgo))

	// Subagent JSONL is RECENT
	addSubagentFile(t, basePath, "proj-a", "agent-recent.jsonl", []string{
		createTestJSONLEntry(now, "claude-opus-4-20250514", 100, 50, "recent-m", "recent-r"),
	})

	loader := New()
	ctx := context.Background()
	entries, err := loader.LoadFromPathWithOptions(ctx, basePath, &LoaderOptions{
		ModifiedWithin: 24 * time.Hour,
	})
	require.NoError(t, err)

	// Should pick up only the recent subagent entry (old main filtered out by mtime),
	// proving the project was NOT skipped.
	assert.Len(t, entries, 1, "project with recent subagent activity must not be skipped")
	if len(entries) == 1 {
		assert.Equal(t, 100, entries[0].InputTokens)
	}
}
