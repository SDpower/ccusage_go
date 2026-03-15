package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sdpower/ccusage-go/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCalculator implements CostCalculator for testing
type mockCalculator struct {
	costPerEntry float64
}

func (m *mockCalculator) CalculateCost(entry *types.UsageEntry) error {
	entry.Cost = m.costPerEntry
	return nil
}

// createTestJSONLEntry creates a JSONL line with the given parameters
func createTestJSONLEntry(ts time.Time, model string, inputTokens, outputTokens int, messageID, requestID string) string {
	entry := map[string]interface{}{
		"timestamp":    ts.Format(time.RFC3339),
		"model":        model,
		"requestId":    requestID,
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

// setupTestProject creates a temporary project structure for testing
func setupTestProject(t *testing.T) (basePath string, cleanup func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "project_cache_test")
	require.NoError(t, err)

	// Create projects directory structure
	projectsDir := filepath.Join(tmpDir, "projects")
	require.NoError(t, os.MkdirAll(projectsDir, 0o755))

	cleanup = func() {
		os.RemoveAll(tmpDir)
	}
	return tmpDir, cleanup
}

// addProjectFile adds a JSONL file to a project directory
func addProjectFile(t *testing.T, basePath, projectName, fileName string, lines []string) string {
	t.Helper()
	projectDir := filepath.Join(basePath, "projects", projectName)
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	filePath := filepath.Join(projectDir, fileName)
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))
	return filePath
}

func TestNewProjectDetection(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	lines := []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 100, 50, "msg1", "req1"),
	}
	addProjectFile(t, basePath, "project-a", "session.jsonl", lines)

	cache := NewIncrementalCache()
	loader := New()
	calc := &mockCalculator{costPerEntry: 0.01}

	entries, changed, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed, "first load should report changed")
	assert.Len(t, entries, 1, "should have 1 entry")
	assert.Equal(t, 0.01, entries[0].Cost, "cost should be calculated")
}

func TestFileChangeDetection(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	lines := []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 100, 50, "msg1", "req1"),
	}
	filePath := addProjectFile(t, basePath, "project-a", "session.jsonl", lines)

	cache := NewIncrementalCache()
	loader := New()
	calc := &mockCalculator{costPerEntry: 0.01}

	// First load
	_, changed, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed)

	// Second load — no change
	_, changed, err = cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.False(t, changed, "no change should be detected")

	// Modify the file — change ModTime by rewriting with new content
	time.Sleep(10 * time.Millisecond) // ensure different ModTime
	newLines := []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 100, 50, "msg1", "req1"),
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 200, 100, "msg2", "req2"),
	}
	content := ""
	for _, line := range newLines {
		content += line + "\n"
	}
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	// Third load — should detect change
	entries, changed, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed, "file modification should be detected")
	assert.Len(t, entries, 2, "should have 2 entries after append")
}

func TestFileAppendOnly(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	lines := []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 100, 50, "msg1", "req1"),
	}
	filePath := addProjectFile(t, basePath, "project-a", "session.jsonl", lines)

	cache := NewIncrementalCache()
	loader := New()
	calc := &mockCalculator{costPerEntry: 0.01}

	// First load
	entries, changed, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Len(t, entries, 1)

	// Append new entry (JSONL is append-only)
	time.Sleep(10 * time.Millisecond)
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	newLine := createTestJSONLEntry(now, "claude-sonnet-4-20250514", 300, 150, "msg3", "req3")
	_, err = f.WriteString(newLine + "\n")
	require.NoError(t, err)
	f.Close()

	// Second load — should detect size/modtime change and re-read
	entries, changed, err = cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed, "file append should be detected")
	// Due to full re-read of changed file + dedup, we should get both entries
	assert.GreaterOrEqual(t, len(entries), 2, "should have at least 2 entries after append")
}

func TestProjectRemoval(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	lines := []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 100, 50, "msg1", "req1"),
	}
	addProjectFile(t, basePath, "project-a", "session.jsonl", lines)
	addProjectFile(t, basePath, "project-b", "session.jsonl", []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 200, 100, "msg2", "req2"),
	})

	cache := NewIncrementalCache()
	loader := New()
	calc := &mockCalculator{costPerEntry: 0.01}

	// First load — both projects
	entries, changed, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Len(t, entries, 2)

	// Remove project-b
	require.NoError(t, os.RemoveAll(filepath.Join(basePath, "projects", "project-b")))

	// Second load — should detect removal
	entries, changed, err = cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed, "project removal should be detected")
	assert.Len(t, entries, 1, "should have 1 entry after removal")
}

func TestFileDeletedInProject(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	addProjectFile(t, basePath, "project-a", "session1.jsonl", []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 100, 50, "msg1", "req1"),
	})
	addProjectFile(t, basePath, "project-a", "session2.jsonl", []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 200, 100, "msg2", "req2"),
	})

	cache := NewIncrementalCache()
	loader := New()
	calc := &mockCalculator{costPerEntry: 0.01}

	// First load
	entries, changed, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Len(t, entries, 2)

	// Delete one file
	require.NoError(t, os.Remove(filepath.Join(basePath, "projects", "project-a", "session2.jsonl")))

	// Second load — should trigger full reload of project
	entries, changed, err = cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed, "file deletion should trigger full reload")
	assert.Len(t, entries, 1, "should only have entries from remaining file")
}

func TestNoChangeSkip(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	addProjectFile(t, basePath, "project-a", "session.jsonl", []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 100, 50, "msg1", "req1"),
	})

	cache := NewIncrementalCache()
	loader := New()
	calc := &mockCalculator{costPerEntry: 0.01}

	// First load
	entries1, changed1, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed1)
	assert.Len(t, entries1, 1)

	// Second load — no change
	entries2, changed2, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.False(t, changed2, "should report no change on second call")
	assert.Len(t, entries2, 1, "should return cached entries")

	// Third load — still no change
	entries3, changed3, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.False(t, changed3, "should still report no change")
	assert.Len(t, entries3, 1)
}

func TestDedupWithinProject(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	// Same entry in two different files within same project
	addProjectFile(t, basePath, "project-a", "session1.jsonl", []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 100, 50, "msg1", "req1"),
	})
	addProjectFile(t, basePath, "project-a", "session2.jsonl", []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 100, 50, "msg1", "req1"), // duplicate
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 200, 100, "msg2", "req2"),
	})

	cache := NewIncrementalCache()
	loader := New()
	calc := &mockCalculator{costPerEntry: 0.01}

	entries, changed, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Len(t, entries, 2, "duplicate entry should be deduplicated within project")
}

func TestMultipleProjects(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	for i := 0; i < 5; i++ {
		projectName := fmt.Sprintf("project-%d", i)
		addProjectFile(t, basePath, projectName, "session.jsonl", []string{
			createTestJSONLEntry(now, "claude-sonnet-4-20250514", 100*(i+1), 50*(i+1),
				fmt.Sprintf("msg-%d", i), fmt.Sprintf("req-%d", i)),
		})
	}

	cache := NewIncrementalCache()
	loader := New()
	calc := &mockCalculator{costPerEntry: 0.01}

	entries, changed, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Len(t, entries, 5)

	// Verify stats
	projectCount, totalEntries, totalFiles := cache.Stats()
	assert.Equal(t, 5, projectCount)
	assert.Equal(t, 5, totalEntries)
	assert.Equal(t, 5, totalFiles)
}

func TestCacheReset(t *testing.T) {
	basePath, cleanup := setupTestProject(t)
	defer cleanup()

	now := time.Now()
	addProjectFile(t, basePath, "project-a", "session.jsonl", []string{
		createTestJSONLEntry(now, "claude-sonnet-4-20250514", 100, 50, "msg1", "req1"),
	})

	cache := NewIncrementalCache()
	loader := New()
	calc := &mockCalculator{costPerEntry: 0.01}

	_, _, err := cache.Update(loader, calc, basePath, 24*time.Hour)
	require.NoError(t, err)

	projectCount, _, _ := cache.Stats()
	assert.Equal(t, 1, projectCount)

	cache.Reset()
	projectCount, totalEntries, totalFiles := cache.Stats()
	assert.Equal(t, 0, projectCount)
	assert.Equal(t, 0, totalEntries)
	assert.Equal(t, 0, totalFiles)
}
