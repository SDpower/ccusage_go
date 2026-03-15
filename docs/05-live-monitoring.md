# 即時監控模組設計

## 1. 模組概述

即時監控模組提供實時的使用量追蹤和成本預測功能，讓使用者能夠即時了解當前的使用狀況、燃燒率和預計成本。該模組支援動態更新的終端介面，並提供視覺化的使用趨勢。

### 1.1 主要功能
- 即時數據流處理
- 動態終端 UI 更新
- 使用量燃燒率計算
- 成本預測與警告
- 會話追蹤與統計
- 自動刷新機制

### 1.2 對應 TypeScript 模組
- `_live-monitor.ts` → `monitor/live.go`
- `_live-rendering.ts` → `monitor/renderer.go`
- `commands/_blocks.live.ts` → `commands/blocks_live.go`

## 2. 即時監控架構

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│ File Watcher │────▶│ Data Stream  │────▶│   Analyzer   │
└──────────────┘     └──────────────┘     └──────┬───────┘
                                                  │
                     ┌────────────────────────────┼────────────────────────────┐
                     │                            │                            │
              ┌──────▼──────┐            ┌───────▼──────┐            ┌────────▼────────┐
              │  Calculator │            │   Predictor  │            │    Alerter      │
              └──────┬──────┘            └───────┬──────┘            └────────┬────────┘
                     │                            │                            │
                     └────────────────────────────┼────────────────────────────┘
                                                  │
                                           ┌──────▼──────┐
                                           │   Renderer  │
                                           └──────┬──────┘
                                                  │
                                           ┌──────▼──────┐
                                           │   Terminal  │
                                           └─────────────┘
```

## 3. 檔案監控系統

### 3.1 檔案監視器

```go
package monitor

import (
    "context"
    "path/filepath"
    "github.com/fsnotify/fsnotify"
)

type FileWatcher struct {
    watcher    *fsnotify.Watcher
    paths      []string
    pattern    string
    eventChan  chan FileEvent
    errorChan  chan error
}

type FileEvent struct {
    Type      EventType
    Path      string
    Timestamp time.Time
}

type EventType int

const (
    EventTypeCreate EventType = iota
    EventTypeModify
    EventTypeDelete
)

func NewFileWatcher(paths []string) (*FileWatcher, error) {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return nil, err
    }
    
    return &FileWatcher{
        watcher:   watcher,
        paths:     paths,
        pattern:   "usage_*.jsonl",
        eventChan: make(chan FileEvent, 100),
        errorChan: make(chan error, 10),
    }, nil
}

func (fw *FileWatcher) Start(ctx context.Context) error {
    // 添加監視路徑
    for _, path := range fw.paths {
        projectsPath := filepath.Join(path, "projects")
        if err := fw.watcher.Add(projectsPath); err != nil {
            return err
        }
    }
    
    go fw.watchLoop(ctx)
    return nil
}

func (fw *FileWatcher) watchLoop(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
            
        case event, ok := <-fw.watcher.Events:
            if !ok {
                return
            }
            
            if fw.matchPattern(event.Name) {
                fw.handleEvent(event)
            }
            
        case err, ok := <-fw.watcher.Errors:
            if !ok {
                return
            }
            fw.errorChan <- err
        }
    }
}

func (fw *FileWatcher) handleEvent(event fsnotify.Event) {
    var eventType EventType
    
    switch {
    case event.Op&fsnotify.Create == fsnotify.Create:
        eventType = EventTypeCreate
    case event.Op&fsnotify.Write == fsnotify.Write:
        eventType = EventTypeModify
    case event.Op&fsnotify.Remove == fsnotify.Remove:
        eventType = EventTypeDelete
    default:
        return
    }
    
    fw.eventChan <- FileEvent{
        Type:      eventType,
        Path:      event.Name,
        Timestamp: time.Now(),
    }
}
```

### 3.2 增量數據讀取器

```go
type IncrementalReader struct {
    offsets map[string]int64 // 檔案偏移記錄
    mutex   sync.RWMutex
}

func NewIncrementalReader() *IncrementalReader {
    return &IncrementalReader{
        offsets: make(map[string]int64),
    }
}

func (ir *IncrementalReader) ReadNew(filepath string) ([]UsageEntry, error) {
    ir.mutex.Lock()
    defer ir.mutex.Unlock()
    
    file, err := os.Open(filepath)
    if err != nil {
        return nil, err
    }
    defer file.Close()
    
    // 獲取上次讀取位置
    offset, exists := ir.offsets[filepath]
    if exists {
        if _, err := file.Seek(offset, 0); err != nil {
            return nil, err
        }
    }
    
    // 讀取新數據
    var entries []UsageEntry
    scanner := bufio.NewScanner(file)
    
    for scanner.Scan() {
        var entry UsageEntry
        if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
            continue // 跳過錯誤行
        }
        entries = append(entries, entry)
    }
    
    // 更新偏移
    newOffset, _ := file.Seek(0, io.SeekCurrent)
    ir.offsets[filepath] = newOffset
    
    return entries, scanner.Err()
}
```

## 4. 數據流處理

### 4.1 實時數據流

```go
type DataStream struct {
    watcher    *FileWatcher
    reader     *IncrementalReader
    processor  *StreamProcessor
    outputChan chan StreamData
}

type StreamData struct {
    Entries   []UsageEntry
    Timestamp time.Time
    Source    string
}

func NewDataStream(watcher *FileWatcher) *DataStream {
    return &DataStream{
        watcher:    watcher,
        reader:     NewIncrementalReader(),
        processor:  NewStreamProcessor(),
        outputChan: make(chan StreamData, 100),
    }
}

func (ds *DataStream) Start(ctx context.Context) error {
    // 初始載入所有現有數據
    if err := ds.loadInitialData(); err != nil {
        return err
    }
    
    // 開始監聽變化
    go ds.processEvents(ctx)
    
    return nil
}

func (ds *DataStream) processEvents(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
            
        case event := <-ds.watcher.Events():
            if event.Type == EventTypeModify || event.Type == EventTypeCreate {
                entries, err := ds.reader.ReadNew(event.Path)
                if err != nil {
                    log.Error("Failed to read file", "path", event.Path, "error", err)
                    continue
                }
                
                if len(entries) > 0 {
                    processed := ds.processor.Process(entries)
                    ds.outputChan <- StreamData{
                        Entries:   processed,
                        Timestamp: time.Now(),
                        Source:    event.Path,
                    }
                }
            }
        }
    }
}
```

### 4.2 流處理器

```go
type StreamProcessor struct {
    filters    []Filter
    transforms []Transform
}

type Filter func(UsageEntry) bool
type Transform func(UsageEntry) UsageEntry

func (sp *StreamProcessor) Process(entries []UsageEntry) []UsageEntry {
    result := make([]UsageEntry, 0, len(entries))
    
    for _, entry := range entries {
        // 應用過濾器
        if !sp.applyFilters(entry) {
            continue
        }
        
        // 應用轉換
        transformed := sp.applyTransforms(entry)
        result = append(result, transformed)
    }
    
    return result
}

func (sp *StreamProcessor) AddFilter(filter Filter) {
    sp.filters = append(sp.filters, filter)
}

func (sp *StreamProcessor) AddTransform(transform Transform) {
    sp.transforms = append(sp.transforms, transform)
}
```

## 5. 即時分析引擎

### 5.1 燃燒率計算器

```go
type BurnRateCalculator struct {
    window    time.Duration
    history   []BurnRatePoint
    mutex     sync.RWMutex
}

type BurnRatePoint struct {
    Timestamp   time.Time
    Tokens      int
    Cost        float64
}

type BurnRate struct {
    TokensPerHour float64
    CostPerHour   float64
    TokensPerDay  float64
    CostPerDay    float64
    Trend         string // "increasing", "decreasing", "stable"
}

func (brc *BurnRateCalculator) Calculate() *BurnRate {
    brc.mutex.RLock()
    defer brc.mutex.RUnlock()
    
    if len(brc.history) < 2 {
        return nil
    }
    
    // 計算時間範圍
    first := brc.history[0]
    last := brc.history[len(brc.history)-1]
    duration := last.Timestamp.Sub(first.Timestamp)
    
    if duration == 0 {
        return nil
    }
    
    // 計算總量
    totalTokens := 0
    totalCost := 0.0
    for _, point := range brc.history {
        totalTokens += point.Tokens
        totalCost += point.Cost
    }
    
    // 計算速率
    hours := duration.Hours()
    rate := &BurnRate{
        TokensPerHour: float64(totalTokens) / hours,
        CostPerHour:   totalCost / hours,
        TokensPerDay:  float64(totalTokens) / hours * 24,
        CostPerDay:    totalCost / hours * 24,
    }
    
    // 計算趨勢
    rate.Trend = brc.calculateTrend()
    
    return rate
}

func (brc *BurnRateCalculator) calculateTrend() string {
    if len(brc.history) < 10 {
        return "stable"
    }
    
    // 比較前半部分和後半部分的平均值
    mid := len(brc.history) / 2
    
    var firstHalfAvg, secondHalfAvg float64
    for i := 0; i < mid; i++ {
        firstHalfAvg += brc.history[i].Cost
    }
    firstHalfAvg /= float64(mid)
    
    for i := mid; i < len(brc.history); i++ {
        secondHalfAvg += brc.history[i].Cost
    }
    secondHalfAvg /= float64(len(brc.history) - mid)
    
    diff := (secondHalfAvg - firstHalfAvg) / firstHalfAvg
    
    if diff > 0.1 {
        return "increasing"
    } else if diff < -0.1 {
        return "decreasing"
    }
    return "stable"
}
```

### 5.2 預測引擎

```go
type PredictionEngine struct {
    burnRateCalc *BurnRateCalculator
    blockDuration time.Duration
}

type Prediction struct {
    EstimatedBlockCost   float64
    EstimatedBlockTokens int
    TimeRemaining        time.Duration
    Confidence           float64
    Warning              *Warning
}

type Warning struct {
    Level   WarningLevel
    Message string
}

type WarningLevel int

const (
    WarningLevelInfo WarningLevel = iota
    WarningLevelCaution
    WarningLevelDanger
)

func (pe *PredictionEngine) Predict(currentBlock *BlockReport) *Prediction {
    burnRate := pe.burnRateCalc.Calculate()
    if burnRate == nil {
        return nil
    }
    
    elapsed := time.Since(currentBlock.StartTime)
    remaining := pe.blockDuration - elapsed
    
    if remaining <= 0 {
        return &Prediction{
            EstimatedBlockCost:   currentBlock.TotalCost,
            EstimatedBlockTokens: currentBlock.TotalTokens,
            TimeRemaining:        0,
            Confidence:           1.0,
        }
    }
    
    // 基於燃燒率預測
    remainingHours := remaining.Hours()
    estimatedAdditionalCost := burnRate.CostPerHour * remainingHours
    estimatedAdditionalTokens := int(burnRate.TokensPerHour * remainingHours)
    
    prediction := &Prediction{
        EstimatedBlockCost:   currentBlock.TotalCost + estimatedAdditionalCost,
        EstimatedBlockTokens: currentBlock.TotalTokens + estimatedAdditionalTokens,
        TimeRemaining:        remaining,
        Confidence:           pe.calculateConfidence(elapsed),
    }
    
    // 檢查警告
    prediction.Warning = pe.checkWarning(prediction)
    
    return prediction
}

func (pe *PredictionEngine) checkWarning(pred *Prediction) *Warning {
    const (
        cautionThreshold = 100.0  // $100
        dangerThreshold  = 200.0  // $200
    )
    
    if pred.EstimatedBlockCost > dangerThreshold {
        return &Warning{
            Level:   WarningLevelDanger,
            Message: fmt.Sprintf("Estimated cost $%.2f exceeds danger threshold", pred.EstimatedBlockCost),
        }
    }
    
    if pred.EstimatedBlockCost > cautionThreshold {
        return &Warning{
            Level:   WarningLevelCaution,
            Message: fmt.Sprintf("Estimated cost $%.2f exceeds caution threshold", pred.EstimatedBlockCost),
        }
    }
    
    return nil
}
```

## 6. 終端渲染系統 (使用 Bubble Tea)

### 6.1 Bubble Tea 即時監控介面

```go
package monitor

import (
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/bubbles/progress"
    "github.com/charmbracelet/bubbles/spinner"
    "github.com/charmbracelet/lipgloss"
)

type LiveMonitorModel struct {
    spinner     spinner.Model
    progress    progress.Model
    burnRate    *BurnRate
    prediction  *Prediction
    sessions    []*LiveSession
    width       int
    height      int
    refreshRate time.Duration
    dataChan    <-chan MonitoringData
}

// 初始化即時監控
func NewLiveMonitor(dataChan <-chan MonitoringData) LiveMonitorModel {
    s := spinner.New()
    s.Spinner = spinner.Dot
    s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
    
    return LiveMonitorModel{
        spinner:     s,
        progress:    progress.New(progress.WithDefaultGradient()),
        refreshRate: time.Second,
        dataChan:    dataChan,
    }
}

// Bubble Tea Init
func (m LiveMonitorModel) Init() tea.Cmd {
    return tea.Batch(
        m.spinner.Tick,
        m.waitForData(),
    )
}

// 等待資料更新
func (m LiveMonitorModel) waitForData() tea.Cmd {
    return func() tea.Msg {
        select {
        case data := <-m.dataChan:
            return data
        case <-time.After(m.refreshRate):
            return tickMsg{}
        }
    }
}

type tickMsg struct{}

// Bubble Tea Update
func (m LiveMonitorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        m.progress.Width = msg.Width - 4
        
    case MonitoringData:
        m.burnRate = msg.BurnRate
        m.prediction = msg.Prediction
        m.sessions = msg.Sessions
        return m, m.waitForData()
        
    case tickMsg:
        return m, m.waitForData()
        
    case tea.KeyMsg:
        switch msg.String() {
        case "q", "ctrl+c":
            return m, tea.Quit
        }
        
    case spinner.TickMsg:
        var cmd tea.Cmd
        m.spinner, cmd = m.spinner.Update(msg)
        return m, cmd
    }
    
    return m, nil
}

// Bubble Tea View
func (m LiveMonitorModel) View() string {
    if m.width == 0 {
        return "Initializing..."
    }
    
    var s strings.Builder
    
    // 標題
    titleStyle := lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("86")).
        MarginBottom(1)
    
    s.WriteString(titleStyle.Render("🔥 Live Usage Monitor"))
    s.WriteString("\n\n")
    
    // 燃燒率
    if m.burnRate != nil {
        s.WriteString(m.renderBurnRate())
        s.WriteString("\n")
    }
    
    // 預測
    if m.prediction != nil {
        s.WriteString(m.renderPrediction())
        s.WriteString("\n")
    }
    
    // 活躍會話
    if len(m.sessions) > 0 {
        s.WriteString(m.renderSessions())
        s.WriteString("\n")
    }
    
    // 進度條
    if m.prediction != nil && m.prediction.TimeRemaining > 0 {
        progress := 1.0 - (m.prediction.TimeRemaining.Seconds() / (5 * time.Hour).Seconds())
        s.WriteString(m.progress.ViewAs(progress))
        s.WriteString("\n")
    }
    
    // Footer
    footerStyle := lipgloss.NewStyle().
        Foreground(lipgloss.Color("241")).
        MarginTop(1)
    
    s.WriteString(footerStyle.Render("Press 'q' to quit • Updates every second"))
    
    return s.String()
}

func (m LiveMonitorModel) renderBurnRate() string {
    style := lipgloss.NewStyle().
        BorderStyle(lipgloss.RoundedBorder()).
        BorderForeground(lipgloss.Color("62")).
        Padding(1, 2)
    
    content := fmt.Sprintf(
        "💰 Burn Rate\n\n"+
        "Tokens/Hour: %s\n"+
        "Cost/Hour: %s\n"+
        "Trend: %s",
        formatNumber(int(m.burnRate.TokensPerHour)),
        formatCurrency(m.burnRate.CostPerHour),
        m.renderTrend(m.burnRate.Trend),
    )
    
    return style.Render(content)
}

func (m LiveMonitorModel) renderTrend(trend string) string {
    switch trend {
    case "increasing":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("📈 " + trend)
    case "decreasing":
        return lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("📉 " + trend)
    default:
        return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("➡️ " + trend)
    }
}
```

### 6.2 佈局管理器

```go
type Layout struct {
    sections []Section
    widthFunc func() int
}

type Section struct {
    Name     string
    Height   int
    Renderer func(width, height int, data interface{}) string
}

func (l *Layout) Render(data RenderData) string {
    var output strings.Builder
    width := l.widthFunc()
    
    for _, section := range l.sections {
        content := section.Renderer(width, section.Height, data)
        output.WriteString(content)
        output.WriteString("\n")
    }
    
    return output.String()
}

func createDefaultLayout() *Layout {
    return &Layout{
        sections: []Section{
            {
                Name:     "header",
                Height:   3,
                Renderer: renderHeader,
            },
            {
                Name:     "stats",
                Height:   10,
                Renderer: renderStats,
            },
            {
                Name:     "chart",
                Height:   15,
                Renderer: renderChart,
            },
            {
                Name:     "warnings",
                Height:   5,
                Renderer: renderWarnings,
            },
        },
        widthFunc: getTerminalWidth,
    }
}
```

### 6.3 圖表渲染

```go
type ChartRenderer struct {
    width      int
    height     int
    dataPoints []ChartPoint
}

type ChartPoint struct {
    Time  time.Time
    Value float64
    Label string
}

func (cr *ChartRenderer) RenderLineChart() string {
    if len(cr.dataPoints) == 0 {
        return "No data available"
    }
    
    // 找出最大最小值
    minVal, maxVal := cr.findMinMax()
    
    // 創建圖表矩陣
    chart := cr.createChartMatrix()
    
    // 繪製數據點
    cr.plotPoints(chart, minVal, maxVal)
    
    // 繪製軸線
    cr.drawAxes(chart)
    
    // 轉換為字串
    return cr.matrixToString(chart)
}

func (cr *ChartRenderer) plotPoints(chart [][]rune, minVal, maxVal float64) {
    for i, point := range cr.dataPoints {
        x := i * cr.width / len(cr.dataPoints)
        y := cr.height - int((point.Value-minVal)/(maxVal-minVal)*float64(cr.height))
        
        if x >= 0 && x < cr.width && y >= 0 && y < cr.height {
            chart[y][x] = '●'
        }
    }
}

func (cr *ChartRenderer) drawAxes(chart [][]rune) {
    // 繪製 Y 軸
    for y := 0; y < cr.height; y++ {
        chart[y][0] = '│'
    }
    
    // 繪製 X 軸
    for x := 0; x < cr.width; x++ {
        chart[cr.height-1][x] = '─'
    }
    
    // 原點
    chart[cr.height-1][0] = '└'
}
```

## 7. 會話追蹤

### 7.1 活躍會話監控

```go
type SessionMonitor struct {
    sessions map[string]*LiveSession
    mutex    sync.RWMutex
}

type LiveSession struct {
    ID            string
    StartTime     time.Time
    LastActivity  time.Time
    TotalTokens   int
    TotalCost     float64
    RequestCount  int
    IsActive      bool
    BurnRate      *BurnRate
}

func (sm *SessionMonitor) UpdateSession(entry UsageEntry) {
    sm.mutex.Lock()
    defer sm.mutex.Unlock()
    
    session, exists := sm.sessions[entry.SessionID]
    if !exists {
        session = &LiveSession{
            ID:        entry.SessionID,
            StartTime: entry.Timestamp,
            IsActive:  true,
        }
        sm.sessions[entry.SessionID] = session
    }
    
    // 更新會話資訊
    session.LastActivity = entry.Timestamp
    session.TotalTokens += entry.InputTokens + entry.OutputTokens
    session.TotalCost += entry.Cost
    session.RequestCount++
    
    // 更新燃燒率
    session.BurnRate = sm.calculateSessionBurnRate(session)
    
    // 檢查是否仍活躍
    if time.Since(session.LastActivity) > 5*time.Hour {
        session.IsActive = false
    }
}

func (sm *SessionMonitor) GetActiveSessions() []*LiveSession {
    sm.mutex.RLock()
    defer sm.mutex.RUnlock()
    
    var active []*LiveSession
    for _, session := range sm.sessions {
        if session.IsActive {
            active = append(active, session)
        }
    }
    
    // 按最後活動時間排序
    sort.Slice(active, func(i, j int) bool {
        return active[i].LastActivity.After(active[j].LastActivity)
    })
    
    return active
}
```

## 8. 警報系統

### 8.1 警報管理器

```go
type AlertManager struct {
    rules     []AlertRule
    handlers  []AlertHandler
    history   []Alert
    mutex     sync.RWMutex
}

type AlertRule struct {
    Name      string
    Condition func(data MonitoringData) bool
    Level     AlertLevel
    Message   string
}

type Alert struct {
    Rule      AlertRule
    Timestamp time.Time
    Data      interface{}
    Handled   bool
}

type AlertLevel int

const (
    AlertLevelInfo AlertLevel = iota
    AlertLevelWarning
    AlertLevelCritical
)

func (am *AlertManager) Check(data MonitoringData) {
    am.mutex.Lock()
    defer am.mutex.Unlock()
    
    for _, rule := range am.rules {
        if rule.Condition(data) {
            alert := Alert{
                Rule:      rule,
                Timestamp: time.Now(),
                Data:      data,
            }
            
            am.history = append(am.history, alert)
            am.notify(alert)
        }
    }
}

func (am *AlertManager) notify(alert Alert) {
    for _, handler := range am.handlers {
        go handler.Handle(alert)
    }
}

// 預設警報規則
func createDefaultAlertRules() []AlertRule {
    return []AlertRule{
        {
            Name: "high_burn_rate",
            Condition: func(data MonitoringData) bool {
                return data.BurnRate != nil && data.BurnRate.CostPerHour > 50.0
            },
            Level:   AlertLevelWarning,
            Message: "High burn rate detected",
        },
        {
            Name: "block_limit_approaching",
            Condition: func(data MonitoringData) bool {
                if data.CurrentBlock == nil {
                    return false
                }
                remaining := data.CurrentBlock.TimeRemaining
                return remaining < 30*time.Minute && data.CurrentBlock.TotalCost > 150.0
            },
            Level:   AlertLevelCritical,
            Message: "Block limit approaching with high cost",
        },
    }
}
```

## 9. 效能優化

### 9.0 增量快取 (v0.11.1)

`blocks --live` 使用 `IncrementalCache` 以 project 目錄為單位追蹤檔案狀態（ModTime+Size），避免每次 tick 完整重載。無變動時直接回傳快取結果，CPU 使用率降低 68%。詳見 `docs/15-blocks-live-implementation.md`。

### 9.1 緩衝區管理

```go
type BufferManager struct {
    buffers map[string]*CircularBuffer
    mutex   sync.RWMutex
}

type CircularBuffer struct {
    data     []interface{}
    capacity int
    head     int
    tail     int
    size     int
    mutex    sync.RWMutex
}

func NewCircularBuffer(capacity int) *CircularBuffer {
    return &CircularBuffer{
        data:     make([]interface{}, capacity),
        capacity: capacity,
    }
}

func (cb *CircularBuffer) Push(item interface{}) {
    cb.mutex.Lock()
    defer cb.mutex.Unlock()
    
    cb.data[cb.tail] = item
    cb.tail = (cb.tail + 1) % cb.capacity
    
    if cb.size < cb.capacity {
        cb.size++
    } else {
        cb.head = (cb.head + 1) % cb.capacity
    }
}

func (cb *CircularBuffer) GetAll() []interface{} {
    cb.mutex.RLock()
    defer cb.mutex.RUnlock()
    
    result := make([]interface{}, cb.size)
    for i := 0; i < cb.size; i++ {
        idx := (cb.head + i) % cb.capacity
        result[i] = cb.data[idx]
    }
    
    return result
}
```

### 9.2 資源管理

```go
type ResourceManager struct {
    maxMemory   int64
    maxGoroutines int
    currentMem  int64
    goroutines  int
    mutex       sync.RWMutex
}

func (rm *ResourceManager) CanAllocate(size int64) bool {
    rm.mutex.RLock()
    defer rm.mutex.RUnlock()
    
    return rm.currentMem+size <= rm.maxMemory
}

func (rm *ResourceManager) Allocate(size int64) bool {
    rm.mutex.Lock()
    defer rm.mutex.Unlock()
    
    if rm.currentMem+size > rm.maxMemory {
        return false
    }
    
    rm.currentMem += size
    return true
}

func (rm *ResourceManager) Release(size int64) {
    rm.mutex.Lock()
    defer rm.mutex.Unlock()
    
    rm.currentMem -= size
    if rm.currentMem < 0 {
        rm.currentMem = 0
    }
}
```

## 10. 測試策略

### 10.1 單元測試

```go
func TestBurnRateCalculator(t *testing.T) {
    calc := NewBurnRateCalculator(time.Hour)
    
    // 添加測試數據
    now := time.Now()
    for i := 0; i < 10; i++ {
        calc.AddPoint(BurnRatePoint{
            Timestamp: now.Add(time.Duration(i) * 10 * time.Minute),
            Tokens:    1000 * (i + 1),
            Cost:      0.1 * float64(i+1),
        })
    }
    
    rate := calc.Calculate()
    
    assert.NotNil(t, rate)
    assert.Greater(t, rate.TokensPerHour, 0.0)
    assert.Greater(t, rate.CostPerHour, 0.0)
    assert.Contains(t, []string{"increasing", "decreasing", "stable"}, rate.Trend)
}
```

### 10.2 整合測試

```go
func TestLiveMonitoringIntegration(t *testing.T) {
    // 創建測試環境
    tmpDir := t.TempDir()
    testFile := filepath.Join(tmpDir, "usage_test.jsonl")
    
    // 啟動監控
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    monitor := NewLiveMonitor()
    err := monitor.Start(ctx, []string{tmpDir})
    require.NoError(t, err)
    
    // 模擬數據寫入
    go simulateDataWrites(testFile)
    
    // 等待並驗證
    time.Sleep(5 * time.Second)
    
    stats := monitor.GetCurrentStats()
    assert.Greater(t, stats.TotalEntries, 0)
    assert.NotNil(t, stats.BurnRate)
}
```

## 11. 與 TypeScript 版本對照

| TypeScript 函數 | Go 函數 | 說明 |
|----------------|---------|------|
| startLiveMonitoring() | StartLiveMonitoring() | 啟動即時監控 |
| calculateBurnRate() | CalculateBurnRate() | 計算燃燒率 |
| projectBlockUsage() | PredictBlockUsage() | 預測區塊使用 |
| renderLiveChart() | RenderChart() | 渲染即時圖表 |