# 報告生成模組設計

## 1. 模組概述

報告生成模組負責將處理後的使用數據轉換為各種格式的報告，包括日報、月報、週報、會話報告和區塊報告。每種報告類型都有其特定的聚合邏輯和展示方式。

### 1.1 報告類型
- **Daily Report**: 按日期聚合的使用統計
- **Monthly Report**: 按月份聚合的使用統計
- **Weekly Report**: 按週聚合的使用統計
- **Session Report**: 按會話分組的使用統計
- **Blocks Report**: 5小時計費視窗的使用統計

### 1.2 對應 TypeScript 模組
- `commands/daily.ts` → `commands/daily.go`
- `commands/monthly.ts` → `commands/monthly.go`
- `commands/weekly.ts` → `commands/weekly.go`
- `commands/session.ts` → `commands/session.go`
- `commands/blocks.ts` → `commands/blocks.go`

## 2. 報告生成架構

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Raw Data   │────▶│  Aggregator │────▶│  Generator  │
└─────────────┘     └─────────────┘     └──────┬──────┘
                                               │
                    ┌──────────────────────────┼──────────────────────────┐
                    │                          │                          │
             ┌──────▼──────┐          ┌───────▼──────┐          ┌────────▼────────┐
             │   Sorter    │          │   Filter     │          │   Calculator    │
             └──────┬──────┘          └───────┬──────┘          └────────┬────────┘
                    │                          │                          │
                    └──────────────────────────┼──────────────────────────┘
                                               │
                                        ┌──────▼──────┐
                                        │   Report    │
                                        └─────────────┘
```

## 3. Daily Report 模組

### 3.1 資料結構

```go
package commands

type DailyReport struct {
    Date            string    `json:"date"`
    InputTokens     int       `json:"input_tokens"`
    OutputTokens    int       `json:"output_tokens"`
    CacheTokens     int       `json:"cache_creation_tokens"`
    CacheReadTokens int       `json:"cache_read_tokens"`
    TotalTokens     int       `json:"total_tokens"`
    TotalCost       float64   `json:"total_cost"`
    ModelsUsed      []string  `json:"models_used"`
    ModelBreakdowns []ModelBreakdown `json:"model_breakdowns,omitempty"`
    Project         string    `json:"project,omitempty"`
}

type ModelBreakdown struct {
    Model           string  `json:"model"`
    InputTokens     int     `json:"input_tokens"`
    OutputTokens    int     `json:"output_tokens"`
    CacheTokens     int     `json:"cache_tokens"`
    CacheReadTokens int     `json:"cache_read_tokens"`
    Cost            float64 `json:"cost"`
}
```

### 3.2 聚合邏輯

```go
type DailyAggregator struct {
    timezone *time.Location
    locale   string
}

func (da *DailyAggregator) Aggregate(entries []UsageEntry, options AggregateOptions) []DailyReport {
    // 按日期分組
    grouped := da.groupByDate(entries)
    
    // 應用過濾器
    if options.Since != nil || options.Until != nil {
        grouped = da.filterByDateRange(grouped, options.Since, options.Until)
    }
    
    if options.Project != "" {
        grouped = da.filterByProject(grouped, options.Project)
    }
    
    // 計算統計
    reports := make([]DailyReport, 0, len(grouped))
    for date, entries := range grouped {
        report := DailyReport{
            Date: date,
        }
        
        // 聚合 tokens
        for _, entry := range entries {
            report.InputTokens += entry.InputTokens
            report.OutputTokens += entry.OutputTokens
            report.CacheTokens += entry.CacheTokens
            report.CacheReadTokens += entry.CacheReadTokens
            report.TotalCost += entry.Cost
        }
        
        report.TotalTokens = report.InputTokens + report.OutputTokens + 
                            report.CacheTokens + report.CacheReadTokens
        
        // 收集使用的模型
        report.ModelsUsed = da.collectModels(entries)
        
        // 如果需要模型細分
        if options.Breakdown {
            report.ModelBreakdowns = da.calculateModelBreakdowns(entries)
        }
        
        reports = append(reports, report)
    }
    
    // 排序
    da.sortReports(reports, options.Order)
    
    return reports
}

func (da *DailyAggregator) groupByDate(entries []UsageEntry) map[string][]UsageEntry {
    grouped := make(map[string][]UsageEntry)
    
    for _, entry := range entries {
        localTime := entry.Timestamp.In(da.timezone)
        dateKey := localTime.Format("2006-01-02")
        grouped[dateKey] = append(grouped[dateKey], entry)
    }
    
    return grouped
}
```

### 3.3 專案分組

```go
type ProjectGrouper struct {
    normalizer *ProjectNormalizer
}

func (pg *ProjectGrouper) GroupByProject(reports []DailyReport) map[string][]DailyReport {
    grouped := make(map[string][]DailyReport)
    
    for _, report := range reports {
        project := pg.normalizer.Normalize(report.Project)
        grouped[project] = append(grouped[project], report)
    }
    
    return grouped
}

type ProjectNormalizer struct {
    rules []NormalizationRule
}

func (pn *ProjectNormalizer) Normalize(path string) string {
    // 提取專案名稱
    parts := strings.Split(path, "/")
    if len(parts) > 0 {
        return parts[len(parts)-1]
    }
    return "unknown"
}
```

## 4. Monthly Report 模組

### 4.1 月報聚合

```go
type MonthlyAggregator struct {
    baseAggregator *DailyAggregator
}

func (ma *MonthlyAggregator) Aggregate(entries []UsageEntry, options AggregateOptions) []MonthlyReport {
    // 先生成日報
    dailyReports := ma.baseAggregator.Aggregate(entries, options)
    
    // 按月聚合
    grouped := make(map[string]*MonthlyReport)
    
    for _, daily := range dailyReports {
        // 提取月份
        date, _ := time.Parse("2006-01-02", daily.Date)
        monthKey := date.Format("2006-01")
        
        if monthly, exists := grouped[monthKey]; exists {
            monthly.InputTokens += daily.InputTokens
            monthly.OutputTokens += daily.OutputTokens
            monthly.CacheTokens += daily.CacheTokens
            monthly.CacheReadTokens += daily.CacheReadTokens
            monthly.TotalCost += daily.TotalCost
            monthly.DaysActive++
            ma.mergeModels(monthly, daily.ModelsUsed)
        } else {
            grouped[monthKey] = &MonthlyReport{
                Month:           monthKey,
                InputTokens:     daily.InputTokens,
                OutputTokens:    daily.OutputTokens,
                CacheTokens:     daily.CacheTokens,
                CacheReadTokens: daily.CacheReadTokens,
                TotalCost:       daily.TotalCost,
                ModelsUsed:      daily.ModelsUsed,
                DaysActive:      1,
            }
        }
    }
    
    // 轉換為 slice 並排序
    reports := ma.toSlice(grouped)
    ma.sortReports(reports, options.Order)
    
    return reports
}
```

### 4.2 月度統計

```go
type MonthlyStats struct {
    AverageDailyCost   float64
    AverageDailyTokens int
    PeakDay            string
    PeakDayCost        float64
    Trend              float64 // 與上月相比的變化百分比
}

func CalculateMonthlyStats(reports []MonthlyReport) map[string]MonthlyStats {
    stats := make(map[string]MonthlyStats)
    
    for i, report := range reports {
        stat := MonthlyStats{
            AverageDailyCost:   report.TotalCost / float64(report.DaysActive),
            AverageDailyTokens: report.TotalTokens / report.DaysActive,
        }
        
        // 計算趨勢
        if i > 0 {
            prevCost := reports[i-1].TotalCost
            if prevCost > 0 {
                stat.Trend = ((report.TotalCost - prevCost) / prevCost) * 100
            }
        }
        
        stats[report.Month] = stat
    }
    
    return stats
}
```

## 5. Weekly Report 模組

### 5.1 週報聚合

```go
type WeeklyAggregator struct {
    startOfWeek time.Weekday // 週的起始日（預設週一）
}

func (wa *WeeklyAggregator) Aggregate(entries []UsageEntry, options AggregateOptions) []WeeklyReport {
    grouped := make(map[string]*WeeklyReport)
    
    for _, entry := range entries {
        weekKey := wa.getWeekKey(entry.Timestamp)
        
        if weekly, exists := grouped[weekKey]; exists {
            wa.updateWeekly(weekly, entry)
        } else {
            grouped[weekKey] = wa.newWeekly(weekKey, entry)
        }
    }
    
    return wa.toSortedSlice(grouped, options.Order)
}

func (wa *WeeklyAggregator) getWeekKey(t time.Time) string {
    // 計算週的開始日期
    year, week := t.ISOWeek()
    return fmt.Sprintf("%d-W%02d", year, week)
}

func (wa *WeeklyAggregator) getWeekRange(weekKey string) (start, end time.Time) {
    // 解析週數
    var year, week int
    fmt.Sscanf(weekKey, "%d-W%d", &year, &week)
    
    // 計算週的開始和結束日期
    jan1 := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
    weekStart := jan1.AddDate(0, 0, (week-1)*7)
    
    // 調整到週一
    for weekStart.Weekday() != time.Monday {
        weekStart = weekStart.AddDate(0, 0, -1)
    }
    
    weekEnd := weekStart.AddDate(0, 0, 6)
    return weekStart, weekEnd
}
```

## 6. Session Report 模組

### 6.1 會話識別

```go
type SessionIdentifier struct {
    gapThreshold   time.Duration // 會話間隔閾值（預設5小時）
    minDuration    time.Duration // 最小會話時長
}

func (si *SessionIdentifier) IdentifySessions(entries []UsageEntry) []Session {
    // 按時間排序
    sort.Slice(entries, func(i, j int) bool {
        return entries[i].Timestamp.Before(entries[j].Timestamp)
    })
    
    var sessions []Session
    var currentSession *Session
    
    for _, entry := range entries {
        if currentSession == nil {
            currentSession = si.newSession(entry)
            continue
        }
        
        // 檢查是否屬於當前會話
        timeSinceLastEntry := entry.Timestamp.Sub(currentSession.LastActivity)
        if timeSinceLastEntry > si.gapThreshold {
            // 結束當前會話，開始新會話
            if currentSession.Duration() >= si.minDuration {
                sessions = append(sessions, *currentSession)
            }
            currentSession = si.newSession(entry)
        } else {
            // 更新當前會話
            currentSession.AddEntry(entry)
        }
    }
    
    // 添加最後一個會話
    if currentSession != nil && currentSession.Duration() >= si.minDuration {
        sessions = append(sessions, *currentSession)
    }
    
    return sessions
}
```

### 6.2 會話統計

```go
type SessionReport struct {
    SessionID       string        `json:"session_id"`
    StartTime       time.Time     `json:"start_time"`
    EndTime         time.Time     `json:"end_time"`
    Duration        time.Duration `json:"duration"`
    InputTokens     int          `json:"input_tokens"`
    OutputTokens    int          `json:"output_tokens"`
    TotalCost       float64      `json:"total_cost"`
    ModelsUsed      []string     `json:"models_used"`
    RequestCount    int          `json:"request_count"`
    AverageInterval time.Duration `json:"average_interval"`
}

func GenerateSessionReport(session Session) SessionReport {
    report := SessionReport{
        SessionID:    session.ID,
        StartTime:    session.StartTime,
        EndTime:      session.EndTime,
        Duration:     session.Duration(),
        RequestCount: len(session.Entries),
    }
    
    // 計算 token 總量
    for _, entry := range session.Entries {
        report.InputTokens += entry.InputTokens
        report.OutputTokens += entry.OutputTokens
        report.TotalCost += entry.Cost
    }
    
    // 收集使用的模型
    modelSet := make(map[string]bool)
    for _, entry := range session.Entries {
        modelSet[entry.Model] = true
    }
    for model := range modelSet {
        report.ModelsUsed = append(report.ModelsUsed, model)
    }
    
    // 計算平均請求間隔
    if report.RequestCount > 1 {
        report.AverageInterval = report.Duration / time.Duration(report.RequestCount-1)
    }
    
    return report
}
```

## 7. Blocks Report 模組

### 7.1 5小時區塊計算

```go
type BlockCalculator struct {
    blockDuration time.Duration // 5小時
    warningThreshold float64   // 警告閾值
}

func (bc *BlockCalculator) CalculateBlocks(entries []UsageEntry) []BlockReport {
    // 按時間排序
    sort.Slice(entries, func(i, j int) bool {
        return entries[i].Timestamp.Before(entries[j].Timestamp)
    })
    
    var blocks []BlockReport
    var currentBlock *BlockReport
    
    for _, entry := range entries {
        if currentBlock == nil {
            currentBlock = bc.newBlock(entry.Timestamp)
        }
        
        // 檢查是否超出當前區塊
        if entry.Timestamp.Sub(currentBlock.StartTime) >= bc.blockDuration {
            blocks = append(blocks, *currentBlock)
            currentBlock = bc.newBlock(entry.Timestamp)
        }
        
        currentBlock.AddEntry(entry)
    }
    
    // 添加最後一個區塊
    if currentBlock != nil && currentBlock.HasData() {
        blocks = append(blocks, *currentBlock)
    }
    
    // 標記活躍區塊和警告
    bc.markActiveBlocks(blocks)
    bc.checkWarnings(blocks)
    
    return blocks
}

func (bc *BlockCalculator) markActiveBlocks(blocks []BlockReport) {
    now := time.Now()
    
    for i := range blocks {
        block := &blocks[i]
        
        // 檢查是否為活躍區塊
        if now.Sub(block.StartTime) < bc.blockDuration {
            block.IsActive = true
            block.TimeRemaining = bc.blockDuration - now.Sub(block.StartTime)
        }
        
        // 檢查是否有間隙
        if i > 0 {
            prevBlock := blocks[i-1]
            gap := block.StartTime.Sub(prevBlock.EndTime)
            if gap > time.Hour {
                block.IsGap = true
                block.GapDuration = gap
            }
        }
    }
}
```

### 7.2 區塊預測

```go
type BlockPredictor struct {
    historyWindow int // 歷史資料視窗大小
}

func (bp *BlockPredictor) PredictUsage(currentBlock BlockReport, history []BlockReport) BlockPrediction {
    prediction := BlockPrediction{
        BlockID: currentBlock.ID,
    }
    
    if !currentBlock.IsActive {
        return prediction
    }
    
    // 計算燃燒率
    elapsed := time.Since(currentBlock.StartTime)
    if elapsed > 0 {
        tokensPerMinute := float64(currentBlock.TotalTokens) / elapsed.Minutes()
        costPerMinute := currentBlock.TotalCost / elapsed.Minutes()
        
        remaining := currentBlock.TimeRemaining
        prediction.EstimatedTokens = currentBlock.TotalTokens + 
                                    int(tokensPerMinute * remaining.Minutes())
        prediction.EstimatedCost = currentBlock.TotalCost + 
                                  (costPerMinute * remaining.Minutes())
    }
    
    // 基於歷史資料調整預測
    if len(history) >= bp.historyWindow {
        avgBlockCost := bp.calculateAverage(history)
        prediction.Confidence = bp.calculateConfidence(prediction.EstimatedCost, avgBlockCost)
    }
    
    return prediction
}
```

## 8. 報告過濾器

### 8.1 日期範圍過濾

```go
type DateRangeFilter struct {
    since *time.Time
    until *time.Time
}

func (df *DateRangeFilter) Filter(reports []interface{}) []interface{} {
    var filtered []interface{}
    
    for _, report := range reports {
        reportDate := df.extractDate(report)
        
        if df.since != nil && reportDate.Before(*df.since) {
            continue
        }
        
        if df.until != nil && reportDate.After(*df.until) {
            continue
        }
        
        filtered = append(filtered, report)
    }
    
    return filtered
}
```

### 8.2 專案過濾

```go
type ProjectFilter struct {
    projectName string
    fuzzyMatch  bool
}

func (pf *ProjectFilter) Filter(reports []interface{}) []interface{} {
    var filtered []interface{}
    
    for _, report := range reports {
        projectField := pf.extractProject(report)
        
        if pf.fuzzyMatch {
            if strings.Contains(strings.ToLower(projectField), 
                              strings.ToLower(pf.projectName)) {
                filtered = append(filtered, report)
            }
        } else {
            if projectField == pf.projectName {
                filtered = append(filtered, report)
            }
        }
    }
    
    return filtered
}
```

## 9. 排序器

### 9.1 多欄位排序

```go
type ReportSorter struct {
    fields []SortField
}

type SortField struct {
    Name       string
    Descending bool
}

func (rs *ReportSorter) Sort(reports interface{}) {
    switch r := reports.(type) {
    case []DailyReport:
        rs.sortDaily(r)
    case []MonthlyReport:
        rs.sortMonthly(r)
    case []SessionReport:
        rs.sortSession(r)
    case []BlockReport:
        rs.sortBlock(r)
    }
}

func (rs *ReportSorter) sortDaily(reports []DailyReport) {
    sort.Slice(reports, func(i, j int) bool {
        for _, field := range rs.fields {
            var cmp int
            
            switch field.Name {
            case "date":
                cmp = strings.Compare(reports[i].Date, reports[j].Date)
            case "cost":
                cmp = compareFloat(reports[i].TotalCost, reports[j].TotalCost)
            case "tokens":
                cmp = compareInt(reports[i].TotalTokens, reports[j].TotalTokens)
            }
            
            if cmp != 0 {
                if field.Descending {
                    return cmp > 0
                }
                return cmp < 0
            }
        }
        return false
    })
}
```

## 10. 報告快取

### 10.1 報告快取管理

```go
type ReportCache struct {
    cache    map[string]CachedReport
    mutex    sync.RWMutex
    ttl      time.Duration
    maxSize  int
}

type CachedReport struct {
    Data      interface{}
    Generated time.Time
    Hash      string
}

func (rc *ReportCache) Get(key string) (interface{}, bool) {
    rc.mutex.RLock()
    defer rc.mutex.RUnlock()
    
    cached, exists := rc.cache[key]
    if !exists {
        return nil, false
    }
    
    if time.Since(cached.Generated) > rc.ttl {
        return nil, false
    }
    
    return cached.Data, true
}

func (rc *ReportCache) Set(key string, data interface{}) {
    rc.mutex.Lock()
    defer rc.mutex.Unlock()
    
    // LRU 淘汰
    if len(rc.cache) >= rc.maxSize {
        rc.evictOldest()
    }
    
    rc.cache[key] = CachedReport{
        Data:      data,
        Generated: time.Now(),
        Hash:      rc.computeHash(data),
    }
}
```

## 11. 效能指標

### 11.1 報告生成效能

| 報告類型 | 資料量 | 目標時間 | 記憶體使用 |
|---------|-------|---------|-----------|
| Daily | 10000 條 | < 50ms | < 20MB |
| Monthly | 100000 條 | < 200ms | < 50MB |
| Session | 50000 條 | < 150ms | < 30MB |
| Blocks | 100000 條 | < 300ms | < 40MB |

### 11.2 優化策略

1. **並行處理**：多個報告類型可以並行生成
2. **增量計算**：利用快取避免重複計算
3. **延遲載入**：按需載入詳細資料
4. **索引優化**：為常用查詢建立索引

## 12. 測試策略

### 12.1 單元測試範例

```go
func TestDailyAggregator_Aggregate(t *testing.T) {
    aggregator := NewDailyAggregator(time.UTC, "en-US")
    
    entries := []UsageEntry{
        {
            Timestamp:    time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
            InputTokens:  100,
            OutputTokens: 50,
            Cost:        0.01,
            Model:       "claude-3",
        },
        {
            Timestamp:    time.Date(2024, 1, 1, 15, 0, 0, 0, time.UTC),
            InputTokens:  200,
            OutputTokens: 100,
            Cost:        0.02,
            Model:       "claude-3",
        },
    }
    
    reports := aggregator.Aggregate(entries, AggregateOptions{})
    
    assert.Len(t, reports, 1)
    assert.Equal(t, "2024-01-01", reports[0].Date)
    assert.Equal(t, 300, reports[0].InputTokens)
    assert.Equal(t, 150, reports[0].OutputTokens)
    assert.Equal(t, 0.03, reports[0].TotalCost)
}
```

## 13. 與 TypeScript 版本對照

| TypeScript 函數 | Go 函數 | 說明 |
|----------------|---------|------|
| loadDailyUsageData() | GenerateDailyReport() | 生成日報 |
| loadMonthlyUsageData() | GenerateMonthlyReport() | 生成月報 |
| loadSessionBlockData() | GenerateSessionReport() | 生成會話報告 |
| identifySessionBlocks() | IdentifySessions() | 識別會話 |
| calculateBurnRate() | CalculateBurnRate() | 計算燃燒率 |