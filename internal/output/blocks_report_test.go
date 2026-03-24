package output

import (
	"testing"
	"time"

	"github.com/sdpower/ccusage-go/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestBlocksReportContainsCostBreakdown(t *testing.T) {
	now := time.Now()
	blocks := []types.SessionBlock{
		{
			ID:        now.Add(-time.Hour).Format(time.RFC3339),
			StartTime: now.Add(-time.Hour),
			EndTime:   now.Add(4 * time.Hour),
			TokenCounts: types.TokenCounts{
				InputTokens:              100,
				OutputTokens:             50,
				CacheCreationInputTokens: 500,
				CacheReadInputTokens:     2000,
			},
			CostUSD:            4.0,
			APICostUSD:         2.5,
			CacheCreateCostUSD: 1.0,
			CacheReadCostUSD:   0.5,
			Models:             []string{"claude-sonnet-4-5-20250514"},
		},
	}

	formatter := NewTableWriterFormatter(true)
	output := formatter.FormatBlocksReport(blocks, 0)

	assert.Contains(t, output, "CC Cost", "Blocks report should have CC Cost column")
	assert.Contains(t, output, "CR Cost", "Blocks report should have CR Cost column")
	assert.Contains(t, output, "API Cost", "Blocks report should have API Cost column")
	assert.Contains(t, output, "Input", "Blocks report should have Input column")
	assert.Contains(t, output, "Output", "Blocks report should have Output column")
}
