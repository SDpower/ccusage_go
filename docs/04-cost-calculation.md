# 成本計算模組設計

## 1. 模組概述

成本計算模組負責精確計算各種 Claude 模型的使用成本，包括輸入/輸出 token、快取創建/讀取的費用計算。模組支援動態獲取最新價格資訊，並提供離線模式支援。

### 1.1 主要功能
- Token 計數與分類
- 模型價格管理
- 成本計算引擎
- 價格快取機制
- 離線價格支援
- 成本聚合與統計

### 1.2 對應 TypeScript 模組
- `calculate-cost.ts` → `calculator/cost.go`
- `pricing-fetcher.ts` → `pricing/fetcher.go`
- `_token-utils.ts` → `calculator/tokens.go`

## 2. 成本計算架構

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ Usage Entry  │────▶│Token Counter │────▶│   Pricing    │
└──────────────┘     └──────────────┘     └──────┬───────┘
                                                  │
                     ┌────────────────────────────┼────────────────────────────┐
                     │                            │                            │
              ┌──────▼──────┐             ┌──────▼──────┐            ┌────────▼────────┐
              │ Price Cache │             │Price Fetcher│            │Offline Pricing │
              └──────┬──────┘             └──────┬──────┘            └────────┬────────┘
                     │                            │                            │
                     └────────────────────────────┼────────────────────────────┘
                                                  │
                                           ┌──────▼──────┐
                                           │Cost Result  │
                                           └─────────────┘
```

## 3. Token 計算系統

### 3.1 Token 類型定義

```go
package calculator

type TokenType int

const (
    TokenTypeInput TokenType = iota
    TokenTypeOutput
    TokenTypeCacheCreation
    TokenTypeCacheRead
)

type TokenCount struct {
    InputTokens        int `json:"input_tokens"`
    OutputTokens       int `json:"output_tokens"`
    CacheCreationTokens int `json:"cache_creation_tokens"`
    CacheReadTokens    int `json:"cache_read_tokens"`
}

func (tc *TokenCount) Total() int {
    return tc.InputTokens + tc.OutputTokens + 
           tc.CacheCreationTokens + tc.CacheReadTokens
}

func (tc *TokenCount) Add(other TokenCount) {
    tc.InputTokens += other.InputTokens
    tc.OutputTokens += other.OutputTokens
    tc.CacheCreationTokens += other.CacheCreationTokens
    tc.CacheReadTokens += other.CacheReadTokens
}
```

### 3.2 Token 計算器

```go
type TokenCalculator struct {
    contextLimit map[string]int // 各模型的上下文限制
}

func NewTokenCalculator() *TokenCalculator {
    return &TokenCalculator{
        contextLimit: map[string]int{
            "claude-3-opus":    200000,
            "claude-3-sonnet":  200000,
            "claude-3-haiku":   200000,
            "claude-3.5-sonnet": 200000,
        },
    }
}

func (tc *TokenCalculator) CalculateTokens(entry UsageEntry) TokenCount {
    count := TokenCount{
        InputTokens:         entry.InputTokens,
        OutputTokens:        entry.OutputTokens,
        CacheCreationTokens: entry.CacheCreationTokens,
        CacheReadTokens:     entry.CacheReadTokens,
    }
    
    // 驗證 token 數量
    if err := tc.validateTokenCount(count, entry.Model); err != nil {
        log.Warn("Invalid token count", "error", err)
    }
    
    return count
}

func (tc *TokenCalculator) validateTokenCount(count TokenCount, model string) error {
    limit, exists := tc.contextLimit[model]
    if !exists {
        return fmt.Errorf("unknown model: %s", model)
    }
    
    total := count.InputTokens + count.OutputTokens
    if total > limit {
        return fmt.Errorf("token count %d exceeds limit %d for model %s", 
                          total, limit, model)
    }
    
    return nil
}
```

### 3.3 Token 聚合器

```go
type TokenAggregator struct {
    groups map[string]*TokenCount
    mutex  sync.RWMutex
}

func NewTokenAggregator() *TokenAggregator {
    return &TokenAggregator{
        groups: make(map[string]*TokenCount),
    }
}

func (ta *TokenAggregator) Aggregate(key string, count TokenCount) {
    ta.mutex.Lock()
    defer ta.mutex.Unlock()
    
    if existing, exists := ta.groups[key]; exists {
        existing.Add(count)
    } else {
        ta.groups[key] = &count
    }
}

func (ta *TokenAggregator) GetTotal() TokenCount {
    ta.mutex.RLock()
    defer ta.mutex.RUnlock()
    
    total := TokenCount{}
    for _, count := range ta.groups {
        total.Add(*count)
    }
    
    return total
}

func (ta *TokenAggregator) GetByGroup(key string) (TokenCount, bool) {
    ta.mutex.RLock()
    defer ta.mutex.RUnlock()
    
    count, exists := ta.groups[key]
    if !exists {
        return TokenCount{}, false
    }
    
    return *count, true
}
```

## 4. 價格管理系統

### 4.1 價格模型定義

```go
package pricing

type ModelPricing struct {
    Model               string  `json:"model"`
    InputPrice          float64 `json:"input_price"`          // 每百萬 token
    OutputPrice         float64 `json:"output_price"`         // 每百萬 token
    CacheCreationPrice  float64 `json:"cache_creation_price"` // 每百萬 token
    CacheReadPrice      float64 `json:"cache_read_price"`     // 每百萬 token
    LastUpdated         time.Time `json:"last_updated"`
}

type PricingTable struct {
    prices map[string]*ModelPricing
    mutex  sync.RWMutex
}

func NewPricingTable() *PricingTable {
    return &PricingTable{
        prices: make(map[string]*ModelPricing),
    }
}

func (pt *PricingTable) GetPrice(model string) (*ModelPricing, error) {
    pt.mutex.RLock()
    defer pt.mutex.RUnlock()
    
    price, exists := pt.prices[model]
    if !exists {
        return nil, fmt.Errorf("pricing not found for model: %s", model)
    }
    
    return price, nil
}

func (pt *PricingTable) UpdatePrice(pricing ModelPricing) {
    pt.mutex.Lock()
    defer pt.mutex.Unlock()
    
    pricing.LastUpdated = time.Now()
    pt.prices[pricing.Model] = &pricing
}
```

### 4.2 價格獲取器

```go
type PricingFetcher struct {
    apiURL      string
    httpClient  *http.Client
    cache       *PricingCache
    offline     bool
    retryPolicy *RetryPolicy
}

func NewPricingFetcher(offline bool) *PricingFetcher {
    return &PricingFetcher{
        apiURL: "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json",
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
        },
        cache: NewPricingCache(),
        offline: offline,
        retryPolicy: &RetryPolicy{
            MaxRetries: 3,
            Delay:      time.Second,
        },
    }
}

func (pf *PricingFetcher) FetchPricing(ctx context.Context) (*PricingTable, error) {
    // 檢查快取
    if cached, hit := pf.cache.Get(); hit {
        return cached, nil
    }
    
    // 離線模式使用預設價格
    if pf.offline {
        return pf.loadOfflinePricing()
    }
    
    // 從 API 獲取
    table, err := pf.fetchFromAPI(ctx)
    if err != nil {
        // 降級到離線價格
        log.Warn("Failed to fetch pricing, using offline data", "error", err)
        return pf.loadOfflinePricing()
    }
    
    // 更新快取
    pf.cache.Set(table)
    
    return table, nil
}

func (pf *PricingFetcher) fetchFromAPI(ctx context.Context) (*PricingTable, error) {
    var lastErr error
    
    for attempt := 0; attempt < pf.retryPolicy.MaxRetries; attempt++ {
        if attempt > 0 {
            time.Sleep(pf.retryPolicy.Delay * time.Duration(attempt))
        }
        
        req, err := http.NewRequestWithContext(ctx, "GET", pf.apiURL, nil)
        if err != nil {
            return nil, err
        }
        
        resp, err := pf.httpClient.Do(req)
        if err != nil {
            lastErr = err
            continue
        }
        defer resp.Body.Close()
        
        if resp.StatusCode != http.StatusOK {
            lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
            continue
        }
        
        return pf.parsePricingResponse(resp.Body)
    }
    
    return nil, fmt.Errorf("failed after %d retries: %w", 
                           pf.retryPolicy.MaxRetries, lastErr)
}
```

### 4.3 離線價格資料

```go
func (pf *PricingFetcher) loadOfflinePricing() (*PricingTable, error) {
    // 內嵌的預設價格資料
    defaultPrices := []ModelPricing{
        {
            Model:              "claude-3-opus-20240229",
            InputPrice:         15.0,
            OutputPrice:        75.0,
            CacheCreationPrice: 18.75,
            CacheReadPrice:     1.875,
        },
        {
            Model:              "claude-3-5-sonnet-20241022",
            InputPrice:         3.0,
            OutputPrice:        15.0,
            CacheCreationPrice: 3.75,
            CacheReadPrice:     0.3,
        },
        {
            Model:              "claude-3-haiku-20240307",
            InputPrice:         0.25,
            OutputPrice:        1.25,
            CacheCreationPrice: 0.3,
            CacheReadPrice:     0.03,
        },
    }
    
    table := NewPricingTable()
    for _, price := range defaultPrices {
        table.UpdatePrice(price)
    }
    
    return table, nil
}
```

## 5. 成本計算引擎

### 5.1 成本計算器

```go
type CostCalculator struct {
    pricingTable *PricingTable
    precision    int // 小數位數
}

func NewCostCalculator(pricingTable *PricingTable) *CostCalculator {
    return &CostCalculator{
        pricingTable: pricingTable,
        precision:    6,
    }
}

func (cc *CostCalculator) Calculate(model string, tokens TokenCount) (float64, error) {
    pricing, err := cc.pricingTable.GetPrice(model)
    if err != nil {
        return 0, err
    }
    
    // 計算各部分成本（價格是每百萬 token）
    inputCost := float64(tokens.InputTokens) / 1_000_000 * pricing.InputPrice
    outputCost := float64(tokens.OutputTokens) / 1_000_000 * pricing.OutputPrice
    cacheCreationCost := float64(tokens.CacheCreationTokens) / 1_000_000 * pricing.CacheCreationPrice
    cacheReadCost := float64(tokens.CacheReadTokens) / 1_000_000 * pricing.CacheReadPrice
    
    totalCost := inputCost + outputCost + cacheCreationCost + cacheReadCost
    
    // 四捨五入到指定精度
    return cc.round(totalCost), nil
}

func (cc *CostCalculator) round(value float64) float64 {
    multiplier := math.Pow(10, float64(cc.precision))
    return math.Round(value*multiplier) / multiplier
}

type CostBreakdown struct {
    InputCost         float64 `json:"input_cost"`
    OutputCost        float64 `json:"output_cost"`
    CacheCreationCost float64 `json:"cache_creation_cost"`
    CacheReadCost     float64 `json:"cache_read_cost"`
    TotalCost         float64 `json:"total_cost"`
}

func (cc *CostCalculator) CalculateDetailed(model string, tokens TokenCount) (*CostBreakdown, error) {
    pricing, err := cc.pricingTable.GetPrice(model)
    if err != nil {
        return nil, err
    }
    
    breakdown := &CostBreakdown{
        InputCost:         cc.round(float64(tokens.InputTokens) / 1_000_000 * pricing.InputPrice),
        OutputCost:        cc.round(float64(tokens.OutputTokens) / 1_000_000 * pricing.OutputPrice),
        CacheCreationCost: cc.round(float64(tokens.CacheCreationTokens) / 1_000_000 * pricing.CacheCreationPrice),
        CacheReadCost:     cc.round(float64(tokens.CacheReadTokens) / 1_000_000 * pricing.CacheReadPrice),
    }
    
    breakdown.TotalCost = cc.round(
        breakdown.InputCost + 
        breakdown.OutputCost + 
        breakdown.CacheCreationCost + 
        breakdown.CacheReadCost,
    )
    
    return breakdown, nil
}
```

### 5.2 批次成本計算

```go
type BatchCostCalculator struct {
    calculator *CostCalculator
    workers    int
}

func NewBatchCostCalculator(calculator *CostCalculator, workers int) *BatchCostCalculator {
    return &BatchCostCalculator{
        calculator: calculator,
        workers:    workers,
    }
}

func (bcc *BatchCostCalculator) CalculateBatch(entries []UsageEntry) ([]CostResult, error) {
    results := make([]CostResult, len(entries))
    errChan := make(chan error, len(entries))
    
    // 使用 worker pool
    sem := make(chan struct{}, bcc.workers)
    var wg sync.WaitGroup
    
    for i, entry := range entries {
        wg.Add(1)
        go func(idx int, e UsageEntry) {
            defer wg.Done()
            
            sem <- struct{}{}
            defer func() { <-sem }()
            
            tokens := TokenCount{
                InputTokens:         e.InputTokens,
                OutputTokens:        e.OutputTokens,
                CacheCreationTokens: e.CacheCreationTokens,
                CacheReadTokens:     e.CacheReadTokens,
            }
            
            cost, err := bcc.calculator.Calculate(e.Model, tokens)
            if err != nil {
                errChan <- err
                return
            }
            
            results[idx] = CostResult{
                Entry: e,
                Cost:  cost,
            }
        }(i, entry)
    }
    
    wg.Wait()
    close(errChan)
    
    // 收集錯誤
    var errs []error
    for err := range errChan {
        errs = append(errs, err)
    }
    
    if len(errs) > 0 {
        return results, fmt.Errorf("batch calculation had %d errors: %v", len(errs), errs[0])
    }
    
    return results, nil
}
```

## 6. 成本聚合與統計

### 6.1 成本聚合器

```go
type CostAggregator struct {
    aggregates map[string]*CostAggregate
    mutex      sync.RWMutex
}

type CostAggregate struct {
    TotalCost   float64
    TokenCounts TokenCount
    EntryCount  int
    Models      map[string]int // 模型使用次數
}

func NewCostAggregator() *CostAggregator {
    return &CostAggregator{
        aggregates: make(map[string]*CostAggregate),
    }
}

func (ca *CostAggregator) Add(key string, cost float64, tokens TokenCount, model string) {
    ca.mutex.Lock()
    defer ca.mutex.Unlock()
    
    agg, exists := ca.aggregates[key]
    if !exists {
        agg = &CostAggregate{
            Models: make(map[string]int),
        }
        ca.aggregates[key] = agg
    }
    
    agg.TotalCost += cost
    agg.TokenCounts.Add(tokens)
    agg.EntryCount++
    agg.Models[model]++
}

func (ca *CostAggregator) GetStatistics(key string) (*CostStatistics, bool) {
    ca.mutex.RLock()
    defer ca.mutex.RUnlock()
    
    agg, exists := ca.aggregates[key]
    if !exists {
        return nil, false
    }
    
    stats := &CostStatistics{
        TotalCost:    agg.TotalCost,
        AverageCost:  agg.TotalCost / float64(agg.EntryCount),
        TotalTokens:  agg.TokenCounts.Total(),
        TokenCounts:  agg.TokenCounts,
        EntryCount:   agg.EntryCount,
        CostPerToken: agg.TotalCost / float64(agg.TokenCounts.Total()),
    }
    
    // 找出最常用的模型
    var maxCount int
    for model, count := range agg.Models {
        if count > maxCount {
            stats.MostUsedModel = model
            maxCount = count
        }
    }
    
    return stats, true
}
```

### 6.2 成本趨勢分析

```go
type CostTrendAnalyzer struct {
    windowSize int
    history    []CostDataPoint
    mutex      sync.RWMutex
}

type CostDataPoint struct {
    Timestamp time.Time
    Cost      float64
    Tokens    int
}

func (cta *CostTrendAnalyzer) AddDataPoint(point CostDataPoint) {
    cta.mutex.Lock()
    defer cta.mutex.Unlock()
    
    cta.history = append(cta.history, point)
    
    // 保持視窗大小
    if len(cta.history) > cta.windowSize {
        cta.history = cta.history[len(cta.history)-cta.windowSize:]
    }
}

func (cta *CostTrendAnalyzer) CalculateTrend() *TrendAnalysis {
    cta.mutex.RLock()
    defer cta.mutex.RUnlock()
    
    if len(cta.history) < 2 {
        return nil
    }
    
    analysis := &TrendAnalysis{}
    
    // 計算移動平均
    var sumCost float64
    var sumTokens int
    for _, point := range cta.history {
        sumCost += point.Cost
        sumTokens += point.Tokens
    }
    
    analysis.AverageCost = sumCost / float64(len(cta.history))
    analysis.AverageTokens = sumTokens / len(cta.history)
    
    // 計算趨勢（簡單線性回歸）
    var sumX, sumY, sumXY, sumX2 float64
    for i, point := range cta.history {
        x := float64(i)
        y := point.Cost
        sumX += x
        sumY += y
        sumXY += x * y
        sumX2 += x * x
    }
    
    n := float64(len(cta.history))
    analysis.Slope = (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
    
    // 判斷趨勢方向
    if analysis.Slope > 0.01 {
        analysis.Direction = "increasing"
    } else if analysis.Slope < -0.01 {
        analysis.Direction = "decreasing"
    } else {
        analysis.Direction = "stable"
    }
    
    return analysis
}
```

## 7. 價格快取機制

### 7.1 多層快取

```go
type PricingCache struct {
    memory *MemoryCache
    disk   *DiskCache
    ttl    time.Duration
}

type MemoryCache struct {
    data      *PricingTable
    timestamp time.Time
    mutex     sync.RWMutex
}

func (pc *PricingCache) Get() (*PricingTable, bool) {
    // 先檢查記憶體快取
    if table, hit := pc.memory.Get(); hit {
        return table, true
    }
    
    // 再檢查磁碟快取
    if table, hit := pc.disk.Get(); hit {
        // 更新記憶體快取
        pc.memory.Set(table)
        return table, true
    }
    
    return nil, false
}

func (pc *PricingCache) Set(table *PricingTable) {
    pc.memory.Set(table)
    pc.disk.Set(table)
}

func (mc *MemoryCache) Get() (*PricingTable, bool) {
    mc.mutex.RLock()
    defer mc.mutex.RUnlock()
    
    if mc.data == nil {
        return nil, false
    }
    
    if time.Since(mc.timestamp) > time.Hour {
        return nil, false
    }
    
    return mc.data, true
}
```

## 8. 效能優化

### 8.1 預計算優化

```go
type PrecomputedPricing struct {
    model      string
    multiplier map[TokenType]float64 // 預計算的乘數
}

func NewPrecomputedPricing(model string, pricing *ModelPricing) *PrecomputedPricing {
    return &PrecomputedPricing{
        model: model,
        multiplier: map[TokenType]float64{
            TokenTypeInput:         pricing.InputPrice / 1_000_000,
            TokenTypeOutput:        pricing.OutputPrice / 1_000_000,
            TokenTypeCacheCreation: pricing.CacheCreationPrice / 1_000_000,
            TokenTypeCacheRead:     pricing.CacheReadPrice / 1_000_000,
        },
    }
}

func (pp *PrecomputedPricing) Calculate(tokens TokenCount) float64 {
    return float64(tokens.InputTokens)*pp.multiplier[TokenTypeInput] +
           float64(tokens.OutputTokens)*pp.multiplier[TokenTypeOutput] +
           float64(tokens.CacheCreationTokens)*pp.multiplier[TokenTypeCacheCreation] +
           float64(tokens.CacheReadTokens)*pp.multiplier[TokenTypeCacheRead]
}
```

### 8.2 批次處理優化

```go
type OptimizedBatchCalculator struct {
    precomputed map[string]*PrecomputedPricing
    mutex       sync.RWMutex
}

func (obc *OptimizedBatchCalculator) CalculateBatch(entries []UsageEntry) []float64 {
    results := make([]float64, len(entries))
    
    // 按模型分組以重用預計算
    grouped := obc.groupByModel(entries)
    
    for model, indices := range grouped {
        pc := obc.getOrCreatePrecomputed(model)
        
        for _, idx := range indices {
            entry := entries[idx]
            tokens := TokenCount{
                InputTokens:         entry.InputTokens,
                OutputTokens:        entry.OutputTokens,
                CacheCreationTokens: entry.CacheCreationTokens,
                CacheReadTokens:     entry.CacheReadTokens,
            }
            results[idx] = pc.Calculate(tokens)
        }
    }
    
    return results
}
```

## 9. 測試策略

### 9.1 單元測試

```go
func TestCostCalculator_Calculate(t *testing.T) {
    pricingTable := NewPricingTable()
    pricingTable.UpdatePrice(ModelPricing{
        Model:              "claude-3-opus",
        InputPrice:         15.0,
        OutputPrice:        75.0,
        CacheCreationPrice: 18.75,
        CacheReadPrice:     1.875,
    })
    
    calculator := NewCostCalculator(pricingTable)
    
    testCases := []struct {
        name     string
        model    string
        tokens   TokenCount
        expected float64
    }{
        {
            name:  "basic calculation",
            model: "claude-3-opus",
            tokens: TokenCount{
                InputTokens:  1000000,
                OutputTokens: 500000,
            },
            expected: 52.5, // 15 + 37.5
        },
        {
            name:  "with cache",
            model: "claude-3-opus",
            tokens: TokenCount{
                InputTokens:         1000000,
                OutputTokens:        500000,
                CacheCreationTokens: 100000,
                CacheReadTokens:     50000,
            },
            expected: 54.46875, // 15 + 37.5 + 1.875 + 0.09375
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            cost, err := calculator.Calculate(tc.model, tc.tokens)
            assert.NoError(t, err)
            assert.InDelta(t, tc.expected, cost, 0.000001)
        })
    }
}
```

### 9.2 基準測試

```go
func BenchmarkCostCalculation(b *testing.B) {
    pricingTable := setupTestPricingTable()
    calculator := NewCostCalculator(pricingTable)
    
    tokens := TokenCount{
        InputTokens:         100000,
        OutputTokens:        50000,
        CacheCreationTokens: 10000,
        CacheReadTokens:     5000,
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = calculator.Calculate("claude-3-opus", tokens)
    }
}
```

## 10. 與 TypeScript 版本對照

| TypeScript 函數 | Go 函數 | 說明 |
|----------------|---------|------|
| calculateTotals() | CalculateTotals() | 計算總計 |
| getTotalTokens() | GetTotalTokens() | 獲取總 token 數 |
| PricingFetcher.fetchPricing() | FetchPricing() | 獲取價格 |
| calculateCost() | Calculate() | 計算成本 |

## 11. 錯誤處理

```go
type CostError struct {
    Type    CostErrorType
    Model   string
    Message string
    Cause   error
}

type CostErrorType int

const (
    CostErrorUnknownModel CostErrorType = iota
    CostErrorInvalidTokens
    CostErrorPricingUnavailable
    CostErrorCalculation
)

func (e *CostError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %s (model: %s): %v", 
                          e.Type, e.Message, e.Model, e.Cause)
    }
    return fmt.Sprintf("%s: %s (model: %s)", e.Type, e.Message, e.Model)
}
```