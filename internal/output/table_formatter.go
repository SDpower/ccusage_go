package output

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sdpower/ccusage-go/internal/types"
)

// TableFormatter handles enhanced table formatting similar to TypeScript version
type TableFormatter struct {
	noColor bool
}

func NewTableFormatter(noColor bool) *TableFormatter {
	return &TableFormatter{noColor: noColor}
}

func (f *TableFormatter) FormatDailyReport(entries []types.UsageEntry) string {
	// Group entries by date
	dailyGroups := f.groupByDate(entries)
	
	if len(dailyGroups) == 0 {
		return f.formatEmptyReport()
	}

	// Create table
	var output strings.Builder
	
	// Title
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	if f.noColor {
		titleStyle = lipgloss.NewStyle()
	}
	
	output.WriteString("\n")
	output.WriteString(titleStyle.Render(" ╭──────────────────────────────────────────╮"))
	output.WriteString("\n")
	output.WriteString(titleStyle.Render(" │                                          │"))
	output.WriteString("\n")
	output.WriteString(titleStyle.Render(" │  Claude Code Token Usage Report - Daily  │"))
	output.WriteString("\n")
	output.WriteString(titleStyle.Render(" │                                          │"))
	output.WriteString("\n")
	output.WriteString(titleStyle.Render(" ╰──────────────────────────────────────────╯"))
	output.WriteString("\n\n")

	// Table header
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("36"))
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("90"))
	
	if f.noColor {
		headerStyle = lipgloss.NewStyle()
		borderStyle = lipgloss.NewStyle()
	}

	// Header row
	output.WriteString(borderStyle.Render("┌──────────┬────────────────┬──────────┬──────────┬────────────┬─────────────┬─────────────┬──────────┐"))
	output.WriteString("\n")
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render(" Date     "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render(" Models         "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("    Input "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("   Output "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("      Cache "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("  Cache Read "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("       Total "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("     Cost "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString("\n")
	
	// Column headers second row
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("          "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("                "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("          "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("          "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("     Create "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("             "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("      Tokens "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render("    (USD) "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString("\n")
	
	output.WriteString(borderStyle.Render("├──────────┼────────────────┼──────────┼──────────┼────────────┼─────────────┼─────────────┼──────────┤"))
	output.WriteString("\n")
	
	// Sort dates
	var dates []string
	for date := range dailyGroups {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	
	var totalInput, totalOutput, totalCache, totalCacheRead, totalTokens int
	var totalCost float64
	
	// Data rows
	for _, date := range dates {
		group := dailyGroups[date]
		
		// Calculate aggregates for this date
		var input, outputTokens, cache, cacheRead, tokens int
		var cost float64
		models := make(map[string]bool)
		
		for _, entry := range group {
			input += entry.InputTokens
			outputTokens += entry.OutputTokens
			tokens += entry.TotalTokens
			cost += entry.Cost
			
			if entry.Model != "" {
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
		
		totalInput += input
		totalOutput += outputTokens
		totalCache += cache
		totalCacheRead += cacheRead
		totalTokens += tokens
		totalCost += cost
		
		// Format models list
		var modelList []string
		for model := range models {
			// Shorten model names
			shortModel := f.shortenModelName(model)
			modelList = append(modelList, shortModel)
		}
		sort.Strings(modelList)
		modelsStr := "- " + strings.Join(modelList, "\n- ")
		if len(modelsStr) > 14 {
			modelsStr = modelsStr[:14]
		}
		
		// Output row
		output.WriteString(borderStyle.Render("│"))
		output.WriteString(fmt.Sprintf(" %-8s ", date))
		output.WriteString(borderStyle.Render("│"))
		output.WriteString(fmt.Sprintf(" %-14s ", modelsStr))
		output.WriteString(borderStyle.Render("│"))
		output.WriteString(fmt.Sprintf(" %8s ", f.formatLargeNumber(input)))
		output.WriteString(borderStyle.Render("│"))
		output.WriteString(fmt.Sprintf(" %8s ", f.formatLargeNumber(outputTokens)))
		output.WriteString(borderStyle.Render("│"))
		output.WriteString(fmt.Sprintf(" %10s ", f.formatLargeNumber(cache)))
		output.WriteString(borderStyle.Render("│"))
		output.WriteString(fmt.Sprintf(" %11s ", f.formatLargeNumber(cacheRead)))
		output.WriteString(borderStyle.Render("│"))
		output.WriteString(fmt.Sprintf(" %11s ", f.formatLargeNumber(tokens)))
		output.WriteString(borderStyle.Render("│"))
		output.WriteString(fmt.Sprintf(" $%7.2f ", cost))
		output.WriteString(borderStyle.Render("│"))
		output.WriteString("\n")
	}
	
	// Total row
	output.WriteString(borderStyle.Render("├──────────┼────────────────┼──────────┼──────────┼────────────┼─────────────┼─────────────┼──────────┤"))
	output.WriteString("\n")
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(headerStyle.Render(" Total    "))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(fmt.Sprintf(" %-14s ", ""))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(fmt.Sprintf(" %8s ", f.formatLargeNumber(totalInput)))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(fmt.Sprintf(" %8s ", f.formatLargeNumber(totalOutput)))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(fmt.Sprintf(" %10s ", f.formatLargeNumber(totalCache)))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(fmt.Sprintf(" %11s ", f.formatLargeNumber(totalCacheRead)))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(fmt.Sprintf(" %11s ", f.formatLargeNumber(totalTokens)))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString(fmt.Sprintf(" $%7.2f ", totalCost))
	output.WriteString(borderStyle.Render("│"))
	output.WriteString("\n")
	output.WriteString(borderStyle.Render("└──────────┴────────────────┴──────────┴──────────┴────────────┴─────────────┴─────────────┴──────────┘"))
	output.WriteString("\n")
	
	return output.String()
}

func (f *TableFormatter) groupByDate(entries []types.UsageEntry) map[string][]types.UsageEntry {
	groups := make(map[string][]types.UsageEntry)
	
	for _, entry := range entries {
		dateKey := entry.Timestamp.Format("2006-01-02")
		groups[dateKey] = append(groups[dateKey], entry)
	}
	
	return groups
}

func (f *TableFormatter) formatEmptyReport() string {
	var output strings.Builder
	
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	if f.noColor {
		titleStyle = lipgloss.NewStyle()
	}
	
	output.WriteString("\n")
	output.WriteString(titleStyle.Render(" ╭──────────────────────────────────────────╮"))
	output.WriteString("\n")
	output.WriteString(titleStyle.Render(" │                                          │"))
	output.WriteString("\n")
	output.WriteString(titleStyle.Render(" │  Claude Code Token Usage Report - Daily  │"))
	output.WriteString("\n")
	output.WriteString(titleStyle.Render(" │                                          │"))
	output.WriteString("\n")
	output.WriteString(titleStyle.Render(" ╰──────────────────────────────────────────╯"))
	output.WriteString("\n\n")
	output.WriteString("No usage data found for the specified period.\n")
	
	return output.String()
}

func (f *TableFormatter) shortenModelName(model string) string {
	// Shorten common model names
	replacements := map[string]string{
		"claude-3-5-sonnet-20241022": "sonnet-4",
		"claude-3-5-sonnet-20240620": "sonnet-3.5",
		"claude-3-opus-20240229":     "opus-4",
		"claude-3-sonnet-20240229":   "sonnet-3",
		"claude-3-haiku-20240307":    "haiku-3",
		"gpt-4o":                     "gpt-4o",
		"gpt-4o-mini":                "gpt-4o-mini",
		"gpt-4":                      "gpt-4",
		"gpt-3.5-turbo":              "gpt-3.5",
	}
	
	if short, ok := replacements[model]; ok {
		return short
	}
	
	// If not found, return truncated version
	if len(model) > 12 {
		return model[:12]
	}
	return model
}

func (f *TableFormatter) formatLargeNumber(n int) string {
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