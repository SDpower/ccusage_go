package commands

import (
	"fmt"
	"time"

	"github.com/sdpower/ccusage-go/internal/calculator"
	"github.com/sdpower/ccusage-go/internal/loader"
	"github.com/sdpower/ccusage-go/internal/output"
	"github.com/sdpower/ccusage-go/internal/pricing"
	"github.com/sdpower/ccusage-go/internal/types"
	"github.com/spf13/cobra"
)

func NewDailyCommand() *cobra.Command {
	var (
		date       string
		format     string
		dataPath   string
		noColor    bool
		responsive bool
		debug      bool
		timezone   string
		since      string
		until      string
	)

	cmd := &cobra.Command{
		Use:   "daily",
		Short: "Generate daily usage report",
		Long:  `Generate a daily usage report for Claude Code usage data.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse date
			var targetDate time.Time
			var err error

			if date == "" {
				targetDate = time.Now()
			} else {
				targetDate, err = time.Parse("2006-01-02", date)
				if err != nil {
					return fmt.Errorf("invalid date format, use YYYY-MM-DD: %w", err)
				}
			}

			// Determine data path
			if dataPath == "" {
				dataPath = getDefaultDataPath()
			}

			// Load timezone if specified (BEFORE loading data)
			var loc *time.Location
			if timezone != "" {
				var err error
				loc, err = time.LoadLocation(timezone)
				if err != nil {
					return fmt.Errorf("invalid timezone %s: %w", timezone, err)
				}
			} else {
				loc = time.Local
			}

			// Initialize services
			pricingService := pricing.NewService()
			calc := calculator.New(pricingService)
			dataLoader := loader.New()
			dataLoader.SetDebug(debug)
			dataLoader.SetTimezone(loc) // Apply timezone to data loading (BEFORE loading data)

			formatter := output.NewFormatter(output.FormatterOptions{
				Format:     format,
				NoColor:    noColor,
				Responsive: responsive,
			})

			// Load data
			entries, err := dataLoader.LoadFromPath(cmd.Context(), dataPath)
			if err != nil {
				return fmt.Errorf("failed to load usage data: %w", err)
			}

			// Calculate costs
			entries, err = calc.CalculateCosts(cmd.Context(), entries)
			if err != nil {
				return fmt.Errorf("failed to calculate costs: %w", err)
			}

			// For table format, use the tablewriter formatter
			if format == "table" {
				tableFormatter := output.NewTableWriterFormatter(noColor)
				tableFormatter.SetTimezone(loc)
				
				// If no specific date, show all dates grouped
				if date == "" {
					// Convert since/until from YYYYMMDD to YYYY-MM-DD format
					sinceDate := ""
					untilDate := ""
					if since != "" && len(since) == 8 {
						sinceDate = fmt.Sprintf("%s-%s-%s", since[:4], since[4:6], since[6:8])
					}
					if until != "" && len(until) == 8 {
						untilDate = fmt.Sprintf("%s-%s-%s", until[:4], until[4:6], until[6:8])
					}
					output := tableFormatter.FormatDailyReportWithFilter(entries, sinceDate, untilDate)
					fmt.Print(output)
				} else {
					// Filter entries for the target date
					filteredEntries := []types.UsageEntry{}
					startOfDay := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, targetDate.Location())
					endOfDay := startOfDay.Add(24 * time.Hour)
					
					for _, entry := range entries {
						// Include entries that are >= startOfDay and < endOfDay
						if (entry.Timestamp.Equal(startOfDay) || entry.Timestamp.After(startOfDay)) && entry.Timestamp.Before(endOfDay) {
							filteredEntries = append(filteredEntries, entry)
						}
					}
					
					output := tableFormatter.FormatDailyReport(filteredEntries)
					fmt.Print(output)
				}
			} else {
				// Generate report for JSON/CSV
				report := calc.GenerateDailyReport(entries, targetDate)
				
				// Format and output
				output, err := formatter.FormatUsageReport(report)
				if err != nil {
					return fmt.Errorf("failed to format report: %w", err)
				}
				
				fmt.Print(output)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&date, "date", "d", "", "Date to generate report for (YYYY-MM-DD, defaults to today)")
	cmd.Flags().StringVarP(&format, "format", "f", "table", "Output format (table, json, csv)")
	cmd.Flags().StringVar(&dataPath, "data-path", "", "Path to Claude data directory")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVar(&responsive, "responsive", true, "Enable responsive table layout")
	cmd.Flags().BoolVar(&debug, "debug", false, "Show debug information")
	cmd.Flags().StringVarP(&timezone, "timezone", "z", "", "Timezone for date grouping (e.g., UTC, America/New_York, Asia/Tokyo). Default: system timezone")
	cmd.Flags().StringVarP(&since, "since", "s", "", "Filter from date (YYYYMMDD format)")
	cmd.Flags().StringVarP(&until, "until", "u", "", "Filter until date (YYYYMMDD format)")

	return cmd
}
