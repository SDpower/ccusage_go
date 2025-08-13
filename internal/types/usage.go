package types

import (
	"time"
)

type UsageEntry struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	ProjectPath  string                 `json:"project_path"`
	Model        string                 `json:"model"`
	InputTokens  int                    `json:"input_tokens"`
	OutputTokens int                    `json:"output_tokens"`
	TotalTokens  int                    `json:"total_tokens"`
	Cost         float64                `json:"cost,omitempty"`
	SessionID    string                 `json:"session_id"`
	BlockType    string                 `json:"block_type,omitempty"`
	Raw          map[string]interface{} `json:"-"`
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
	SessionID    string        `json:"session_id"`
	StartTime    time.Time     `json:"start_time"`
	EndTime      time.Time     `json:"end_time"`
	Duration     time.Duration `json:"duration"`
	TotalCost    float64       `json:"total_cost"`
	TotalTokens  int           `json:"total_tokens"`
	RequestCount int           `json:"request_count"`
	ProjectPath  string        `json:"project_path"`
}

type BlockInfo struct {
	BlockType   string    `json:"block_type"`
	Count       int       `json:"count"`
	TotalTokens int       `json:"total_tokens"`
	TotalCost   float64   `json:"total_cost"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}
