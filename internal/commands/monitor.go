package commands

import (
	"fmt"
	"time"

	"github.com/sdpower/ccusage-go/internal/monitor"
	"github.com/spf13/cobra"
)

func NewMonitorCommand() *cobra.Command {
	var (
		dataPath   string
		interval   int
		noColor    bool
		continuous bool
	)

	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Monitor Claude Code usage in real-time",
		Long:  `Monitor Claude Code usage data in real-time with live dashboard.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine data path
			if dataPath == "" {
				dataPath = getDefaultDataPath()
			}

			// Initialize monitor
			mon := monitor.New(monitor.Options{
				DataPath:   dataPath,
				Interval:   time.Duration(interval) * time.Second,
				NoColor:    noColor,
				Continuous: continuous,
			})

			// Start monitoring
			ctx := cmd.Context()
			if err := mon.Start(ctx); err != nil {
				return fmt.Errorf("failed to start monitor: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataPath, "data-path", "", "Path to Claude data directory")
	cmd.Flags().IntVar(&interval, "interval", 5, "Update interval in seconds")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVar(&continuous, "continuous", true, "Run continuously")

	return cmd
}
