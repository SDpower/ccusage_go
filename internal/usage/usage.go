package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	usageAPIURL = "https://api.anthropic.com/api/oauth/usage"
	cacheTTL    = 5 * time.Minute
)

// UsageLimitEntry represents a single usage limit tier
type UsageLimitEntry struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

// UsageResponse represents the API response for usage limits
type UsageResponse struct {
	FiveHour       *UsageLimitEntry `json:"five_hour"`
	SevenDay       *UsageLimitEntry `json:"seven_day"`
	SevenDaySonnet *UsageLimitEntry `json:"seven_day_sonnet"`
	SevenDayOpus   *UsageLimitEntry `json:"seven_day_opus"`
}

// Client handles fetching usage limits from the Claude OAuth API
type Client struct {
	httpClient *http.Client
	cache      *UsageResponse
	cacheTime  time.Time
	cacheMux   sync.RWMutex
}

// NewClient creates a new usage client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetUsage returns the current usage limits, using cache if available.
// Returns nil on any error (graceful degradation).
func (c *Client) GetUsage(ctx context.Context) *UsageResponse {
	// Check cache
	c.cacheMux.RLock()
	if c.cache != nil && time.Since(c.cacheTime) < cacheTTL {
		cached := c.cache
		c.cacheMux.RUnlock()
		return cached
	}
	c.cacheMux.RUnlock()

	// Fetch fresh data
	resp, err := c.fetchUsage(ctx)
	if err != nil {
		return nil
	}

	// Update cache
	c.cacheMux.Lock()
	c.cache = resp
	c.cacheTime = time.Now()
	c.cacheMux.Unlock()

	return resp
}

// fetchUsage calls the OAuth usage API, with 401 auto-refresh retry
func (c *Client) fetchUsage(ctx context.Context) (*UsageResponse, error) {
	token, err := getValidToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth token: %w", err)
	}

	resp, err := c.doUsageRequest(ctx, token)
	if err != nil {
		return nil, err
	}

	// 401 → refresh token → 重試一次
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		newToken, refreshErr := forceRefreshToken()
		if refreshErr != nil {
			return nil, fmt.Errorf("usage API returned 401 and refresh failed: %w", refreshErr)
		}
		resp, err = c.doUsageRequest(ctx, newToken)
		if err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage API returned status %d", resp.StatusCode)
	}

	var usageResp UsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&usageResp); err != nil {
		return nil, fmt.Errorf("failed to decode usage response: %w", err)
	}

	return &usageResp, nil
}

// doUsageRequest 執行 usage API HTTP 請求
func (c *Client) doUsageRequest(ctx context.Context, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", usageAPIURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "claude-code/2.1.92")

	return c.httpClient.Do(req)
}

// FormatResetTime formats a reset time string for display in the given timezone.
// Same day: "Resets 2:00am (Asia/Taipei)"
// Different day: "Resets Mar 13 at 10am (Asia/Taipei)"
func FormatResetTime(resetsAt string, loc *time.Location) string {
	t, err := time.Parse(time.RFC3339, resetsAt)
	if err != nil {
		return ""
	}

	localTime := t.In(loc)
	now := time.Now().In(loc)
	tzName := loc.String()

	// Format hour with am/pm
	hour := localTime.Format("3:04pm")
	// Simplify ":00" minutes
	if localTime.Minute() == 0 {
		hour = localTime.Format("3pm")
	}

	if localTime.YearDay() == now.YearDay() && localTime.Year() == now.Year() {
		return fmt.Sprintf("Resets %s (%s)", hour, tzName)
	}

	return fmt.Sprintf("Resets %s at %s (%s)",
		localTime.Format("Jan 2"), hour, tzName)
}
