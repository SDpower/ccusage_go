# 類型系統設計

## 1. 模組概述

類型系統模組定義了整個應用程式的核心資料結構和類型，確保類型安全和資料一致性。這個模組使用 Go 的強類型特性，實作類似 TypeScript branded types 的類型安全機制。

### 1.1 主要功能
- 核心資料結構定義
- 類型安全保證
- 資料驗證機制
- 介面定義
- 自定義類型別名
- 序列化/反序列化支援

### 1.2 對應 TypeScript 模組
- `_types.ts` → `types/core.go`
- 各種 schema 定義 → `types/schemas.go`
- 類型工廠函數 → `types/factories.go`

## 2. 類型系統架構

```
┌────────────────────────────────────────────┐
│              Core Types                     │
│  (基礎類型：時間、ID、金額等)                 │
└────────────────┬───────────────────────────┘
                 │
┌────────────────▼───────────────────────────┐
│           Domain Types                      │
│  (領域類型：使用記錄、報告、會話等)            │
└────────────────┬───────────────────────────┘
                 │
┌────────────────▼───────────────────────────┐
│           Aggregate Types                   │
│  (聚合類型：統計、總計、趨勢等)              │
└────────────────┬───────────────────────────┘
                 │
┌────────────────▼───────────────────────────┐
│           Interface Types                   │
│  (介面定義：Formatter、Calculator等)         │
└────────────────────────────────────────────┘
```

## 3. 核心類型定義

### 3.1 品牌類型實作

```go
package types

import (
    "encoding/json"
    "fmt"
    "regexp"
    "time"
)

// 品牌類型 - 使用結構體包裝實現類型安全
type (
    ModelName    struct{ value string }
    SessionID    struct{ value string }
    RequestID    struct{ value string }
    MessageID    struct{ value string }
    ProjectPath  struct{ value string }
    DailyDate    struct{ value string }
    MonthlyDate  struct{ value string }
    WeeklyDate   struct{ value string }
    FilterDate   struct{ value string }
    ISOTimestamp struct{ value time.Time }
    Version      struct{ value string }
)

// ModelName 方法
func NewModelName(value string) (ModelName, error) {
    if value == "" {
        return ModelName{}, fmt.Errorf("model name cannot be empty")
    }
    return ModelName{value: value}, nil
}

func (m ModelName) String() string {
    return m.value
}

func (m ModelName) MarshalJSON() ([]byte, error) {
    return json.Marshal(m.value)
}

func (m *ModelName) UnmarshalJSON(data []byte) error {
    var value string
    if err := json.Unmarshal(data, &value); err != nil {
        return err
    }
    
    model, err := NewModelName(value)
    if err != nil {
        return err
    }
    
    *m = model
    return nil
}

// SessionID 方法
func NewSessionID(value string) (SessionID, error) {
    if value == "" {
        return SessionID{}, fmt.Errorf("session ID cannot be empty")
    }
    
    // 驗證格式
    if !isValidUUID(value) {
        return SessionID{}, fmt.Errorf("invalid session ID format")
    }
    
    return SessionID{value: value}, nil
}

func (s SessionID) String() string {
    return s.value
}

func isValidUUID(s string) bool {
    pattern := `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
    matched, _ := regexp.MatchString(pattern, s)
    return matched
}
```

### 3.2 日期類型

```go
// DailyDate 實作
func NewDailyDate(value string) (DailyDate, error) {
    pattern := `^\d{4}-\d{2}-\d{2}$`
    matched, _ := regexp.MatchString(pattern, value)
    
    if !matched {
        return DailyDate{}, fmt.Errorf("date must be in YYYY-MM-DD format")
    }
    
    // 驗證日期有效性
    _, err := time.Parse("2006-01-02", value)
    if err != nil {
        return DailyDate{}, fmt.Errorf("invalid date: %w", err)
    }
    
    return DailyDate{value: value}, nil
}

func (d DailyDate) String() string {
    return d.value
}

func (d DailyDate) Time() (time.Time, error) {
    return time.Parse("2006-01-02", d.value)
}

func (d DailyDate) AddDays(days int) (DailyDate, error) {
    t, err := d.Time()
    if err != nil {
        return DailyDate{}, err
    }
    
    newTime := t.AddDate(0, 0, days)
    return NewDailyDate(newTime.Format("2006-01-02"))
}

// MonthlyDate 實作
func NewMonthlyDate(value string) (MonthlyDate, error) {
    pattern := `^\d{4}-\d{2}$`
    matched, _ := regexp.MatchString(pattern, value)
    
    if !matched {
        return MonthlyDate{}, fmt.Errorf("date must be in YYYY-MM format")
    }
    
    return MonthlyDate{value: value}, nil
}

func (m MonthlyDate) String() string {
    return m.value
}

func (m MonthlyDate) StartOfMonth() (time.Time, error) {
    t, err := time.Parse("2006-01", m.value)
    if err != nil {
        return time.Time{}, err
    }
    return t, nil
}

func (m MonthlyDate) EndOfMonth() (time.Time, error) {
    start, err := m.StartOfMonth()
    if err != nil {
        return time.Time{}, err
    }
    
    return start.AddDate(0, 1, -1), nil
}
```

## 4. 領域類型

### 4.1 使用記錄類型

```go
type UsageEntry struct {
    Timestamp           ISOTimestamp `json:"timestamp"`
    Model              ModelName    `json:"model"`
    InputTokens        int          `json:"input_tokens"`
    OutputTokens       int          `json:"output_tokens"`
    CacheCreationTokens int         `json:"cache_creation_tokens"`
    CacheReadTokens    int          `json:"cache_read_tokens"`
    Cost               float64      `json:"cost"`
    SessionID          SessionID    `json:"session_id"`
    RequestID          RequestID    `json:"request_id"`
    MessageID          MessageID    `json:"message_id,omitempty"`
    ProjectPath        ProjectPath  `json:"project_path,omitempty"`
    Version            Version      `json:"version,omitempty"`
}

func (u *UsageEntry) Validate() error {
    if u.InputTokens < 0 || u.OutputTokens < 0 {
        return fmt.Errorf("negative token count")
    }
    
    if u.Cost < 0 {
        return fmt.Errorf("negative cost")
    }
    
    return nil
}

func (u *UsageEntry) TotalTokens() int {
    return u.InputTokens + u.OutputTokens + 
           u.CacheCreationTokens + u.CacheReadTokens
}

func (u *UsageEntry) EffectiveCost() float64 {
    // 如果沒有預計算的成本，返回0
    if u.Cost == 0 {
        return 0
    }
    return u.Cost
}
```

### 4.2 報告類型

```go
type DailyUsage struct {
    Date            DailyDate        `json:"date"`
    InputTokens     int             `json:"input_tokens"`
    OutputTokens    int             `json:"output_tokens"`
    CacheTokens     int             `json:"cache_creation_tokens"`
    CacheReadTokens int             `json:"cache_read_tokens"`
    TotalTokens     int             `json:"total_tokens"`
    TotalCost       float64         `json:"total_cost"`
    ModelsUsed      []ModelName     `json:"models_used"`
    ModelBreakdowns []ModelBreakdown `json:"model_breakdowns,omitempty"`
    Project         *ProjectPath     `json:"project,omitempty"`
}

type ModelBreakdown struct {
    Model           ModelName `json:"model"`
    InputTokens     int      `json:"input_tokens"`
    OutputTokens    int      `json:"output_tokens"`
    CacheTokens     int      `json:"cache_tokens"`
    CacheReadTokens int      `json:"cache_read_tokens"`
    Cost            float64  `json:"cost"`
    RequestCount    int      `json:"request_count"`
}

type MonthlyUsage struct {
    Month           MonthlyDate     `json:"month"`
    InputTokens     int            `json:"input_tokens"`
    OutputTokens    int            `json:"output_tokens"`
    CacheTokens     int            `json:"cache_creation_tokens"`
    CacheReadTokens int            `json:"cache_read_tokens"`
    TotalTokens     int            `json:"total_tokens"`
    TotalCost       float64        `json:"total_cost"`
    DaysActive      int            `json:"days_active"`
    ModelsUsed      []ModelName    `json:"models_used"`
    DailyAverage    float64        `json:"daily_average"`
    Project         *ProjectPath    `json:"project,omitempty"`
}

type SessionUsage struct {
    SessionID       SessionID       `json:"session_id"`
    StartTime       time.Time      `json:"start_time"`
    EndTime         time.Time      `json:"end_time"`
    Duration        time.Duration  `json:"duration"`
    InputTokens     int           `json:"input_tokens"`
    OutputTokens    int           `json:"output_tokens"`
    TotalCost       float64       `json:"total_cost"`
    ModelsUsed      []ModelName   `json:"models_used"`
    RequestCount    int           `json:"request_count"`
    Project         *ProjectPath   `json:"project,omitempty"`
}
```

### 4.3 會話區塊類型

```go
type SessionBlock struct {
    ID              string        `json:"id"`
    StartTime       time.Time    `json:"start_time"`
    EndTime         time.Time    `json:"end_time"`
    ActualEndTime   *time.Time   `json:"actual_end_time,omitempty"`
    InputTokens     int          `json:"input_tokens"`
    OutputTokens    int          `json:"output_tokens"`
    CacheTokens     int          `json:"cache_tokens"`
    CacheReadTokens int          `json:"cache_read_tokens"`
    TotalTokens     int          `json:"total_tokens"`
    TotalCost       float64      `json:"total_cost"`
    ModelsUsed      []ModelName  `json:"models_used"`
    RequestCount    int          `json:"request_count"`
    IsActive        bool         `json:"is_active"`
    IsGap           bool         `json:"is_gap,omitempty"`
    TimeRemaining   time.Duration `json:"time_remaining,omitempty"`
    Project         *ProjectPath  `json:"project,omitempty"`
}

func (sb *SessionBlock) Duration() time.Duration {
    if sb.ActualEndTime != nil {
        return sb.ActualEndTime.Sub(sb.StartTime)
    }
    return sb.EndTime.Sub(sb.StartTime)
}

func (sb *SessionBlock) IsWithinWarningThreshold(threshold float64) bool {
    return sb.TotalCost > threshold
}

func (sb *SessionBlock) PercentageUsed() float64 {
    if !sb.IsActive {
        return 100.0
    }
    
    elapsed := time.Since(sb.StartTime)
    total := sb.EndTime.Sub(sb.StartTime)
    
    if total == 0 {
        return 0
    }
    
    return (elapsed.Seconds() / total.Seconds()) * 100
}
```

## 5. 聚合類型

### 5.1 統計類型

```go
type Statistics struct {
    Count      int     `json:"count"`
    Sum        float64 `json:"sum"`
    Average    float64 `json:"average"`
    Min        float64 `json:"min"`
    Max        float64 `json:"max"`
    StdDev     float64 `json:"std_dev"`
    Median     float64 `json:"median"`
    Percentile95 float64 `json:"p95"`
}

func CalculateStatistics(values []float64) Statistics {
    if len(values) == 0 {
        return Statistics{}
    }
    
    stats := Statistics{
        Count: len(values),
        Min:   values[0],
        Max:   values[0],
    }
    
    // 計算總和
    for _, v := range values {
        stats.Sum += v
        if v < stats.Min {
            stats.Min = v
        }
        if v > stats.Max {
            stats.Max = v
        }
    }
    
    stats.Average = stats.Sum / float64(stats.Count)
    
    // 計算標準差
    var variance float64
    for _, v := range values {
        variance += math.Pow(v-stats.Average, 2)
    }
    stats.StdDev = math.Sqrt(variance / float64(stats.Count))
    
    // 計算中位數和百分位數
    sorted := make([]float64, len(values))
    copy(sorted, values)
    sort.Float64s(sorted)
    
    stats.Median = sorted[len(sorted)/2]
    stats.Percentile95 = sorted[int(float64(len(sorted))*0.95)]
    
    return stats
}
```

### 5.2 趨勢類型

```go
type Trend struct {
    Direction   TrendDirection `json:"direction"`
    Slope       float64       `json:"slope"`
    Correlation float64       `json:"correlation"`
    Prediction  float64       `json:"prediction"`
    Confidence  float64       `json:"confidence"`
}

type TrendDirection string

const (
    TrendIncreasing TrendDirection = "increasing"
    TrendDecreasing TrendDirection = "decreasing"
    TrendStable     TrendDirection = "stable"
)

func CalculateTrend(dataPoints []DataPoint) Trend {
    if len(dataPoints) < 2 {
        return Trend{Direction: TrendStable}
    }
    
    // 線性回歸計算
    var sumX, sumY, sumXY, sumX2 float64
    n := float64(len(dataPoints))
    
    for i, point := range dataPoints {
        x := float64(i)
        y := point.Value
        
        sumX += x
        sumY += y
        sumXY += x * y
        sumX2 += x * x
    }
    
    // 計算斜率
    slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
    
    // 計算相關係數
    meanY := sumY / n
    var ssTotal, ssResidual float64
    
    for i, point := range dataPoints {
        x := float64(i)
        y := point.Value
        yPred := slope*x + (sumY-slope*sumX)/n
        
        ssTotal += math.Pow(y-meanY, 2)
        ssResidual += math.Pow(y-yPred, 2)
    }
    
    correlation := math.Sqrt(1 - ssResidual/ssTotal)
    
    // 判斷趨勢方向
    var direction TrendDirection
    if math.Abs(slope) < 0.01 {
        direction = TrendStable
    } else if slope > 0 {
        direction = TrendIncreasing
    } else {
        direction = TrendDecreasing
    }
    
    // 預測下一個值
    nextX := float64(len(dataPoints))
    prediction := slope*nextX + (sumY-slope*sumX)/n
    
    return Trend{
        Direction:   direction,
        Slope:       slope,
        Correlation: correlation,
        Prediction:  prediction,
        Confidence:  correlation, // 簡化：使用相關係數作為信心度
    }
}

type DataPoint struct {
    Time  time.Time `json:"time"`
    Value float64   `json:"value"`
    Label string    `json:"label,omitempty"`
}
```

## 6. 介面定義

### 6.1 核心介面

```go
// Validator 介面
type Validator interface {
    Validate() error
}

// Formatter 介面
type Formatter interface {
    Format() string
}

// Calculator 介面
type Calculator interface {
    Calculate() (float64, error)
}

// Aggregator 介面
type Aggregator interface {
    Aggregate(entries []UsageEntry) interface{}
}

// Filter 介面
type Filter interface {
    Apply(entries []UsageEntry) []UsageEntry
}

// Sorter 介面
type Sorter interface {
    Sort(data interface{})
}
```

### 6.2 資料存取介面

```go
// Reader 介面
type Reader interface {
    Read(ctx context.Context, path string) ([]UsageEntry, error)
}

// Writer 介面
type Writer interface {
    Write(ctx context.Context, path string, data []UsageEntry) error
}

// Cache 介面
type Cache interface {
    Get(key string) (interface{}, bool)
    Set(key string, value interface{}, ttl time.Duration)
    Delete(key string)
    Clear()
}

// Store 介面
type Store interface {
    Reader
    Writer
}
```

### 6.3 業務邏輯介面

```go
// ReportGenerator 介面
type ReportGenerator interface {
    Generate(entries []UsageEntry, options GenerateOptions) (interface{}, error)
}

// CostCalculator 介面
type CostCalculator interface {
    Calculate(model ModelName, tokens TokenCount) (float64, error)
}

// PricingProvider 介面
type PricingProvider interface {
    GetPricing(model ModelName) (*ModelPricing, error)
    UpdatePricing(pricing ModelPricing) error
}

// Monitor 介面
type Monitor interface {
    Start(ctx context.Context) error
    Stop() error
    GetStatus() MonitorStatus
}
```

## 7. 選項類型

### 7.1 產生選項

```go
type GenerateOptions struct {
    // 時間範圍
    Since *time.Time
    Until *time.Time
    
    // 過濾條件
    Project  *ProjectPath
    Models   []ModelName
    MinCost  float64
    MaxCost  float64
    
    // 排序
    SortBy    string
    SortOrder SortOrder
    
    // 輸出選項
    Breakdown bool
    Detailed  bool
    
    // 本地化
    Timezone *time.Location
    Locale   string
}

type SortOrder string

const (
    SortOrderAsc  SortOrder = "asc"
    SortOrderDesc SortOrder = "desc"
)

func (o *GenerateOptions) Validate() error {
    if o.Since != nil && o.Until != nil {
        if o.Since.After(*o.Until) {
            return fmt.Errorf("since date must be before until date")
        }
    }
    
    if o.MinCost < 0 || o.MaxCost < 0 {
        return fmt.Errorf("cost filters cannot be negative")
    }
    
    if o.MinCost > o.MaxCost && o.MaxCost > 0 {
        return fmt.Errorf("min cost cannot be greater than max cost")
    }
    
    return nil
}
```

### 7.2 成本模式

```go
type CostMode string

const (
    CostModeInput     CostMode = "input"
    CostModeOutput    CostMode = "output"
    CostModeTotal     CostMode = "total"
    CostModeMaxOutput CostMode = "maxOutput"
    CostModeMaxTotal  CostMode = "maxTotal"
)

func (cm CostMode) IsValid() bool {
    switch cm {
    case CostModeInput, CostModeOutput, CostModeTotal, 
         CostModeMaxOutput, CostModeMaxTotal:
        return true
    default:
        return false
    }
}

func (cm CostMode) Calculate(entry UsageEntry, pricing *ModelPricing) float64 {
    switch cm {
    case CostModeInput:
        return calculateInputCost(entry, pricing)
    case CostModeOutput:
        return calculateOutputCost(entry, pricing)
    case CostModeTotal:
        return calculateTotalCost(entry, pricing)
    case CostModeMaxOutput:
        return calculateMaxOutputCost(entry, pricing)
    case CostModeMaxTotal:
        return calculateMaxTotalCost(entry, pricing)
    default:
        return 0
    }
}
```

## 8. 常數定義

### 8.1 系統常數

```go
const (
    // 時間相關
    DefaultBlockDuration = 5 * time.Hour
    SessionGapThreshold  = 5 * time.Hour
    RefreshInterval      = 1 * time.Second
    
    // 限制相關
    MaxTokensPerRequest  = 200000
    MaxCostWarning      = 100.0
    MaxCostDanger       = 200.0
    
    // 顯示相關
    DefaultTerminalWidth  = 80
    CompactModeThreshold = 100
    MaxColumnWidth       = 50
    
    // 快取相關
    DefaultCacheTTL     = 1 * time.Hour
    MaxCacheSize        = 100 * 1024 * 1024 // 100MB
)
```

### 8.2 預設值

```go
var (
    DefaultTimezone = time.UTC
    DefaultLocale   = "en-US"
    DefaultModels   = []string{
        "claude-3-opus-20240229",
        "claude-3-5-sonnet-20241022",
        "claude-3-haiku-20240307",
    }
    
    DefaultPaths = []string{
        "~/.claude/projects",
        "~/.config/claude/projects",
    }
)
```

## 9. 類型轉換

### 9.1 轉換函數

```go
// 字串轉換
func ParseDailyDate(s string) (DailyDate, error) {
    return NewDailyDate(s)
}

func ParseMonthlyDate(s string) (MonthlyDate, error) {
    return NewMonthlyDate(s)
}

func ParseFilterDate(s string) (FilterDate, error) {
    // 支援多種格式
    formats := []string{
        "20060102",
        "2006-01-02",
        "2006/01/02",
    }
    
    for _, format := range formats {
        if _, err := time.Parse(format, s); err == nil {
            normalized := strings.ReplaceAll(s, "-", "")
            normalized = strings.ReplaceAll(normalized, "/", "")
            return FilterDate{value: normalized}, nil
        }
    }
    
    return FilterDate{}, fmt.Errorf("invalid date format")
}

// 類型轉換
func ConvertToDaily(monthly MonthlyDate) ([]DailyDate, error) {
    start, err := monthly.StartOfMonth()
    if err != nil {
        return nil, err
    }
    
    end, err := monthly.EndOfMonth()
    if err != nil {
        return nil, err
    }
    
    var dates []DailyDate
    for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
        date, err := NewDailyDate(d.Format("2006-01-02"))
        if err != nil {
            return nil, err
        }
        dates = append(dates, date)
    }
    
    return dates, nil
}
```

## 10. 序列化支援

### 10.1 JSON 序列化

```go
// 自定義 JSON 序列化
func (u UsageEntry) MarshalJSON() ([]byte, error) {
    type Alias UsageEntry
    return json.Marshal(&struct {
        Timestamp string `json:"timestamp"`
        *Alias
    }{
        Timestamp: u.Timestamp.value.Format(time.RFC3339),
        Alias:     (*Alias)(&u),
    })
}

func (u *UsageEntry) UnmarshalJSON(data []byte) error {
    type Alias UsageEntry
    aux := &struct {
        Timestamp string `json:"timestamp"`
        *Alias
    }{
        Alias: (*Alias)(u),
    }
    
    if err := json.Unmarshal(data, &aux); err != nil {
        return err
    }
    
    t, err := time.Parse(time.RFC3339, aux.Timestamp)
    if err != nil {
        return err
    }
    
    u.Timestamp = ISOTimestamp{value: t}
    return nil
}
```

## 11. 測試策略

### 11.1 類型測試

```go
func TestBrandedTypes(t *testing.T) {
    t.Run("ModelName", func(t *testing.T) {
        // 測試有效名稱
        model, err := NewModelName("claude-3-opus")
        assert.NoError(t, err)
        assert.Equal(t, "claude-3-opus", model.String())
        
        // 測試空名稱
        _, err = NewModelName("")
        assert.Error(t, err)
    })
    
    t.Run("DailyDate", func(t *testing.T) {
        // 測試有效日期
        date, err := NewDailyDate("2024-01-01")
        assert.NoError(t, err)
        assert.Equal(t, "2024-01-01", date.String())
        
        // 測試無效格式
        _, err = NewDailyDate("01-01-2024")
        assert.Error(t, err)
        
        // 測試日期運算
        tomorrow, err := date.AddDays(1)
        assert.NoError(t, err)
        assert.Equal(t, "2024-01-02", tomorrow.String())
    })
}

func TestSerialization(t *testing.T) {
    entry := UsageEntry{
        InputTokens:  100,
        OutputTokens: 50,
        Cost:        0.01,
    }
    
    // 序列化
    data, err := json.Marshal(entry)
    assert.NoError(t, err)
    
    // 反序列化
    var decoded UsageEntry
    err = json.Unmarshal(data, &decoded)
    assert.NoError(t, err)
    
    assert.Equal(t, entry.InputTokens, decoded.InputTokens)
}
```

## 12. 與 TypeScript 版本對照

| TypeScript 類型 | Go 類型 | 說明 |
|----------------|---------|------|
| branded types (z.brand) | struct wrapper | 品牌類型實作 |
| z.string().min(1) | validation in constructor | 建構函數驗證 |
| z.infer<> | 直接類型定義 | 類型推斷 |
| union types | interface{} or custom type | 聯合類型 |
| optional fields | pointer types | 可選欄位 |