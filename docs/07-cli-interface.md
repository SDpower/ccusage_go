# CLI 介面設計

## 1. 模組概述

CLI 介面模組提供使用者與程式互動的命令列介面，包括命令解析、參數處理、錯誤處理和幫助系統。該模組使用 Cobra 框架構建，提供豐富的命令列功能。

### 1.1 主要功能
- 命令結構設計與解析
- 參數和標誌處理
- 共享參數配置
- 命令自動補全
- 錯誤訊息處理
- 使用說明生成

### 1.2 對應 TypeScript 模組
- `commands/index.ts` → `cmd/ccusage/main.go`
- `_shared-args.ts` → `commands/shared.go`
- 各命令檔案 → `commands/*.go`

## 2. CLI 架構設計

```
                    ┌──────────────┐
                    │  Main Entry  │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │  Root Command│
                    └──────┬───────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
  ┌─────▼─────┐    ┌──────▼──────┐    ┌──────▼──────┐
  │   Daily   │    │   Monthly   │    │   Session   │
  └───────────┘    └─────────────┘    └─────────────┘
        │                  │                  │
  ┌─────▼─────┐    ┌──────▼──────┐    ┌──────▼──────┐
  │   Blocks  │    │   Weekly    │    │ Statusline  │
  └───────────┘    └─────────────┘    └─────────────┘
```

## 3. 主程式進入點

### 3.1 主程式結構

```go
package main

import (
    "os"
    "github.com/spf13/cobra"
    "github.com/username/ccusage/internal/commands"
    "github.com/username/ccusage/internal/config"
    "github.com/username/ccusage/internal/logger"
)

func main() {
    // 初始化設定
    cfg := config.Load()
    
    // 初始化日誌
    log := logger.New(cfg.LogLevel)
    
    // 建立根命令
    rootCmd := createRootCommand()
    
    // 執行命令
    if err := rootCmd.Execute(); err != nil {
        log.Error("Command execution failed", "error", err)
        os.Exit(1)
    }
}

func createRootCommand() *cobra.Command {
    rootCmd := &cobra.Command{
        Use:     "ccusage",
        Short:   "Analyze Claude Code token usage and costs",
        Long:    `ccusage is a CLI tool for analyzing Claude Code usage data from local JSONL files.`,
        Version: version,
        RunE:    runDefaultCommand,
    }
    
    // 添加全域標誌
    rootCmd.PersistentFlags().BoolP("json", "j", false, "Output in JSON format")
    rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug mode")
    rootCmd.PersistentFlags().String("path", "", "Custom Claude data directory")
    
    // 添加子命令
    rootCmd.AddCommand(
        commands.NewDailyCommand(),
        commands.NewMonthlyCommand(),
        commands.NewWeeklyCommand(),
        commands.NewSessionCommand(),
        commands.NewBlocksCommand(),
        commands.NewStatuslineCommand(),
        commands.NewMCPCommand(),
    )
    
    return rootCmd
}

func runDefaultCommand(cmd *cobra.Command, args []string) error {
    // 無子命令時執行 daily 作為預設
    return commands.NewDailyCommand().Execute()
}
```

### 3.2 版本資訊管理

```go
var (
    version   = "dev"
    commit    = "none"
    date      = "unknown"
    builtBy   = "unknown"
)

func versionCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "version",
        Short: "Print version information",
        Run: func(cmd *cobra.Command, args []string) {
            fmt.Printf("ccusage version %s\n", version)
            fmt.Printf("  commit: %s\n", commit)
            fmt.Printf("  built at: %s\n", date)
            fmt.Printf("  built by: %s\n", builtBy)
        },
    }
}
```

## 4. 命令結構設計

### 4.1 基礎命令介面

```go
package commands

type Command interface {
    Execute(ctx context.Context) error
    Validate() error
}

type BaseCommand struct {
    Name        string
    Description string
    Flags       *FlagSet
    Config      *Config
}

func (bc *BaseCommand) PreRun() error {
    // 共用的預處理邏輯
    if err := bc.Validate(); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }
    
    // 載入配置
    if err := bc.loadConfig(); err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }
    
    return nil
}

func (bc *BaseCommand) PostRun() error {
    // 共用的後處理邏輯
    return nil
}
```

### 4.2 Daily 命令實作

```go
func NewDailyCommand() *cobra.Command {
    var opts DailyOptions
    
    cmd := &cobra.Command{
        Use:   "daily",
        Short: "Show usage report grouped by date",
        Long: `Generate daily usage reports showing token consumption and costs aggregated by date.
        
Examples:
  # Basic daily report
  ccusage daily
  
  # Filter by date range
  ccusage daily --since 20240101 --until 20240131
  
  # JSON output with breakdown
  ccusage daily --json --breakdown
  
  # Group by project
  ccusage daily --instances --project myproject`,
        RunE: func(cmd *cobra.Command, args []string) error {
            return runDaily(cmd.Context(), opts)
        },
    }
    
    // 添加命令特定標誌
    cmd.Flags().StringVar(&opts.Since, "since", "", "Start date (YYYYMMDD)")
    cmd.Flags().StringVar(&opts.Until, "until", "", "End date (YYYYMMDD)")
    cmd.Flags().BoolVar(&opts.Breakdown, "breakdown", false, "Show per-model breakdown")
    cmd.Flags().BoolVarP(&opts.Instances, "instances", "i", false, "Group by project/instance")
    cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Filter by project name")
    
    // 添加共享標誌
    addSharedFlags(cmd, &opts.SharedOptions)
    
    return cmd
}

type DailyOptions struct {
    SharedOptions
    Since      string
    Until      string
    Breakdown  bool
    Instances  bool
    Project    string
}

func runDaily(ctx context.Context, opts DailyOptions) error {
    // 驗證參數
    if err := validateDailyOptions(opts); err != nil {
        return err
    }
    
    // 載入資料
    loader := loader.New(opts.Path)
    entries, err := loader.Load(ctx)
    if err != nil {
        return fmt.Errorf("failed to load data: %w", err)
    }
    
    // 生成報告
    generator := reports.NewDailyGenerator()
    report, err := generator.Generate(entries, opts.ToGenerateOptions())
    if err != nil {
        return fmt.Errorf("failed to generate report: %w", err)
    }
    
    // 輸出報告
    formatter := output.NewFormatter(opts.OutputFormat())
    return formatter.Format(report, os.Stdout)
}
```

## 5. 共享參數配置

### 5.1 共享參數定義

```go
type SharedOptions struct {
    // 輸出選項
    JSON      bool
    JQ        string
    CSV       bool
    
    // 過濾選項
    Mode      string
    Order     string
    Timezone  string
    Locale    string
    
    // 系統選項
    Offline   bool
    Debug     bool
    Path      string
}

func addSharedFlags(cmd *cobra.Command, opts *SharedOptions) {
    // 輸出格式
    cmd.Flags().BoolVarP(&opts.JSON, "json", "j", false, "Output in JSON format")
    cmd.Flags().StringVar(&opts.JQ, "jq", "", "Process JSON output with jq expression")
    cmd.Flags().BoolVar(&opts.CSV, "csv", false, "Output in CSV format")
    
    // 過濾和排序
    cmd.Flags().StringVarP(&opts.Mode, "mode", "m", "input", 
        "Cost calculation mode (input|output|total|maxOutput|maxTotal)")
    cmd.Flags().StringVarP(&opts.Order, "order", "o", "asc", 
        "Sort order (asc|desc)")
    
    // 本地化
    cmd.Flags().StringVar(&opts.Timezone, "timezone", "", 
        "Timezone for date grouping (e.g., America/New_York)")
    cmd.Flags().StringVar(&opts.Locale, "locale", "", 
        "Locale for formatting (e.g., en-US, ja-JP)")
    
    // 系統
    cmd.Flags().BoolVar(&opts.Offline, "offline", false, 
        "Use offline pricing data")
    cmd.Flags().BoolVarP(&opts.Debug, "debug", "d", false, 
        "Enable debug output")
    cmd.Flags().StringVar(&opts.Path, "path", "", 
        "Custom Claude data directory")
}

func (so *SharedOptions) OutputFormat() output.Format {
    switch {
    case so.JSON || so.JQ != "":
        return output.FormatJSON
    case so.CSV:
        return output.FormatCSV
    default:
        return output.FormatTable
    }
}
```

### 5.2 參數驗證

```go
type Validator struct {
    rules []ValidationRule
}

type ValidationRule struct {
    Name      string
    Condition func() bool
    Message   string
}

func validateDailyOptions(opts DailyOptions) error {
    validator := &Validator{
        rules: []ValidationRule{
            {
                Name: "date_range",
                Condition: func() bool {
                    if opts.Since == "" && opts.Until == "" {
                        return true
                    }
                    return isValidDateRange(opts.Since, opts.Until)
                },
                Message: "Invalid date range: since must be before until",
            },
            {
                Name: "mode",
                Condition: func() bool {
                    validModes := []string{"input", "output", "total", "maxOutput", "maxTotal"}
                    return contains(validModes, opts.Mode)
                },
                Message: "Invalid mode. Must be one of: input, output, total, maxOutput, maxTotal",
            },
            {
                Name: "order",
                Condition: func() bool {
                    return opts.Order == "asc" || opts.Order == "desc"
                },
                Message: "Invalid order. Must be 'asc' or 'desc'",
            },
        },
    }
    
    return validator.Validate()
}

func (v *Validator) Validate() error {
    for _, rule := range v.rules {
        if !rule.Condition() {
            return fmt.Errorf("%s: %s", rule.Name, rule.Message)
        }
    }
    return nil
}

func isValidDateRange(since, until string) bool {
    if since == "" || until == "" {
        return true
    }
    
    sinceDate, err := parseDate(since)
    if err != nil {
        return false
    }
    
    untilDate, err := parseDate(until)
    if err != nil {
        return false
    }
    
    return !sinceDate.After(untilDate)
}
```

## 6. 錯誤處理

### 6.1 錯誤類型定義

```go
type CLIError struct {
    Type    ErrorType
    Command string
    Message string
    Cause   error
    Help    string
}

type ErrorType int

const (
    ErrorTypeInvalidArgs ErrorType = iota
    ErrorTypeFileNotFound
    ErrorTypePermission
    ErrorTypeNetwork
    ErrorTypeInternal
)

func (e *CLIError) Error() string {
    var msg strings.Builder
    
    msg.WriteString(fmt.Sprintf("Error in %s command: %s", e.Command, e.Message))
    
    if e.Cause != nil {
        msg.WriteString(fmt.Sprintf("\nCause: %v", e.Cause))
    }
    
    if e.Help != "" {
        msg.WriteString(fmt.Sprintf("\n\nHelp: %s", e.Help))
    }
    
    return msg.String()
}

func (e *CLIError) ExitCode() int {
    switch e.Type {
    case ErrorTypeInvalidArgs:
        return 2
    case ErrorTypeFileNotFound:
        return 3
    case ErrorTypePermission:
        return 4
    case ErrorTypeNetwork:
        return 5
    default:
        return 1
    }
}
```

### 6.2 錯誤處理器

```go
type ErrorHandler struct {
    verbose bool
    color   bool
}

func NewErrorHandler(verbose, color bool) *ErrorHandler {
    return &ErrorHandler{
        verbose: verbose,
        color:   color,
    }
}

func (eh *ErrorHandler) Handle(err error) {
    if err == nil {
        return
    }
    
    // 檢查是否為 CLI 錯誤
    var cliErr *CLIError
    if errors.As(err, &cliErr) {
        eh.handleCLIError(cliErr)
        os.Exit(cliErr.ExitCode())
    }
    
    // 處理一般錯誤
    eh.handleGenericError(err)
    os.Exit(1)
}

func (eh *ErrorHandler) handleCLIError(err *CLIError) {
    if eh.color {
        fmt.Fprintf(os.Stderr, "\033[31mError:\033[0m %s\n", err.Message)
    } else {
        fmt.Fprintf(os.Stderr, "Error: %s\n", err.Message)
    }
    
    if eh.verbose && err.Cause != nil {
        fmt.Fprintf(os.Stderr, "Cause: %v\n", err.Cause)
    }
    
    if err.Help != "" {
        fmt.Fprintf(os.Stderr, "\n%s\n", err.Help)
    }
}
```

## 7. 自動補全

### 7.1 Bash 補全

```go
func generateBashCompletion(cmd *cobra.Command) string {
    var buf bytes.Buffer
    cmd.GenBashCompletion(&buf)
    return buf.String()
}

func installBashCompletion() error {
    completion := generateBashCompletion(rootCmd)
    
    homeDir, err := os.UserHomeDir()
    if err != nil {
        return err
    }
    
    completionPath := filepath.Join(homeDir, ".bash_completion.d", "ccusage")
    
    if err := os.MkdirAll(filepath.Dir(completionPath), 0755); err != nil {
        return err
    }
    
    return os.WriteFile(completionPath, []byte(completion), 0644)
}
```

### 7.2 Zsh 補全

```go
func generateZshCompletion(cmd *cobra.Command) string {
    var buf bytes.Buffer
    cmd.GenZshCompletion(&buf)
    return buf.String()
}

func installZshCompletion() error {
    completion := generateZshCompletion(rootCmd)
    
    // Zsh 補全檔案位置
    completionPath := "/usr/local/share/zsh/site-functions/_ccusage"
    
    return os.WriteFile(completionPath, []byte(completion), 0644)
}
```

### 7.3 自定義補全

```go
func customCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
    // 專案名稱補全
    if cmd.Name() == "daily" && strings.Contains(cmd.CommandPath(), "--project") {
        return getProjectNames(), cobra.ShellCompDirectiveNoFileComp
    }
    
    // 日期補全
    if strings.HasPrefix(toComplete, "--since=") || strings.HasPrefix(toComplete, "--until=") {
        return getDateSuggestions(), cobra.ShellCompDirectiveNoFileComp
    }
    
    // 模式補全
    if strings.HasPrefix(toComplete, "--mode=") {
        return []string{"input", "output", "total", "maxOutput", "maxTotal"}, 
               cobra.ShellCompDirectiveNoFileComp
    }
    
    return nil, cobra.ShellCompDirectiveDefault
}

func getProjectNames() []string {
    // 從資料中提取專案名稱
    loader := loader.New("")
    projects, _ := loader.GetProjects(context.Background())
    return projects
}

func getDateSuggestions() []string {
    // 提供最近日期建議
    suggestions := []string{}
    now := time.Now()
    
    for i := 0; i < 7; i++ {
        date := now.AddDate(0, 0, -i)
        suggestions = append(suggestions, date.Format("20060102"))
    }
    
    return suggestions
}
```

## 8. 幫助系統

### 8.1 幫助文字生成

```go
type HelpGenerator struct {
    command *cobra.Command
    verbose bool
}

func (hg *HelpGenerator) Generate() string {
    var help strings.Builder
    
    // 命令描述
    help.WriteString(fmt.Sprintf("%s\n\n", hg.command.Long))
    
    // 使用方法
    help.WriteString("Usage:\n")
    help.WriteString(fmt.Sprintf("  %s\n\n", hg.command.UseLine()))
    
    // 範例
    if examples := hg.getExamples(); len(examples) > 0 {
        help.WriteString("Examples:\n")
        for _, example := range examples {
            help.WriteString(fmt.Sprintf("  %s\n", example))
        }
        help.WriteString("\n")
    }
    
    // 標誌
    help.WriteString("Flags:\n")
    help.WriteString(hg.formatFlags())
    
    // 全域標誌
    if hg.verbose {
        help.WriteString("\nGlobal Flags:\n")
        help.WriteString(hg.formatGlobalFlags())
    }
    
    return help.String()
}

func (hg *HelpGenerator) formatFlags() string {
    var flags strings.Builder
    
    hg.command.Flags().VisitAll(func(flag *pflag.Flag) {
        if flag.Hidden {
            return
        }
        
        shorthand := ""
        if flag.Shorthand != "" {
            shorthand = fmt.Sprintf("-%s, ", flag.Shorthand)
        }
        
        flags.WriteString(fmt.Sprintf("  %s--%s %s\n", 
            shorthand, flag.Name, flag.Usage))
            
        if flag.DefValue != "" && flag.DefValue != "false" {
            flags.WriteString(fmt.Sprintf("        (default: %s)\n", flag.DefValue))
        }
    })
    
    return flags.String()
}
```

### 8.2 互動式幫助

```go
type InteractiveHelp struct {
    commands map[string]*cobra.Command
}

func NewInteractiveHelp() *InteractiveHelp {
    return &InteractiveHelp{
        commands: make(map[string]*cobra.Command),
    }
}

func (ih *InteractiveHelp) Start() error {
    reader := bufio.NewReader(os.Stdin)
    
    for {
        fmt.Print("\nccusage help> ")
        input, err := reader.ReadString('\n')
        if err != nil {
            return err
        }
        
        input = strings.TrimSpace(input)
        
        switch input {
        case "quit", "exit", "q":
            return nil
        case "list", "ls":
            ih.listCommands()
        default:
            ih.showHelp(input)
        }
    }
}

func (ih *InteractiveHelp) listCommands() {
    fmt.Println("\nAvailable commands:")
    fmt.Println("  daily     - Show daily usage report")
    fmt.Println("  monthly   - Show monthly usage report")
    fmt.Println("  weekly    - Show weekly usage report")
    fmt.Println("  session   - Show session-based report")
    fmt.Println("  blocks    - Show 5-hour block report")
    fmt.Println("  statusline - Show compact status line")
    fmt.Println("  mcp       - Start MCP server")
}
```

## 9. 配置管理

### 9.1 配置載入

```go
type CLIConfig struct {
    DefaultCommand string            `json:"default_command"`
    DefaultFlags   map[string]string `json:"default_flags"`
    Aliases        map[string]string `json:"aliases"`
    ColorScheme    string           `json:"color_scheme"`
    OutputFormat   string           `json:"output_format"`
}

func LoadConfig() (*CLIConfig, error) {
    // 配置檔案路徑優先級
    paths := []string{
        "./.ccusage.json",
        "$HOME/.config/ccusage/config.json",
        "$HOME/.ccusage.json",
    }
    
    for _, path := range paths {
        expanded := os.ExpandEnv(path)
        if _, err := os.Stat(expanded); err == nil {
            return loadConfigFile(expanded)
        }
    }
    
    // 返回預設配置
    return defaultConfig(), nil
}

func loadConfigFile(path string) (*CLIConfig, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    var config CLIConfig
    if err := json.Unmarshal(data, &config); err != nil {
        return nil, err
    }
    
    return &config, nil
}

func defaultConfig() *CLIConfig {
    return &CLIConfig{
        DefaultCommand: "daily",
        DefaultFlags:   make(map[string]string),
        Aliases:        make(map[string]string),
        ColorScheme:    "auto",
        OutputFormat:   "table",
    }
}
```

### 9.2 別名系統

```go
type AliasManager struct {
    aliases map[string][]string
}

func NewAliasManager(config *CLIConfig) *AliasManager {
    am := &AliasManager{
        aliases: make(map[string][]string),
    }
    
    // 載入配置的別名
    for alias, command := range config.Aliases {
        am.aliases[alias] = strings.Fields(command)
    }
    
    // 添加預設別名
    am.addDefaultAliases()
    
    return am
}

func (am *AliasManager) addDefaultAliases() {
    am.aliases["d"] = []string{"daily"}
    am.aliases["m"] = []string{"monthly"}
    am.aliases["s"] = []string{"session"}
    am.aliases["b"] = []string{"blocks"}
    am.aliases["today"] = []string{"daily", "--since", getTodayDate()}
    am.aliases["yesterday"] = []string{"daily", "--since", getYesterdayDate(), 
                                      "--until", getYesterdayDate()}
}

func (am *AliasManager) Expand(args []string) []string {
    if len(args) == 0 {
        return args
    }
    
    // 檢查第一個參數是否為別名
    if expanded, exists := am.aliases[args[0]]; exists {
        return append(expanded, args[1:]...)
    }
    
    return args
}
```

## 10. 測試策略

### 10.1 命令測試

```go
func TestDailyCommand(t *testing.T) {
    tests := []struct {
        name     string
        args     []string
        wantErr  bool
        contains string
    }{
        {
            name:     "basic daily report",
            args:     []string{"daily"},
            wantErr:  false,
            contains: "Date",
        },
        {
            name:     "with date range",
            args:     []string{"daily", "--since", "20240101", "--until", "20240131"},
            wantErr:  false,
            contains: "2024-01",
        },
        {
            name:    "invalid date format",
            args:    []string{"daily", "--since", "invalid"},
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cmd := NewDailyCommand()
            buf := new(bytes.Buffer)
            cmd.SetOut(buf)
            cmd.SetErr(buf)
            cmd.SetArgs(tt.args)
            
            err := cmd.Execute()
            
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                if tt.contains != "" {
                    assert.Contains(t, buf.String(), tt.contains)
                }
            }
        })
    }
}
```

### 10.2 參數驗證測試

```go
func TestValidation(t *testing.T) {
    tests := []struct {
        name    string
        opts    DailyOptions
        wantErr bool
    }{
        {
            name: "valid options",
            opts: DailyOptions{
                Since: "20240101",
                Until: "20240131",
                Mode:  "input",
                Order: "asc",
            },
            wantErr: false,
        },
        {
            name: "invalid date range",
            opts: DailyOptions{
                Since: "20240131",
                Until: "20240101",
            },
            wantErr: true,
        },
        {
            name: "invalid mode",
            opts: DailyOptions{
                Mode: "invalid",
            },
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateDailyOptions(tt.opts)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

## 11. 與 TypeScript 版本對照

| TypeScript 元素 | Go 元素 | 說明 |
|----------------|---------|------|
| gunshi CLI framework | Cobra | CLI 框架 |
| define() | NewCommand() | 命令定義 |
| sharedCommandConfig | SharedOptions | 共享參數 |
| ctx.values | cmd.Flags() | 參數獲取 |
| process.exit() | os.Exit() | 程式退出 |

## 12. 效能優化

### 12.1 命令快取

```go
type CommandCache struct {
    cache map[string]*CachedResult
    ttl   time.Duration
    mutex sync.RWMutex
}

type CachedResult struct {
    Data      interface{}
    Timestamp time.Time
}

func (cc *CommandCache) Get(key string) (interface{}, bool) {
    cc.mutex.RLock()
    defer cc.mutex.RUnlock()
    
    if cached, exists := cc.cache[key]; exists {
        if time.Since(cached.Timestamp) < cc.ttl {
            return cached.Data, true
        }
    }
    
    return nil, false
}

func (cc *CommandCache) Set(key string, data interface{}) {
    cc.mutex.Lock()
    defer cc.mutex.Unlock()
    
    cc.cache[key] = &CachedResult{
        Data:      data,
        Timestamp: time.Now(),
    }
}
```