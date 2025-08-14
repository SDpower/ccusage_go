package output

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
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

func (f *TableWriterFormatter) SetTimezone(loc *time.Location) {
	if loc != nil {
		f.timezone = loc
	}
}

func (f *TableWriterFormatter) FormatDailyReport(entries []types.UsageEntry) string {
	return f.FormatDailyReportWithFilter(entries, "", "")
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
	output.WriteString(" ╭──────────────────────────────────────────╮")
	output.WriteString("\n")
	output.WriteString(" │                                          │")
	output.WriteString("\n")
	output.WriteString(" │  Claude Code Token Usage Report - Daily  │")
	output.WriteString("\n")
	output.WriteString(" │                                          │")
	output.WriteString("\n")
	output.WriteString(" ╰──────────────────────────────────────────╯")
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
	)
	
	// Set headers with multi-line support
	table.Header([]string{
		"Date\n",
		"Models\n",
		"Input\n",
		"Output\n",
		"Cache\nCreate",
		"Cache Read\n",
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
			shortModel := f.shortenModelName(model)
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

func (f *TableWriterFormatter) formatEmptyReport() string {
	var output strings.Builder
	
	// Title - use default white color
	output.WriteString("\n")
	output.WriteString(" ╭──────────────────────────────────────────╮")
	output.WriteString("\n")
	output.WriteString(" │                                          │")
	output.WriteString("\n")
	output.WriteString(" │  Claude Code Token Usage Report - Daily  │")
	output.WriteString("\n")
	output.WriteString(" │                                          │")
	output.WriteString("\n")
	output.WriteString(" ╰──────────────────────────────────────────╯")
	output.WriteString("\n\n")
	output.WriteString("No usage data found for the specified period.\n")
	
	return output.String()
}

func (f *TableWriterFormatter) shortenModelName(model string) string {
	// Use regex to extract model type and version, similar to TypeScript version
	// The TypeScript regex is: /claude-(\w+)-(\d+)-\d+/
	// This matches the first two parts after "claude-" and ignores the rest
	// Examples:
	// claude-sonnet-4-20250514 -> sonnet-4
	// claude-opus-4-1-20250805 -> opus-4 (matches claude-opus-4, ignores -1-20250805)
	// claude-haiku-3-20240307 -> haiku-3
	
	// Use the same pattern as TypeScript: claude-{type}-{version}-{anything}
	re := regexp.MustCompile(`^claude-(\w+)-(\d+)-`)
	if matches := re.FindStringSubmatch(model); matches != nil {
		// Return type-version format
		return fmt.Sprintf("%s-%s", matches[1], matches[2])
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