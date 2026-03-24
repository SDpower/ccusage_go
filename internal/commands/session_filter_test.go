package commands

import (
	"testing"
	"time"

	"github.com/sdpower/ccusage-go/internal/types"
	"github.com/stretchr/testify/assert"
)

func createTestEntries() []types.UsageEntry {
	ts := time.Now()
	return []types.UsageEntry{
		{ID: "1", Timestamp: ts, SessionID: "sess-aaa", SessionName: "feature-alpha", ProjectPath: "/project/a"},
		{ID: "2", Timestamp: ts, SessionID: "sess-aaa", SessionName: "feature-alpha", ProjectPath: "/project/a"},
		{ID: "3", Timestamp: ts, SessionID: "sess-bbb", SessionName: "bugfix-beta", ProjectPath: "/project/b"},
		{ID: "4", Timestamp: ts, SessionID: "sess-ccc", SessionName: "", ProjectPath: "/project/c"},
	}
}

func TestFilterBySessionID(t *testing.T) {
	entries := createTestEntries()
	filtered := filterEntriesBySessionID(entries, "sess-aaa")

	assert.Len(t, filtered, 2)
	for _, e := range filtered {
		assert.Equal(t, "sess-aaa", e.SessionID)
	}
}

func TestFilterBySessionName(t *testing.T) {
	entries := createTestEntries()
	filtered := filterEntriesBySessionName(entries, "bugfix-beta")

	assert.Len(t, filtered, 1)
	assert.Equal(t, "sess-bbb", filtered[0].SessionID)
}

func TestFilterBySessionIDNoMatch(t *testing.T) {
	entries := createTestEntries()
	filtered := filterEntriesBySessionID(entries, "nonexistent")

	assert.Len(t, filtered, 0)
}

func TestFilterBySessionNameNoMatch(t *testing.T) {
	entries := createTestEntries()
	filtered := filterEntriesBySessionName(entries, "nonexistent")

	assert.Len(t, filtered, 0)
}

func TestFilterBySessionNameEmpty(t *testing.T) {
	entries := createTestEntries()
	// Entry 4 has empty SessionName, should not match empty string filter
	filtered := filterEntriesBySessionName(entries, "")

	// Empty filter should return all entries (no filtering)
	assert.Len(t, filtered, len(entries))
}
