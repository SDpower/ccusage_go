# 技術實作細節文檔

## 1. Goroutine 使用指南

### 1.1 並發模式

#### Worker Pool 模式
```go
type WorkerPool struct {
    workers   int
    taskQueue chan Task
    results   chan Result
    wg        sync.WaitGroup
}

func NewWorkerPool(workers int) *WorkerPool {
    return &WorkerPool{
        workers:   workers,
        taskQueue: make(chan Task, workers*2),
        results:   make(chan Result, workers),
    }
}

func (wp *WorkerPool) Start(ctx context.Context) {
    for i := 0; i < wp.workers; i++ {
        wp.wg.Add(1)
        go wp.worker(ctx, i)
    }
}

func (wp *WorkerPool) worker(ctx context.Context, id int) {
    defer wp.wg.Done()
    
    for {
        select {
        case <-ctx.Done():
            return
        case task, ok := <-wp.taskQueue:
            if !ok {
                return
            }
            
            result := wp.processTask(task)
            
            select {
            case wp.results <- result:
            case <-ctx.Done():
                return
            }
        }
    }
}

func (wp *WorkerPool) Submit(task Task) error {
    select {
    case wp.taskQueue <- task:
        return nil
    default:
        return errors.New("task queue full")
    }
}

func (wp *WorkerPool) Close() {
    close(wp.taskQueue)
    wp.wg.Wait()
    close(wp.results)
}
```

#### Fan-Out/Fan-In 模式
```go
func FanOutFanIn(ctx context.Context, input <-chan Data) <-chan Result {
    // Fan-out
    numWorkers := runtime.NumCPU()
    workers := make([]<-chan Result, numWorkers)
    
    for i := 0; i < numWorkers; i++ {
        workers[i] = processData(ctx, input)
    }
    
    // Fan-in
    return merge(ctx, workers...)
}

func processData(ctx context.Context, input <-chan Data) <-chan Result {
    output := make(chan Result)
    
    go func() {
        defer close(output)
        
        for data := range input {
            select {
            case <-ctx.Done():
                return
            case output <- process(data):
            }
        }
    }()
    
    return output
}

func merge(ctx context.Context, channels ...<-chan Result) <-chan Result {
    var wg sync.WaitGroup
    output := make(chan Result)
    
    multiplex := func(c <-chan Result) {
        defer wg.Done()
        for result := range c {
            select {
            case <-ctx.Done():
                return
            case output <- result:
            }
        }
    }
    
    wg.Add(len(channels))
    for _, c := range channels {
        go multiplex(c)
    }
    
    go func() {
        wg.Wait()
        close(output)
    }()
    
    return output
}
```

### 1.2 同步機制

#### 使用 sync.Mutex
```go
type SafeCounter struct {
    mu    sync.RWMutex
    count int64
}

func (sc *SafeCounter) Increment() {
    sc.mu.Lock()
    defer sc.mu.Unlock()
    sc.count++
}

func (sc *SafeCounter) Value() int64 {
    sc.mu.RLock()
    defer sc.mu.RUnlock()
    return sc.count
}

// 使用 atomic 替代 mutex 以提升效能
type AtomicCounter struct {
    count int64
}

func (ac *AtomicCounter) Increment() {
    atomic.AddInt64(&ac.count, 1)
}

func (ac *AtomicCounter) Value() int64 {
    return atomic.LoadInt64(&ac.count)
}
```

#### 使用 Channel 進行同步
```go
type Coordinator struct {
    tasks    chan Task
    results  chan Result
    done     chan struct{}
    errors   chan error
}

func (c *Coordinator) Run(ctx context.Context) error {
    errGroup, ctx := errgroup.WithContext(ctx)
    
    // 生產者
    errGroup.Go(func() error {
        defer close(c.tasks)
        return c.produceTasks(ctx)
    })
    
    // 消費者
    for i := 0; i < runtime.NumCPU(); i++ {
        errGroup.Go(func() error {
            return c.consumeTasks(ctx)
        })
    }
    
    // 結果收集器
    errGroup.Go(func() error {
        return c.collectResults(ctx)
    })
    
    return errGroup.Wait()
}
```

### 1.3 Context 使用最佳實踐

```go
// Context 傳遞與取消
func ProcessWithTimeout(ctx context.Context, timeout time.Duration) error {
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    
    resultCh := make(chan Result, 1)
    errCh := make(chan error, 1)
    
    go func() {
        result, err := longRunningOperation()
        if err != nil {
            errCh <- err
            return
        }
        resultCh <- result
    }()
    
    select {
    case <-ctx.Done():
        return ctx.Err()
    case err := <-errCh:
        return err
    case result := <-resultCh:
        return processResult(result)
    }
}

// Context 值傳遞
type contextKey string

const (
    requestIDKey contextKey = "requestID"
    userIDKey    contextKey = "userID"
)

func WithRequestID(ctx context.Context, requestID string) context.Context {
    return context.WithValue(ctx, requestIDKey, requestID)
}

func GetRequestID(ctx context.Context) string {
    if v := ctx.Value(requestIDKey); v != nil {
        return v.(string)
    }
    return ""
}
```

## 2. 錯誤處理模式

### 2.1 錯誤包裝與追蹤

```go
// 自定義錯誤類型
type AppError struct {
    Code      string
    Message   string
    Err       error
    Stack     []byte
    Timestamp time.Time
}

func NewAppError(code, message string, err error) *AppError {
    return &AppError{
        Code:      code,
        Message:   message,
        Err:       err,
        Stack:     debug.Stack(),
        Timestamp: time.Now(),
    }
}

func (e *AppError) Error() string {
    if e.Err != nil {
        return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
    }
    return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
    return e.Err
}

// 錯誤處理鏈
func ProcessData(data []byte) error {
    if err := validateData(data); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }
    
    result, err := parseData(data)
    if err != nil {
        return fmt.Errorf("parsing failed: %w", err)
    }
    
    if err := saveResult(result); err != nil {
        return fmt.Errorf("save failed: %w", err)
    }
    
    return nil
}

// 錯誤恢復
func SafeExecute(fn func() error) (err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("panic recovered: %v\nstack: %s", r, debug.Stack())
        }
    }()
    
    return fn()
}
```

### 2.2 錯誤分類與處理

```go
// 錯誤分類
type ErrorCategory int

const (
    ErrorCategoryValidation ErrorCategory = iota
    ErrorCategoryNetwork
    ErrorCategoryDatabase
    ErrorCategoryBusiness
    ErrorCategoryInternal
)

type CategorizedError struct {
    Category ErrorCategory
    Code     string
    Message  string
    Cause    error
    Retry    bool
}

func (e *CategorizedError) ShouldRetry() bool {
    return e.Retry
}

// 統一錯誤處理器
type ErrorHandler struct {
    handlers map[ErrorCategory]func(error) error
}

func NewErrorHandler() *ErrorHandler {
    eh := &ErrorHandler{
        handlers: make(map[ErrorCategory]func(error) error),
    }
    
    // 註冊預設處理器
    eh.Register(ErrorCategoryNetwork, handleNetworkError)
    eh.Register(ErrorCategoryDatabase, handleDatabaseError)
    
    return eh
}

func (eh *ErrorHandler) Handle(err error) error {
    var catErr *CategorizedError
    if !errors.As(err, &catErr) {
        return err
    }
    
    if handler, exists := eh.handlers[catErr.Category]; exists {
        return handler(err)
    }
    
    return err
}
```

## 3. 測試策略

### 3.1 單元測試模式

```go
// Table-driven 測試
func TestCalculateCost(t *testing.T) {
    tests := []struct {
        name     string
        input    TokenCount
        model    string
        expected float64
        wantErr  bool
    }{
        {
            name: "valid calculation",
            input: TokenCount{
                InputTokens:  1000,
                OutputTokens: 500,
            },
            model:    "claude-3-opus",
            expected: 0.0525,
            wantErr:  false,
        },
        {
            name:     "unknown model",
            input:    TokenCount{},
            model:    "unknown",
            expected: 0,
            wantErr:  true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := CalculateCost(tt.model, tt.input)
            
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            
            assert.NoError(t, err)
            assert.InDelta(t, tt.expected, result, 0.0001)
        })
    }
}

// 子測試
func TestDataLoader(t *testing.T) {
    loader := NewLoader()
    
    t.Run("LoadFile", func(t *testing.T) {
        t.Run("ValidFile", func(t *testing.T) {
            data, err := loader.LoadFile("testdata/valid.jsonl")
            assert.NoError(t, err)
            assert.NotEmpty(t, data)
        })
        
        t.Run("InvalidFile", func(t *testing.T) {
            _, err := loader.LoadFile("testdata/invalid.jsonl")
            assert.Error(t, err)
        })
    })
    
    t.Run("ParseEntry", func(t *testing.T) {
        // 測試解析邏輯
    })
}
```

### 3.2 Mock 與 Stub

```go
// 介面定義
type DataStore interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Set(ctx context.Context, key string, value []byte) error
}

// Mock 實作
type MockDataStore struct {
    mock.Mock
}

func (m *MockDataStore) Get(ctx context.Context, key string) ([]byte, error) {
    args := m.Called(ctx, key)
    return args.Get(0).([]byte), args.Error(1)
}

func (m *MockDataStore) Set(ctx context.Context, key string, value []byte) error {
    args := m.Called(ctx, key, value)
    return args.Error(0)
}

// 測試使用 Mock
func TestService(t *testing.T) {
    mockStore := new(MockDataStore)
    service := NewService(mockStore)
    
    // 設定期望
    mockStore.On("Get", mock.Anything, "test-key").Return([]byte("test-value"), nil)
    
    // 執行測試
    result, err := service.Process(context.Background(), "test-key")
    
    // 驗證
    assert.NoError(t, err)
    assert.Equal(t, "processed: test-value", result)
    mockStore.AssertExpectations(t)
}
```

### 3.3 整合測試

```go
// 測試環境設置
type TestEnvironment struct {
    TempDir    string
    TestData   string
    HTTPServer *httptest.Server
    Cancel     context.CancelFunc
}

func SetupTestEnvironment(t *testing.T) *TestEnvironment {
    t.Helper()
    
    env := &TestEnvironment{
        TempDir:  t.TempDir(),
        TestData: filepath.Join("testdata", "fixtures"),
    }
    
    // 啟動 mock HTTP server
    env.HTTPServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Mock API 響應
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"status": "ok"}`))
    }))
    
    // 清理函數
    t.Cleanup(func() {
        env.HTTPServer.Close()
        if env.Cancel != nil {
            env.Cancel()
        }
    })
    
    return env
}

// 整合測試範例
func TestIntegration_EndToEnd(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }
    
    env := SetupTestEnvironment(t)
    
    // 準備測試資料
    testFile := filepath.Join(env.TempDir, "usage.jsonl")
    createTestData(t, testFile)
    
    // 執行完整流程
    loader := NewLoader()
    data, err := loader.Load(context.Background(), env.TempDir)
    require.NoError(t, err)
    
    calculator := NewCalculator()
    report, err := calculator.GenerateReport(data)
    require.NoError(t, err)
    
    // 驗證結果
    assert.NotEmpty(t, report)
    assert.Greater(t, report.TotalCost, 0.0)
}
```

## 4. 效能基準測試

### 4.1 基準測試設計

```go
// 基本基準測試
func BenchmarkJSONParsing(b *testing.B) {
    data := generateTestData(1000)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        var entries []UsageEntry
        _ = json.Unmarshal(data, &entries)
    }
}

// 並行基準測試
func BenchmarkParallelProcessing(b *testing.B) {
    data := generateLargeDataset()
    
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            processData(data)
        }
    })
}

// 子基準測試
func BenchmarkDataLoader(b *testing.B) {
    b.Run("SmallFile", func(b *testing.B) {
        benchmarkLoadFile(b, "testdata/small.jsonl")
    })
    
    b.Run("MediumFile", func(b *testing.B) {
        benchmarkLoadFile(b, "testdata/medium.jsonl")
    })
    
    b.Run("LargeFile", func(b *testing.B) {
        benchmarkLoadFile(b, "testdata/large.jsonl")
    })
}

func benchmarkLoadFile(b *testing.B, filename string) {
    loader := NewLoader()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = loader.LoadFile(filename)
    }
}
```

### 4.2 記憶體分析

```go
// 記憶體基準測試
func BenchmarkMemoryUsage(b *testing.B) {
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        data := make([]UsageEntry, 10000)
        for j := range data {
            data[j] = UsageEntry{
                InputTokens:  100,
                OutputTokens: 50,
            }
        }
        _ = processEntries(data)
    }
}

// pprof 整合
func TestWithProfiling(t *testing.T) {
    if os.Getenv("ENABLE_PROFILING") != "1" {
        t.Skip("profiling not enabled")
    }
    
    // CPU profiling
    cpuFile, _ := os.Create("cpu.prof")
    defer cpuFile.Close()
    
    pprof.StartCPUProfile(cpuFile)
    defer pprof.StopCPUProfile()
    
    // 執行測試邏輯
    runIntensiveOperation()
    
    // Memory profiling
    memFile, _ := os.Create("mem.prof")
    defer memFile.Close()
    
    runtime.GC()
    pprof.WriteHeapProfile(memFile)
}
```

## 5. 部署與打包

### 5.1 建置腳本 (Makefile)

```makefile
.PHONY: all build test clean

# 變數定義
BINARY_NAME=ccusage
VERSION=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date +%FT%T%z)
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -s -w"

# 目標
all: test build

build:
	@echo "Building ${BINARY_NAME}..."
	go build ${LDFLAGS} -o bin/${BINARY_NAME} cmd/ccusage/main.go

build-all:
	@echo "Building for multiple platforms..."
	GOOS=darwin GOARCH=amd64 go build ${LDFLAGS} -o bin/${BINARY_NAME}-darwin-amd64 cmd/ccusage/main.go
	GOOS=darwin GOARCH=arm64 go build ${LDFLAGS} -o bin/${BINARY_NAME}-darwin-arm64 cmd/ccusage/main.go
	GOOS=linux GOARCH=amd64 go build ${LDFLAGS} -o bin/${BINARY_NAME}-linux-amd64 cmd/ccusage/main.go
	GOOS=windows GOARCH=amd64 go build ${LDFLAGS} -o bin/${BINARY_NAME}-windows-amd64.exe cmd/ccusage/main.go

test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

benchmark:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem -run=^# ./...

lint:
	@echo "Running linters..."
	golangci-lint run

clean:
	@echo "Cleaning..."
	rm -rf bin/ coverage.* *.prof

install: build
	@echo "Installing ${BINARY_NAME}..."
	cp bin/${BINARY_NAME} ${GOPATH}/bin/

docker-build:
	@echo "Building Docker image..."
	docker build -t ccusage:${VERSION} .

release: test build-all
	@echo "Creating release..."
	tar -czf releases/${BINARY_NAME}-${VERSION}-darwin-amd64.tar.gz -C bin ${BINARY_NAME}-darwin-amd64
	tar -czf releases/${BINARY_NAME}-${VERSION}-linux-amd64.tar.gz -C bin ${BINARY_NAME}-linux-amd64
	zip releases/${BINARY_NAME}-${VERSION}-windows-amd64.zip bin/${BINARY_NAME}-windows-amd64.exe
```

### 5.2 Docker 配置

```dockerfile
# Multi-stage build
FROM golang:1.21-alpine AS builder

# 安裝建置依賴
RUN apk add --no-cache git make

# 設定工作目錄
WORKDIR /app

# 複製 go mod 檔案
COPY go.mod go.sum ./
RUN go mod download

# 複製源碼
COPY . .

# 建置應用
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o ccusage cmd/ccusage/main.go

# 最終鏡像
FROM alpine:latest

# 安裝運行時依賴
RUN apk --no-cache add ca-certificates tzdata

# 創建非 root 用戶
RUN adduser -D -g '' appuser

# 複製二進制
COPY --from=builder /app/ccusage /usr/local/bin/

# 設定用戶
USER appuser

# 入口點
ENTRYPOINT ["ccusage"]
```

### 5.3 CI/CD 配置 (GitHub Actions)

```yaml
name: CI/CD Pipeline

on:
  push:
    branches: [ main, develop ]
    tags: [ 'v*' ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go-version: [1.20, 1.21]
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: ${{ matrix.go-version }}
    
    - name: Cache Go modules
      uses: actions/cache@v3
      with:
        path: ~/go/pkg/mod
        key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
        restore-keys: |
          ${{ runner.os }}-go-
    
    - name: Install dependencies
      run: go mod download
    
    - name: Run tests
      run: make test
    
    - name: Run linters
      uses: golangci/golangci-lint-action@v3
      with:
        version: latest
    
    - name: Upload coverage
      uses: codecov/codecov-action@v3
      with:
        file: ./coverage.out

  build:
    needs: test
    runs-on: ubuntu-latest
    if: startsWith(github.ref, 'refs/tags/')
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: 1.21
    
    - name: Build binaries
      run: make build-all
    
    - name: Create Release
      uses: actions/create-release@v1
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      with:
        tag_name: ${{ github.ref }}
        release_name: Release ${{ github.ref }}
        draft: false
        prerelease: false
    
    - name: Upload Release Assets
      uses: actions/upload-release-asset@v1
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      with:
        upload_url: ${{ steps.create_release.outputs.upload_url }}
        asset_path: ./bin/
        asset_name: ccusage-binaries.zip
        asset_content_type: application/zip
```

## 6. 安全最佳實踐

### 6.1 輸入驗證

```go
// 輸入消毒
func SanitizeInput(input string) string {
    // 移除控制字符
    input = strings.Map(func(r rune) rune {
        if unicode.IsControl(r) {
            return -1
        }
        return r
    }, input)
    
    // 限制長度
    const maxLength = 1000
    if len(input) > maxLength {
        input = input[:maxLength]
    }
    
    return strings.TrimSpace(input)
}

// 路徑遍歷防護
func SafeJoinPath(base, userPath string) (string, error) {
    // 清理路徑
    cleaned := filepath.Clean(userPath)
    
    // 檢查相對路徑
    if filepath.IsAbs(cleaned) {
        return "", errors.New("absolute paths not allowed")
    }
    
    // 檢查路徑遍歷
    if strings.Contains(cleaned, "..") {
        return "", errors.New("path traversal detected")
    }
    
    // 安全連接
    joined := filepath.Join(base, cleaned)
    
    // 確保結果在基礎路徑內
    if !strings.HasPrefix(joined, base) {
        return "", errors.New("path outside base directory")
    }
    
    return joined, nil
}
```

### 6.2 密碼與敏感資料

```go
// 安全儲存敏感資料
type SecureString struct {
    data []byte
}

func NewSecureString(value string) *SecureString {
    return &SecureString{
        data: []byte(value),
    }
}

func (s *SecureString) String() string {
    return string(s.data)
}

func (s *SecureString) Clear() {
    // 覆寫記憶體
    for i := range s.data {
        s.data[i] = 0
    }
}

// 環境變數管理
func GetSecureEnv(key string, defaultValue ...string) string {
    value := os.Getenv(key)
    if value == "" && len(defaultValue) > 0 {
        return defaultValue[0]
    }
    
    // 記錄存取但不記錄值
    log.Debug("Accessed secure environment variable", "key", key)
    
    return value
}
```

## 7. 監控與日誌

### 7.1 結構化日誌

```go
// 日誌包裝器
type Logger struct {
    *zap.SugaredLogger
    requestID string
}

func NewLogger(requestID string) *Logger {
    config := zap.NewProductionConfig()
    config.Level = zap.NewAtomicLevelAt(getLogLevel())
    
    logger, _ := config.Build()
    
    return &Logger{
        SugaredLogger: logger.Sugar(),
        requestID:     requestID,
    }
}

func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
    args := make([]interface{}, 0, len(fields)*2)
    for k, v := range fields {
        args = append(args, k, v)
    }
    
    return &Logger{
        SugaredLogger: l.With(args...),
        requestID:     l.requestID,
    }
}

// 使用範例
func ProcessRequest(ctx context.Context, req Request) error {
    logger := NewLogger(req.ID).WithFields(map[string]interface{}{
        "user_id": req.UserID,
        "action":  req.Action,
    })
    
    logger.Info("Processing request")
    
    if err := validate(req); err != nil {
        logger.Error("Validation failed", "error", err)
        return err
    }
    
    logger.Info("Request processed successfully")
    return nil
}
```

### 7.2 指標收集

```go
// Prometheus 指標
var (
    requestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ccusage_requests_total",
            Help: "Total number of requests",
        },
        []string{"method", "status"},
    )
    
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "ccusage_request_duration_seconds",
            Help:    "Request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method"},
    )
)

func init() {
    prometheus.MustRegister(requestsTotal)
    prometheus.MustRegister(requestDuration)
}

// 中間件
func MetricsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        
        wrapped := &responseWriter{ResponseWriter: w}
        next.ServeHTTP(wrapped, r)
        
        duration := time.Since(start).Seconds()
        
        requestsTotal.WithLabelValues(r.Method, strconv.Itoa(wrapped.status)).Inc()
        requestDuration.WithLabelValues(r.Method).Observe(duration)
    })
}
```

## 8. 程式碼品質工具

### 8.1 Linter 配置 (.golangci.yml)

```yaml
linters:
  enable:
    - gofmt
    - golint
    - govet
    - errcheck
    - staticcheck
    - gosimple
    - ineffassign
    - deadcode
    - typecheck
    - gosec
    - dupl
    - gocyclo
    - gocognit

linters-settings:
  gocyclo:
    min-complexity: 15
  dupl:
    threshold: 100
  gosec:
    excludes:
      - G104  # Unhandled errors
  gofmt:
    simplify: true

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - dupl
        - gosec
```

### 8.2 Pre-commit Hooks

```yaml
# .pre-commit-config.yaml
repos:
  - repo: local
    hooks:
      - id: go-fmt
        name: go fmt
        entry: go fmt ./...
        language: system
        types: [go]
        
      - id: go-test
        name: go test
        entry: go test ./...
        language: system
        types: [go]
        
      - id: go-lint
        name: golangci-lint
        entry: golangci-lint run
        language: system
        types: [go]
```

## 9. 除錯技巧

### 9.1 Delve 除錯器使用

```bash
# 安裝 delve
go install github.com/go-delve/delve/cmd/dlv@latest

# 除錯執行
dlv debug cmd/ccusage/main.go -- daily --debug

# 設置斷點
(dlv) break main.main
(dlv) break loader.Load

# 執行
(dlv) continue

# 檢查變數
(dlv) print entries
(dlv) locals

# 步進
(dlv) next
(dlv) step
```

### 9.2 追蹤與分析

```go
// 追蹤執行時間
func TraceTime(name string) func() {
    start := time.Now()
    log.Debug("Starting", "operation", name)
    
    return func() {
        duration := time.Since(start)
        log.Debug("Completed", "operation", name, "duration", duration)
    }
}

// 使用範例
func LoadData() error {
    defer TraceTime("LoadData")()
    
    // 實際邏輯
    return nil
}

// 條件除錯
func DebugPrint(condition bool, format string, args ...interface{}) {
    if condition || os.Getenv("DEBUG") == "1" {
        fmt.Printf("[DEBUG] "+format+"\n", args...)
    }
}
```

## 10. 總結

本文檔涵蓋了 ccusage Go 版本開發的主要技術細節，包括：

1. **並發編程**：正確使用 goroutines 和同步機制
2. **錯誤處理**：統一的錯誤處理模式
3. **測試策略**：完整的測試覆蓋
4. **效能優化**：基準測試和分析
5. **部署流程**：自動化建置和發布
6. **安全實踐**：輸入驗證和敏感資料處理
7. **監控日誌**：結構化日誌和指標收集
8. **程式碼品質**：工具和最佳實踐

這些技術細節確保了專案的高品質、高效能和可維護性。