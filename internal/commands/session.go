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

func NewSessionCommand() *cobra.Command {
	var (
		format     string
		dataPath   string
		noColor    bool
		responsive bool
		timezone   string
		since      string
		until      string
	)

	cmd := &cobra.Command{
		Use:   "session",
		Short: "Generate session usage report",
		Long:  `Generate a session-based usage report for Claude Code usage data.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine data path
			if dataPath == "" {
				dataPath = getDefaultDataPath()
			}

			// Initialize services
			pricingService := pricing.NewService()
			calc := calculator.New(pricingService)
			dataLoader := loader.New()

			// Set timezone if specified
			if timezone != "" {
				loc, err := time.LoadLocation(timezone)
				if err != nil {
					return fmt.Errorf("invalid timezone %s: %w", timezone, err)
				}
				dataLoader.SetTimezone(loc)
			}

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

			// Apply date filters if specified
			if since != "" || until != "" {
				entries = filterEntriesByDate(entries, since, until)
			}

			// Calculate costs
			entries, err = calc.CalculateCosts(cmd.Context(), entries)
			if err != nil {
				return fmt.Errorf("failed to calculate costs: %w", err)
			}

			// Generate session report
			sessions := calc.GenerateSessionReport(entries)

			// Format and output
			output, err := formatter.FormatSessionReport(sessions)
			if err != nil {
				return fmt.Errorf("failed to format report: %w", err)
			}

			fmt.Print(output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "table", "Output format (table, json, csv)")
	cmd.Flags().StringVar(&dataPath, "data-path", "", "Path to Claude data directory")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVar(&responsive, "responsive", true, "Enable responsive table layout")
	cmd.Flags().StringVarP(&timezone, "timezone", "z", "", "Timezone for date grouping")
	cmd.Flags().StringVar(&since, "since", "", "Start date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&until, "until", "", "End date (YYYY-MM-DD)")

	return cmd
}
