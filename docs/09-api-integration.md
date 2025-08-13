# API 整合模組設計

## 1. 模組概述

API 整合模組負責與外部服務的通訊，包括 LiteLLM 價格 API、MCP (Model Context Protocol) Server 實作，以及其他可能的第三方服務整合。模組提供統一的 HTTP 客戶端、重試機制和錯誤處理。

### 1.1 主要功能
- LiteLLM 價格資料獲取
- MCP Server 實作
- HTTP 客戶端管理
- 重試與容錯機制
- 離線模式支援
- API 認證處理

### 1.2 對應 TypeScript 模組
- `pricing-fetcher.ts` → `api/pricing.go`
- `mcp.ts` → `api/mcp/server.go`
- HTTP 相關功能 → `api/client.go`

## 2. API 整合架構

```
┌──────────────────────────────────────────┐
│            API Gateway                    │
│         (統一入口、認證、限流)              │
└────────────────┬─────────────────────────┘
                 │
    ┌────────────┼────────────┐
    │            │            │
┌───▼───┐   ┌───▼───┐   ┌───▼───┐
│Pricing│   │  MCP  │   │Future │
│  API  │   │Server │   │  APIs │
└───┬───┘   └───┬───┘   └───┬───┘
    │            │            │
    └────────────┼────────────┘
                 │
         ┌───────▼───────┐
         │  HTTP Client  │
         │  (重試、快取)  │
         └───────────────┘
```

## 3. HTTP 客戶端

### 3.1 基礎客戶端

```go
package api

import (
    "context"
    "net/http"
    "time"
)

type HTTPClient struct {
    client      *http.Client
    baseURL     string
    headers     map[string]string
    timeout     time.Duration
    retryPolicy *RetryPolicy
    rateLimiter *RateLimiter
}

type RetryPolicy struct {
    MaxRetries     int
    InitialDelay   time.Duration
    MaxDelay       time.Duration
    Multiplier     float64
    RetryableErrors []int
}

func NewHTTPClient(options ClientOptions) *HTTPClient {
    return &HTTPClient{
        client: &http.Client{
            Timeout: options.Timeout,
            Transport: &http.Transport{
                MaxIdleConns:        100,
                MaxIdleConnsPerHost: 10,
                IdleConnTimeout:     90 * time.Second,
            },
        },
        baseURL:     options.BaseURL,
        headers:     options.Headers,
        timeout:     options.Timeout,
        retryPolicy: defaultRetryPolicy(),
        rateLimiter: NewRateLimiter(options.RateLimit),
    }
}

func defaultRetryPolicy() *RetryPolicy {
    return &RetryPolicy{
        MaxRetries:   3,
        InitialDelay: 1 * time.Second,
        MaxDelay:     30 * time.Second,
        Multiplier:   2.0,
        RetryableErrors: []int{
            http.StatusTooManyRequests,
            http.StatusInternalServerError,
            http.StatusBadGateway,
            http.StatusServiceUnavailable,
            http.StatusGatewayTimeout,
        },
    }
}

func (c *HTTPClient) Get(ctx context.Context, path string) (*Response, error) {
    return c.request(ctx, "GET", path, nil)
}

func (c *HTTPClient) Post(ctx context.Context, path string, body interface{}) (*Response, error) {
    return c.request(ctx, "POST", path, body)
}

func (c *HTTPClient) request(ctx context.Context, method, path string, body interface{}) (*Response, error) {
    // 速率限制
    if err := c.rateLimiter.Wait(ctx); err != nil {
        return nil, err
    }
    
    var lastErr error
    delay := c.retryPolicy.InitialDelay
    
    for attempt := 0; attempt <= c.retryPolicy.MaxRetries; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(delay):
                delay = c.calculateNextDelay(delay)
            case <-ctx.Done():
                return nil, ctx.Err()
            }
        }
        
        resp, err := c.doRequest(ctx, method, path, body)
        if err != nil {
            lastErr = err
            if !c.isRetryableError(err) {
                return nil, err
            }
            continue
        }
        
        if !c.shouldRetry(resp.StatusCode) {
            return resp, nil
        }
        
        lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
    }
    
    return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (c *HTTPClient) doRequest(ctx context.Context, method, path string, body interface{}) (*Response, error) {
    url := c.baseURL + path
    
    var bodyReader io.Reader
    if body != nil {
        jsonBody, err := json.Marshal(body)
        if err != nil {
            return nil, err
        }
        bodyReader = bytes.NewReader(jsonBody)
    }
    
    req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
    if err != nil {
        return nil, err
    }
    
    // 設置 headers
    for key, value := range c.headers {
        req.Header.Set(key, value)
    }
    
    if body != nil {
        req.Header.Set("Content-Type", "application/json")
    }
    
    resp, err := c.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    data, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, err
    }
    
    return &Response{
        StatusCode: resp.StatusCode,
        Headers:    resp.Header,
        Body:       data,
    }, nil
}
```

### 3.2 速率限制

```go
type RateLimiter struct {
    rate     int           // 每秒請求數
    burst    int           // 突發請求數
    limiter  *rate.Limiter
}

func NewRateLimiter(ratePerSecond int) *RateLimiter {
    if ratePerSecond <= 0 {
        ratePerSecond = 10 // 預設每秒10個請求
    }
    
    return &RateLimiter{
        rate:    ratePerSecond,
        burst:   ratePerSecond * 2,
        limiter: rate.NewLimiter(rate.Limit(ratePerSecond), ratePerSecond*2),
    }
}

func (rl *RateLimiter) Wait(ctx context.Context) error {
    return rl.limiter.Wait(ctx)
}

func (rl *RateLimiter) Allow() bool {
    return rl.limiter.Allow()
}
```

## 4. LiteLLM 價格 API

### 4.1 價格 API 客戶端

```go
package pricing

import (
    "context"
    "encoding/json"
    "fmt"
)

const (
    LiteLLMPricingURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
)

type PricingClient struct {
    httpClient *api.HTTPClient
    cache      *PricingCache
    offline    bool
}

type LiteLLMResponse map[string]ModelInfo

type ModelInfo struct {
    MaxTokens              int     `json:"max_tokens"`
    MaxInputTokens        int     `json:"max_input_tokens"`
    MaxOutputTokens       int     `json:"max_output_tokens"`
    InputCostPerToken     float64 `json:"input_cost_per_token"`
    OutputCostPerToken    float64 `json:"output_cost_per_token"`
    LiteLLMProvider       string  `json:"litellm_provider"`
    Mode                  string  `json:"mode"`
    SupportsFunctionCalling bool  `json:"supports_function_calling"`
}

func NewPricingClient(offline bool) *PricingClient {
    return &PricingClient{
        httpClient: api.NewHTTPClient(api.ClientOptions{
            BaseURL: "",
            Timeout: 10 * time.Second,
        }),
        cache:   NewPricingCache(),
        offline: offline,
    }
}

func (pc *PricingClient) FetchPricing(ctx context.Context) (*PricingTable, error) {
    // 檢查快取
    if cached := pc.cache.Get(); cached != nil {
        return cached, nil
    }
    
    // 離線模式
    if pc.offline {
        return pc.loadOfflinePricing()
    }
    
    // 從 API 獲取
    resp, err := pc.httpClient.Get(ctx, LiteLLMPricingURL)
    if err != nil {
        // 降級到離線模式
        log.Warn("Failed to fetch pricing from API, falling back to offline", "error", err)
        return pc.loadOfflinePricing()
    }
    
    // 解析響應
    var litellmData LiteLLMResponse
    if err := json.Unmarshal(resp.Body, &litellmData); err != nil {
        return nil, fmt.Errorf("failed to parse pricing data: %w", err)
    }
    
    // 轉換為內部格式
    table := pc.convertToPricingTable(litellmData)
    
    // 更新快取
    pc.cache.Set(table)
    
    return table, nil
}

func (pc *PricingClient) convertToPricingTable(data LiteLLMResponse) *PricingTable {
    table := NewPricingTable()
    
    // Claude 模型映射
    modelMappings := map[string]string{
        "claude-3-opus":        "claude-3-opus-20240229",
        "claude-3-5-sonnet":    "claude-3-5-sonnet-20241022",
        "claude-3-haiku":       "claude-3-haiku-20240307",
        "claude-3-sonnet":      "claude-3-sonnet-20240229",
    }
    
    for litellmName, info := range data {
        // 只處理 Claude 模型
        if info.LiteLLMProvider != "anthropic" {
            continue
        }
        
        // 轉換價格（從每 token 到每百萬 token）
        pricing := ModelPricing{
            Model:              litellmName,
            InputPrice:         info.InputCostPerToken * 1_000_000,
            OutputPrice:        info.OutputCostPerToken * 1_000_000,
            MaxTokens:          info.MaxTokens,
            MaxInputTokens:     info.MaxInputTokens,
            MaxOutputTokens:    info.MaxOutputTokens,
        }
        
        // 計算快取價格（Claude 特定）
        pricing.CacheCreationPrice = pricing.InputPrice * 1.25
        pricing.CacheReadPrice = pricing.InputPrice * 0.1
        
        table.UpdatePricing(pricing)
        
        // 添加別名
        for shortName, fullName := range modelMappings {
            if fullName == litellmName {
                aliasPricing := pricing
                aliasPricing.Model = shortName
                table.UpdatePricing(aliasPricing)
            }
        }
    }
    
    return table
}
```

### 4.2 離線價格資料

```go
func (pc *PricingClient) loadOfflinePricing() (*PricingTable, error) {
    // 內嵌的價格資料（編譯時包含）
    offlineData := `{
        "claude-3-opus-20240229": {
            "max_tokens": 200000,
            "input_cost_per_token": 0.000015,
            "output_cost_per_token": 0.000075,
            "litellm_provider": "anthropic"
        },
        "claude-3-5-sonnet-20241022": {
            "max_tokens": 200000,
            "input_cost_per_token": 0.000003,
            "output_cost_per_token": 0.000015,
            "litellm_provider": "anthropic"
        },
        "claude-3-haiku-20240307": {
            "max_tokens": 200000,
            "input_cost_per_token": 0.00000025,
            "output_cost_per_token": 0.00000125,
            "litellm_provider": "anthropic"
        }
    }`
    
    var data LiteLLMResponse
    if err := json.Unmarshal([]byte(offlineData), &data); err != nil {
        return nil, err
    }
    
    return pc.convertToPricingTable(data), nil
}
```

### 4.3 價格快取

```go
type PricingCache struct {
    data      *PricingTable
    timestamp time.Time
    ttl       time.Duration
    mutex     sync.RWMutex
}

func NewPricingCache() *PricingCache {
    return &PricingCache{
        ttl: 1 * time.Hour,
    }
}

func (pc *PricingCache) Get() *PricingTable {
    pc.mutex.RLock()
    defer pc.mutex.RUnlock()
    
    if pc.data == nil {
        return nil
    }
    
    if time.Since(pc.timestamp) > pc.ttl {
        return nil
    }
    
    return pc.data
}

func (pc *PricingCache) Set(table *PricingTable) {
    pc.mutex.Lock()
    defer pc.mutex.Unlock()
    
    pc.data = table
    pc.timestamp = time.Now()
}

func (pc *PricingCache) Invalidate() {
    pc.mutex.Lock()
    defer pc.mutex.Unlock()
    
    pc.data = nil
}
```

## 5. MCP Server 實作

### 5.1 MCP Server 結構

```go
package mcp

import (
    "context"
    "encoding/json"
    "net/http"
)

type MCPServer struct {
    server      *http.Server
    dataLoader  *loader.Loader
    calculator  *calculator.CostCalculator
    port        int
}

type MCPRequest struct {
    Method string          `json:"method"`
    Params json.RawMessage `json:"params"`
    ID     interface{}     `json:"id"`
}

type MCPResponse struct {
    Result interface{} `json:"result,omitempty"`
    Error  *MCPError   `json:"error,omitempty"`
    ID     interface{} `json:"id"`
}

type MCPError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

func NewMCPServer(port int) *MCPServer {
    return &MCPServer{
        port:       port,
        dataLoader: loader.New(""),
        calculator: calculator.New(),
    }
}

func (s *MCPServer) Start(ctx context.Context) error {
    mux := http.NewServeMux()
    mux.HandleFunc("/", s.handleRequest)
    
    s.server = &http.Server{
        Addr:    fmt.Sprintf(":%d", s.port),
        Handler: mux,
    }
    
    log.Info("Starting MCP server", "port", s.port)
    
    go func() {
        <-ctx.Done()
        s.server.Shutdown(context.Background())
    }()
    
    return s.server.ListenAndServe()
}

func (s *MCPServer) handleRequest(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    
    var req MCPRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        s.sendError(w, req.ID, -32700, "Parse error")
        return
    }
    
    // 處理不同的方法
    var result interface{}
    var err error
    
    switch req.Method {
    case "usage.daily":
        result, err = s.handleDailyUsage(req.Params)
    case "usage.monthly":
        result, err = s.handleMonthlyUsage(req.Params)
    case "usage.session":
        result, err = s.handleSessionUsage(req.Params)
    case "cost.calculate":
        result, err = s.handleCostCalculation(req.Params)
    default:
        s.sendError(w, req.ID, -32601, "Method not found")
        return
    }
    
    if err != nil {
        s.sendError(w, req.ID, -32603, err.Error())
        return
    }
    
    s.sendResponse(w, req.ID, result)
}
```

### 5.2 MCP 方法處理

```go
func (s *MCPServer) handleDailyUsage(params json.RawMessage) (interface{}, error) {
    var opts struct {
        Since   string `json:"since"`
        Until   string `json:"until"`
        Project string `json:"project"`
    }
    
    if err := json.Unmarshal(params, &opts); err != nil {
        return nil, err
    }
    
    // 載入資料
    entries, err := s.dataLoader.Load(context.Background())
    if err != nil {
        return nil, err
    }
    
    // 生成日報
    generator := reports.NewDailyGenerator()
    report, err := generator.Generate(entries, reports.GenerateOptions{
        Since:   parseDate(opts.Since),
        Until:   parseDate(opts.Until),
        Project: opts.Project,
    })
    
    if err != nil {
        return nil, err
    }
    
    return report, nil
}

func (s *MCPServer) handleCostCalculation(params json.RawMessage) (interface{}, error) {
    var opts struct {
        Model        string `json:"model"`
        InputTokens  int    `json:"input_tokens"`
        OutputTokens int    `json:"output_tokens"`
    }
    
    if err := json.Unmarshal(params, &opts); err != nil {
        return nil, err
    }
    
    cost, err := s.calculator.Calculate(
        opts.Model,
        TokenCount{
            InputTokens:  opts.InputTokens,
            OutputTokens: opts.OutputTokens,
        },
    )
    
    if err != nil {
        return nil, err
    }
    
    return map[string]interface{}{
        "cost":         cost,
        "model":        opts.Model,
        "input_tokens": opts.InputTokens,
        "output_tokens": opts.OutputTokens,
    }, nil
}

func (s *MCPServer) sendResponse(w http.ResponseWriter, id interface{}, result interface{}) {
    resp := MCPResponse{
        Result: result,
        ID:     id,
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}

func (s *MCPServer) sendError(w http.ResponseWriter, id interface{}, code int, message string) {
    resp := MCPResponse{
        Error: &MCPError{
            Code:    code,
            Message: message,
        },
        ID: id,
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}
```

### 5.3 WebSocket 支援

```go
type WebSocketServer struct {
    upgrader websocket.Upgrader
    clients  map[*websocket.Conn]bool
    mutex    sync.RWMutex
}

func NewWebSocketServer() *WebSocketServer {
    return &WebSocketServer{
        upgrader: websocket.Upgrader{
            CheckOrigin: func(r *http.Request) bool {
                return true // 允許所有來源（生產環境應該更嚴格）
            },
        },
        clients: make(map[*websocket.Conn]bool),
    }
}

func (ws *WebSocketServer) HandleConnection(w http.ResponseWriter, r *http.Request) {
    conn, err := ws.upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Error("Failed to upgrade connection", "error", err)
        return
    }
    defer conn.Close()
    
    ws.addClient(conn)
    defer ws.removeClient(conn)
    
    for {
        var req MCPRequest
        if err := conn.ReadJSON(&req); err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                log.Error("WebSocket error", "error", err)
            }
            break
        }
        
        // 處理請求
        result, err := ws.processRequest(req)
        
        resp := MCPResponse{
            ID: req.ID,
        }
        
        if err != nil {
            resp.Error = &MCPError{
                Code:    -32603,
                Message: err.Error(),
            }
        } else {
            resp.Result = result
        }
        
        if err := conn.WriteJSON(resp); err != nil {
            log.Error("Failed to write response", "error", err)
            break
        }
    }
}

func (ws *WebSocketServer) Broadcast(message interface{}) {
    ws.mutex.RLock()
    defer ws.mutex.RUnlock()
    
    for client := range ws.clients {
        if err := client.WriteJSON(message); err != nil {
            log.Error("Failed to broadcast to client", "error", err)
        }
    }
}
```

## 6. 第三方 API 整合

### 6.1 通用 API 客戶端介面

```go
type APIClient interface {
    Get(ctx context.Context, endpoint string, params map[string]string) ([]byte, error)
    Post(ctx context.Context, endpoint string, body interface{}) ([]byte, error)
    Put(ctx context.Context, endpoint string, body interface{}) ([]byte, error)
    Delete(ctx context.Context, endpoint string) error
}

type BaseAPIClient struct {
    httpClient *HTTPClient
    apiKey     string
    baseURL    string
}

func (c *BaseAPIClient) addAuth(req *http.Request) {
    if c.apiKey != "" {
        req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
    }
}
```

### 6.2 Webhook 支援

```go
type WebhookClient struct {
    url     string
    secret  string
    timeout time.Duration
}

type WebhookPayload struct {
    Event     string      `json:"event"`
    Timestamp time.Time   `json:"timestamp"`
    Data      interface{} `json:"data"`
}

func NewWebhookClient(url, secret string) *WebhookClient {
    return &WebhookClient{
        url:     url,
        secret:  secret,
        timeout: 10 * time.Second,
    }
}

func (wc *WebhookClient) Send(ctx context.Context, event string, data interface{}) error {
    payload := WebhookPayload{
        Event:     event,
        Timestamp: time.Now(),
        Data:      data,
    }
    
    jsonPayload, err := json.Marshal(payload)
    if err != nil {
        return err
    }
    
    req, err := http.NewRequestWithContext(ctx, "POST", wc.url, bytes.NewReader(jsonPayload))
    if err != nil {
        return err
    }
    
    // 添加簽名
    signature := wc.generateSignature(jsonPayload)
    req.Header.Set("X-Webhook-Signature", signature)
    req.Header.Set("Content-Type", "application/json")
    
    client := &http.Client{Timeout: wc.timeout}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("webhook failed with status %d", resp.StatusCode)
    }
    
    return nil
}

func (wc *WebhookClient) generateSignature(payload []byte) string {
    h := hmac.New(sha256.New, []byte(wc.secret))
    h.Write(payload)
    return hex.EncodeToString(h.Sum(nil))
}
```

## 7. GraphQL 支援

### 7.1 GraphQL 客戶端

```go
type GraphQLClient struct {
    httpClient *HTTPClient
    endpoint   string
}

type GraphQLRequest struct {
    Query     string                 `json:"query"`
    Variables map[string]interface{} `json:"variables,omitempty"`
}

type GraphQLResponse struct {
    Data   json.RawMessage `json:"data"`
    Errors []GraphQLError  `json:"errors,omitempty"`
}

type GraphQLError struct {
    Message string `json:"message"`
    Path    []string `json:"path,omitempty"`
}

func (gc *GraphQLClient) Query(ctx context.Context, query string, variables map[string]interface{}, result interface{}) error {
    req := GraphQLRequest{
        Query:     query,
        Variables: variables,
    }
    
    resp, err := gc.httpClient.Post(ctx, gc.endpoint, req)
    if err != nil {
        return err
    }
    
    var gqlResp GraphQLResponse
    if err := json.Unmarshal(resp.Body, &gqlResp); err != nil {
        return err
    }
    
    if len(gqlResp.Errors) > 0 {
        return fmt.Errorf("GraphQL errors: %v", gqlResp.Errors)
    }
    
    return json.Unmarshal(gqlResp.Data, result)
}
```

## 8. 認證機制

### 8.1 Token 管理

```go
type TokenManager struct {
    storage TokenStorage
    mutex   sync.RWMutex
}

type TokenStorage interface {
    Get(key string) (string, error)
    Set(key string, token string, expiry time.Time) error
    Delete(key string) error
}

type Token struct {
    Value  string    `json:"value"`
    Expiry time.Time `json:"expiry"`
}

func (tm *TokenManager) GetToken(service string) (string, error) {
    tm.mutex.RLock()
    defer tm.mutex.RUnlock()
    
    token, err := tm.storage.Get(service)
    if err != nil {
        return "", err
    }
    
    return token, nil
}

func (tm *TokenManager) RefreshToken(service string, refreshFunc func() (string, time.Time, error)) (string, error) {
    tm.mutex.Lock()
    defer tm.mutex.Unlock()
    
    // 獲取新 token
    token, expiry, err := refreshFunc()
    if err != nil {
        return "", err
    }
    
    // 儲存 token
    if err := tm.storage.Set(service, token, expiry); err != nil {
        return "", err
    }
    
    return token, nil
}
```

### 8.2 OAuth2 支援

```go
type OAuth2Client struct {
    config *oauth2.Config
    token  *oauth2.Token
    mutex  sync.RWMutex
}

func NewOAuth2Client(clientID, clientSecret, redirectURL string, scopes []string) *OAuth2Client {
    return &OAuth2Client{
        config: &oauth2.Config{
            ClientID:     clientID,
            ClientSecret: clientSecret,
            RedirectURL:  redirectURL,
            Scopes:       scopes,
            Endpoint: oauth2.Endpoint{
                AuthURL:  "https://provider.com/oauth/authorize",
                TokenURL: "https://provider.com/oauth/token",
            },
        },
    }
}

func (oc *OAuth2Client) GetAuthURL(state string) string {
    return oc.config.AuthCodeURL(state)
}

func (oc *OAuth2Client) ExchangeCode(ctx context.Context, code string) error {
    token, err := oc.config.Exchange(ctx, code)
    if err != nil {
        return err
    }
    
    oc.mutex.Lock()
    oc.token = token
    oc.mutex.Unlock()
    
    return nil
}

func (oc *OAuth2Client) GetClient(ctx context.Context) *http.Client {
    oc.mutex.RLock()
    defer oc.mutex.RUnlock()
    
    return oc.config.Client(ctx, oc.token)
}
```

## 9. 錯誤處理

### 9.1 API 錯誤類型

```go
type APIError struct {
    Code       string `json:"code"`
    Message    string `json:"message"`
    StatusCode int    `json:"status_code"`
    Details    interface{} `json:"details,omitempty"`
}

func (e *APIError) Error() string {
    return fmt.Sprintf("API error %s: %s (HTTP %d)", e.Code, e.Message, e.StatusCode)
}

func NewAPIError(statusCode int, code, message string) *APIError {
    return &APIError{
        Code:       code,
        Message:    message,
        StatusCode: statusCode,
    }
}

// 預定義錯誤
var (
    ErrUnauthorized = NewAPIError(401, "UNAUTHORIZED", "Authentication required")
    ErrForbidden    = NewAPIError(403, "FORBIDDEN", "Access denied")
    ErrNotFound     = NewAPIError(404, "NOT_FOUND", "Resource not found")
    ErrRateLimit    = NewAPIError(429, "RATE_LIMIT", "Too many requests")
    ErrServerError  = NewAPIError(500, "SERVER_ERROR", "Internal server error")
)
```

## 10. 測試策略

### 10.1 Mock HTTP 客戶端

```go
type MockHTTPClient struct {
    responses map[string]*Response
    errors    map[string]error
    calls     []string
}

func NewMockHTTPClient() *MockHTTPClient {
    return &MockHTTPClient{
        responses: make(map[string]*Response),
        errors:    make(map[string]error),
        calls:     []string{},
    }
}

func (m *MockHTTPClient) SetResponse(path string, response *Response) {
    m.responses[path] = response
}

func (m *MockHTTPClient) SetError(path string, err error) {
    m.errors[path] = err
}

func (m *MockHTTPClient) Get(ctx context.Context, path string) (*Response, error) {
    m.calls = append(m.calls, fmt.Sprintf("GET %s", path))
    
    if err, exists := m.errors[path]; exists {
        return nil, err
    }
    
    if resp, exists := m.responses[path]; exists {
        return resp, nil
    }
    
    return nil, fmt.Errorf("no mock response for %s", path)
}
```

### 10.2 整合測試

```go
func TestPricingAPI(t *testing.T) {
    // 啟動 mock server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        mockData := `{
            "claude-3-opus-20240229": {
                "input_cost_per_token": 0.000015,
                "output_cost_per_token": 0.000075
            }
        }`
        w.Write([]byte(mockData))
    }))
    defer server.Close()
    
    // 測試客戶端
    client := NewPricingClient(false)
    client.httpClient.baseURL = server.URL
    
    pricing, err := client.FetchPricing(context.Background())
    
    assert.NoError(t, err)
    assert.NotNil(t, pricing)
    
    price, err := pricing.GetPrice("claude-3-opus-20240229")
    assert.NoError(t, err)
    assert.Equal(t, 15.0, price.InputPrice)
}
```

## 11. 與 TypeScript 版本對照

| TypeScript 元素 | Go 元素 | 說明 |
|----------------|---------|------|
| fetch() | http.Client | HTTP 請求 |
| Result.try() | error handling | 錯誤處理 |
| Promise | goroutines/channels | 非同步處理 |
| retry logic | RetryPolicy | 重試機制 |
| MCP integration | MCP Server | MCP 協議實作 |