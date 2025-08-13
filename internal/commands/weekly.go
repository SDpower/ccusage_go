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

func NewWeeklyCommand() *cobra.Command {
	var (
		week       string
		format     string
		dataPath   string
		noColor    bool
		responsive bool
	)

	cmd := &cobra.Command{
		Use:   "weekly",
		Short: "Generate weekly usage report",
		Long:  `Generate a weekly usage report for Claude Code usage data.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse week
			var year, weekNum int
			var err error

			if week == "" {
				year, weekNum = time.Now().ISOWeek()
			} else {
				parts := strings.Split(week, "-W")
				if len(parts) != 2 {
					return fmt.Errorf("invalid week format, use YYYY-WNN")
				}

				year, err = strconv.Atoi(parts[0])
				if err != nil {
					return fmt.Errorf("invalid year: %w", err)
				}

				weekNum, err = strconv.Atoi(parts[1])
				if err != nil {
					return fmt.Errorf("invalid week: %w", err)
				}

				if weekNum < 1 || weekNum > 53 {
					return fmt.Errorf("week must be between 1 and 53")
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
			report := calc.GenerateWeeklyReport(entries, year, weekNum)

			// Format and output
			output, err := formatter.FormatUsageReport(report)
			if err != nil {
				return fmt.Errorf("failed to format report: %w", err)
			}

			fmt.Print(output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&week, "week", "w", "", "Week to generate report for (YYYY-WNN, defaults to current week)")
	cmd.Flags().StringVarP(&format, "format", "f", "table", "Output format (table, json, csv)")
	cmd.Flags().StringVar(&dataPath, "data-path", "", "Path to Claude data directory")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVar(&responsive, "responsive", true, "Enable responsive table layout")

	return cmd
}
