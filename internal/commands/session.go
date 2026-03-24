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
		format      string
		dataPath    string
		noColor     bool
		responsive  bool
		timezone    string
		since       string
		until       string
		sessionID   string
		sessionName string
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

			// Apply session filters
			if sessionID != "" {
				entries = filterEntriesBySessionID(entries, sessionID)
			}
			if sessionName != "" {
				entries = filterEntriesBySessionName(entries, sessionName)
			}
			if (sessionID != "" || sessionName != "") && len(entries) == 0 {
				fmt.Println("No entries found for the specified session filter")
				return nil
			}

			// Calculate costs
			entries, err = calc.CalculateCosts(cmd.Context(), entries)
			if err != nil {
				return fmt.Errorf("failed to calculate costs: %w", err)
			}

			// Generate session report
			sessions := calc.GenerateSessionReport(entries)

			// Detail mode: show per-file breakdown when filtering by session
			isFiltered := sessionID != "" || sessionName != ""
			if isFiltered && format == "table" {
				fileStats := calc.AggregateBySourceFile(entries)
				tableFormatter := output.NewTableWriterFormatter(noColor)
				if timezone != "" {
					loc, _ := time.LoadLocation(timezone)
					tableFormatter.SetTimezone(loc)
				}
				result := tableFormatter.FormatSessionDetailReport(sessions, fileStats)
				fmt.Print(result)
				return nil
			}

			// Format and output
			result, err := formatter.FormatSessionReport(sessions)
			if err != nil {
				return fmt.Errorf("failed to format report: %w", err)
			}

			fmt.Print(result)
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
	cmd.Flags().StringVar(&sessionID, "session-id", "", "Filter by session UUID")
	cmd.Flags().StringVar(&sessionName, "session-name", "", "Filter by session name (exact match)")

	return cmd
}
