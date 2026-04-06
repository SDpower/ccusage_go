package usage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	tokenURL = "https://platform.claude.com/v1/oauth/token"
	clientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
)

// refreshMux 防止同時多次 refresh
var refreshMux sync.Mutex

// refreshTokenRequest 是 refresh token 的 HTTP 請求 body
type refreshTokenRequest struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	Scope        string `json:"scope"`
}

// refreshTokenResponse 是 refresh token 的 HTTP 回應
type refreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds
	Scope        string `json:"scope"`
}

// refreshCredential 使用 refresh token 換取新的 access token
func refreshCredential(cred *oauthCredential) (*oauthCredential, error) {
	return refreshCredentialWithURL(cred, tokenURL)
}

// refreshCredentialWithURL 可指定 URL（用於測試）
func refreshCredentialWithURL(cred *oauthCredential, url string) (*oauthCredential, error) {
	if cred.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	body := refreshTokenRequest{
		GrantType:    "refresh_token",
		RefreshToken: cred.RefreshToken,
		ClientID:     clientID,
		Scope:        strings.Join(cred.Scopes, " "),
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal refresh request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh token returned status %d", resp.StatusCode)
	}

	var tokenResp refreshTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	// 組合新 credential
	newCred := &oauthCredential{
		AccessToken:      tokenResp.AccessToken,
		RefreshToken:     tokenResp.RefreshToken,
		ExpiresAt:        time.Now().UnixMilli() + tokenResp.ExpiresIn*1000,
		Scopes:           cred.Scopes,
		RateLimitTier:    cred.RateLimitTier,
		SubscriptionType: cred.SubscriptionType,
	}

	// 伺服器未回傳新 refresh token 時保留舊的
	if newCred.RefreshToken == "" {
		newCred.RefreshToken = cred.RefreshToken
	}

	// 回應有新 scopes 時更新
	if tokenResp.Scope != "" {
		newCred.Scopes = strings.Split(tokenResp.Scope, " ")
	}

	return newCred, nil
}

// saveCredential 將更新後的 credential 寫回儲存
func saveCredential(cred *oauthCredential) error {
	if runtime.GOOS == "darwin" {
		return saveCredentialToKeychain(cred)
	}
	return saveCredentialToFile(cred)
}

// saveCredentialToKeychain 寫回 macOS Keychain
func saveCredentialToKeychain(cred *oauthCredential) error {
	data := credentialsFile{ClaudeAiOauth: cred}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// -U: 若已存在則更新
	cmd := exec.Command("security", "add-generic-password",
		"-U",
		"-s", "Claude Code-credentials",
		"-a", "Claude Code",
		"-w", string(jsonData),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("keychain write failed: %w (%s)", err, string(output))
	}
	return nil
}

// saveCredentialToFile 寫回 .credentials.json（Linux/Windows）
func saveCredentialToFile(cred *oauthCredential) error {
	configDir := getConfigDir()
	credPath := filepath.Join(configDir, ".credentials.json")

	// 讀取現有檔案以保留其他欄位
	var existing credentialsFile
	if data, err := os.ReadFile(credPath); err == nil {
		json.Unmarshal(data, &existing)
	}
	existing.ClaudeAiOauth = cred

	jsonData, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(credPath, jsonData, 0600)
}

// getValidToken 取得有效的 access token。
// 如果 token 過期，嘗試 refresh。
func getValidToken() (string, error) {
	refreshMux.Lock()
	defer refreshMux.Unlock()

	cred, err := GetOAuthCredential()
	if err != nil {
		return "", err
	}

	// Token 未過期，直接回傳
	if !cred.isExpired() {
		return cred.AccessToken, nil
	}

	// Token 過期，嘗試 refresh
	newCred, err := refreshCredential(cred)
	if err != nil {
		return "", fmt.Errorf("token expired and refresh failed: %w", err)
	}

	// 寫回（失敗不影響本次使用）
	saveCredential(newCred)

	return newCred.AccessToken, nil
}

// forceRefreshToken 強制 refresh（用於 401 重試）
func forceRefreshToken() (string, error) {
	refreshMux.Lock()
	defer refreshMux.Unlock()

	cred, err := GetOAuthCredential()
	if err != nil {
		return "", err
	}

	if cred.RefreshToken == "" {
		return "", fmt.Errorf("no refresh token available for forced refresh")
	}

	newCred, err := refreshCredential(cred)
	if err != nil {
		return "", fmt.Errorf("forced refresh failed: %w", err)
	}

	saveCredential(newCred)

	return newCred.AccessToken, nil
}
