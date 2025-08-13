package commands

import (
	"fmt"

	"github.com/sdpower/ccusage-go/internal/calculator"
	"github.com/sdpower/ccusage-go/internal/loader"
	"github.com/sdpower/ccusage-go/internal/output"
	"github.com/sdpower/ccusage-go/internal/pricing"
	"github.com/spf13/cobra"
)

func NewBlocksCommand() *cobra.Command {
	var (
		format     string
		dataPath   string
		noColor    bool
		responsive bool
	)

	cmd := &cobra.Command{
		Use:   "blocks",
		Short: "Generate blocks usage report",
		Long:  `Generate a blocks-based usage report for Claude Code usage data.`,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			// Generate blocks report
			blocks := calc.GenerateBlocksReport(entries)

			// Format and output
			output, err := formatter.FormatBlocksReport(blocks)
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

	return cmd
}
