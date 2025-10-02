package output

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/sdpower/ccusage-go/internal/calculator"
	"github.com/sdpower/ccusage-go/internal/types"
)

// TableWriterFormatter uses tablewriter for better table formatting
type TableWriterFormatter struct {
	noColor  bool
	timezone *time.Location
}

func NewTableWriterFormatter(noColor bool) *TableWriterFormatter {
	return &TableWriterFormatter{
		noColor:  noColor,
		timezone: time.Local, // Default to local timezone
	}
}

// formatNumberWithCommas formats a number with thousand separators
func formatNumberWithCommas(n int) string {
	if n < 0 {
		return "-" + formatNumberWithCommas(-n)
	}
	if n < 1000 {
		return strconv.Itoa(n)
	}
	return formatNumberWithCommas(n/1000) + "," + fmt.Sprintf("%03d", n%1000)
}

func (f *TableWriterFormatter) SetTimezone(loc *time.Location) {
	if loc != nil {
		f.timezone = loc
	}
}

func (f *TableWriterFormatter) FormatDailyReport(entries []types.UsageEntry) string {
	return f.FormatDailyReportWithFilter(entries, "", "")
}

func (f *TableWriterFormatter) FormatMonthlyReport(entries []types.UsageEntry) string {
	return f.FormatMonthlyReportWithFilter(entries, "", "")
}

func (f *TableWriterFormatter) FormatDailyReportWithFilter(entries []types.UsageEntry, since, until string) string {
	// Group entries by date
	dailyGroups := f.groupByDate(entries)
	
	if len(dailyGroups) == 0 {
		return f.formatEmptyReport()
	}

	var output strings.Builder
	
	// Title - use default white color
	output.WriteString("\n")
	output.WriteString(" ╭────────────────────────────────────────────────────╮")
	output.WriteString("\n")
	output.WriteString(" │                                                    │")
	output.WriteString("\n")
	output.WriteString(" │  Claude Code Token Usage Report - Daily (WITH GO)  │")
	output.WriteString("\n")
	output.WriteString(" │                                                    │")
	output.WriteString("\n")
	output.WriteString(" ╰────────────────────────────────────────────────────╯")
	output.WriteString("\n\n")

	// Create table buffer
	var buf bytes.Buffer
	
	// Create table with tablewriter v1.0.9 API
	table := tablewriter.NewTable(&buf,
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Settings: tw.Settings{Separators: tw.Separators{BetweenRows: tw.On}},
		})),
		tablewriter.WithConfig(tablewriter.Config{
			Row: tw.CellConfig{
				Alignment: tw.CellAlignment{Global: tw.AlignRight},
			},
		}),
		tablewriter.WithHeaderAutoFormat(tw.Off), // Disable auto uppercase
	)
	
	// Set headers with multi-line support
	table.Header([]string{
		"Date\n",
		"Models\n",
		"Input\n",
		"Output\n",
		"Cache\nCreate",
		"Cache\nRead",
		"Total\nTokens",
		"Cost\n(USD)",
	})
	
	// Sort dates
	var dates []string
	for date := range dailyGroups {
		// Apply date filter if specified (convert YYYY-MM-DD to YYYYMMDD for comparison)
		dateForComparison := strings.ReplaceAll(date, "-", "")
		if since != "" && dateForComparison < since {
			continue
		}
		if until != "" && dateForComparison > until {
			continue
		}
		dates = append(dates, date)
	}
	sort.Strings(dates)
	
	var totalInput, totalOutput, totalCache, totalCacheRead, totalTokens int
	var totalCost float64
	
	// Process each date
	for _, date := range dates {
		group := dailyGroups[date]
		
		// Calculate aggregates for this date
		var input, outputTokens, cache, cacheRead, tokens int
		var cost float64
		models := make(map[string]bool)
		
		for _, entry := range group {
			input += entry.InputTokens
			outputTokens += entry.OutputTokens
			cost += entry.Cost
			
			// Skip synthetic model in display (but still count its tokens/cost)
			if entry.Model != "" && entry.Model != "<synthetic>" {
				models[entry.Model] = true
			}
			
			// Get cache values from Raw
			if cc, ok := entry.Raw["cache_creation_input_tokens"].(int); ok {
				cache += cc
			}
			if cr, ok := entry.Raw["cache_read_input_tokens"].(int); ok {
				cacheRead += cr
			}
		}
		
		// Calculate total tokens including cache (matches TypeScript's getTotalTokens)
		tokens = input + outputTokens + cache + cacheRead
		
		totalInput += input
		totalOutput += outputTokens
		totalCache += cache
		totalCacheRead += cacheRead
		totalTokens += tokens
		totalCost += cost
		
		// Format models list
		var modelList []string
		for model := range models {
			shortModel := ShortenModelName(model)
			modelList = append(modelList, shortModel)
		}
		sort.Strings(modelList)
		
		// Format date as YYYY\nMM-DD
		dateParts := strings.Split(date, "-")
		formattedDate := date
		if len(dateParts) == 3 {
			formattedDate = fmt.Sprintf("%s\n%s-%s", dateParts[0], dateParts[1], dateParts[2])
		}
		
		// Format models with bullet points on separate lines
		modelsStr := ""
		if len(modelList) > 0 {
			for j, model := range modelList {
				if j > 0 {
					modelsStr += "\n"
				}
				modelsStr += "- " + model
			}
		} else {
			modelsStr = "-"
		}
		
		// Add row to table
		table.Append([]string{
			formattedDate,
			modelsStr,
			f.formatLargeNumber(input),
			f.formatLargeNumber(outputTokens),
			f.formatLargeNumber(cache),
			f.formatLargeNumber(cacheRead),
			f.formatLargeNumber(tokens),
			fmt.Sprintf("$%.2f", cost),
		})
	}
	
	// Set footer
	table.Footer([]string{
		"Total",
		"",
		f.formatLargeNumber(totalInput),
		f.formatLargeNumber(totalOutput),
		f.formatLargeNumber(totalCache),
		f.formatLargeNumber(totalCacheRead),
		f.formatLargeNumber(totalTokens),
		fmt.Sprintf("$%.2f", totalCost),
	})
	
	// Render table
	table.Render()
	
	// Apply color styling if enabled
	tableOutput := buf.String()
	if !f.noColor {
		// Apply colors to table elements
		gray := "\033[90m"     // Gray color for borders
		cyan := "\033[36m"     // Cyan color for headers
		yellow := "\033[33m"   // Yellow color for Total row
		reset := "\033[0m"     // Reset color
		
		lines := strings.Split(tableOutput, "\n")
		var coloredOutput strings.Builder
		
		for i, line := range lines {
			if line == "" {
				coloredOutput.WriteString("\n")
				continue
			}
			
			// Check if this is a pure border line (no data)
			if strings.HasPrefix(line, "┌") || strings.HasPrefix(line, "├") || strings.HasPrefix(line, "└") {
				// Pure border line - all gray
				coloredOutput.WriteString(gray + line + reset)
			} else if strings.Contains(line, "│") {
				// Line with data and borders
				parts := strings.Split(line, "│")
				for j, part := range parts {
					if j > 0 {
						coloredOutput.WriteString(gray + "│" + reset)
					}
					
					// Check content type
					if i <= 2 && strings.TrimSpace(part) != "" {
						// Header rows - use cyan
						coloredOutput.WriteString(cyan + part + reset)
					} else if strings.Contains(part, "Total") || (strings.Contains(line, "Total") && strings.TrimSpace(part) != "") {
						// Total row - use yellow for all content
						coloredOutput.WriteString(yellow + part + reset)
					} else {
						// Regular data - use default color (white)
						coloredOutput.WriteString(part)
					}
				}
			} else {
				// Other lines
				coloredOutput.WriteString(line)
			}
			
			if i < len(lines)-1 {
				coloredOutput.WriteString("\n")
			}
		}
		
		output.WriteString(coloredOutput.String())
	} else {
		output.WriteString(tableOutput)
	}
	
	return output.String()
}

func (f *TableWriterFormatter) groupByDate(entries []types.UsageEntry) map[string][]types.UsageEntry {
	groups := make(map[string][]types.UsageEntry)
	
	for _, entry := range entries {
		// Skip invalid timestamps
		if entry.Timestamp.IsZero() || entry.Timestamp.Year() < 2020 {
			continue
		}
		
		// Use pre-computed DateKey from loader (already converted to correct timezone)
		// This matches TypeScript's approach where timezone conversion happens during data loading
		dateKey := entry.DateKey
		if dateKey == "" {
			// Fallback to timezone conversion if DateKey not set (for compatibility)
			timeInZone := entry.Timestamp.In(f.timezone)
			dateKey = timeInZone.Format("2006-01-02")
		}
		
		groups[dateKey] = append(groups[dateKey], entry)
	}
	
	return groups
}

func (f *TableWriterFormatter) FormatMonthlyReportWithFilter(entries []types.UsageEntry, since, until string) string {
	// Group entries by month
	monthlyGroups := f.groupByMonth(entries)
	
	if len(monthlyGroups) == 0 {
		return f.formatEmptyMonthlyReport()
	}

	var output strings.Builder
	
	// Title - use default white color
	output.WriteString(" ╭──────────────────────────────────────────────────────╮\n")
	output.WriteString(" │                                                      │\n")
	output.WriteString(" │  Claude Code Token Usage Report - Monthly (WITH GO) │\n")
	output.WriteString(" │                                                      │\n")
	output.WriteString(" ╰──────────────────────────────────────────────────────╯\n\n")

	// Create table buffer
	var buf bytes.Buffer
	
	// Create table with tablewriter v1.0.9 API
	table := tablewriter.NewTable(&buf,
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Settings: tw.Settings{Separators: tw.Separators{BetweenRows: tw.On}},
		})),
		tablewriter.WithConfig(tablewriter.Config{
			Row: tw.CellConfig{
				Alignment: tw.CellAlignment{Global: tw.AlignRight},
			},
		}),
		tablewriter.WithHeaderAutoFormat(tw.Off), // Disable auto uppercase
	)
	
	// Set headers with multi-line support
	table.Header([]string{
		"Month\n",
		"Models\n",
		"Input\n",
		"Output\n",
		"Cache\nCreate",
		"Cache\nRead",
		"Total\nTokens",
		"Cost\n(USD)",
	})
	
	// Sort months
	var months []string
	for month := range monthlyGroups {
		// Apply month filter if specified
		if since != "" && month < since {
			continue
		}
		if until != "" && month > until {
			continue
		}
		months = append(months, month)
	}
	sort.Strings(months)
	
	var totalInput, totalOutput, totalCache, totalCacheRead, totalTokens int
	var totalCost float64
	
	// Process each month
	for _, month := range months {
		monthEntries := monthlyGroups[month]
		
		// Aggregate data for this month
		var monthInput, monthOutput, monthCache, monthCacheRead, monthTotalTokens int
		var monthCost float64
		modelMap := make(map[string]bool)
		
		for _, entry := range monthEntries {
			monthInput += entry.InputTokens
			monthOutput += entry.OutputTokens
			monthCost += entry.Cost
			monthTotalTokens += entry.TotalTokens
			
			// Track cache tokens from Raw data
			if entry.Raw != nil {
				if cc, ok := entry.Raw["cache_creation_input_tokens"].(int); ok {
					monthCache += cc
				}
				if cr, ok := entry.Raw["cache_read_input_tokens"].(int); ok {
					monthCacheRead += cr
				}
			}
			
			// Skip synthetic model in display (but still count its tokens/cost)
			if entry.Model != "" && entry.Model != "<synthetic>" {
				modelMap[entry.Model] = true
			}
		}
		
		// Format models list (same logic as daily format)
		simplifiedModels := make(map[string]bool)
		for model := range modelMap {
			shortModel := ShortenModelName(model)
			simplifiedModels[shortModel] = true
		}
		
		var models []string
		for model := range simplifiedModels {
			models = append(models, model)
		}
		sort.Strings(models)
		modelsStr := "- " + strings.Join(models, "\n- ")
		
		// Add totals
		totalInput += monthInput
		totalOutput += monthOutput
		totalCache += monthCache
		totalCacheRead += monthCacheRead
		totalTokens += monthTotalTokens
		totalCost += monthCost
		
		// Format month as YYYY-MM (keep original format for monthly)
		formattedMonth := month
		
		// Add row
		table.Append([]string{
			formattedMonth,
			modelsStr,
			f.formatLargeNumber(monthInput),
			f.formatLargeNumber(monthOutput),
			f.formatLargeNumber(monthCache),
			f.formatLargeNumber(monthCacheRead),
			f.formatLargeNumber(monthTotalTokens),
			fmt.Sprintf("$%.2f", monthCost),
		})
	}
	
	// Set footer
	table.Footer([]string{
		"Total",
		"",
		f.formatLargeNumber(totalInput),
		f.formatLargeNumber(totalOutput),
		f.formatLargeNumber(totalCache),
		f.formatLargeNumber(totalCacheRead),
		f.formatLargeNumber(totalTokens),
		fmt.Sprintf("$%.2f", totalCost),
	})
	
	// Render table
	table.Render()
	tableOutput := buf.String()
	
	// Apply color styling if enabled (same as daily format)
	if !f.noColor {
		// Apply colors to table elements
		gray := "\033[90m"     // Gray color for borders
		cyan := "\033[36m"     // Cyan color for headers
		yellow := "\033[33m"   // Yellow color for Total row
		reset := "\033[0m"     // Reset color
		
		lines := strings.Split(tableOutput, "\n")
		var coloredOutput strings.Builder
		
		for i, line := range lines {
			if line == "" {
				coloredOutput.WriteString("\n")
				continue
			}
			
			// Check if this is a pure border line (no data)
			if strings.HasPrefix(line, "┌") || strings.HasPrefix(line, "├") || strings.HasPrefix(line, "└") {
				// Pure border line - all gray
				coloredOutput.WriteString(gray + line + reset)
			} else if strings.Contains(line, "│") {
				// Line with data and borders
				parts := strings.Split(line, "│")
				for j, part := range parts {
					if j > 0 {
						coloredOutput.WriteString(gray + "│" + reset)
					}
					
					// Check content type
					if i <= 2 && strings.TrimSpace(part) != "" {
						// Header rows - use cyan
						coloredOutput.WriteString(cyan + part + reset)
					} else if strings.Contains(part, "Total") || (strings.Contains(line, "Total") && strings.TrimSpace(part) != "") {
						// Total row - use yellow for all content
						coloredOutput.WriteString(yellow + part + reset)
					} else {
						// Regular data - use default color (white)
						coloredOutput.WriteString(part)
					}
				}
			} else {
				// Other lines
				coloredOutput.WriteString(line)
			}
			
			if i < len(lines)-1 {
				coloredOutput.WriteString("\n")
			}
		}
		
		output.WriteString(coloredOutput.String())
	} else {
		output.WriteString(tableOutput)
	}
	
	return output.String()
}

func (f *TableWriterFormatter) groupByMonth(entries []types.UsageEntry) map[string][]types.UsageEntry {
	groups := make(map[string][]types.UsageEntry)
	
	for _, entry := range entries {
		// Skip invalid timestamps
		if entry.Timestamp.IsZero() || entry.Timestamp.Year() < 2020 {
			continue
		}
		
		// Use pre-computed DateKey from loader (already converted to correct timezone)
		// Extract month (YYYY-MM) from DateKey (YYYY-MM-DD)
		monthKey := ""
		if entry.DateKey != "" && len(entry.DateKey) >= 7 {
			monthKey = entry.DateKey[:7] // Take first 7 characters: YYYY-MM
		} else {
			// Fallback to timezone conversion if DateKey not set
			timeInZone := entry.Timestamp.In(f.timezone)
			monthKey = timeInZone.Format("2006-01")
		}
		
		groups[monthKey] = append(groups[monthKey], entry)
	}
	
	return groups
}

func (f *TableWriterFormatter) formatEmptyMonthlyReport() string {
	var output strings.Builder
	
	// Title - use default white color
	output.WriteString(" ╭──────────────────────────────────────────────────────╮\n")
	output.WriteString(" │                                                      │\n")
	output.WriteString(" │  Claude Code Token Usage Report - Monthly (WITH GO) │\n")
	output.WriteString(" │                                                      │\n")
	output.WriteString(" ╰──────────────────────────────────────────────────────╯\n\n")
	
	output.WriteString("No usage data found for the specified criteria.\n")
	
	return output.String()
}

func (f *TableWriterFormatter) formatEmptyReport() string {
	var output strings.Builder
	
	// Title - use default white color
	output.WriteString("\n")
	output.WriteString(" ╭────────────────────────────────────────────────────╮")
	output.WriteString("\n")
	output.WriteString(" │                                                    │")
	output.WriteString("\n")
	output.WriteString(" │  Claude Code Token Usage Report - Daily (WITH GO)  │")
	output.WriteString("\n")
	output.WriteString(" │                                                    │")
	output.WriteString("\n")
	output.WriteString(" ╰────────────────────────────────────────────────────╯")
	output.WriteString("\n\n")
	output.WriteString("No usage data found for the specified period.\n")
	
	return output.String()
}

// ShortenModelName 簡化 model 名稱為顯示格式（公用函數）
func ShortenModelName(model string) string {
	// 處理新的 model ID 格式，支援 4.1 和 4.5 版本
	// Examples:
	// claude-opus-4-1-20250805 -> Opus-4.1
	// claude-sonnet-4-5-20250929 -> Sonnet-4.5
	// claude-opus-4-20250514 -> Opus-4
	// claude-sonnet-4-20250514 -> Sonnet-4
	// claude-haiku-3-20240307 -> Haiku-3

	// 首先嘗試匹配帶小版本號的格式: claude-{type}-{major}-{minor}-{date}
	re := regexp.MustCompile(`^claude-(\w+)-(\d+)-(\d+)-\d+`)
	if matches := re.FindStringSubmatch(model); matches != nil {
		modelType := strings.Title(strings.ToLower(matches[1]))  // 首字母大寫
		majorVersion := matches[2]
		minorVersion := matches[3]
		return fmt.Sprintf("%s-%s.%s", modelType, majorVersion, minorVersion)
	}

	// 然後嘗試匹配標準格式: claude-{type}-{version}-{date}
	re = regexp.MustCompile(`^claude-(\w+)-(\d+)-\d+`)
	if matches := re.FindStringSubmatch(model); matches != nil {
		modelType := strings.Title(strings.ToLower(matches[1]))  // 首字母大寫
		version := matches[2]
		return fmt.Sprintf("%s-%s", modelType, version)
	}
	
	// Special handling for known non-Claude models
	knownModels := map[string]string{
		"gpt-4o":        "gpt-4o",
		"gpt-4o-mini":   "gpt-4o-mini",
		"gpt-4":         "gpt-4",
		"gpt-3.5-turbo": "gpt-3.5",
	}
	
	if short, ok := knownModels[model]; ok {
		return short
	}
	
	// If no pattern matches, return truncated version
	if len(model) > 12 {
		return model[:12]
	}
	return model
}

func (f *TableWriterFormatter) formatLargeNumber(n int) string {
	if n == 0 {
		return "-"
	}
	
	// Format with comma separators
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	
	var result []rune
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, r)
	}
	
	return string(result)
}

func (f *TableWriterFormatter) FormatSessionReport(sessions []types.SessionInfo) string {
	return f.FormatSessionReportWithFilter(sessions, "", "")
}

func (f *TableWriterFormatter) FormatSessionReportWithFilter(sessions []types.SessionInfo, since, until string) string {
	if len(sessions) == 0 {
		return f.formatEmptySessionReport()
	}

	var output strings.Builder
	
	// Title - use default white color
	output.WriteString(" ╭──────────────────────────────────────────────────────────╮\n")
	output.WriteString(" │                                                          │\n")
	output.WriteString(" │  Claude Code Token Usage Report - By Session (WITH GO)  │\n")
	output.WriteString(" │                                                          │\n")
	output.WriteString(" ╰──────────────────────────────────────────────────────────╯\n\n")

	// Create table buffer
	var buf bytes.Buffer
	
	// Create table with tablewriter v1.0.9 API
	table := tablewriter.NewTable(&buf,
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Settings: tw.Settings{Separators: tw.Separators{BetweenRows: tw.On}},
		})),
		tablewriter.WithConfig(tablewriter.Config{
			Row: tw.CellConfig{
				Alignment: tw.CellAlignment{Global: tw.AlignRight},
			},
		}),
		tablewriter.WithHeaderAutoFormat(tw.Off), // Disable auto uppercase
	)
	
	// Set headers with multi-line support
	table.Header([]string{
		"Session\n",
		"Models\n",
		"Input\n",
		"Output\n",
		"Cache\nCreate",
		"Cache\nRead",
		"Total\nTokens",
		"Cost\n(USD)",
		"Last\nActivity",
	})
	
	var totalInput, totalOutput, totalCache, totalCacheRead, totalTokens int
	var totalCost float64
	
	// Process each session
	for _, session := range sessions {
		// Apply date filter if specified
		lastActivity := session.LastActivity.Format("2006-01-02")
		if since != "" && lastActivity < since {
			continue
		}
		if until != "" && lastActivity > until {
			continue
		}
		
		// Extract project name from session ID or project path
		sessionDisplay := f.extractSessionDisplayName(session.SessionID, session.ProjectPath)
		
		// Format models list (same logic as daily format)
		simplifiedModels := make(map[string]bool)
		for _, model := range session.ModelsUsed {
			shortModel := ShortenModelName(model)
			simplifiedModels[shortModel] = true
		}
		
		var models []string
		for model := range simplifiedModels {
			models = append(models, model)
		}
		sort.Strings(models)
		modelsStr := "- " + strings.Join(models, "\n- ")
		if len(models) == 0 {
			modelsStr = "-"
		}
		
		totalInput += session.InputTokens
		totalOutput += session.OutputTokens
		totalCache += session.CacheCreationTokens
		totalCacheRead += session.CacheReadTokens
		totalTokens += session.TotalTokens
		totalCost += session.TotalCost
		
		// Add row to table
		table.Append([]string{
			sessionDisplay,
			modelsStr,
			f.formatLargeNumber(session.InputTokens),
			f.formatLargeNumber(session.OutputTokens),
			f.formatLargeNumber(session.CacheCreationTokens),
			f.formatLargeNumber(session.CacheReadTokens),
			f.formatLargeNumber(session.TotalTokens),
			fmt.Sprintf("$%.2f", session.TotalCost),
			lastActivity,
		})
	}
	
	// Set footer
	table.Footer([]string{
		"Total",
		"",
		f.formatLargeNumber(totalInput),
		f.formatLargeNumber(totalOutput),
		f.formatLargeNumber(totalCache),
		f.formatLargeNumber(totalCacheRead),
		f.formatLargeNumber(totalTokens),
		fmt.Sprintf("$%.2f", totalCost),
		"",
	})
	
	// Render table
	table.Render()
	
	// Apply color styling if enabled
	tableOutput := buf.String()
	if !f.noColor {
		// Apply colors to table elements (same as daily format)
		gray := "\033[90m"     // Gray color for borders
		cyan := "\033[36m"     // Cyan color for headers
		yellow := "\033[33m"   // Yellow color for Total row
		reset := "\033[0m"     // Reset color
		
		lines := strings.Split(tableOutput, "\n")
		var coloredOutput strings.Builder
		
		for i, line := range lines {
			if line == "" {
				coloredOutput.WriteString("\n")
				continue
			}
			
			// Check if this is a pure border line (no data)
			if strings.HasPrefix(line, "┌") || strings.HasPrefix(line, "├") || strings.HasPrefix(line, "└") {
				// Pure border line - all gray
				coloredOutput.WriteString(gray + line + reset)
			} else if strings.Contains(line, "│") {
				// Line with data and borders
				parts := strings.Split(line, "│")
				for j, part := range parts {
					if j > 0 {
						coloredOutput.WriteString(gray + "│" + reset)
					}
					
					// Check content type
					if i <= 2 && strings.TrimSpace(part) != "" {
						// Header rows - use cyan
						coloredOutput.WriteString(cyan + part + reset)
					} else if strings.Contains(part, "Total") || (strings.Contains(line, "Total") && strings.TrimSpace(part) != "") {
						// Total row - use yellow for all content
						coloredOutput.WriteString(yellow + part + reset)
					} else {
						// Regular data - use default color (white)
						coloredOutput.WriteString(part)
					}
				}
			} else {
				// Other lines
				coloredOutput.WriteString(line)
			}
			
			if i < len(lines)-1 {
				coloredOutput.WriteString("\n")
			}
		}
		
		output.WriteString(coloredOutput.String())
	} else {
		output.WriteString(tableOutput)
	}
	
	return output.String()
}

func (f *TableWriterFormatter) extractSessionDisplayName(sessionID, projectPath string) string {
	// sessionID is now the project path itself
	// Project paths look like: /path/to/projects/project-name
	// We need to extract just the meaningful project name part
	
	if sessionID == "unknown" || sessionID == "" {
		return "unknown"
	}
	
	// First check if this is a path containing "projects" directory
	parts := strings.Split(sessionID, string(os.PathSeparator))
	
	// Find the "projects" directory
	projectName := ""
	for i, part := range parts {
		if part == "projects" && i+1 < len(parts) {
			// The next part is the actual project name
			projectName = parts[i+1]
			break
		}
	}
	
	// If no projects directory found, use the last part
	if projectName == "" {
		projectName = parts[len(parts)-1]
	}
	
	// Clean up the project name
	projectName = strings.TrimPrefix(projectName, "-")
	
	// Use regex to extract meaningful project name patterns
	// Pattern 1: Match src-ProjectName or similar patterns
	srcProjectRe := regexp.MustCompile(`(?:^|-)(?:go_)?(?:src|react_src|python_src)[_-]([A-Za-z][A-Za-z0-9_-]+)`)
	if matches := srcProjectRe.FindStringSubmatch(projectName); len(matches) > 1 {
		return "src-" + matches[1]
	}
	
	// Pattern 2: Match blog-category-name pattern (e.g., blog-tech-news)
	blogRe := regexp.MustCompile(`blog-([a-z]+)-([a-z]+)`)
	if matches := blogRe.FindStringSubmatch(projectName); len(matches) > 2 {
		return "blog-" + matches[1] + "-" + matches[2]
	}
	
	// Pattern 3: Extract last meaningful segment that looks like a project name
	// Skip common path segments and volume identifiers
	segments := strings.Split(projectName, "-")
	
	// Filter out system/path segments using regex
	systemSegmentRe := regexp.MustCompile(`^(Volumes?|Users?|home|var|tmp|opt|usr|bin|lib|etc|[A-Z0-9]+_[A-Z0-9]+|^\d+[A-Z]+$)$`)
	userNameRe := regexp.MustCompile(`^[a-z]+$`) // Simple lowercase words are often usernames
	
	var meaningfulSegments []string
	foundSrc := false
	
	for i, segment := range segments {
		// Skip system directories and volume identifiers
		if systemSegmentRe.MatchString(segment) {
			continue
		}
		
		// Skip single lowercase words (often usernames) unless they're after "src"
		if userNameRe.MatchString(segment) && !foundSrc && len(segment) < 8 {
			continue
		}
		
		// Track if we found "src" or similar
		if segment == "src" || strings.HasSuffix(segment, "_src") {
			foundSrc = true
			// If next segment exists, combine them
			if i+1 < len(segments) && !systemSegmentRe.MatchString(segments[i+1]) {
				return "src-" + segments[i+1]
			}
		}
		
		// Collect meaningful segments
		if len(segment) > 2 && !systemSegmentRe.MatchString(segment) {
			meaningfulSegments = append(meaningfulSegments, segment)
		}
	}
	
	// Return the last meaningful segment(s)
	if len(meaningfulSegments) > 0 {
		// If we have multiple meaningful segments, check for common patterns
		if len(meaningfulSegments) >= 2 {
			lastTwo := meaningfulSegments[len(meaningfulSegments)-2:]
			// Check if it's a compound name like "claude-agents" or "ccusage-go"
			if len(lastTwo[0]) > 2 && len(lastTwo[1]) > 2 {
				return lastTwo[0] + "-" + lastTwo[1]
			}
		}
		// Return the last meaningful segment
		return meaningfulSegments[len(meaningfulSegments)-1]
	}
	
	// Final fallback: if nothing meaningful found, return a shortened version
	if len(segments) > 0 {
		return segments[len(segments)-1]
	}
	
	return "unknown"
}

func isDateLike(s string) bool {
	// Check if string looks like a year (4 digits) or month/day (1-2 digits)
	if len(s) == 4 || len(s) <= 2 {
		for _, r := range s {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	return false
}

func isSystemDirectory(name string) bool {
	// Common system directories to skip
	systemDirs := map[string]bool{
		"home": true, "Users": true, "usr": true, "var": true, 
		"tmp": true, "opt": true, "etc": true, "lib": true,
		"bin": true, "sbin": true, "dev": true, "proc": true,
		"sys": true, "root": true, "mnt": true, "media": true,
		"Volumes": true, "Applications": true, "Library": true,
	}
	return systemDirs[name]
}

func isUUID(s string) bool {
	// Simple UUID check: 8-4-4-4-12 format
	parts := strings.Split(s, "-")
	return len(parts) == 5 && len(parts[0]) == 8 && len(parts[1]) == 4 && len(parts[2]) == 4 && len(parts[3]) == 4 && len(parts[4]) == 12
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func isTimestampLike(s string) bool {
	// Check if string looks like a timestamp (all digits and long)
	if len(s) < 8 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (f *TableWriterFormatter) formatEmptySessionReport() string {
	var output strings.Builder
	
	// Title - use default white color
	output.WriteString(" ╭────────────────────────────────────────────────────────────╮\n")
	output.WriteString(" │                                                          │\n")
	output.WriteString(" │  Claude Code Token Usage Report - By Session (WITH GO)  │\n")
	output.WriteString(" │                                                          │\n")
	output.WriteString(" ╰────────────────────────────────────────────────────────────╯\n\n")
	
	output.WriteString("No session data found for the specified criteria.\n")
	
	return output.String()
}

// FormatBlocksReport formats session blocks report in table format
func (f *TableWriterFormatter) FormatBlocksReport(blocks []types.SessionBlock, tokenLimit int) string {
	if len(blocks) == 0 {
		return f.formatEmptyBlocksReport()
	}

	var output strings.Builder
	
	// Title box
	output.WriteString("\n")
	output.WriteString(" ╭───────────────────────────────────────────────────────────────╮\n")
	output.WriteString(" │                                                               │\n")
	output.WriteString(" │  Claude Code Token Usage Report - Session Blocks (WITH GO)  │\n")
	output.WriteString(" │                                                               │\n")
	output.WriteString(" ╰───────────────────────────────────────────────────────────────╯\n\n")

	// Create table buffer
	var buf bytes.Buffer
	
	// Create table with tablewriter v1.0.9 API
	table := tablewriter.NewTable(&buf,
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Settings: tw.Settings{Separators: tw.Separators{BetweenRows: tw.On}},
		})),
		tablewriter.WithConfig(tablewriter.Config{
			Row: tw.CellConfig{
				Alignment: tw.CellAlignment{Global: tw.AlignRight},
			},
		}),
		tablewriter.WithHeaderAutoFormat(tw.Off), // Disable auto uppercase
	)
	
	// Build headers dynamically
	headers := []string{
		"Block Start",
		"Duration/Status",
		"Models",
		"Tokens",
	}
	
	// Add % column if token limit is set
	if tokenLimit > 0 {
		headers = append(headers, "%")
	}
	
	headers = append(headers, "Cost")
	
	table.Header(headers)
	
	// Process each block
	for _, block := range blocks {
		if block.IsGap {
			// Gap row
			row := []string{
				f.formatBlockTime(block, false),
				"(inactive)",
				"-",
				"-",
			}
			if tokenLimit > 0 {
				row = append(row, "-")
			}
			row = append(row, "-")
			
			// Add gray coloring in post-processing
			table.Append(row)
		} else {
			totalTokens := block.TokenCounts.GetTotal()
			
			// Format time
			timeStr := f.formatBlockTime(block, false)
			
			// Status/Duration
			var statusStr string
			if block.IsActive {
				statusStr = "ACTIVE" // Will be colored green later
			} else {
				statusStr = ""
			}
			
			// Format models
			modelsStr := f.formatBlockModels(block.Models)
			
			// Format tokens
			tokensStr := formatNumberWithCommas(totalTokens)
			
			// Build row
			row := []string{
				timeStr,
				statusStr,
				modelsStr,
				tokensStr,
			}
			
			// Add percentage if token limit is set
			if tokenLimit > 0 {
				percentage := float64(totalTokens) / float64(tokenLimit) * 100
				percentStr := fmt.Sprintf("%.1f%%", percentage)
				row = append(row, percentStr)
			}
			
			// Add cost
			costStr := fmt.Sprintf("$%.2f", block.CostUSD)
			row = append(row, costStr)
			
			table.Append(row)
			
			// Add REMAINING and PROJECTED rows for active blocks
			if block.IsActive {
				// REMAINING row - only show if token limit is set
				if tokenLimit > 0 {
					currentTokens := totalTokens
					remainingTokens := tokenLimit - currentTokens
					if remainingTokens < 0 {
						remainingTokens = 0
					}
					
					remainingPercent := float64(remainingTokens) / float64(tokenLimit) * 100
					
					remainingRow := []string{
						fmt.Sprintf("(assuming %s token limit)", formatNumberWithCommas(tokenLimit)),
						"REMAINING", // Will be colored blue
						"",
						formatNumberWithCommas(remainingTokens),
						fmt.Sprintf("%.1f%%", remainingPercent),
						"",
					}
					table.Append(remainingRow)
				}
				
				// PROJECTED row
				if projection := calculator.ProjectBlockUsage(block); projection != nil {
					projectedRow := []string{
						"(assuming current burn rate)",
						"PROJECTED", // Will be colored yellow
						"",
						formatNumberWithCommas(projection.TotalTokens),
					}
					
					if tokenLimit > 0 {
						percentage := float64(projection.TotalTokens) / float64(tokenLimit) * 100
						projectedRow = append(projectedRow, fmt.Sprintf("%.1f%%", percentage))
					}
					
					projectedRow = append(projectedRow, fmt.Sprintf("$%.2f", projection.TotalCost))
					table.Append(projectedRow)
				}
			}
		}
	}
	
	// Render the table
	table.Render()
	tableOutput := buf.String()
	
	// Apply coloring if not disabled
	if !f.noColor {
		var coloredOutput strings.Builder
		lines := strings.Split(tableOutput, "\n")
		
		// ANSI color codes
		gray := "\033[90m"
		cyan := "\033[36m"
		green := "\033[32m"
		yellow := "\033[33m"
		blue := "\033[34m"
		red := "\033[31m"
		reset := "\033[0m"
		
		for i, line := range lines {
			// Check if this is a pure border line
			if strings.HasPrefix(line, "┌") || strings.HasPrefix(line, "├") || strings.HasPrefix(line, "└") {
				coloredOutput.WriteString(gray + line + reset)
			} else if strings.Contains(line, "│") {
				// Line with data and borders
				
				// Check for special rows
				if strings.Contains(line, "(inactive)") {
					// Gap row - all gray
					coloredOutput.WriteString(gray + line + reset)
				} else if strings.Contains(line, "ACTIVE") {
					// Active block row
					parts := strings.Split(line, "│")
					for j, part := range parts {
						if j > 0 {
							coloredOutput.WriteString(gray + "│" + reset)
						}
						
						if strings.Contains(part, "ACTIVE") {
							// Replace ACTIVE with green colored version
							colored := strings.Replace(part, "ACTIVE", green+"ACTIVE"+reset, 1)
							coloredOutput.WriteString(colored)
						} else if i <= 2 && strings.TrimSpace(part) != "" {
							// Header rows - use cyan
							coloredOutput.WriteString(cyan + part + reset)
						} else {
							coloredOutput.WriteString(part)
						}
					}
				} else if strings.Contains(line, "REMAINING") {
					// Remaining row
					parts := strings.Split(line, "│")
					for j, part := range parts {
						if j > 0 {
							coloredOutput.WriteString(gray + "│" + reset)
						}
						
						if strings.Contains(part, "REMAINING") {
							colored := strings.Replace(part, "REMAINING", blue+"REMAINING"+reset, 1)
							coloredOutput.WriteString(colored)
						} else if strings.Contains(part, "(assuming") {
							coloredOutput.WriteString(gray + part + reset)
						} else {
							coloredOutput.WriteString(part)
						}
					}
				} else if strings.Contains(line, "PROJECTED") {
					// Projected row
					parts := strings.Split(line, "│")
					for j, part := range parts {
						if j > 0 {
							coloredOutput.WriteString(gray + "│" + reset)
						}
						
						if strings.Contains(part, "PROJECTED") {
							colored := strings.Replace(part, "PROJECTED", yellow+"PROJECTED"+reset, 1)
							coloredOutput.WriteString(colored)
						} else if strings.Contains(part, "(assuming") {
							coloredOutput.WriteString(gray + part + reset)
						} else {
							// Check if this is a token value that exceeds limit
							trimmed := strings.TrimSpace(part)
							if tokenLimit > 0 && j == 4 { // Tokens column
								// Try to parse the number
								numStr := strings.ReplaceAll(trimmed, ",", "")
								if num, err := strconv.Atoi(numStr); err == nil && num > tokenLimit {
									coloredOutput.WriteString(red + part + reset)
								} else {
									coloredOutput.WriteString(part)
								}
							} else {
								coloredOutput.WriteString(part)
							}
						}
					}
				} else {
					// Regular data row
					parts := strings.Split(line, "│")
					for j, part := range parts {
						if j > 0 {
							coloredOutput.WriteString(gray + "│" + reset)
						}
						
						if i <= 2 && strings.TrimSpace(part) != "" {
							// Header rows - use cyan
							coloredOutput.WriteString(cyan + part + reset)
						} else {
							// Check for percentage over 100%
							trimmed := strings.TrimSpace(part)
							if strings.HasSuffix(trimmed, "%") {
								percentStr := strings.TrimSuffix(trimmed, "%")
								if percent, err := strconv.ParseFloat(percentStr, 64); err == nil && percent > 100 {
									coloredOutput.WriteString(red + part + reset)
								} else {
									coloredOutput.WriteString(part)
								}
							} else {
								coloredOutput.WriteString(part)
							}
						}
					}
				}
			} else {
				coloredOutput.WriteString(line)
			}
			
			if i < len(lines)-1 {
				coloredOutput.WriteString("\n")
			}
		}
		
		output.WriteString(coloredOutput.String())
	} else {
		output.WriteString(tableOutput)
	}
	
	return output.String()
}

func (f *TableWriterFormatter) formatBlockTime(block types.SessionBlock, compact bool) string {
	start := block.StartTime.In(f.timezone)
	
	if block.IsGap {
		end := block.EndTime.In(f.timezone)
		duration := end.Sub(start)
		hours := int(duration.Hours())
		
		if compact {
			return fmt.Sprintf("%s - %s\n(%dh gap)",
				start.Format("01/02, 3:04 PM"),
				end.Format("3:04 PM"),
				hours)
		}
		return fmt.Sprintf("%s - %s (%dh gap)",
			start.Format("2006-01-02, 3:04:05 PM"),
			end.Format("2006-01-02, 3:04:05 PM"),
			hours)
	}
	
	// For non-gap blocks
	var duration time.Duration
	if block.ActualEndTime != nil {
		duration = block.ActualEndTime.Sub(block.StartTime)
	} else {
		duration = time.Since(block.StartTime)
	}
	
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	
	if block.IsActive {
		now := time.Now()
		elapsed := now.Sub(block.StartTime)
		remaining := block.EndTime.Sub(now)
		
		elapsedHours := int(elapsed.Hours())
		elapsedMins := int(elapsed.Minutes()) % 60
		remainingHours := int(remaining.Hours())
		remainingMins := int(remaining.Minutes()) % 60
		
		if compact {
			return fmt.Sprintf("%s\n(%dh%dm/%dh%dm)",
				start.Format("01/02, 3:04 PM"),
				elapsedHours, elapsedMins,
				remainingHours, remainingMins)
		}
		return fmt.Sprintf("%s (%dh %dm elapsed, %dh %dm remaining)",
			start.Format("2006-01-02, 3:04:05 PM"),
			elapsedHours, elapsedMins,
			remainingHours, remainingMins)
	}
	
	if compact {
		if hours > 0 {
			return fmt.Sprintf("%s (%dh %dm)",
				start.Format("01/02, 3:04 PM"),
				hours, minutes)
		}
		return fmt.Sprintf("%s (%dm)",
			start.Format("01/02, 3:04 PM"),
			minutes)
	}
	
	if hours > 0 {
		return fmt.Sprintf("%s (%dh %dm)",
			start.Format("2006-01-02, 3:00:00 PM"),
			hours, minutes)
	}
	return fmt.Sprintf("%s (%dm)",
		start.Format("2006-01-02, 3:00:00 PM"),
		minutes)
}

func (f *TableWriterFormatter) formatBlockModels(models []string) string {
	if len(models) == 0 {
		return "-"
	}
	
	// Simplify model names
	simplifiedModels := make(map[string]bool)
	for _, model := range models {
		shortModel := ShortenModelName(model)
		simplifiedModels[shortModel] = true
	}
	
	// Convert to sorted slice
	var uniqueModels []string
	for model := range simplifiedModels {
		uniqueModels = append(uniqueModels, model)
	}
	sort.Strings(uniqueModels)
	
	// Format with bullet points like TypeScript version
	return "- " + strings.Join(uniqueModels, "\n- ")
}

func (f *TableWriterFormatter) formatEmptyBlocksReport() string {
	var output strings.Builder
	
	output.WriteString("\n")
	output.WriteString(" ╭───────────────────────────────────────────────────────────────╮\n")
	output.WriteString(" │                                                               │\n")
	output.WriteString(" │  Claude Code Token Usage Report - Session Blocks (WITH GO)  │\n")
	output.WriteString(" │                                                               │\n")
	output.WriteString(" ╰───────────────────────────────────────────────────────────────╯\n\n")
	
	output.WriteString("No session blocks found for the specified criteria.\n")
	
	return output.String()
}