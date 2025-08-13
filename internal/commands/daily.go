package commands

import (
	"fmt"
	"time"

	"github.com/sdpower/ccusage-go/internal/calculator"
	"github.com/sdpower/ccusage-go/internal/loader"
	"github.com/sdpower/ccusage-go/internal/output"
	"github.com/sdpower/ccusage-go/internal/pricing"
	"github.com/spf13/cobra"
)

func NewDailyCommand() *cobra.Command {
	var (
		date       string
		format     string
		dataPath   string
		noColor    bool
		responsive bool
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

			// Initialize services
			pricingService := pricing.NewService()
			calc := calculator.New(pricingService)
			dataLoader := loader.New()

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

			// Generate report
			report := calc.GenerateDailyReport(entries, targetDate)

			// Format and output
			output, err := formatter.FormatUsageReport(report)
			if err != nil {
				return fmt.Errorf("failed to format report: %w", err)
			}

			fmt.Print(output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&date, "date", "d", "", "Date to generate report for (YYYY-MM-DD, defaults to today)")
	cmd.Flags().StringVarP(&format, "format", "f", "table", "Output format (table, json, csv)")
	cmd.Flags().StringVar(&dataPath, "data-path", "", "Path to Claude data directory")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVar(&responsive, "responsive", true, "Enable responsive table layout")

	return cmd
}
