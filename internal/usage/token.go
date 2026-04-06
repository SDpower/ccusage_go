package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// credentialsFile represents the structure of .credentials.json
type credentialsFile struct {
	ClaudeAiOauth *oauthCredential `json:"claudeAiOauth"`
}

type oauthCredential struct {
	AccessToken      string   `json:"accessToken"`
	RefreshToken     string   `json:"refreshToken"`
	ExpiresAt        int64    `json:"expiresAt"` // Unix timestamp ms
	Scopes           []string `json:"scopes"`
	RateLimitTier    string   `json:"rateLimitTier"`
	SubscriptionType string   `json:"subscriptionType"`
}

// isExpired 檢查 token 是否已過期
func (c *oauthCredential) isExpired() bool {
	if c.ExpiresAt == 0 {
		return false // 無過期時間，視為不過期（env token）
	}
	return c.ExpiresAt <= time.Now().UnixMilli()
}

// GetOAuthCredential 取得完整 OAuth credential。
// 優先級：環境變數 > Keychain (macOS) > .credentials.json
func GetOAuthCredential() (*oauthCredential, error) {
	// 1. 環境變數（只有 token，無 refresh 能力）
	if token, ok := getTokenFromEnv(); ok {
		return &oauthCredential{AccessToken: token}, nil
	}

	// 2. macOS Keychain（v2.x 預設儲存位置）
	if runtime.GOOS == "darwin" {
		if cred, err := getCredentialFromKeychain(); err == nil && cred != nil {
			return cred, nil
		}
	}

	// 3. .credentials.json（Linux/Windows 或 macOS fallback）
	if cred, err := getCredentialFromFile(); err == nil && cred != nil {
		return cred, nil
	}

	return nil, fmt.Errorf("no OAuth credential found")
}

// GetOAuthToken 取得 access token string（向後相容包裝）
func GetOAuthToken() (string, error) {
	cred, err := GetOAuthCredential()
	if err != nil {
		return "", err
	}
	return cred.AccessToken, nil
}

// getTokenFromEnv reads token from CLAUDE_CODE_OAUTH_TOKEN environment variable
func getTokenFromEnv() (string, bool) {
	token := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN")
	return token, token != ""
}

// getCredentialFromFile 從 .credentials.json 讀取完整 credential
func getCredentialFromFile() (*oauthCredential, error) {
	configDir := getConfigDir()
	credPath := filepath.Join(configDir, ".credentials.json")
	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil, err
	}

	var creds credentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	if creds.ClaudeAiOauth == nil || creds.ClaudeAiOauth.AccessToken == "" {
		return nil, fmt.Errorf("no access token in credentials file")
	}

	return creds.ClaudeAiOauth, nil
}

// getCredentialFromKeychain 從 macOS Keychain 讀取完整 credential
func getCredentialFromKeychain() (*oauthCredential, error) {
	cmd := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil, fmt.Errorf("empty keychain entry")
	}

	// Keychain 儲存完整 credentials JSON
	var creds credentialsFile
	if err := json.Unmarshal([]byte(trimmed), &creds); err == nil {
		if creds.ClaudeAiOauth != nil && creds.ClaudeAiOauth.AccessToken != "" {
			return creds.ClaudeAiOauth, nil
		}
	}

	// 非 JSON，視為裸 token
	return &oauthCredential{AccessToken: trimmed}, nil
}

// getConfigDir 回傳 Claude config 目錄路徑
func getConfigDir() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".claude")
	}
	return filepath.Join(home, ".claude")
}
