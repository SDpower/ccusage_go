package calculator

import (
	"context"
	"sort"
	"time"

	"github.com/sdpower/ccusage-go/internal/types"
)

type Calculator struct {
	pricingService PricingService
}

type PricingService interface {
	GetModelPrice(ctx context.Context, model string) (inputPrice, outputPrice, cacheCreatePrice, cacheReadPrice float64, err error)
}

func New(pricingService PricingService) *Calculator {
	return &Calculator{
		pricingService: pricingService,
	}
}

func (c *Calculator) CalculateCosts(ctx context.Context, entries []types.UsageEntry) ([]types.UsageEntry, error) {
	for i := range entries {
		if entries[i].Cost == 0 {
			c.calculateSingleCost(ctx, &entries[i])
		}
	}
	return entries, nil
}

// CalculateCost implements the loader.CostCalculator interface for stream processing
func (c *Calculator) CalculateCost(entry *types.UsageEntry) error {
	if entry.Cost == 0 {
		c.calculateSingleCost(context.Background(), entry)
	}
	return nil
}

// calculateSingleCost calculates cost for a single entry
func (c *Calculator) calculateSingleCost(ctx context.Context, entry *types.UsageEntry) {
	inputPrice, outputPrice, cacheCreatePrice, cacheReadPrice, err := c.pricingService.GetModelPrice(ctx, entry.Model)
	if err != nil {
		// Continue without cost if pricing fails
		return
	}

	// Calculate cost using per-token pricing (not per-1000 tokens)
	cost := float64(entry.InputTokens)*inputPrice +
		float64(entry.OutputTokens)*outputPrice
	
	// Add cache token costs if present
	if entry.Raw != nil {
		if cacheCreate, ok := entry.Raw["cache_creation_input_tokens"].(int); ok {
			cost += float64(cacheCreate) * cacheCreatePrice
		}
		if cacheRead, ok := entry.Raw["cache_read_input_tokens"].(int); ok {
			cost += float64(cacheRead) * cacheReadPrice
		}
	}
	
	entry.Cost = cost
}

func (c *Calculator) GenerateDailyReport(entries []types.UsageEntry, date time.Time) types.UsageReport {
	filteredEntries := c.filterByDate(entries, date)
	return c.generateReport(filteredEntries, "daily", date, date.Add(24*time.Hour))
}

func (c *Calculator) GenerateMonthlyReport(entries []types.UsageEntry, year int, month int) types.UsageReport {
	// Note: Since timezone conversion is now handled at the loader level via DateKey,
	// this method is primarily used for JSON/CSV output formats.
	// For table format, entries are already timezone-converted with DateKey set.
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	filteredEntries := c.filterByDateRange(entries, start, end)
	return c.generateReport(filteredEntries, "monthly", start, end)
}

func (c *Calculator) GenerateWeeklyReport(entries []types.UsageEntry, year int, week int) types.UsageReport {
	start := c.getWeekStart(year, week)
	end := start.Add(7 * 24 * time.Hour)

	filteredEntries := c.filterByDateRange(entries, start, end)
	return c.generateReport(filteredEntries, "weekly", start, end)
}

func (c *Calculator) GenerateSessionReport(entries []types.UsageEntry) []types.SessionInfo {
	sessionMap := make(map[string][]types.UsageEntry)

	// Group by project path instead of session ID (like TypeScript version)
	for _, entry := range entries {
		// Use project path as the grouping key
		projectKey := entry.ProjectPath
		if projectKey == "" {
			projectKey = "unknown"
		}
		sessionMap[projectKey] = append(sessionMap[projectKey], entry)
	}

	var sessions []types.SessionInfo
	for projectPath, sessionEntries := range sessionMap {
		if len(sessionEntries) == 0 {
			continue
		}

		sort.Slice(sessionEntries, func(i, j int) bool {
			return sessionEntries[i].Timestamp.Before(sessionEntries[j].Timestamp)
		})

		session := types.SessionInfo{
			SessionID:    projectPath, // Use project path as session ID for display
			StartTime:    sessionEntries[0].Timestamp,
			EndTime:      sessionEntries[len(sessionEntries)-1].Timestamp,
			RequestCount: len(sessionEntries),
			ProjectPath:  projectPath,
			LastActivity: sessionEntries[len(sessionEntries)-1].Timestamp, // Use last entry timestamp
		}

		session.Duration = session.EndTime.Sub(session.StartTime)
		
		// Track unique models
		modelSet := make(map[string]bool)

		for _, entry := range sessionEntries {
			session.TotalCost += entry.Cost
			session.TotalTokens += entry.TotalTokens
			session.InputTokens += entry.InputTokens
			session.OutputTokens += entry.OutputTokens
			
			// Track models (exclude synthetic)
			if entry.Model != "" && entry.Model != "<synthetic>" {
				modelSet[entry.Model] = true
			}
			
			// Extract cache tokens from Raw data
			if entry.Raw != nil {
				if cc, ok := entry.Raw["cache_creation_input_tokens"].(int); ok {
					session.CacheCreationTokens += cc
				}
				if cr, ok := entry.Raw["cache_read_input_tokens"].(int); ok {
					session.CacheReadTokens += cr
				}
			}
		}
		
		// Convert model set to sorted slice
		for model := range modelSet {
			session.ModelsUsed = append(session.ModelsUsed, model)
		}
		sort.Strings(session.ModelsUsed)

		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.Before(sessions[j].StartTime)
	})

	return sessions
}

func (c *Calculator) GenerateBlocksReport(entries []types.UsageEntry) []types.BlockInfo {
	blockMap := make(map[string]*types.BlockInfo)

	for _, entry := range entries {
		if entry.BlockType == "" {
			continue
		}

		if block, exists := blockMap[entry.BlockType]; exists {
			block.Count++
			block.TotalTokens += entry.TotalTokens
			block.TotalCost += entry.Cost

			if entry.Timestamp.Before(block.FirstSeen) {
				block.FirstSeen = entry.Timestamp
			}
			if entry.Timestamp.After(block.LastSeen) {
				block.LastSeen = entry.Timestamp
			}
		} else {
			blockMap[entry.BlockType] = &types.BlockInfo{
				BlockType:   entry.BlockType,
				Count:       1,
				TotalTokens: entry.TotalTokens,
				TotalCost:   entry.Cost,
				FirstSeen:   entry.Timestamp,
				LastSeen:    entry.Timestamp,
			}
		}
	}

	var blocks []types.BlockInfo
	for _, block := range blockMap {
		blocks = append(blocks, *block)
	}

	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Count > blocks[j].Count
	})

	return blocks
}

func (c *Calculator) filterByDate(entries []types.UsageEntry, date time.Time) []types.UsageEntry {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.Add(24 * time.Hour)
	return c.filterByDateRange(entries, start, end)
}

func (c *Calculator) filterByDateRange(entries []types.UsageEntry, start, end time.Time) []types.UsageEntry {
	var filtered []types.UsageEntry
	for _, entry := range entries {
		// Include entries that are >= start and < end
		// This ensures we don't miss entries exactly at the start time
		if (entry.Timestamp.Equal(start) || entry.Timestamp.After(start)) && entry.Timestamp.Before(end) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (c *Calculator) generateReport(entries []types.UsageEntry, period string, start, end time.Time) types.UsageReport {
	summary := c.calculateSummary(entries)

	return types.UsageReport{
		Period:      period,
		StartTime:   start,
		EndTime:     end,
		TotalCost:   summary.TotalCost,
		TotalTokens: summary.TotalTokens,
		Entries:     entries,
		Summary:     summary,
	}
}

func (c *Calculator) calculateSummary(entries []types.UsageEntry) types.UsageSummary {
	summary := types.UsageSummary{
		Models:   make(map[string]int),
		Projects: make(map[string]int),
	}

	for _, entry := range entries {
		summary.TotalRequests++
		summary.TotalCost += entry.Cost
		summary.TotalTokens += entry.TotalTokens
		summary.InputTokens += entry.InputTokens
		summary.OutputTokens += entry.OutputTokens

		// Skip synthetic model in statistics
		if entry.Model != "<synthetic>" {
			summary.Models[entry.Model]++
		}
		summary.Projects[entry.ProjectPath]++
	}

	if summary.TotalRequests > 0 {
		summary.AverageCost = summary.TotalCost / float64(summary.TotalRequests)
	}

	return summary
}

func (c *Calculator) getWeekStart(year, week int) time.Time {
	jan1 := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)

	// Find first Monday
	daysToMonday := (8 - int(jan1.Weekday())) % 7
	firstMonday := jan1.AddDate(0, 0, daysToMonday)

	// Week 1 starts on first Monday
	weekStart := firstMonday.AddDate(0, 0, (week-1)*7)

	return weekStart
}
