package calculator

import (
	"sort"
	"time"

	"github.com/sdpower/ccusage-go/internal/types"
)

const (
	// DefaultSessionDurationHours is Claude's billing block duration
	DefaultSessionDurationHours = 5
	// BlocksWarningThreshold is the percentage threshold for warnings
	BlocksWarningThreshold = 0.8 // 80%
)

// floorToHour floors a timestamp to the beginning of the hour
func floorToHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
}

// IdentifySessionBlocks groups entries into time-based blocks with gap detection
func (c *Calculator) IdentifySessionBlocks(entries []types.UsageEntry, sessionDurationHours int) []types.SessionBlock {
	if len(entries) == 0 {
		return []types.SessionBlock{}
	}

	if sessionDurationHours <= 0 {
		sessionDurationHours = DefaultSessionDurationHours
	}

	sessionDuration := time.Duration(sessionDurationHours) * time.Hour
	blocks := []types.SessionBlock{}

	// Sort entries by timestamp
	sortedEntries := make([]types.UsageEntry, len(entries))
	copy(sortedEntries, entries)
	sort.Slice(sortedEntries, func(i, j int) bool {
		return sortedEntries[i].Timestamp.Before(sortedEntries[j].Timestamp)
	})

	var currentBlockStart *time.Time
	var currentBlockEntries []types.UsageEntry
	now := time.Now()

	for _, entry := range sortedEntries {
		entryTime := entry.Timestamp

		if currentBlockStart == nil {
			// First entry - start a new block (floored to the hour)
			floored := floorToHour(entryTime)
			currentBlockStart = &floored
			currentBlockEntries = []types.UsageEntry{entry}
		} else {
			timeSinceBlockStart := entryTime.Sub(*currentBlockStart)
			lastEntry := currentBlockEntries[len(currentBlockEntries)-1]
			timeSinceLastEntry := entryTime.Sub(lastEntry.Timestamp)

			if timeSinceBlockStart > sessionDuration || timeSinceLastEntry > sessionDuration {
				// Close current block
				block := c.createBlock(*currentBlockStart, currentBlockEntries, now, sessionDuration)
				blocks = append(blocks, block)

				// Add gap block if there's a significant gap
				if timeSinceLastEntry > sessionDuration {
					gapBlock := c.createGapBlock(lastEntry.Timestamp, entryTime, sessionDuration)
					if gapBlock != nil {
						blocks = append(blocks, *gapBlock)
					}
				}

				// Start new block (floored to the hour)
				floored := floorToHour(entryTime)
				currentBlockStart = &floored
				currentBlockEntries = []types.UsageEntry{entry}
			} else {
				// Add to current block
				currentBlockEntries = append(currentBlockEntries, entry)
			}
		}
	}

	// Close the last block
	if currentBlockStart != nil && len(currentBlockEntries) > 0 {
		block := c.createBlock(*currentBlockStart, currentBlockEntries, now, sessionDuration)
		blocks = append(blocks, block)
	}

	return blocks
}

// createBlock creates a session block from a start time and usage entries
func (c *Calculator) createBlock(startTime time.Time, entries []types.UsageEntry, now time.Time, sessionDuration time.Duration) types.SessionBlock {
	endTime := startTime.Add(sessionDuration)
	var actualEndTime *time.Time
	if len(entries) > 0 {
		lastTime := entries[len(entries)-1].Timestamp
		actualEndTime = &lastTime
	}

	// Check if block is active
	isActive := false
	if actualEndTime != nil {
		timeSinceLastActivity := now.Sub(*actualEndTime)
		isActive = timeSinceLastActivity < sessionDuration && now.Before(endTime)
	}

	// Aggregate token counts
	tokenCounts := types.TokenCounts{}
	costUSD := 0.0
	modelMap := make(map[string]bool)
	var usageLimitResetTime *time.Time

	for _, entry := range entries {
		tokenCounts.InputTokens += entry.InputTokens
		tokenCounts.OutputTokens += entry.OutputTokens
		
		// Extract cache tokens from Raw data if available
		if entry.Raw != nil {
			if cc, ok := entry.Raw["cache_creation_input_tokens"].(int); ok {
				tokenCounts.CacheCreationInputTokens += cc
			}
			if cr, ok := entry.Raw["cache_read_input_tokens"].(int); ok {
				tokenCounts.CacheReadInputTokens += cr
			}
			// Check for usage limit reset time
			if resetTime, ok := entry.Raw["usage_limit_reset_time"].(string); ok {
				if t, err := time.Parse(time.RFC3339, resetTime); err == nil {
					usageLimitResetTime = &t
				}
			}
		}
		
		costUSD += entry.Cost
		if entry.Model != "" {
			modelMap[entry.Model] = true
		}
	}

	// Convert model map to sorted slice
	models := []string{}
	for model := range modelMap {
		models = append(models, model)
	}
	sort.Strings(models)

	return types.SessionBlock{
		ID:                  startTime.Format(time.RFC3339),
		StartTime:           startTime,
		EndTime:             endTime,
		ActualEndTime:       actualEndTime,
		IsActive:            isActive,
		IsGap:               false,
		Entries:             entries,
		TokenCounts:         tokenCounts,
		CostUSD:             costUSD,
		Models:              models,
		UsageLimitResetTime: usageLimitResetTime,
	}
}

// createGapBlock creates a gap block representing periods with no activity
func (c *Calculator) createGapBlock(lastActivityTime, nextActivityTime time.Time, sessionDuration time.Duration) *types.SessionBlock {
	// Only create gap blocks for gaps longer than the session duration
	gapDuration := nextActivityTime.Sub(lastActivityTime)
	if gapDuration <= sessionDuration {
		return nil
	}

	gapStart := lastActivityTime.Add(sessionDuration)
	gapEnd := nextActivityTime

	return &types.SessionBlock{
		ID:            "gap-" + gapStart.Format(time.RFC3339),
		StartTime:     gapStart,
		EndTime:       gapEnd,
		ActualEndTime: nil,
		IsActive:      false,
		IsGap:         true,
		Entries:       []types.UsageEntry{},
		TokenCounts:   types.TokenCounts{},
		CostUSD:       0,
		Models:        []string{},
	}
}

// CalculateBurnRate calculates the burn rate for a session block
func CalculateBurnRate(block types.SessionBlock) *types.BurnRate {
	if len(block.Entries) == 0 || block.IsGap {
		return nil
	}

	firstEntry := block.Entries[0].Timestamp
	lastEntry := block.Entries[len(block.Entries)-1].Timestamp
	durationMinutes := lastEntry.Sub(firstEntry).Minutes()

	if durationMinutes <= 0 {
		return nil
	}

	totalTokens := float64(block.TokenCounts.GetTotal())
	tokensPerMinute := totalTokens / durationMinutes

	// For burn rate indicator, use only input and output tokens
	nonCacheTokens := float64(block.TokenCounts.InputTokens + block.TokenCounts.OutputTokens)
	tokensPerMinuteForIndicator := nonCacheTokens / durationMinutes

	costPerHour := (block.CostUSD / durationMinutes) * 60

	return &types.BurnRate{
		TokensPerMinute:             tokensPerMinute,
		TokensPerMinuteForIndicator: tokensPerMinuteForIndicator,
		CostPerHour:                 costPerHour,
	}
}

// ProjectBlockUsage projects total usage for an active session block
func ProjectBlockUsage(block types.SessionBlock) *types.ProjectedUsage {
	if !block.IsActive || block.IsGap {
		return nil
	}

	burnRate := CalculateBurnRate(block)
	if burnRate == nil {
		return nil
	}

	now := time.Now()
	remainingTime := block.EndTime.Sub(now)
	remainingMinutes := remainingTime.Minutes()
	if remainingMinutes < 0 {
		remainingMinutes = 0
	}

	// Current tokens plus projected additional tokens
	currentTokens := block.TokenCounts.GetTotal()
	additionalTokens := int(burnRate.TokensPerMinute * remainingMinutes)
	totalTokens := currentTokens + additionalTokens

	// Current cost plus projected additional cost
	additionalCost := (burnRate.CostPerHour / 60) * remainingMinutes
	totalCost := block.CostUSD + additionalCost

	return &types.ProjectedUsage{
		TotalTokens:      totalTokens,
		TotalCost:        totalCost,
		RemainingMinutes: remainingMinutes,
	}
}

// FilterRecentBlocks filters blocks to include only those from the last N days
func FilterRecentBlocks(blocks []types.SessionBlock, days int) []types.SessionBlock {
	cutoff := time.Now().AddDate(0, 0, -days)
	filtered := []types.SessionBlock{}
	
	for _, block := range blocks {
		if block.StartTime.After(cutoff) {
			filtered = append(filtered, block)
		}
	}
	
	return filtered
}

// GetMaxTokensFromBlocks finds the maximum token count from all non-gap, inactive blocks
func GetMaxTokensFromBlocks(blocks []types.SessionBlock) int {
	maxTokens := 0
	for _, block := range blocks {
		if !block.IsGap && !block.IsActive {
			blockTokens := block.TokenCounts.GetTotal()
			if blockTokens > maxTokens {
				maxTokens = blockTokens
			}
		}
	}
	return maxTokens
}