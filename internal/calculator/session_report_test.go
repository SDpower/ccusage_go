package calculator

import (
	"context"
	"testing"
	"time"

	"github.com/sdpower/ccusage-go/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPricing implements PricingService for testing
type mockPricing struct {
	inputPrice      float64
	outputPrice     float64
	cacheCreatePrice float64
	cacheReadPrice  float64
}

func (m *mockPricing) GetModelPrice(ctx context.Context, model string) (float64, float64, float64, float64, error) {
	return m.inputPrice, m.outputPrice, m.cacheCreatePrice, m.cacheReadPrice, nil
}

func TestCalculateCostSeparatesAPICost(t *testing.T) {
	pricing := &mockPricing{
		inputPrice:       0.01,  // $0.01 per token
		outputPrice:      0.03,  // $0.03 per token
		cacheCreatePrice: 0.005, // $0.005 per token
		cacheReadPrice:   0.001, // $0.001 per token
	}
	calc := New(pricing)

	entries := []types.UsageEntry{
		{
			Model:        "claude-sonnet-4-5-20250514",
			InputTokens:  100,
			OutputTokens: 50,
			Raw: map[string]interface{}{
				"cache_creation_input_tokens": 200,
				"cache_read_input_tokens":     500,
			},
		},
	}

	result, err := calc.CalculateCosts(context.Background(), entries)
	require.NoError(t, err)
	require.Len(t, result, 1)

	// APICost = input*0.01 + output*0.03 = 100*0.01 + 50*0.03 = 1.0 + 1.5 = 2.5
	assert.InDelta(t, 2.5, result[0].APICost, 0.001, "APICost should only include input + output")

	// Cost = APICost + cache_create*0.005 + cache_read*0.001 = 2.5 + 200*0.005 + 500*0.001 = 2.5 + 1.0 + 0.5 = 4.0
	assert.InDelta(t, 4.0, result[0].Cost, 0.001, "Cost should include all tokens including cache")

	// CacheCreateCost = 200 * 0.005 = 1.0
	assert.InDelta(t, 1.0, result[0].CacheCreateCost, 0.001, "CacheCreateCost should be cache_create * cacheCreatePrice")
	// CacheReadCost = 500 * 0.001 = 0.5
	assert.InDelta(t, 0.5, result[0].CacheReadCost, 0.001, "CacheReadCost should be cache_read * cacheReadPrice")
}

func TestGenerateSessionReportAPICost(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp: ts, ProjectPath: "/project/a", SessionID: "s1",
			Model: "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			Cost: 4.0, APICost: 2.5,
		},
		{
			Timestamp: ts.Add(time.Minute), ProjectPath: "/project/a", SessionID: "s1",
			Model: "claude-sonnet-4-5-20250514",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
			Cost: 8.0, APICost: 5.0,
		},
	}

	calc := New(nil)
	sessions := calc.GenerateSessionReport(entries)

	require.Len(t, sessions, 1)
	assert.InDelta(t, 7.5, sessions[0].TotalAPICost, 0.001)
	assert.InDelta(t, 12.0, sessions[0].TotalCost, 0.001)
}

func TestAggregateBySourceFileAPICost(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp: ts, SourceFile: "/data/main.jsonl",
			Model: "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			Cost: 4.0, APICost: 2.5,
		},
		{
			Timestamp: ts.Add(time.Minute), SourceFile: "/data/main.jsonl",
			Model: "claude-sonnet-4-5-20250514",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
			Cost: 8.0, APICost: 5.0,
		},
	}

	calc := New(nil)
	stats := calc.AggregateBySourceFile(entries)

	require.Len(t, stats, 1)
	assert.InDelta(t, 7.5, stats[0].APICost, 0.001)
	assert.InDelta(t, 12.0, stats[0].Cost, 0.001)
}

func TestGenerateSessionReportWithSessionName(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp:   ts,
			ProjectPath: "/project/alpha",
			SessionID:   "sess-1",
			SessionName: "feature-login",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		},
		{
			Timestamp:   ts.Add(time.Minute),
			ProjectPath: "/project/alpha",
			SessionID:   "sess-1",
			SessionName: "feature-login",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
		},
	}

	calc := New(nil) // PricingService not needed for GenerateSessionReport
	sessions := calc.GenerateSessionReport(entries)

	require.Len(t, sessions, 1)
	assert.Equal(t, "feature-login", sessions[0].SessionName, "SessionName should be populated from entries")
	assert.Equal(t, "/project/alpha", sessions[0].ProjectPath)
	assert.Equal(t, 2, sessions[0].RequestCount)
	assert.Equal(t, 450, sessions[0].TotalTokens)
}

func TestGenerateSessionReportMultipleSessionNames(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp:   ts,
			ProjectPath: "/project/alpha",
			SessionID:   "sess-1",
			SessionName: "feature-login",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		},
		{
			Timestamp:   ts.Add(time.Minute),
			ProjectPath: "/project/beta",
			SessionID:   "sess-2",
			SessionName: "bugfix-auth",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
		},
	}

	calc := New(nil)
	sessions := calc.GenerateSessionReport(entries)

	require.Len(t, sessions, 2)

	// Sessions sorted by start time
	nameMap := make(map[string]string)
	for _, s := range sessions {
		nameMap[s.ProjectPath] = s.SessionName
	}
	assert.Equal(t, "feature-login", nameMap["/project/alpha"])
	assert.Equal(t, "bugfix-auth", nameMap["/project/beta"])
}

func TestGenerateSessionReportNoSessionName(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp:   ts,
			ProjectPath: "/project/alpha",
			SessionID:   "sess-1",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		},
	}

	calc := New(nil)
	sessions := calc.GenerateSessionReport(entries)

	require.Len(t, sessions, 1)
	assert.Empty(t, sessions[0].SessionName, "SessionName should be empty when entries have no SessionName")
}

func TestGenerateSessionReportCollectsSessionIDs(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp:   ts,
			ProjectPath: "/project/alpha",
			SessionID:   "aaaaaaaa-1111-2222-3333-444444444444",
			SessionName: "feature-x",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		},
		{
			Timestamp:   ts.Add(time.Minute),
			ProjectPath: "/project/alpha",
			SessionID:   "bbbbbbbb-1111-2222-3333-444444444444",
			SessionName: "feature-x",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
		},
		{
			Timestamp:   ts.Add(2 * time.Minute),
			ProjectPath: "/project/alpha",
			SessionID:   "aaaaaaaa-1111-2222-3333-444444444444", // duplicate
			SessionName: "feature-x",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 50, OutputTokens: 25, TotalTokens: 75,
		},
	}

	calc := New(nil)
	sessions := calc.GenerateSessionReport(entries)

	require.Len(t, sessions, 1)
	assert.Equal(t, []string{
		"aaaaaaaa-1111-2222-3333-444444444444",
		"bbbbbbbb-1111-2222-3333-444444444444",
	}, sessions[0].SessionIDs, "SessionIDs should contain unique UUIDs sorted")
}

func TestGenerateSessionReportCollectsSourceFiles(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp:   ts,
			ProjectPath: "/project/alpha",
			SessionID:   "sess-1",
			SourceFile:  "/data/projects/alpha/sess-1.jsonl",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		},
		{
			Timestamp:   ts.Add(time.Minute),
			ProjectPath: "/project/alpha",
			SessionID:   "sess-1",
			SourceFile:  "/data/projects/alpha/sess-1/subagents/agent-abc.jsonl",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
		},
		{
			Timestamp:   ts.Add(2 * time.Minute),
			ProjectPath: "/project/alpha",
			SessionID:   "sess-1",
			SourceFile:  "/data/projects/alpha/sess-1.jsonl", // duplicate
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 50, OutputTokens: 25, TotalTokens: 75,
		},
	}

	calc := New(nil)
	sessions := calc.GenerateSessionReport(entries)

	require.Len(t, sessions, 1)
	assert.Equal(t, []string{
		"/data/projects/alpha/sess-1.jsonl",
		"/data/projects/alpha/sess-1/subagents/agent-abc.jsonl",
	}, sessions[0].SourceFiles, "SourceFiles should contain unique file paths sorted")
}

func TestGenerateSessionReportCacheCosts(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp: ts, ProjectPath: "/project/a", SessionID: "s1",
			Model: "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			Cost: 4.0, APICost: 2.5, CacheCreateCost: 1.0, CacheReadCost: 0.5,
		},
		{
			Timestamp: ts.Add(time.Minute), ProjectPath: "/project/a", SessionID: "s1",
			Model: "claude-sonnet-4-5-20250514",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
			Cost: 8.0, APICost: 5.0, CacheCreateCost: 1.0, CacheReadCost: 0.5,
		},
	}

	calc := New(nil)
	sessions := calc.GenerateSessionReport(entries)

	require.Len(t, sessions, 1)
	assert.InDelta(t, 2.0, sessions[0].CacheCreateCost, 0.001, "CacheCreateCost should be sum of entries")
	assert.InDelta(t, 1.0, sessions[0].CacheReadCost, 0.001, "CacheReadCost should be sum of entries")
}

func TestAggregateBySourceFileCacheCosts(t *testing.T) {
	ts := time.Now()
	entries := []types.UsageEntry{
		{
			Timestamp: ts, SourceFile: "/data/main.jsonl",
			Model: "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			Cost: 4.0, APICost: 2.5, CacheCreateCost: 1.0, CacheReadCost: 0.5,
			Raw: map[string]interface{}{"cache_creation_input_tokens": 200, "cache_read_input_tokens": 500},
		},
		{
			Timestamp: ts.Add(time.Minute), SourceFile: "/data/main.jsonl",
			Model: "claude-sonnet-4-5-20250514",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
			Cost: 8.0, APICost: 5.0, CacheCreateCost: 1.0, CacheReadCost: 0.5,
			Raw: map[string]interface{}{"cache_creation_input_tokens": 200, "cache_read_input_tokens": 500},
		},
	}

	calc := New(nil)
	stats := calc.AggregateBySourceFile(entries)

	require.Len(t, stats, 1)
	assert.InDelta(t, 2.0, stats[0].CacheCreateCost, 0.001, "CacheCreateCost should be sum of entries")
	assert.InDelta(t, 1.0, stats[0].CacheReadCost, 0.001, "CacheReadCost should be sum of entries")
}

func TestAggregateBySourceFile(t *testing.T) {
	ts := time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC)
	entries := []types.UsageEntry{
		{
			Timestamp:   ts,
			SourceFile:  "/data/main.jsonl",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			Cost: 1.0,
			Raw:  map[string]interface{}{"cache_creation_input_tokens": 500, "cache_read_input_tokens": 2000},
		},
		{
			Timestamp:   ts.Add(time.Minute),
			SourceFile:  "/data/main.jsonl",
			Model:       "claude-opus-4-6-20250514",
			InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
			Cost: 2.0,
			Raw:  map[string]interface{}{"cache_creation_input_tokens": 1000, "cache_read_input_tokens": 3000},
		},
		{
			Timestamp:   ts.Add(2 * time.Minute),
			SourceFile:  "/data/subagents/agent-abc.jsonl",
			Model:       "claude-sonnet-4-5-20250514",
			InputTokens: 50, OutputTokens: 25, TotalTokens: 75,
			Cost: 0.5,
			Raw:  map[string]interface{}{"cache_creation_input_tokens": 200, "cache_read_input_tokens": 800},
		},
	}

	calc := New(nil)
	stats := calc.AggregateBySourceFile(entries)

	require.Len(t, stats, 2)

	// Sorted by file path
	assert.Equal(t, "/data/main.jsonl", stats[0].FilePath)
	assert.Equal(t, 300, stats[0].InputTokens)
	assert.Equal(t, 150, stats[0].OutputTokens)
	assert.Equal(t, 1500, stats[0].CacheCreateTokens)
	assert.Equal(t, 5000, stats[0].CacheReadTokens)
	assert.Equal(t, 3.0, stats[0].Cost)
	assert.Equal(t, 2, stats[0].EntryCount)
	assert.Len(t, stats[0].ModelsUsed, 2)

	assert.Equal(t, "/data/subagents/agent-abc.jsonl", stats[1].FilePath)
	assert.Equal(t, 50, stats[1].InputTokens)
	assert.Equal(t, 25, stats[1].OutputTokens)
	assert.Equal(t, 0.5, stats[1].Cost)
	assert.Equal(t, 1, stats[1].EntryCount)
}
