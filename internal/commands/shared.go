package commands

import (
	"os"
	"path/filepath"
)

func getDefaultDataPath() string {
	// Check environment variable first
	if claudeConfigDir := os.Getenv("CLAUDE_CONFIG_DIR"); claudeConfigDir != "" {
		return claudeConfigDir
	}

	// Default paths based on Claude Code configuration
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "."
	}

	// Check ~/.claude/projects first
	claudePath := filepath.Join(homeDir, ".claude", "projects")
	if _, err := os.Stat(claudePath); err == nil {
		return claudePath
	}

	// Check ~/.config/claude/projects
	configPath := filepath.Join(homeDir, ".config", "claude", "projects")
	if _, err := os.Stat(configPath); err == nil {
		return configPath
	}

	// Fall back to ~/.claude/projects as default
	return claudePath
}
