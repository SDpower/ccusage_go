package types

import (
	"time"
)

// CacheMissReason captures Anthropic's diagnostics.cache_miss_reason payload
// (added 2026-Q2). Useful for explaining sudden cost spikes.
type CacheMissReason struct {
	Type                    string `json:"type"`
	CacheMissedInputTokens  int    `json:"cache_missed_input_tokens"`
}

type UsageEntry struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	DateKey      string                 `json:"date_key,omitempty"` // YYYY-MM-DD format in specified timezone
	ProjectPath  string                 `json:"project_path"`
	Model        string                 `json:"model"`
	InputTokens  int                    `json:"input_tokens"`
	OutputTokens int                    `json:"output_tokens"`
	// First-class cache token fields (replaces Raw["cache_*"] indirection)
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	TotalTokens  int                    `json:"total_tokens"`
	Cost         float64                `json:"cost,omitempty"`
	APICost        float64                `json:"api_cost,omitempty"`  // input + output only, no cache
	CacheCreateCost float64               `json:"cache_create_cost,omitempty"`
	CacheReadCost  float64                `json:"cache_read_cost,omitempty"`
	// server_tool_use billing fields (Anthropic web tools)
	WebSearchRequests int     `json:"web_search_requests,omitempty"`
	WebFetchRequests  int     `json:"web_fetch_requests,omitempty"`
	WebSearchCost     float64 `json:"web_search_cost,omitempty"`
	WebFetchCost      float64 `json:"web_fetch_cost,omitempty"`
	SessionID      string                 `json:"session_id"`
	SessionName  string                 `json:"session_name,omitempty"`
	BlockType    string                 `json:"block_type,omitempty"`
	// Conversation-tree metadata; loader keeps these for breakdown reporting
	// but never filters on IsSidechain (sub-agent calls are independently billed).
	IsSidechain bool `json:"is_sidechain,omitempty"`
	IsMeta      bool `json:"is_meta,omitempty"`
	// Optional Anthropic cache-miss diagnostic
	CacheMissReason *CacheMissReason       `json:"cache_miss_reason,omitempty"`
	SourceFile      string                 `json:"-"`
	Raw             map[string]interface{} `json:"-"`
}

type UsageReport struct {
	Period      string       `json:"period"`
	StartTime   time.Time    `json:"start_time"`
	EndTime     time.Time    `json:"end_time"`
	TotalCost   float64      `json:"total_cost"`
	TotalTokens int          `json:"total_tokens"`
	Entries     []UsageEntry `json:"entries"`
	Summary     UsageSummary `json:"summary"`
}

type UsageSummary struct {
	TotalRequests int            `json:"total_requests"`
	TotalCost     float64        `json:"total_cost"`
	TotalTokens   int            `json:"total_tokens"`
	InputTokens   int            `json:"input_tokens"`
	OutputTokens  int            `json:"output_tokens"`
	Models        map[string]int `json:"models"`
	Projects      map[string]int `json:"projects"`
	AverageCost   float64        `json:"average_cost"`
}

type SessionInfo struct {
	SessionID            string        `json:"session_id"`
	StartTime            time.Time     `json:"start_time"`
	EndTime              time.Time     `json:"end_time"`
	Duration             time.Duration `json:"duration"`
	TotalCost            float64       `json:"total_cost"`
	TotalAPICost         float64       `json:"total_api_cost"`
	TotalTokens          int           `json:"total_tokens"`
	InputTokens          int           `json:"input_tokens"`
	OutputTokens         int           `json:"output_tokens"`
	CacheCreationTokens  int           `json:"cache_creation_tokens"`
	CacheCreateCost      float64       `json:"cache_create_cost"`
	CacheReadTokens      int           `json:"cache_read_tokens"`
	CacheReadCost        float64       `json:"cache_read_cost"`
	RequestCount         int           `json:"request_count"`
	ProjectPath          string        `json:"project_path"`
	SessionName          string        `json:"session_name,omitempty"`
	SessionIDs           []string      `json:"session_ids,omitempty"`
	SourceFiles          []string      `json:"source_files,omitempty"`
	ModelsUsed           []string      `json:"models_used"`
	LastActivity         time.Time     `json:"last_activity"`
}

type SourceFileStat struct {
	FilePath          string    `json:"file_path"`
	InputTokens       int       `json:"input_tokens"`
	OutputTokens      int       `json:"output_tokens"`
	CacheCreateTokens int       `json:"cache_create_tokens"`
	CacheCreateCost   float64   `json:"cache_create_cost"`
	CacheReadTokens   int       `json:"cache_read_tokens"`
	CacheReadCost     float64   `json:"cache_read_cost"`
	TotalTokens       int       `json:"total_tokens"`
	Cost              float64   `json:"cost"`
	APICost           float64   `json:"api_cost"`
	ModelsUsed        []string  `json:"models_used"`
	LastActivity      time.Time `json:"last_activity"`
	EntryCount        int       `json:"entry_count"`
}

type BlockInfo struct {
	BlockType   string    `json:"block_type"`
	Count       int       `json:"count"`
	TotalTokens int       `json:"total_tokens"`
	TotalCost   float64   `json:"total_cost"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}
