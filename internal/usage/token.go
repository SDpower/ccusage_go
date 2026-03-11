package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// credentialsFile represents the structure of .credentials.json
type credentialsFile struct {
	ClaudeAiOauth *oauthCredential `json:"claudeAiOauth"`
}

type oauthCredential struct {
	AccessToken      string   `json:"accessToken"`
	RefreshToken     string   `json:"refreshToken"`
	ExpiresAt        int64    `json:"expiresAt"`
	Scopes           []string `json:"scopes"`
	RateLimitTier    string   `json:"rateLimitTier"`
	SubscriptionType string   `json:"subscriptionType"`
}

// GetOAuthToken retrieves the Claude OAuth token from available sources.
// Priority: environment variable > credentials file > macOS Keychain
func GetOAuthToken() (string, error) {
	// 1. Environment variable
	if token, ok := getTokenFromEnv(); ok {
		return token, nil
	}

	// 2. Credentials file
	if token, err := getTokenFromFile(); err == nil && token != "" {
		return token, nil
	}

	// 3. macOS Keychain (darwin only)
	if runtime.GOOS == "darwin" {
		if token, err := getTokenFromKeychain(); err == nil && token != "" {
			return token, nil
		}
	}

	return "", fmt.Errorf("no OAuth token found")
}

// getTokenFromEnv reads token from CLAUDE_CODE_OAUTH_TOKEN environment variable
func getTokenFromEnv() (string, bool) {
	token := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN")
	return token, token != ""
}

// getTokenFromFile reads token from .credentials.json
func getTokenFromFile() (string, error) {
	// Try CLAUDE_CONFIG_DIR first, then default ~/.claude
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(home, ".claude")
	}

	credPath := filepath.Join(configDir, ".credentials.json")
	data, err := os.ReadFile(credPath)
	if err != nil {
		return "", err
	}

	var creds credentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", err
	}

	if creds.ClaudeAiOauth == nil || creds.ClaudeAiOauth.AccessToken == "" {
		return "", fmt.Errorf("no access token in credentials file")
	}

	return creds.ClaudeAiOauth.AccessToken, nil
}

// getTokenFromKeychain reads token from macOS Keychain
func getTokenFromKeychain() (string, error) {
	cmd := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	token := string(output)
	if token == "" {
		return "", fmt.Errorf("empty keychain entry")
	}

	// Try parsing as JSON (keychain may store the full credentials JSON)
	var creds credentialsFile
	if err := json.Unmarshal(output, &creds); err == nil {
		if creds.ClaudeAiOauth != nil && creds.ClaudeAiOauth.AccessToken != "" {
			return creds.ClaudeAiOauth.AccessToken, nil
		}
	}

	// If not JSON, treat as raw token (trimming whitespace)
	return strings.TrimSpace(token), nil
}
