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
			report := calc.GenerateMonthlyReport(entries, year, monthNum)

			// Format and output
			output, err := formatter.FormatUsageReport(report)
			if err != nil {
				return fmt.Errorf("failed to format report: %w", err)
			}

			fmt.Print(output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&month, "month", "m", "", "Month to generate report for (YYYY-MM, defaults to current month)")
	cmd.Flags().StringVarP(&format, "format", "f", "table", "Output format (table, json, csv)")
	cmd.Flags().StringVar(&dataPath, "data-path", "", "Path to Claude data directory")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVar(&responsive, "responsive", true, "Enable responsive table layout")

	return cmd
}
