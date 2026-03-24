package output

import (
	"strings"
	"testing"
	"time"

	"github.com/sdpower/ccusage-go/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestSessionReportShowsSessionName(t *testing.T) {
	sessions := []types.SessionInfo{
		{
			SessionID:   "/path/projects/test-project",
			SessionName: "feature-login",
			SessionIDs:  []string{"ca81db6e-cb9b-4b53-995b-f5d58b0e52f1"},
			ProjectPath: "/path/projects/test-project",
			StartTime:   time.Now().Add(-time.Hour),
			EndTime:     time.Now(),
			LastActivity: time.Now(),
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			ModelsUsed:  []string{"claude-sonnet-4-5-20250514"},
		},
	}

	formatter := NewTableWriterFormatter(true) // noColor=true for testing
	output := formatter.FormatSessionReport(sessions)

	assert.Contains(t, output, "feature-login", "Session report should display session name")
}

func TestSessionReportShowsFilesColumn(t *testing.T) {
	sessions := []types.SessionInfo{
		{
			SessionID:   "/path/projects/test-project",
			SessionName: "feature-login",
			SessionIDs:  []string{"ca81db6e-cb9b-4b53-995b-f5d58b0e52f1"},
			SourceFiles: []string{
				"/data/sess-1.jsonl",
				"/data/sess-1/subagents/agent-a.jsonl",
				"/data/sess-1/subagents/agent-b.jsonl",
			},
			ProjectPath:  "/path/projects/test-project",
			StartTime:    time.Now().Add(-time.Hour),
			EndTime:      time.Now(),
			LastActivity: time.Now(),
			InputTokens:  100, OutputTokens: 50, TotalTokens: 150,
			ModelsUsed:   []string{"claude-sonnet-4-5-20250514"},
		},
	}

	formatter := NewTableWriterFormatter(true)
	output := formatter.FormatSessionReport(sessions)

	assert.Contains(t, output, "Files", "Session report should have Files column header")
	assert.Contains(t, output, "3", "Session report should show 3 source files")
}

func TestSessionReportShowsSessionIDs(t *testing.T) {
	sessions := []types.SessionInfo{
		{
			SessionID:   "/path/projects/test-project",
			SessionName: "feature-login",
			SessionIDs:  []string{"ca81db6e-cb9b-4b53-995b-f5d58b0e52f1", "deadbeef-1234-5678-abcd-ef0123456789"},
			ProjectPath: "/path/projects/test-project",
			StartTime:   time.Now().Add(-time.Hour),
			EndTime:     time.Now(),
			LastActivity: time.Now(),
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			ModelsUsed:  []string{"claude-sonnet-4-5-20250514"},
		},
	}

	formatter := NewTableWriterFormatter(true)
	output := formatter.FormatSessionReport(sessions)

	assert.Contains(t, output, "ca81db6e-cb9b-4b53-995b-f5d58b0e52f1", "Session report should display full session UUID")
	assert.Contains(t, output, "deadbeef-1234-5678-abcd-ef0123456789", "Session report should display all session UUIDs")
}

func TestSessionReportWithoutSessionName(t *testing.T) {
	sessions := []types.SessionInfo{
		{
			SessionID:   "/path/projects/test-project",
			SessionName: "",
			ProjectPath: "/path/projects/test-project",
			StartTime:   time.Now().Add(-time.Hour),
			EndTime:     time.Now(),
			LastActivity: time.Now(),
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			ModelsUsed:  []string{"claude-sonnet-4-5-20250514"},
		},
	}

	formatter := NewTableWriterFormatter(true)
	output := formatter.FormatSessionReport(sessions)

	// Should still work without session name
	assert.Contains(t, output, "Session", "Should have Session column header")
	assert.NotContains(t, output, "feature-login")
}

func TestSessionCSVContainsSessionName(t *testing.T) {
	sessions := []types.SessionInfo{
		{
			SessionID:    "proj-path",
			SessionName:  "my-feature",
			ProjectPath:  "/test/project",
			StartTime:    time.Now().Add(-time.Hour),
			EndTime:      time.Now(),
			Duration:     time.Hour,
			TotalCost:    1.23,
			TotalTokens:  1000,
			RequestCount: 5,
		},
	}

	formatter := NewFormatter(FormatterOptions{Format: "csv"})
	output, err := formatter.FormatSessionReport(sessions)
	assert.NoError(t, err)

	lines := strings.Split(output, "\n")
	assert.Contains(t, lines[0], "session_name", "CSV header should contain session_name")
	assert.Contains(t, lines[1], "my-feature", "CSV row should contain session name")
}

func TestDailyReportContainsSessionsColumn(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp:   ts,
			DateKey:     ts.Format("2006-01-02"),
			SessionID:   "sess-1",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		},
		{
			Timestamp:   ts,
			DateKey:     ts.Format("2006-01-02"),
			SessionID:   "sess-2",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
		},
		{
			Timestamp:   ts,
			DateKey:     ts.Format("2006-01-02"),
			SessionID:   "sess-1", // duplicate session
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 50, OutputTokens: 25, TotalTokens: 75,
		},
	}

	formatter := NewTableWriterFormatter(true)
	output := formatter.FormatDailyReport(entries)

	assert.Contains(t, output, "Sessions", "Daily report should have Sessions column header")
	// 2 unique sessions
	assert.Contains(t, output, "2", "Daily report should show 2 unique sessions")
}

func TestFormatSessionDetailReport(t *testing.T) {
	sessions := []types.SessionInfo{
		{
			SessionID:   "/path/projects/test-project",
			SessionName: "feature-login",
			SessionIDs:  []string{"ca81db6e-cb9b-4b53-995b-f5d58b0e52f1"},
			ProjectPath: "/path/projects/test-project",
			StartTime:   time.Now().Add(-time.Hour),
			EndTime:     time.Now(),
			LastActivity: time.Now(),
			InputTokens: 300, OutputTokens: 150, TotalTokens: 450,
			ModelsUsed:  []string{"claude-sonnet-4-5-20250514"},
		},
	}

	fileStats := []types.SourceFileStat{
		{
			FilePath:    "/data/projects/test/ca81db6e.jsonl",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
			Cost: 1.5, EntryCount: 2,
			ModelsUsed:   []string{"claude-sonnet-4-5-20250514"},
			LastActivity: time.Now(),
		},
		{
			FilePath:    "/data/projects/test/subagents/agent-abc.jsonl",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			Cost: 0.5, EntryCount: 1,
			ModelsUsed:   []string{"claude-sonnet-4-5-20250514"},
			LastActivity: time.Now(),
		},
	}

	formatter := NewTableWriterFormatter(true)
	output := formatter.FormatSessionDetailReport(sessions, fileStats)

	assert.Contains(t, output, "Source", "Should have Source File column header")
	assert.Contains(t, output, "feature-login", "Should display session name")
	assert.Contains(t, output, "ca81db6e.jsonl", "Should display source file name")
	assert.Contains(t, output, "agent-abc.jsonl", "Should display subagent file name")
	assert.Contains(t, output, "Total", "Should have Total footer")
}

func TestDailyReportContainsAPICostColumn(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp: ts, DateKey: ts.Format("2006-01-02"),
			SessionID: "sess-1", Model: "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			Cost: 4.0, APICost: 2.5,
		},
	}

	formatter := NewTableWriterFormatter(true)
	output := formatter.FormatDailyReport(entries)

	assert.Contains(t, output, "API", "Daily report should have API Cost column")
}

func TestSessionDetailReportContainsAPICost(t *testing.T) {
	sessions := []types.SessionInfo{
		{
			SessionID: "/path/test", SessionName: "feat",
			SessionIDs: []string{"uuid-1"}, SourceFiles: []string{"/data/main.jsonl"},
			ProjectPath: "/path/test",
			StartTime: time.Now().Add(-time.Hour), EndTime: time.Now(), LastActivity: time.Now(),
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			TotalCost: 4.0, TotalAPICost: 2.5,
			ModelsUsed: []string{"claude-sonnet-4-5-20250514"},
		},
	}
	fileStats := []types.SourceFileStat{
		{
			FilePath: "/data/main.jsonl",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			Cost: 4.0, APICost: 2.5, EntryCount: 1,
			ModelsUsed: []string{"claude-sonnet-4-5-20250514"},
			LastActivity: time.Now(),
		},
	}

	formatter := NewTableWriterFormatter(true)
	output := formatter.FormatSessionDetailReport(sessions, fileStats)

	assert.Contains(t, output, "API", "Session detail report should have API Cost column")
}

func TestLastActivityHeaderContainsLocaltime(t *testing.T) {
	sessions := []types.SessionInfo{
		{
			SessionID: "/path/test", SessionName: "feat",
			SessionIDs: []string{"uuid-1"},
			ProjectPath: "/path/test",
			StartTime: time.Now().Add(-time.Hour), EndTime: time.Now(), LastActivity: time.Now(),
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			ModelsUsed: []string{"claude-sonnet-4-5-20250514"},
		},
	}

	formatter := NewTableWriterFormatter(true)
	output := formatter.FormatSessionReport(sessions)

	assert.Contains(t, output, "localtime", "Last Activity header should contain localtime")
}

func TestReportsContainCacheCostColumns(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp: ts, DateKey: ts.Format("2006-01-02"),
			SessionID: "sess-1", Model: "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			Cost: 4.0, APICost: 2.5, CacheCreateCost: 1.0, CacheReadCost: 0.5,
		},
	}
	formatter := NewTableWriterFormatter(true)
	output := formatter.FormatDailyReport(entries)
	assert.Contains(t, output, "CC Cost", "Daily report should have CC Cost column")
	assert.Contains(t, output, "CR Cost", "Daily report should have CR Cost column")
}

func TestMonthlyReportContainsSessionsColumn(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp:   ts,
			DateKey:     ts.Format("2006-01-02"),
			SessionID:   "sess-1",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		},
		{
			Timestamp:   ts,
			DateKey:     ts.Format("2006-01-02"),
			SessionID:   "sess-2",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
		},
	}

	formatter := NewTableWriterFormatter(true)
	output := formatter.FormatMonthlyReport(entries)

	assert.Contains(t, output, "Sessions", "Monthly report should have Sessions column header")
}
