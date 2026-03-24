package commands

import (
	"os"
	"path/filepath"

	"github.com/sdpower/ccusage-go/internal/types"
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

func filterEntriesBySessionID(entries []types.UsageEntry, sessionID string) []types.UsageEntry {
	var filtered []types.UsageEntry
	for _, entry := range entries {
		if entry.SessionID == sessionID {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func filterEntriesBySessionName(entries []types.UsageEntry, sessionName string) []types.UsageEntry {
	if sessionName == "" {
		return entries
	}
	var filtered []types.UsageEntry
	for _, entry := range entries {
		if entry.SessionName == sessionName {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func filterEntriesByDate(entries []types.UsageEntry, since, until string) []types.UsageEntry {
	var filtered []types.UsageEntry
	
	for _, entry := range entries {
		// Use DateKey if available, otherwise format timestamp
		dateStr := entry.DateKey
		if dateStr == "" {
			dateStr = entry.Timestamp.Format("2006-01-02")
		}
		
		// Apply date filter
		if since != "" && dateStr < since {
			continue
		}
		if until != "" && dateStr > until {
			continue
		}
		
		filtered = append(filtered, entry)
	}
	
	return filtered
}
