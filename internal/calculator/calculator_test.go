package calculator

import (
	"context"
	"testing"
	"time"

	"github.com/sdpower/ccusage-go/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubPricing returns fixed prices for testing. Web prices come into play in
// the server_tool_use stage; cache prices verify the first-class field path.
type stubPricing struct {
	in, out, cc, cr, web, fetch float64
}

func (s *stubPricing) GetModelPrice(_ context.Context, _ string) (float64, float64, float64, float64, float64, float64, error) {
	return s.in, s.out, s.cc, s.cr, s.web, s.fetch, nil
}

func TestCalculateCost_UsesFirstClassCacheFields(t *testing.T) {
	calc := New(&stubPricing{
		in: 0.000003, out: 0.000015, cc: 0.0000037, cr: 0.0000003,
	})

	entry := types.UsageEntry{
		Model:                    "claude-opus-4-20250514",
		InputTokens:              1000,
		OutputTokens:             500,
		CacheCreationInputTokens: 2000,
		CacheReadInputTokens:     10000,
	}

	require.NoError(t, calc.CalculateCost(&entry))

	expectedAPI := 1000*0.000003 + 500*0.000015
	expectedCC := 2000 * 0.0000037
	expectedCR := 10000 * 0.0000003
	assert.InDelta(t, expectedAPI, entry.APICost, 1e-9, "APICost must equal input+output prices")
	assert.InDelta(t, expectedCC, entry.CacheCreateCost, 1e-9)
	assert.InDelta(t, expectedCR, entry.CacheReadCost, 1e-9)
	assert.InDelta(t, expectedAPI+expectedCC+expectedCR, entry.Cost, 1e-9, "Cost = API + cache costs")
}

func TestCalculateCost_IncludesServerToolUse(t *testing.T) {
	calc := New(&stubPricing{
		in: 0.000003, out: 0.000015,
		web: 0.01, fetch: 0.005,
	})

	entry := types.UsageEntry{
		Model:             "claude-opus-4-20250514",
		InputTokens:       100,
		OutputTokens:      50,
		WebSearchRequests: 3,
		WebFetchRequests:  2,
	}

	require.NoError(t, calc.CalculateCost(&entry))

	expectedAPI := 100*0.000003 + 50*0.000015
	expectedWeb := 3 * 0.01
	expectedFetch := 2 * 0.005

	assert.InDelta(t, expectedAPI, entry.APICost, 1e-9, "APICost must NOT include server_tool_use")
	assert.InDelta(t, expectedWeb, entry.WebSearchCost, 1e-9)
	assert.InDelta(t, expectedFetch, entry.WebFetchCost, 1e-9)
	assert.InDelta(t, expectedAPI+expectedWeb+expectedFetch, entry.Cost, 1e-9,
		"Cost must include server_tool_use charges")
}

func TestAggregateBySession_FirstClassCacheFields(t *testing.T) {
	calc := New(&stubPricing{})
	now := time.Now()

	entries := []types.UsageEntry{
		{
			SessionID:                "s1",
			Timestamp:                now,
			Model:                    "claude-opus-4-20250514",
			InputTokens:              100,
			OutputTokens:             50,
			CacheCreationInputTokens: 200,
			CacheReadInputTokens:     400,
		},
		{
			SessionID:                "s1",
			Timestamp:                now.Add(time.Minute),
			Model:                    "claude-opus-4-20250514",
			InputTokens:              10,
			OutputTokens:             5,
			CacheCreationInputTokens: 20,
			CacheReadInputTokens:     40,
		},
	}

	sessions := calc.GenerateSessionReport(entries)
	require.Len(t, sessions, 1)
	assert.Equal(t, 220, sessions[0].CacheCreationTokens)
	assert.Equal(t, 440, sessions[0].CacheReadTokens)
}
