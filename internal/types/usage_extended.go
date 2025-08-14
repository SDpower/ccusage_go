package types

import "time"

// ExtendedUsageEntry includes cache-related fields
type ExtendedUsageEntry struct {
	UsageEntry
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// DailyAggregation represents aggregated daily usage
type DailyAggregation struct {
	Date                     time.Time         `json:"date"`
	Models                   []string          `json:"models"`
	InputTokens              int               `json:"input_tokens"`
	OutputTokens             int               `json:"output_tokens"`
	CacheCreationInputTokens int               `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int               `json:"cache_read_input_tokens"`
	TotalTokens              int               `json:"total_tokens"`
	TotalCost                float64           `json:"total_cost"`
	Entries                  []UsageEntry      `json:"entries"`
	ModelBreakdown           map[string]*ModelUsage `json:"model_breakdown"`
}

// ModelUsage represents usage per model
type ModelUsage struct {
	Model                    string  `json:"model"`
	InputTokens              int     `json:"input_tokens"`
	OutputTokens             int     `json:"output_tokens"`
	CacheCreationInputTokens int     `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int     `json:"cache_read_input_tokens"`
	TotalTokens              int     `json:"total_tokens"`
	Cost                     float64 `json:"cost"`
	RequestCount             int     `json:"request_count"`
}