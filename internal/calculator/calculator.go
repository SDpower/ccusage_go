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

	// Calculate API cost (input + output only, no cache)
	apiCost := float64(entry.InputTokens)*inputPrice +
		float64(entry.OutputTokens)*outputPrice
	entry.APICost = apiCost

	// Calculate total cost including cache tokens
	cost := apiCost
	if entry.Raw != nil {
		if cacheCreate, ok := entry.Raw["cache_creation_input_tokens"].(int); ok {
			entry.CacheCreateCost = float64(cacheCreate) * cacheCreatePrice
			cost += entry.CacheCreateCost
		}
		if cacheRead, ok := entry.Raw["cache_read_input_tokens"].(int); ok {
			entry.CacheReadCost = float64(cacheRead) * cacheReadPrice
			cost += entry.CacheReadCost
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
		
		// Track unique models, session IDs, and source files
		modelSet := make(map[string]bool)
		sessionIDSet := make(map[string]bool)
		sourceFileSet := make(map[string]bool)

		for _, entry := range sessionEntries {
			session.TotalCost += entry.Cost
			session.TotalAPICost += entry.APICost
			session.CacheCreateCost += entry.CacheCreateCost
			session.CacheReadCost += entry.CacheReadCost
			session.TotalTokens += entry.TotalTokens
			session.InputTokens += entry.InputTokens
			session.OutputTokens += entry.OutputTokens

			// Collect session name from first entry that has one
			if session.SessionName == "" && entry.SessionName != "" {
				session.SessionName = entry.SessionName
			}

			// Track unique session IDs
			if entry.SessionID != "" {
				sessionIDSet[entry.SessionID] = true
			}

			// Track unique source files
			if entry.SourceFile != "" {
				sourceFileSet[entry.SourceFile] = true
			}

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

		// Convert session ID set to sorted slice
		for sid := range sessionIDSet {
			session.SessionIDs = append(session.SessionIDs, sid)
		}
		sort.Strings(session.SessionIDs)

		// Convert source file set to sorted slice
		for sf := range sourceFileSet {
			session.SourceFiles = append(session.SourceFiles, sf)
		}
		sort.Strings(session.SourceFiles)

		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.Before(sessions[j].StartTime)
	})

	return sessions
}

func (c *Calculator) AggregateBySourceFile(entries []types.UsageEntry) []types.SourceFileStat {
	fileMap := make(map[string]*types.SourceFileStat)

	for _, entry := range entries {
		if entry.SourceFile == "" {
			continue
		}

		stat, exists := fileMap[entry.SourceFile]
		if !exists {
			stat = &types.SourceFileStat{FilePath: entry.SourceFile}
			fileMap[entry.SourceFile] = stat
		}

		stat.InputTokens += entry.InputTokens
		stat.OutputTokens += entry.OutputTokens
		stat.Cost += entry.Cost
		stat.APICost += entry.APICost
		stat.CacheCreateCost += entry.CacheCreateCost
		stat.CacheReadCost += entry.CacheReadCost
		stat.EntryCount++

		// Cache tokens from Raw
		if entry.Raw != nil {
			if cc, ok := entry.Raw["cache_creation_input_tokens"].(int); ok {
				stat.CacheCreateTokens += cc
			}
			if cr, ok := entry.Raw["cache_read_input_tokens"].(int); ok {
				stat.CacheReadTokens += cr
			}
		}

		// Total tokens
		stat.TotalTokens = stat.InputTokens + stat.OutputTokens + stat.CacheCreateTokens + stat.CacheReadTokens

		// Track models
		if entry.Model != "" && entry.Model != "<synthetic>" {
			found := false
			for _, m := range stat.ModelsUsed {
				if m == entry.Model {
					found = true
					break
				}
			}
			if !found {
				stat.ModelsUsed = append(stat.ModelsUsed, entry.Model)
			}
		}

		// Track last activity
		if entry.Timestamp.After(stat.LastActivity) {
			stat.LastActivity = entry.Timestamp
		}
	}

	var stats []types.SourceFileStat
	for _, stat := range fileMap {
		sort.Strings(stat.ModelsUsed)
		stats = append(stats, *stat)
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].FilePath < stats[j].FilePath
	})
	return stats
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
