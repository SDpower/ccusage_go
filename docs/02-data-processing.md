# 資料處理模組設計

## 1. 模組概述

資料處理模組負責從本地檔案系統讀取 Claude Code 的使用數據（JSONL 格式），解析並轉換為內部資料結構，是整個系統的數據基礎。

### 1.1 主要功能
- 掃描並定位 Claude 數據目錄
- 並行讀取多個 JSONL 檔案
- 解析並驗證 JSON 數據
- 數據轉換與聚合
- 錯誤處理與恢復
- 記憶體優化管理

### 1.2 對應 TypeScript 模組
- `data-loader.ts` → `loader/loader.go`
- `_session-blocks.ts` → `loader/session.go`
- `_types.ts` → `types/usage.go`

## 2. 資料流程架構

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  File System │────▶│    Scanner   │────▶│   File List  │
└──────────────┘     └──────────────┘     └──────┬───────┘
                                                  │
                                                  ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Aggregator │◀────│    Parser    │◀────│    Reader    │
└──────────────┘     └──────────────┘     └──────────────┘
        │
        ▼
┌──────────────┐
│  Usage Data  │
└──────────────┘
```

## 3. 核心組件設計

### 3.1 檔案掃描器 (Scanner)

```go
package loader

import (
    "context"
    "filepath"
    "os"
)

type Scanner struct {
    basePaths []string
    pattern   string
}

func NewScanner() *Scanner {
    return &Scanner{
        basePaths: getClaudePaths(),
        pattern:   "usage_*.jsonl",
    }
}

func (s *Scanner) Scan(ctx context.Context) ([]string, error) {
    var files []string
    
    for _, basePath := range s.basePaths {
        projectsPath := filepath.Join(basePath, "projects")
        
        err := filepath.Walk(projectsPath, func(path string, info os.FileInfo, err error) error {
            select {
            case <-ctx.Done():
                return ctx.Err()
            default:
                if matched, _ := filepath.Match(s.pattern, filepath.Base(path)); matched {
                    files = append(files, path)
                }
                return nil
            }
        })
        
        if err != nil {
            return nil, err
        }
    }
    
    return files, nil
}
```

### 3.2 並行讀取器 (Parallel Reader)

```go
type ParallelReader struct {
    workerCount int
    bufferSize  int
}

func NewParallelReader(workerCount int) *ParallelReader {
    return &ParallelReader{
        workerCount: workerCount,
        bufferSize:  4096,
    }
}

func (r *ParallelReader) ReadFiles(ctx context.Context, files []string) (<-chan []byte, <-chan error) {
    dataCh := make(chan []byte, r.workerCount)
    errCh := make(chan error, 1)
    
    var wg sync.WaitGroup
    semaphore := make(chan struct{}, r.workerCount)
    
    for _, file := range files {
        wg.Add(1)
        go func(filepath string) {
            defer wg.Done()
            
            semaphore <- struct{}{}
            defer func() { <-semaphore }()
            
            if err := r.readFile(ctx, filepath, dataCh); err != nil {
                select {
                case errCh <- err:
                default:
                }
            }
        }(file)
    }
    
    go func() {
        wg.Wait()
        close(dataCh)
        close(errCh)
    }()
    
    return dataCh, errCh
}
```

### 3.3 JSONL 解析器 (Parser)

```go
type JSONLParser struct {
    decoder     *json.Decoder
    validator   *Validator
    bufferPool  *sync.Pool
}

type UsageEntry struct {
    Timestamp       time.Time `json:"timestamp"`
    Model          string    `json:"model"`
    InputTokens    int       `json:"input_tokens"`
    OutputTokens   int       `json:"output_tokens"`
    CacheTokens    int       `json:"cache_creation_tokens"`
    CacheReadTokens int      `json:"cache_read_tokens"`
    Cost           float64   `json:"cost"`
    SessionID      string    `json:"session_id"`
    RequestID      string    `json:"request_id"`
    ProjectPath    string    `json:"project_path"`
}

func (p *JSONLParser) Parse(data io.Reader) ([]UsageEntry, error) {
    var entries []UsageEntry
    decoder := json.NewDecoder(data)
    
    for {
        var entry UsageEntry
        if err := decoder.Decode(&entry); err != nil {
            if err == io.EOF {
                break
            }
            // 跳過無效行，記錄錯誤但繼續處理
            log.Warn("Failed to parse line", "error", err)
            continue
        }
        
        if err := p.validator.Validate(&entry); err != nil {
            log.Warn("Invalid entry", "error", err)
            continue
        }
        
        entries = append(entries, entry)
    }
    
    return entries, nil
}
```

### 3.4 數據驗證器 (Validator)

```go
type Validator struct {
    minDate time.Time
    maxDate time.Time
}

func (v *Validator) Validate(entry *UsageEntry) error {
    // 驗證時間戳記
    if entry.Timestamp.IsZero() {
        return errors.New("invalid timestamp")
    }
    
    if entry.Timestamp.Before(v.minDate) || entry.Timestamp.After(v.maxDate) {
        return errors.New("timestamp out of range")
    }
    
    // 驗證 token 數量
    if entry.InputTokens < 0 || entry.OutputTokens < 0 {
        return errors.New("negative token count")
    }
    
    // 驗證模型名稱
    if entry.Model == "" {
        return errors.New("empty model name")
    }
    
    // 驗證 session ID 格式
    if !isValidSessionID(entry.SessionID) {
        return errors.New("invalid session ID format")
    }
    
    return nil
}
```

## 4. 記憶體優化策略

### 4.1 串流處理

```go
type StreamProcessor struct {
    chunkSize int
    processor func([]UsageEntry) error
}

func (sp *StreamProcessor) Process(reader io.Reader) error {
    scanner := bufio.NewScanner(reader)
    scanner.Buffer(make([]byte, sp.chunkSize), sp.chunkSize*2)
    
    batch := make([]UsageEntry, 0, 100)
    
    for scanner.Scan() {
        var entry UsageEntry
        if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
            continue
        }
        
        batch = append(batch, entry)
        
        if len(batch) >= 100 {
            if err := sp.processor(batch); err != nil {
                return err
            }
            batch = batch[:0] // 重用 slice
        }
    }
    
    // 處理剩餘數據
    if len(batch) > 0 {
        return sp.processor(batch)
    }
    
    return scanner.Err()
}
```

### 4.2 物件池

```go
var entryPool = sync.Pool{
    New: func() interface{} {
        return &UsageEntry{}
    },
}

var bufferPool = sync.Pool{
    New: func() interface{} {
        return bytes.NewBuffer(make([]byte, 0, 4096))
    },
}

func getEntry() *UsageEntry {
    return entryPool.Get().(*UsageEntry)
}

func putEntry(e *UsageEntry) {
    *e = UsageEntry{} // 重置
    entryPool.Put(e)
}
```

### 4.3 批次處理

```go
type BatchProcessor struct {
    batchSize    int
    flushTimeout time.Duration
    processor    func([]UsageEntry) error
}

func (bp *BatchProcessor) Start(ctx context.Context, input <-chan UsageEntry) error {
    batch := make([]UsageEntry, 0, bp.batchSize)
    ticker := time.NewTicker(bp.flushTimeout)
    defer ticker.Stop()
    
    for {
        select {
        case entry, ok := <-input:
            if !ok {
                // 通道關閉，處理剩餘數據
                if len(batch) > 0 {
                    return bp.processor(batch)
                }
                return nil
            }
            
            batch = append(batch, entry)
            
            if len(batch) >= bp.batchSize {
                if err := bp.processor(batch); err != nil {
                    return err
                }
                batch = batch[:0]
            }
            
        case <-ticker.C:
            // 定時刷新
            if len(batch) > 0 {
                if err := bp.processor(batch); err != nil {
                    return err
                }
                batch = batch[:0]
            }
            
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

## 5. 數據聚合

### 5.1 日期聚合

```go
type DateAggregator struct {
    timezone *time.Location
    locale   string
}

func (da *DateAggregator) AggregateByDay(entries []UsageEntry) map[string]*DailyUsage {
    grouped := make(map[string]*DailyUsage)
    
    for _, entry := range entries {
        // 轉換到指定時區
        localTime := entry.Timestamp.In(da.timezone)
        dateKey := localTime.Format("2006-01-02")
        
        if daily, exists := grouped[dateKey]; exists {
            daily.InputTokens += entry.InputTokens
            daily.OutputTokens += entry.OutputTokens
            daily.CacheTokens += entry.CacheTokens
            daily.CacheReadTokens += entry.CacheReadTokens
            daily.TotalCost += entry.Cost
            daily.AddModel(entry.Model)
        } else {
            grouped[dateKey] = &DailyUsage{
                Date:            dateKey,
                InputTokens:     entry.InputTokens,
                OutputTokens:    entry.OutputTokens,
                CacheTokens:     entry.CacheTokens,
                CacheReadTokens: entry.CacheReadTokens,
                TotalCost:       entry.Cost,
                ModelsUsed:      []string{entry.Model},
            }
        }
    }
    
    return grouped
}
```

### 5.2 會話聚合

```go
type SessionAggregator struct {
    gapThreshold time.Duration
}

func (sa *SessionAggregator) AggregateBySession(entries []UsageEntry) []*SessionBlock {
    // 按時間排序
    sort.Slice(entries, func(i, j int) bool {
        return entries[i].Timestamp.Before(entries[j].Timestamp)
    })
    
    var blocks []*SessionBlock
    var currentBlock *SessionBlock
    
    for _, entry := range entries {
        if currentBlock == nil {
            currentBlock = sa.newBlock(entry)
            continue
        }
        
        // 檢查是否需要新區塊
        if entry.Timestamp.Sub(currentBlock.EndTime) > sa.gapThreshold {
            blocks = append(blocks, currentBlock)
            currentBlock = sa.newBlock(entry)
        } else {
            currentBlock.AddEntry(entry)
        }
    }
    
    if currentBlock != nil {
        blocks = append(blocks, currentBlock)
    }
    
    return blocks
}
```

## 6. 錯誤處理

### 6.1 錯誤類型定義

```go
type LoadError struct {
    Type    ErrorType
    Path    string
    Message string
    Cause   error
}

type ErrorType int

const (
    ErrorTypeFileNotFound ErrorType = iota
    ErrorTypePermission
    ErrorTypeParse
    ErrorTypeValidation
    ErrorTypeIO
)

func (e *LoadError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %s (%s): %v", e.Type, e.Path, e.Message, e.Cause)
    }
    return fmt.Sprintf("%s: %s (%s)", e.Type, e.Path, e.Message)
}
```

### 6.2 錯誤恢復策略

```go
type ErrorHandler struct {
    maxRetries   int
    retryDelay   time.Duration
    skipOnError  bool
    errorLog     []error
}

func (eh *ErrorHandler) Handle(err error, retry func() error) error {
    if eh.skipOnError {
        eh.errorLog = append(eh.errorLog, err)
        return nil // 跳過錯誤繼續處理
    }
    
    for i := 0; i < eh.maxRetries; i++ {
        if err = retry(); err == nil {
            return nil
        }
        
        if !isRetryable(err) {
            break
        }
        
        time.Sleep(eh.retryDelay * time.Duration(i+1))
    }
    
    return err
}
```

## 7. 快取機制

### 7.1 檔案快取

```go
type FileCache struct {
    cache     map[string]*CacheEntry
    mutex     sync.RWMutex
    ttl       time.Duration
    maxSize   int64
    currSize  int64
}

type CacheEntry struct {
    Data      []UsageEntry
    Timestamp time.Time
    Size      int64
}

func (fc *FileCache) Get(path string) ([]UsageEntry, bool) {
    fc.mutex.RLock()
    defer fc.mutex.RUnlock()
    
    entry, exists := fc.cache[path]
    if !exists {
        return nil, false
    }
    
    if time.Since(entry.Timestamp) > fc.ttl {
        return nil, false
    }
    
    return entry.Data, true
}

func (fc *FileCache) Set(path string, data []UsageEntry) {
    fc.mutex.Lock()
    defer fc.mutex.Unlock()
    
    size := int64(len(data) * 200) // 估算大小
    
    // LRU 淘汰策略
    for fc.currSize+size > fc.maxSize && len(fc.cache) > 0 {
        fc.evictOldest()
    }
    
    fc.cache[path] = &CacheEntry{
        Data:      data,
        Timestamp: time.Now(),
        Size:      size,
    }
    fc.currSize += size
}
```

## 8. 效能指標

### 8.1 基準測試

```go
func BenchmarkParallelLoad(b *testing.B) {
    loader := NewParallelReader(runtime.NumCPU())
    files := generateTestFiles(100, 10000) // 100 files, 10000 lines each
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ctx := context.Background()
        dataCh, errCh := loader.ReadFiles(ctx, files)
        
        for range dataCh {
            // consume data
        }
        
        select {
        case err := <-errCh:
            if err != nil {
                b.Fatal(err)
            }
        default:
        }
    }
}
```

### 8.2 效能目標

| 操作 | 目標時間 | 記憶體使用 |
|------|---------|-----------|
| 載入 100MB 數據 | < 200ms | < 50MB |
| 載入 1GB 數據 | < 2s | < 200MB |
| 解析 10000 行 | < 50ms | < 10MB |
| 聚合 100000 條記錄 | < 100ms | < 30MB |

## 9. 與 TypeScript 版本對照

### 9.1 功能對照表

| TypeScript 函數 | Go 函數 | 說明 |
|----------------|---------|------|
| getClaudePaths() | GetClaudePaths() | 獲取 Claude 數據路徑 |
| loadDailyUsageData() | LoadDailyUsage() | 載入日使用數據 |
| loadSessionBlockData() | LoadSessionBlocks() | 載入會話區塊 |
| parseJSONL() | ParseJSONL() | 解析 JSONL |
| aggregateByDate() | AggregateByDate() | 按日期聚合 |

### 9.2 類型對照

| TypeScript 類型 | Go 類型 | 說明 |
|----------------|---------|------|
| UsageEntry | UsageEntry | 使用記錄 |
| DailyUsage | DailyUsage | 日使用統計 |
| SessionBlock | SessionBlock | 會話區塊 |
| LoadOptions | LoadOptions | 載入選項 |

## 10. 測試策略

### 10.1 單元測試

```go
func TestParser_Parse(t *testing.T) {
    parser := NewJSONLParser()
    
    testCases := []struct {
        name     string
        input    string
        expected []UsageEntry
        wantErr  bool
    }{
        {
            name: "valid entry",
            input: `{"timestamp":"2024-01-01T00:00:00Z","model":"claude-3","input_tokens":100}`,
            expected: []UsageEntry{{
                Timestamp:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
                Model:        "claude-3",
                InputTokens:  100,
            }},
            wantErr: false,
        },
        {
            name:     "invalid json",
            input:    `{invalid}`,
            expected: nil,
            wantErr:  true,
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            reader := strings.NewReader(tc.input)
            result, err := parser.Parse(reader)
            
            if tc.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tc.expected, result)
            }
        })
    }
}
```

### 10.2 整合測試

```go
func TestIntegration_LoadAndAggregate(t *testing.T) {
    // 建立測試數據
    tmpDir := t.TempDir()
    createTestJSONLFiles(t, tmpDir)
    
    // 執行載入和聚合
    loader := NewLoader()
    entries, err := loader.Load(context.Background(), tmpDir)
    require.NoError(t, err)
    
    aggregator := NewDateAggregator(time.UTC, "en-US")
    result := aggregator.AggregateByDay(entries)
    
    // 驗證結果
    assert.Len(t, result, 7) // 預期 7 天的數據
    assert.Greater(t, result["2024-01-01"].TotalCost, 0.0)
}
```

## 11. 下一步優化

1. **增量載入**：只載入新增或修改的檔案
2. **壓縮支援**：支援讀取壓縮的 JSONL 檔案
3. **索引建立**：建立時間索引加速查詢
4. **分散式處理**：支援多機器分散處理大量數據
5. **即時更新**：使用 fsnotify 監控檔案變化