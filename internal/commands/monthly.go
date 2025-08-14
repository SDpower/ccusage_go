package commands

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sdpower/ccusage-go/internal/calculator"
	"github.com/sdpower/ccusage-go/internal/loader"
	"github.com/sdpower/ccusage-go/internal/output"
	"github.com/sdpower/ccusage-go/internal/pricing"
	"github.com/spf13/cobra"
)

func NewMonthlyCommand() *cobra.Command {
	var (
		month      string
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
		Use:   "monthly",
		Short: "Generate monthly usage report",
		Long:  `Generate a monthly usage report for Claude Code usage data.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse month
			var year, monthNum int
			var err error

			if month == "" {
				now := time.Now()
				year = now.Year()
				monthNum = int(now.Month())
			} else {
				parts := strings.Split(month, "-")
				if len(parts) != 2 {
					return fmt.Errorf("invalid month format, use YYYY-MM")
				}

				year, err = strconv.Atoi(parts[0])
				if err != nil {
					return fmt.Errorf("invalid year: %w", err)
				}

				monthNum, err = strconv.Atoi(parts[1])
				if err != nil {
					return fmt.Errorf("invalid month: %w", err)
				}

				if monthNum < 1 || monthNum > 12 {
					return fmt.Errorf("month must be between 1 and 12")
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
				
				// Convert since/until from YYYYMM to YYYY-MM format for monthly filtering
				sinceMonth := ""
				untilMonth := ""
				if since != "" && len(since) == 6 {
					sinceMonth = fmt.Sprintf("%s-%s", since[:4], since[4:6])
				}
				if until != "" && len(until) == 6 {
					untilMonth = fmt.Sprintf("%s-%s", until[:4], until[4:6])
				}
				output := tableFormatter.FormatMonthlyReportWithFilter(entries, sinceMonth, untilMonth)
				fmt.Print(output)
			} else {
				// Generate report for JSON/CSV
				report := calc.GenerateMonthlyReport(entries, year, monthNum)
				
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

	cmd.Flags().StringVarP(&month, "month", "m", "", "Month to generate report for (YYYY-MM, defaults to current month)")
	cmd.Flags().StringVarP(&format, "format", "f", "table", "Output format (table, json, csv)")
	cmd.Flags().StringVar(&dataPath, "data-path", "", "Path to Claude data directory")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVar(&responsive, "responsive", true, "Enable responsive table layout")
	cmd.Flags().BoolVar(&debug, "debug", false, "Show debug information")
	cmd.Flags().StringVarP(&timezone, "timezone", "z", "", "Timezone for date grouping (e.g., UTC, America/New_York, Asia/Tokyo). Default: system timezone")
	cmd.Flags().StringVarP(&since, "since", "s", "", "Filter from month (YYYYMM format)")
	cmd.Flags().StringVarP(&until, "until", "u", "", "Filter until month (YYYYMM format)")

	return cmd
}
