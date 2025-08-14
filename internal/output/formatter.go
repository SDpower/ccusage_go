package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/sdpower/ccusage-go/internal/types"
)

type Formatter struct {
	options FormatterOptions
}

type FormatterOptions struct {
	Format     string // "table", "json", "csv"
	NoColor    bool
	Responsive bool
	MaxWidth   int
}

func NewFormatter(opts FormatterOptions) *Formatter {
	if opts.MaxWidth == 0 {
		opts.MaxWidth = 120
	}
	return &Formatter{options: opts}
}

func (f *Formatter) FormatUsageReport(report types.UsageReport) (string, error) {
	switch f.options.Format {
	case "json":
		return f.formatJSON(report)
	case "csv":
		return f.formatCSV(report.Entries)
	default:
		return f.formatTable(report)
	}
}

func (f *Formatter) FormatSessionReport(sessions []types.SessionInfo) (string, error) {
	switch f.options.Format {
	case "json":
		return f.formatJSON(sessions)
	case "csv":
		return f.formatSessionCSV(sessions)
	default:
		// Use tablewriter formatter for better consistency
		tableFormatter := NewTableWriterFormatter(f.options.NoColor)
		return tableFormatter.FormatSessionReport(sessions), nil
	}
}

func (f *Formatter) FormatBlocksReport(blocks []types.BlockInfo) (string, error) {
	switch f.options.Format {
	case "json":
		return f.formatJSON(blocks)
	case "csv":
		return f.formatBlocksCSV(blocks)
	default:
		return f.formatBlocksTable(blocks)
	}
}

func (f *Formatter) FormatJSON(data interface{}) (string, error) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}

func (f *Formatter) formatJSON(data interface{}) (string, error) {
	return f.FormatJSON(data)
}

func (f *Formatter) FormatCSV(data [][]string) (string, error) {
	var output strings.Builder
	for _, row := range data {
		for i, cell := range row {
			if i > 0 {
				output.WriteString(",")
			}
			// Escape quotes in cells
			if strings.Contains(cell, "\"") || strings.Contains(cell, ",") || strings.Contains(cell, "\n") {
				output.WriteString("\"")
				output.WriteString(strings.ReplaceAll(cell, "\"", "\"\""))
				output.WriteString("\"")
			} else {
				output.WriteString(cell)
			}
		}
		output.WriteString("\n")
	}
	return output.String(), nil
}

func (f *Formatter) formatTable(report types.UsageReport) (string, error) {
	var output strings.Builder
	
	// Header
	headerStyle := lipgloss.NewStyle().Bold(true)
	if !f.options.NoColor {
		headerStyle = headerStyle.Foreground(lipgloss.Color("205"))
	}
	
	output.WriteString(headerStyle.Render(fmt.Sprintf("Usage Report - %s", strings.Title(report.Period))))
	output.WriteString("\n\n")
	
	// Summary
	summaryStyle := lipgloss.NewStyle().Padding(1)
	if !f.options.NoColor {
		summaryStyle = summaryStyle.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))
	}
	
	summary := fmt.Sprintf(
		"Period: %s to %s\nTotal Requests: %d\nTotal Cost: $%.4f\nTotal Tokens: %s\nAverage Cost: $%.4f",
		report.StartTime.Format("2006-01-02"),
		report.EndTime.Format("2006-01-02"),
		report.Summary.TotalRequests,
		report.Summary.TotalCost,
		f.formatNumber(report.Summary.TotalTokens),
		report.Summary.AverageCost,
	)
	
	output.WriteString(summaryStyle.Render(summary))
	output.WriteString("\n\n")
	
	// Simple table
	if len(report.Entries) > 0 {
		output.WriteString("Recent Entries:\n")
		output.WriteString(fmt.Sprintf("%-10s %-20s %-30s %-10s %-12s\n", "Time", "Model", "Project", "Tokens", "Cost"))
		output.WriteString(strings.Repeat("-", 85))
		output.WriteString("\n")
		
		for _, entry := range report.Entries {
			projectName := f.getProjectName(entry.ProjectPath)
			if len(projectName) > 28 {
				projectName = projectName[:25] + "..."
			}
			
			output.WriteString(fmt.Sprintf("%-10s %-20s %-30s %-10s $%-10.4f\n",
				entry.Timestamp.Format("15:04:05"),
				f.truncateString(entry.Model, 19),
				projectName,
				f.formatNumber(entry.TotalTokens),
				entry.Cost,
			))
		}
	}
	
	return output.String(), nil
}

func (f *Formatter) formatSessionTable(sessions []types.SessionInfo) (string, error) {
	var output strings.Builder
	
	headerStyle := lipgloss.NewStyle().Bold(true)
	if !f.options.NoColor {
		headerStyle = headerStyle.Foreground(lipgloss.Color("205"))
	}
	
	output.WriteString(headerStyle.Render("Session Report"))
	output.WriteString("\n\n")
	
	if len(sessions) > 0 {
		output.WriteString(fmt.Sprintf("%-16s %-10s %-8s %-10s %-12s %-25s\n", "Start Time", "Duration", "Requests", "Tokens", "Cost", "Project"))
		output.WriteString(strings.Repeat("-", 85))
		output.WriteString("\n")
		
		for _, session := range sessions {
			projectName := f.getProjectName(session.ProjectPath)
			if len(projectName) > 23 {
				projectName = projectName[:20] + "..."
			}
			
			output.WriteString(fmt.Sprintf("%-16s %-10s %-8d %-10s $%-10.4f %-25s\n",
				session.StartTime.Format("2006-01-02 15:04"),
				f.formatDuration(session.Duration),
				session.RequestCount,
				f.formatNumber(session.TotalTokens),
				session.TotalCost,
				projectName,
			))
		}
	}
	
	return output.String(), nil
}

func (f *Formatter) formatBlocksTable(blocks []types.BlockInfo) (string, error) {
	var output strings.Builder
	
	headerStyle := lipgloss.NewStyle().Bold(true)
	if !f.options.NoColor {
		headerStyle = headerStyle.Foreground(lipgloss.Color("205"))
	}
	
	output.WriteString(headerStyle.Render("Blocks Report"))
	output.WriteString("\n\n")
	
	if len(blocks) > 0 {
		output.WriteString(fmt.Sprintf("%-20s %-8s %-10s %-12s %-12s %-12s\n", "Block Type", "Count", "Tokens", "Cost", "First Seen", "Last Seen"))
		output.WriteString(strings.Repeat("-", 85))
		output.WriteString("\n")
		
		for _, block := range blocks {
			output.WriteString(fmt.Sprintf("%-20s %-8d %-10s $%-10.4f %-12s %-12s\n",
				f.truncateString(block.BlockType, 19),
				block.Count,
				f.formatNumber(block.TotalTokens),
				block.TotalCost,
				block.FirstSeen.Format("2006-01-02"),
				block.LastSeen.Format("2006-01-02"),
			))
		}
	}
	
	return output.String(), nil
}

func (f *Formatter) formatCSV(entries []types.UsageEntry) (string, error) {
	var output strings.Builder
	output.WriteString("timestamp,model,project_path,input_tokens,output_tokens,total_tokens,cost,session_id,block_type\n")
	
	for _, entry := range entries {
		output.WriteString(fmt.Sprintf("%s,%s,%s,%d,%d,%d,%.6f,%s,%s\n",
			entry.Timestamp.Format(time.RFC3339),
			entry.Model,
			entry.ProjectPath,
			entry.InputTokens,
			entry.OutputTokens,
			entry.TotalTokens,
			entry.Cost,
			entry.SessionID,
			entry.BlockType,
		))
	}
	
	return output.String(), nil
}

func (f *Formatter) formatSessionCSV(sessions []types.SessionInfo) (string, error) {
	var output strings.Builder
	output.WriteString("session_id,start_time,end_time,duration_seconds,total_cost,total_tokens,request_count,project_path\n")
	
	for _, session := range sessions {
		output.WriteString(fmt.Sprintf("%s,%s,%s,%.0f,%.6f,%d,%d,%s\n",
			session.SessionID,
			session.StartTime.Format(time.RFC3339),
			session.EndTime.Format(time.RFC3339),
			session.Duration.Seconds(),
			session.TotalCost,
			session.TotalTokens,
			session.RequestCount,
			session.ProjectPath,
		))
	}
	
	return output.String(), nil
}

func (f *Formatter) formatBlocksCSV(blocks []types.BlockInfo) (string, error) {
	var output strings.Builder
	output.WriteString("block_type,count,total_tokens,total_cost,first_seen,last_seen\n")
	
	for _, block := range blocks {
		output.WriteString(fmt.Sprintf("%s,%d,%d,%.6f,%s,%s\n",
			block.BlockType,
			block.Count,
			block.TotalTokens,
			block.TotalCost,
			block.FirstSeen.Format(time.RFC3339),
			block.LastSeen.Format(time.RFC3339),
		))
	}
	
	return output.String(), nil
}

func (f *Formatter) getProjectName(path string) string {
	if path == "" {
		return "unknown"
	}
	
	parts := strings.Split(path, string(os.PathSeparator))
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

func (f *Formatter) formatNumber(n int) string {
	str := strconv.Itoa(n)
	if len(str) <= 3 {
		return str
	}
	
	var result strings.Builder
	for i, digit := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(digit)
	}
	
	return result.String()
}

func (f *Formatter) formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	
	if hours > 0 {
		return fmt.Sprintf("%dh%02dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func (f *Formatter) truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}