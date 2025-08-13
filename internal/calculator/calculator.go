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
	GetModelPrice(ctx context.Context, model string) (inputPrice, outputPrice float64, err error)
}

func New(pricingService PricingService) *Calculator {
	return &Calculator{
		pricingService: pricingService,
	}
}

func (c *Calculator) CalculateCosts(ctx context.Context, entries []types.UsageEntry) ([]types.UsageEntry, error) {
	for i := range entries {
		if entries[i].Cost == 0 {
			inputPrice, outputPrice, err := c.pricingService.GetModelPrice(ctx, entries[i].Model)
			if err != nil {
				// Continue without cost if pricing fails
				continue
			}

			cost := float64(entries[i].InputTokens)*inputPrice/1000 +
				float64(entries[i].OutputTokens)*outputPrice/1000
			entries[i].Cost = cost
		}
	}
	return entries, nil
}

func (c *Calculator) GenerateDailyReport(entries []types.UsageEntry, date time.Time) types.UsageReport {
	filteredEntries := c.filterByDate(entries, date)
	return c.generateReport(filteredEntries, "daily", date, date.Add(24*time.Hour))
}

func (c *Calculator) GenerateMonthlyReport(entries []types.UsageEntry, year int, month int) types.UsageReport {
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

	for _, entry := range entries {
		sessionMap[entry.SessionID] = append(sessionMap[entry.SessionID], entry)
	}

	var sessions []types.SessionInfo
	for sessionID, sessionEntries := range sessionMap {
		if len(sessionEntries) == 0 {
			continue
		}

		sort.Slice(sessionEntries, func(i, j int) bool {
			return sessionEntries[i].Timestamp.Before(sessionEntries[j].Timestamp)
		})

		session := types.SessionInfo{
			SessionID:    sessionID,
			StartTime:    sessionEntries[0].Timestamp,
			EndTime:      sessionEntries[len(sessionEntries)-1].Timestamp,
			RequestCount: len(sessionEntries),
			ProjectPath:  sessionEntries[0].ProjectPath,
		}

		session.Duration = session.EndTime.Sub(session.StartTime)

		for _, entry := range sessionEntries {
			session.TotalCost += entry.Cost
			session.TotalTokens += entry.TotalTokens
		}

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
		if entry.Timestamp.After(start) && entry.Timestamp.Before(end) {
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

		summary.Models[entry.Model]++
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
