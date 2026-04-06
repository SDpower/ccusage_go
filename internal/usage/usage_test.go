package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUsageResponseParsing(t *testing.T) {
	jsonData := `{
		"five_hour": {"utilization": 5.0, "resets_at": "2026-03-11T18:00:00+00:00"},
		"seven_day": {"utilization": 50.0, "resets_at": "2026-03-13T02:00:00+00:00"},
		"seven_day_sonnet": {"utilization": 2.0, "resets_at": "2026-03-13T19:00:00+00:00"},
		"seven_day_opus": null
	}`

	var resp UsageResponse
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.FiveHour == nil {
		t.Fatal("FiveHour should not be nil")
	}
	if resp.FiveHour.Utilization != 5.0 {
		t.Errorf("FiveHour.Utilization = %v, want 5.0", resp.FiveHour.Utilization)
	}
	if resp.FiveHour.ResetsAt != "2026-03-11T18:00:00+00:00" {
		t.Errorf("FiveHour.ResetsAt = %v, want 2026-03-11T18:00:00+00:00", resp.FiveHour.ResetsAt)
	}

	if resp.SevenDay == nil {
		t.Fatal("SevenDay should not be nil")
	}
	if resp.SevenDay.Utilization != 50.0 {
		t.Errorf("SevenDay.Utilization = %v, want 50.0", resp.SevenDay.Utilization)
	}

	if resp.SevenDaySonnet == nil {
		t.Fatal("SevenDaySonnet should not be nil")
	}
	if resp.SevenDaySonnet.Utilization != 2.0 {
		t.Errorf("SevenDaySonnet.Utilization = %v, want 2.0", resp.SevenDaySonnet.Utilization)
	}

	if resp.SevenDayOpus != nil {
		t.Error("SevenDayOpus should be nil")
	}
}

func TestUsageResponseAllNull(t *testing.T) {
	jsonData := `{
		"five_hour": null,
		"seven_day": null,
		"seven_day_sonnet": null,
		"seven_day_opus": null
	}`

	var resp UsageResponse
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.FiveHour != nil {
		t.Error("FiveHour should be nil")
	}
	if resp.SevenDay != nil {
		t.Error("SevenDay should be nil")
	}
	if resp.SevenDaySonnet != nil {
		t.Error("SevenDaySonnet should be nil")
	}
	if resp.SevenDayOpus != nil {
		t.Error("SevenDayOpus should be nil")
	}
}

func TestClientCacheHit(t *testing.T) {
	client := NewClient()

	// Set cache manually
	client.cacheMux.Lock()
	client.cache = &UsageResponse{
		FiveHour: &UsageLimitEntry{Utilization: 10.0, ResetsAt: "2026-03-11T18:00:00+00:00"},
	}
	client.cacheTime = time.Now()
	client.cacheMux.Unlock()

	// Should return cached value without hitting API
	result := client.GetUsage(context.Background())
	if result == nil {
		t.Fatal("expected cached result, got nil")
	}
	if result.FiveHour.Utilization != 10.0 {
		t.Errorf("Utilization = %v, want 10.0", result.FiveHour.Utilization)
	}
}

func TestClientCacheExpiry(t *testing.T) {
	client := NewClient()

	// Set expired cache
	client.cacheMux.Lock()
	client.cache = &UsageResponse{
		FiveHour: &UsageLimitEntry{Utilization: 10.0, ResetsAt: "2026-03-11T18:00:00+00:00"},
	}
	client.cacheTime = time.Now().Add(-6 * time.Minute)
	client.cacheMux.Unlock()

	// With expired cache, GetUsage should try to fetch fresh data.
	// It may succeed (if real credentials exist) or return nil.
	// Just verify it doesn't panic.
	_ = client.GetUsage(context.Background())
}

func TestFormatResetTime(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatalf("failed to load timezone: %v", err)
	}

	tests := []struct {
		name     string
		resetAt  string
		expected string
	}{
		{
			name:     "different day",
			resetAt:  "2026-03-13T02:00:00+00:00",
			expected: "Resets Mar 13 at 10am (Asia/Taipei)",
		},
		{
			name:     "different day with minutes",
			resetAt:  "2026-03-14T06:30:00+00:00",
			expected: "Resets Mar 14 at 2:30pm (Asia/Taipei)",
		},
		{
			name:     "invalid time",
			resetAt:  "invalid",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatResetTime(tt.resetAt, loc)
			if result != tt.expected {
				t.Errorf("FormatResetTime(%q) = %q, want %q", tt.resetAt, result, tt.expected)
			}
		})
	}
}

func TestCredentialsFileParsing(t *testing.T) {
	jsonData := `{
		"claudeAiOauth": {
			"accessToken": "test-access-token",
			"refreshToken": "test-refresh-token",
			"expiresAt": 1234567890,
			"scopes": ["scope1", "scope2"],
			"rateLimitTier": "tier1",
			"subscriptionType": "pro"
		}
	}`

	var creds credentialsFile
	if err := json.Unmarshal([]byte(jsonData), &creds); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if creds.ClaudeAiOauth == nil {
		t.Fatal("ClaudeAiOauth should not be nil")
	}
	if creds.ClaudeAiOauth.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %v, want test-access-token", creds.ClaudeAiOauth.AccessToken)
	}
	if creds.ClaudeAiOauth.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %v, want test-refresh-token", creds.ClaudeAiOauth.RefreshToken)
	}
	if creds.ClaudeAiOauth.ExpiresAt != 1234567890 {
		t.Errorf("ExpiresAt = %v, want 1234567890", creds.ClaudeAiOauth.ExpiresAt)
	}
}

func TestGetTokenFromEnv(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "env-token-123")

	token, ok := getTokenFromEnv()
	if !ok {
		t.Error("expected ok=true")
	}
	if token != "env-token-123" {
		t.Errorf("token = %v, want env-token-123", token)
	}
}

func TestGetTokenFromEnvEmpty(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	_, ok := getTokenFromEnv()
	if ok {
		t.Error("expected ok=false for empty env var")
	}
}

func TestGetCredentialFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	credData := `{
		"claudeAiOauth": {
			"accessToken": "file-token-456",
			"refreshToken": "refresh-token",
			"expiresAt": 9999999999999
		}
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, ".credentials.json"), []byte(credData), 0600); err != nil {
		t.Fatalf("failed to write credentials file: %v", err)
	}

	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)

	cred, err := getCredentialFromFile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.AccessToken != "file-token-456" {
		t.Errorf("AccessToken = %v, want file-token-456", cred.AccessToken)
	}
	if cred.RefreshToken != "refresh-token" {
		t.Errorf("RefreshToken = %v, want refresh-token", cred.RefreshToken)
	}
}

func TestGetCredentialFromFileMissing(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())

	_, err := getCredentialFromFile()
	if err == nil {
		t.Error("expected error for missing credentials file")
	}
}

func TestGetCredentialFromFileNoAccessToken(t *testing.T) {
	tmpDir := t.TempDir()
	credData := `{"claudeAiOauth": {"refreshToken": "refresh-only"}}`
	if err := os.WriteFile(filepath.Join(tmpDir, ".credentials.json"), []byte(credData), 0600); err != nil {
		t.Fatalf("failed to write credentials file: %v", err)
	}

	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)

	_, err := getCredentialFromFile()
	if err == nil {
		t.Error("expected error for missing access token")
	}
}

// --- 新增測試：isExpired ---

func TestIsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt int64
		want      bool
	}{
		{"未過期", time.Now().UnixMilli() + 3600*1000, false},
		{"已過期", time.Now().UnixMilli() - 1000, true},
		{"剛好過期", time.Now().UnixMilli(), true},
		{"無過期時間", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cred := &oauthCredential{ExpiresAt: tt.expiresAt}
			if got := cred.isExpired(); got != tt.want {
				t.Errorf("isExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- 新增測試：refreshCredential ---

func TestRefreshCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("missing Content-Type header")
		}

		var req refreshTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.GrantType != "refresh_token" {
			t.Errorf("grant_type = %v, want refresh_token", req.GrantType)
		}
		if req.RefreshToken != "old-refresh" {
			t.Errorf("refresh_token = %v, want old-refresh", req.RefreshToken)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(refreshTokenResponse{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			ExpiresIn:    3600,
			Scope:        "user:profile user:inference",
		})
	}))
	defer server.Close()

	// 暫時替換 tokenURL（透過覆蓋 refreshCredentialWithURL）
	cred := &oauthCredential{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		Scopes:       []string{"user:profile", "user:inference"},
	}

	newCred, err := refreshCredentialWithURL(cred, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newCred.AccessToken != "new-access-token" {
		t.Errorf("AccessToken = %v, want new-access-token", newCred.AccessToken)
	}
	if newCred.RefreshToken != "new-refresh-token" {
		t.Errorf("RefreshToken = %v, want new-refresh-token", newCred.RefreshToken)
	}
	if newCred.isExpired() {
		t.Error("new credential should not be expired")
	}
}

func TestRefreshCredentialNoRefreshToken(t *testing.T) {
	cred := &oauthCredential{AccessToken: "access-only"}
	_, err := refreshCredential(cred)
	if err == nil {
		t.Error("expected error for missing refresh token")
	}
}

func TestRefreshCredentialServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cred := &oauthCredential{
		RefreshToken: "some-refresh",
		Scopes:       []string{"user:profile"},
	}

	_, err := refreshCredentialWithURL(cred, server.URL)
	if err == nil {
		t.Error("expected error for server error response")
	}
}

func TestRefreshCredentialKeepsOldRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 伺服器未回傳新 refresh_token
		fmt.Fprintf(w, `{"access_token": "new-at", "expires_in": 3600, "scope": "user:profile"}`)
	}))
	defer server.Close()

	cred := &oauthCredential{
		RefreshToken: "original-refresh",
		Scopes:       []string{"user:profile"},
	}

	newCred, err := refreshCredentialWithURL(cred, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newCred.RefreshToken != "original-refresh" {
		t.Errorf("RefreshToken = %v, want original-refresh (should keep old)", newCred.RefreshToken)
	}
}

// --- 新增測試：saveCredentialToFile ---

func TestSaveCredentialToFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)

	cred := &oauthCredential{
		AccessToken:  "saved-token",
		RefreshToken: "saved-refresh",
		ExpiresAt:    9999999999999,
		Scopes:       []string{"user:profile"},
	}

	if err := saveCredentialToFile(cred); err != nil {
		t.Fatalf("saveCredentialToFile failed: %v", err)
	}

	// 驗證檔案內容
	data, err := os.ReadFile(filepath.Join(tmpDir, ".credentials.json"))
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}

	var saved credentialsFile
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("failed to parse saved file: %v", err)
	}

	if saved.ClaudeAiOauth.AccessToken != "saved-token" {
		t.Errorf("saved AccessToken = %v, want saved-token", saved.ClaudeAiOauth.AccessToken)
	}
}
