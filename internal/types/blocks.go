package types

import (
	"time"
)

// TokenCounts represents aggregated token counts for different token types
type TokenCounts struct {
	InputTokens               int `json:"input_tokens"`
	OutputTokens              int `json:"output_tokens"`
	CacheCreationInputTokens  int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens      int `json:"cache_read_input_tokens"`
}

// SessionBlock represents a session block (typically 5-hour billing period) with usage data
type SessionBlock struct {
	ID                   string      `json:"id"`                      // ISO string of block start time
	StartTime            time.Time   `json:"start_time"`               // Block start time
	EndTime              time.Time   `json:"end_time"`                 // Block end time (startTime + 5 hours for normal blocks)
	ActualEndTime        *time.Time  `json:"actual_end_time,omitempty"` // Last activity in block
	IsActive             bool        `json:"is_active"`                // Whether this block is currently active
	IsGap                bool        `json:"is_gap"`                   // True if this is a gap block
	Entries              []UsageEntry `json:"entries"`                  // Usage entries in this block
	TokenCounts          TokenCounts `json:"token_counts"`             // Aggregated token counts
	CostUSD              float64     `json:"cost_usd"`                 // Total cost in USD
	Models               []string    `json:"models"`                   // Unique models used
	UsageLimitResetTime  *time.Time  `json:"usage_limit_reset_time,omitempty"` // Claude API usage limit reset time
}

// BurnRate represents usage burn rate calculations
type BurnRate struct {
	TokensPerMinute             float64 `json:"tokens_per_minute"`
	TokensPerMinuteForIndicator float64 `json:"tokens_per_minute_for_indicator"` // Non-cache tokens for threshold indicators
	CostPerHour                 float64 `json:"cost_per_hour"`
}

// ProjectedUsage represents projected usage for remaining time in a session block
type ProjectedUsage struct {
	TotalTokens      int     `json:"total_tokens"`
	TotalCost        float64 `json:"total_cost"`
	RemainingMinutes float64 `json:"remaining_minutes"`
}

// TokenLimitStatus represents the status of token usage against a limit
type TokenLimitStatus struct {
	Limit          int     `json:"limit"`
	ProjectedUsage int     `json:"projected_usage"`
	PercentUsed    float64 `json:"percent_used"`
	Status         string  `json:"status"` // "ok", "warning", or "exceeds"
}

// GetTotalTokens calculates the total number of tokens from TokenCounts
func (tc TokenCounts) GetTotal() int {
	return tc.InputTokens + tc.OutputTokens + tc.CacheCreationInputTokens + tc.CacheReadInputTokens
}