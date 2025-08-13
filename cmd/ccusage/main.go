package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sdpower/ccusage-go/internal/commands"
	"github.com/spf13/cobra"
)

func main() {
	ctx := context.Background()

	rootCmd := &cobra.Command{
		Use:   "ccusage",
		Short: "Claude Code usage analysis tool",
		Long:  `A CLI tool for analyzing Claude Code usage data from local JSONL files.`,
	}

	rootCmd.AddCommand(
		commands.NewDailyCommand(),
		commands.NewMonthlyCommand(),
		commands.NewWeeklyCommand(),
		commands.NewSessionCommand(),
		commands.NewBlocksCommand(),
		commands.NewMonitorCommand(),
	)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
