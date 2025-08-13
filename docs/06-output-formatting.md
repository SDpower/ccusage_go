# 輸出格式化模組設計

## 1. 模組概述

輸出格式化模組負責將處理後的數據以各種格式呈現給使用者，包括表格顯示、JSON 輸出、響應式佈局等。模組採用 Bubble Tea 框架實現現代化的終端使用者介面，提供互動式體驗。

### 1.1 主要功能
- 互動式終端 UI (使用 Bubble Tea)
- 表格格式化與渲染
- JSON 序列化輸出
- 響應式終端佈局
- 樣式與主題管理 (使用 Lip Gloss)
- 國際化支援
- 自適應欄位顯示

### 1.2 對應 TypeScript 模組
- `_utils.ts` → `output/formatter.go`
- `_terminal-utils.ts` → `output/terminal.go`
- 表格相關功能 → `output/table.go`
- 新增 Bubble Tea UI → `output/tui.go`

## 2. 輸出格式化架構

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│    Data      │────▶│  Formatter   │────▶│   Renderer   │
└──────────────┘     └──────┬───────┘     └──────────────┘
                            │
       ┌────────────────────┼────────────────────┐
       │                    │                    │
┌──────▼──────┐     ┌──────▼──────┐     ┌──────▼──────┐
│Table Format │     │JSON Format  │     │ CSV Format  │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                    │                    │
       └────────────────────┼────────────────────┘
                            │
                     ┌──────▼──────┐
                     │   Output    │
                     └─────────────┘
```

## 3. Bubble Tea 終端 UI 系統

### 3.1 Bubble Tea 應用程式架構

```go
package output

import (
    "fmt"
    "strings"
    
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/bubbles/table"
    "github.com/charmbracelet/bubbles/viewport"
    "github.com/charmbracelet/lipgloss"
)

// 主要的 TUI 模型
type Model struct {
    table     table.Model
    viewport  viewport.Model
    ready     bool
    width     int
    height    int
    mode      ViewMode
    data      interface{}
}

type ViewMode int

const (
    ViewModeTable ViewMode = iota
    ViewModeDetail
    ViewModeChart
)

// 初始化模型
func NewModel(data interface{}) Model {
    return Model{
        mode: ViewModeTable,
        data: data,
    }
}

// Bubble Tea 介面實作
func (m Model) Init() tea.Cmd {
    return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        
        if !m.ready {
            m.viewport = viewport.New(msg.Width, msg.Height-4)
            m.table = m.createTable()
            m.ready = true
        } else {
            m.viewport.Width = msg.Width
            m.viewport.Height = msg.Height - 4
        }
        
    case tea.KeyMsg:
        switch msg.String() {
        case "ctrl+c", "q":
            return m, tea.Quit
        case "tab":
            m.mode = (m.mode + 1) % 3
        case "up", "k":
            m.table, cmd = m.table.Update(msg)
        case "down", "j":
            m.table, cmd = m.table.Update(msg)
        }
    }
    
    // 更新當前視圖組件
    switch m.mode {
    case ViewModeTable:
        m.table, cmd = m.table.Update(msg)
    case ViewModeDetail:
        m.viewport, cmd = m.viewport.Update(msg)
    }
    
    return m, cmd
}

func (m Model) View() string {
    if !m.ready {
        return "\n  Initializing..."
    }
    
    var content string
    
    switch m.mode {
    case ViewModeTable:
        content = m.renderTable()
    case ViewModeDetail:
        content = m.renderDetail()
    case ViewModeChart:
        content = m.renderChart()
    }
    
    return m.renderFrame(content)
}
```

### 3.2 Lip Gloss 樣式系統

```go
// 定義樣式
var (
    // 基礎樣式
    baseStyle = lipgloss.NewStyle().
        BorderStyle(lipgloss.NormalBorder()).
        BorderForeground(lipgloss.Color("240"))
    
    // 標題樣式
    titleStyle = lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("230")).
        Background(lipgloss.Color("63")).
        Padding(0, 1)
    
    // 表格樣式
    tableStyle = lipgloss.NewStyle().
        BorderStyle(lipgloss.RoundedBorder()).
        BorderForeground(lipgloss.Color("62")).
        Padding(1, 2)
    
    // 高亮樣式
    highlightStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("205")).
        Bold(true)
    
    // 警告樣式
    warningStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("214")).
        Bold(true)
    
    // 錯誤樣式
    errorStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("196")).
        Bold(true)
)

// 主題管理
type Theme struct {
    Primary   lipgloss.Style
    Secondary lipgloss.Style
    Success   lipgloss.Style
    Warning   lipgloss.Style
    Error     lipgloss.Style
    Muted     lipgloss.Style
}

func NewTheme() *Theme {
    return &Theme{
        Primary:   lipgloss.NewStyle().Foreground(lipgloss.Color("86")),
        Secondary: lipgloss.NewStyle().Foreground(lipgloss.Color("62")),
        Success:   lipgloss.NewStyle().Foreground(lipgloss.Color("82")),
        Warning:   lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
        Error:     lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
        Muted:     lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
    }
}

func (m Model) renderFrame(content string) string {
    // 建立框架
    header := titleStyle.Render("ccusage - Claude Code Usage Analyzer")
    
    footer := lipgloss.NewStyle().
        Foreground(lipgloss.Color("241")).
        Render("Press 'q' to quit • Tab to switch views • ↑↓ to navigate")
    
    frame := lipgloss.JoinVertical(
        lipgloss.Left,
        header,
        content,
        footer,
    )
    
    return baseStyle.Width(m.width).Height(m.height).Render(frame)
}
```

### 3.3 互動式表格

```go
func (m Model) createTable() table.Model {
    columns := []table.Column{
        {Title: "Date", Width: 12},
        {Title: "Input Tokens", Width: 15},
        {Title: "Output Tokens", Width: 15},
        {Title: "Total Cost", Width: 12},
        {Title: "Models", Width: 20},
    }
    
    rows := m.convertDataToRows()
    
    t := table.New(
        table.WithColumns(columns),
        table.WithRows(rows),
        table.WithFocused(true),
        table.WithHeight(m.height - 8),
    )
    
    // 自定義表格樣式
    s := table.DefaultStyles()
    s.Header = s.Header.
        BorderStyle(lipgloss.NormalBorder()).
        BorderForeground(lipgloss.Color("240")).
        BorderBottom(true).
        Bold(false)
    s.Selected = s.Selected.
        Foreground(lipgloss.Color("229")).
        Background(lipgloss.Color("57")).
        Bold(false)
    
    t.SetStyles(s)
    
    return t
}

func (m Model) convertDataToRows() []table.Row {
    var rows []table.Row
    
    // 根據資料類型轉換
    switch data := m.data.(type) {
    case []DailyUsage:
        for _, d := range data {
            rows = append(rows, table.Row{
                d.Date,
                formatNumber(d.InputTokens),
                formatNumber(d.OutputTokens),
                formatCurrency(d.TotalCost),
                strings.Join(d.ModelsUsed, ", "),
            })
        }
    }
    
    return rows
}
```

### 3.4 原始表格建構器（保留作為備選）

```go
package output

import (
    "strings"
    "github.com/olekukonko/tablewriter"
)

type TableBuilder struct {
    headers     []string
    rows        [][]string
    config      *TableConfig
    responsive  bool
}

type TableConfig struct {
    Border          bool
    AutoWrap        bool
    AutoMergeCells  bool
    ColumnAlignment []int
    ColumnColors    []tablewriter.Colors
    HeaderColors    tablewriter.Colors
    RowColors       []tablewriter.Colors
    MaxColumnWidth  int
}

func NewTableBuilder() *TableBuilder {
    return &TableBuilder{
        config: &TableConfig{
            Border:         true,
            AutoWrap:       true,
            MaxColumnWidth: 50,
        },
        responsive: true,
    }
}

func (tb *TableBuilder) SetHeaders(headers ...string) *TableBuilder {
    tb.headers = headers
    return tb
}

func (tb *TableBuilder) AddRow(values ...string) *TableBuilder {
    tb.rows = append(tb.rows, values)
    return tb
}

func (tb *TableBuilder) Build() *Table {
    return &Table{
        headers:    tb.headers,
        rows:       tb.rows,
        config:     tb.config,
        responsive: tb.responsive,
    }
}

type Table struct {
    headers    []string
    rows       [][]string
    config     *TableConfig
    responsive bool
}

func (t *Table) Render(writer io.Writer) error {
    table := tablewriter.NewWriter(writer)
    
    // 設定表格配置
    table.SetBorder(t.config.Border)
    table.SetAutoWrapText(t.config.AutoWrap)
    table.SetAutoMergeCells(t.config.AutoMergeCells)
    
    // 設定標題
    table.SetHeader(t.headers)
    
    // 設定顏色
    if t.config.HeaderColors != nil {
        table.SetHeaderColor(t.config.HeaderColors)
    }
    
    // 添加行
    for i, row := range t.rows {
        if t.config.RowColors != nil && i < len(t.config.RowColors) {
            table.Rich(row, t.config.RowColors[i])
        } else {
            table.Append(row)
        }
    }
    
    table.Render()
    return nil
}
```

### 3.2 響應式表格

```go
type ResponsiveTable struct {
    table           *Table
    terminalWidth   int
    compactMode     bool
    priorityColumns []int // 欄位優先級
}

func NewResponsiveTable(table *Table) *ResponsiveTable {
    return &ResponsiveTable{
        table:         table,
        terminalWidth: getTerminalWidth(),
    }
}

func (rt *ResponsiveTable) SetPriority(columns ...int) {
    rt.priorityColumns = columns
}

func (rt *ResponsiveTable) Render(writer io.Writer) error {
    // 檢查終端寬度
    if rt.terminalWidth < 100 {
        rt.compactMode = true
        return rt.renderCompact(writer)
    }
    
    return rt.renderFull(writer)
}

func (rt *ResponsiveTable) renderCompact(writer io.Writer) error {
    // 選擇優先級高的欄位
    selectedColumns := rt.selectColumns()
    
    // 創建精簡表格
    compactTable := &Table{
        headers: rt.filterHeaders(selectedColumns),
        rows:    rt.filterRows(selectedColumns),
        config:  rt.table.config,
    }
    
    return compactTable.Render(writer)
}

func (rt *ResponsiveTable) selectColumns() []int {
    if len(rt.priorityColumns) > 0 {
        // 根據終端寬度選擇可顯示的欄位
        available := rt.terminalWidth / 15 // 假設每欄平均15字元
        if available > len(rt.priorityColumns) {
            available = len(rt.priorityColumns)
        }
        return rt.priorityColumns[:available]
    }
    
    // 預設選擇前幾個欄位
    columnCount := len(rt.table.headers)
    maxColumns := rt.terminalWidth / 15
    if maxColumns > columnCount {
        maxColumns = columnCount
    }
    
    columns := make([]int, maxColumns)
    for i := 0; i < maxColumns; i++ {
        columns[i] = i
    }
    return columns
}
```

### 3.3 多行格式化

```go
type MultilineFormatter struct {
    separator string
    indent    string
    maxWidth  int
}

func NewMultilineFormatter() *MultilineFormatter {
    return &MultilineFormatter{
        separator: "\n",
        indent:    "  ",
        maxWidth:  50,
    }
}

func (mf *MultilineFormatter) FormatList(items []string) string {
    if len(items) == 0 {
        return "-"
    }
    
    if len(items) == 1 {
        return items[0]
    }
    
    var builder strings.Builder
    for i, item := range items {
        if i > 0 {
            builder.WriteString(mf.separator)
            builder.WriteString(mf.indent)
        }
        
        // 處理長字串
        wrapped := mf.wrapText(item)
        builder.WriteString(wrapped)
    }
    
    return builder.String()
}

func (mf *MultilineFormatter) FormatModels(models []string) string {
    if len(models) == 0 {
        return "-"
    }
    
    // 統一格式：使用項目符號
    var formatted []string
    for _, model := range models {
        formatted = append(formatted, "• "+mf.simplifyModelName(model))
    }
    
    return strings.Join(formatted, "\n")
}

func (mf *MultilineFormatter) simplifyModelName(model string) string {
    // 簡化模型名稱顯示
    replacements := map[string]string{
        "claude-3-opus-20240229":     "Opus",
        "claude-3-5-sonnet-20241022": "Sonnet 3.5",
        "claude-3-haiku-20240307":    "Haiku",
    }
    
    if simplified, exists := replacements[model]; exists {
        return simplified
    }
    
    return model
}
```

## 4. JSON 輸出格式化

### 4.1 JSON 序列化器

```go
type JSONFormatter struct {
    indent      string
    escapeHTML  bool
    sortKeys    bool
}

func NewJSONFormatter() *JSONFormatter {
    return &JSONFormatter{
        indent:     "  ",
        escapeHTML: false,
        sortKeys:   true,
    }
}

func (jf *JSONFormatter) Format(data interface{}) ([]byte, error) {
    encoder := json.NewEncoder(&bytes.Buffer{})
    encoder.SetIndent("", jf.indent)
    encoder.SetEscapeHTML(jf.escapeHTML)
    
    var buf bytes.Buffer
    encoder = json.NewEncoder(&buf)
    encoder.SetIndent("", jf.indent)
    encoder.SetEscapeHTML(jf.escapeHTML)
    
    if err := encoder.Encode(data); err != nil {
        return nil, err
    }
    
    if jf.sortKeys {
        return jf.sortJSONKeys(buf.Bytes())
    }
    
    return buf.Bytes(), nil
}

func (jf *JSONFormatter) FormatCompact(data interface{}) ([]byte, error) {
    return json.Marshal(data)
}

type JSONOutput struct {
    Data      interface{} `json:"data"`
    Metadata  Metadata    `json:"metadata"`
    Timestamp time.Time   `json:"timestamp"`
}

type Metadata struct {
    Version    string `json:"version"`
    Command    string `json:"command"`
    Parameters map[string]interface{} `json:"parameters"`
}
```

### 4.2 JQ 處理器整合

```go
type JQProcessor struct {
    expression string
}

func NewJQProcessor(expression string) *JQProcessor {
    return &JQProcessor{
        expression: expression,
    }
}

func (jp *JQProcessor) Process(jsonData []byte) ([]byte, error) {
    // 使用 gojq 庫處理
    var input interface{}
    if err := json.Unmarshal(jsonData, &input); err != nil {
        return nil, err
    }
    
    query, err := gojq.Parse(jp.expression)
    if err != nil {
        return nil, fmt.Errorf("invalid jq expression: %w", err)
    }
    
    iter := query.Run(input)
    var results []interface{}
    
    for {
        v, ok := iter.Next()
        if !ok {
            break
        }
        if err, ok := v.(error); ok {
            return nil, err
        }
        results = append(results, v)
    }
    
    // 如果只有一個結果，直接返回
    if len(results) == 1 {
        return json.Marshal(results[0])
    }
    
    return json.Marshal(results)
}
```

## 5. 顏色與樣式管理

### 5.1 顏色管理器

```go
type ColorManager struct {
    enabled bool
    theme   *ColorTheme
}

type ColorTheme struct {
    Success   Color
    Warning   Color
    Error     Color
    Info      Color
    Header    Color
    Highlight Color
    Muted     Color
}

type Color struct {
    Foreground string
    Background string
    Attributes []Attribute
}

type Attribute int

const (
    Bold Attribute = iota
    Italic
    Underline
    Blink
    Reverse
)

func NewColorManager() *ColorManager {
    return &ColorManager{
        enabled: isTerminalColorSupported(),
        theme:   DefaultTheme(),
    }
}

func DefaultTheme() *ColorTheme {
    return &ColorTheme{
        Success:   Color{Foreground: "green", Attributes: []Attribute{Bold}},
        Warning:   Color{Foreground: "yellow"},
        Error:     Color{Foreground: "red", Attributes: []Attribute{Bold}},
        Info:      Color{Foreground: "blue"},
        Header:    Color{Foreground: "cyan", Attributes: []Attribute{Bold}},
        Highlight: Color{Foreground: "magenta"},
        Muted:     Color{Foreground: "gray"},
    }
}

func (cm *ColorManager) Apply(text string, color Color) string {
    if !cm.enabled {
        return text
    }
    
    return cm.wrapWithColor(text, color)
}

func (cm *ColorManager) Success(text string) string {
    return cm.Apply(text, cm.theme.Success)
}

func (cm *ColorManager) Warning(text string) string {
    return cm.Apply(text, cm.theme.Warning)
}

func (cm *ColorManager) Error(text string) string {
    return cm.Apply(text, cm.theme.Error)
}
```

### 5.2 樣式格式化器

```go
type StyleFormatter struct {
    colors *ColorManager
}

func (sf *StyleFormatter) FormatCurrency(value float64) string {
    formatted := fmt.Sprintf("$%.2f", value)
    
    if value > 100 {
        return sf.colors.Error(formatted)
    } else if value > 50 {
        return sf.colors.Warning(formatted)
    }
    
    return formatted
}

func (sf *StyleFormatter) FormatNumber(value int) string {
    // 添加千位分隔符
    str := strconv.Itoa(value)
    if value < 1000 {
        return str
    }
    
    var result []string
    for i, r := range str {
        if i > 0 && (len(str)-i)%3 == 0 {
            result = append(result, ",")
        }
        result = append(result, string(r))
    }
    
    return strings.Join(result, "")
}

func (sf *StyleFormatter) FormatPercentage(value float64) string {
    formatted := fmt.Sprintf("%.1f%%", value*100)
    
    if value > 0.9 {
        return sf.colors.Error(formatted)
    } else if value > 0.7 {
        return sf.colors.Warning(formatted)
    }
    
    return formatted
}
```

## 6. 終端工具

### 6.1 終端資訊

```go
type Terminal struct {
    width      int
    height     int
    isColor    bool
    isUnicode  bool
    writer     io.Writer
}

func NewTerminal() *Terminal {
    return &Terminal{
        width:     getTerminalWidth(),
        height:    getTerminalHeight(),
        isColor:   isColorSupported(),
        isUnicode: isUnicodeSupported(),
        writer:    os.Stdout,
    }
}

func getTerminalWidth() int {
    width, _, err := terminal.GetSize(int(os.Stdout.Fd()))
    if err != nil {
        return 80 // 預設寬度
    }
    return width
}

func (t *Terminal) Clear() {
    t.writer.Write([]byte("\033[2J\033[H"))
}

func (t *Terminal) MoveCursor(x, y int) {
    fmt.Fprintf(t.writer, "\033[%d;%dH", y, x)
}

func (t *Terminal) HideCursor() {
    t.writer.Write([]byte("\033[?25l"))
}

func (t *Terminal) ShowCursor() {
    t.writer.Write([]byte("\033[?25h"))
}
```

### 6.2 進度條

```go
type ProgressBar struct {
    total     int
    current   int
    width     int
    showSpeed bool
    startTime time.Time
    terminal  *Terminal
}

func NewProgressBar(total int) *ProgressBar {
    return &ProgressBar{
        total:     total,
        width:     40,
        showSpeed: true,
        startTime: time.Now(),
        terminal:  NewTerminal(),
    }
}

func (pb *ProgressBar) Update(current int) {
    pb.current = current
    pb.render()
}

func (pb *ProgressBar) render() {
    percent := float64(pb.current) / float64(pb.total)
    filled := int(percent * float64(pb.width))
    
    // 建構進度條
    var bar strings.Builder
    bar.WriteString("\r[")
    
    for i := 0; i < pb.width; i++ {
        if i < filled {
            bar.WriteString("█")
        } else {
            bar.WriteString(" ")
        }
    }
    
    bar.WriteString("] ")
    bar.WriteString(fmt.Sprintf("%.1f%%", percent*100))
    
    // 顯示速度
    if pb.showSpeed {
        elapsed := time.Since(pb.startTime)
        speed := float64(pb.current) / elapsed.Seconds()
        bar.WriteString(fmt.Sprintf(" (%.1f/s)", speed))
    }
    
    // 顯示剩餘時間
    if pb.current > 0 {
        elapsed := time.Since(pb.startTime)
        remaining := elapsed * time.Duration(pb.total-pb.current) / time.Duration(pb.current)
        bar.WriteString(fmt.Sprintf(" ETA: %s", remaining.Round(time.Second)))
    }
    
    fmt.Fprint(pb.terminal.writer, bar.String())
}
```

## 7. 國際化支援

### 7.1 本地化管理器

```go
type LocaleManager struct {
    locale      string
    location    *time.Location
    numberFormat NumberFormat
    dateFormat   DateFormat
}

type NumberFormat struct {
    DecimalSeparator  string
    ThousandSeparator string
    CurrencySymbol    string
    CurrencyPosition  string // "prefix" or "suffix"
}

type DateFormat struct {
    ShortDate string
    LongDate  string
    ShortTime string
    LongTime  string
}

func NewLocaleManager(locale string) *LocaleManager {
    location, _ := time.LoadLocation(getTimezoneForLocale(locale))
    
    return &LocaleManager{
        locale:       locale,
        location:     location,
        numberFormat: getNumberFormat(locale),
        dateFormat:   getDateFormat(locale),
    }
}

func getNumberFormat(locale string) NumberFormat {
    formats := map[string]NumberFormat{
        "en-US": {
            DecimalSeparator:  ".",
            ThousandSeparator: ",",
            CurrencySymbol:    "$",
            CurrencyPosition:  "prefix",
        },
        "de-DE": {
            DecimalSeparator:  ",",
            ThousandSeparator: ".",
            CurrencySymbol:    "€",
            CurrencyPosition:  "suffix",
        },
        "ja-JP": {
            DecimalSeparator:  ".",
            ThousandSeparator: ",",
            CurrencySymbol:    "¥",
            CurrencyPosition:  "prefix",
        },
    }
    
    if format, exists := formats[locale]; exists {
        return format
    }
    
    return formats["en-US"] // 預設
}

func (lm *LocaleManager) FormatDate(t time.Time, format string) string {
    localTime := t.In(lm.location)
    
    switch format {
    case "short":
        return localTime.Format(lm.dateFormat.ShortDate)
    case "long":
        return localTime.Format(lm.dateFormat.LongDate)
    default:
        return localTime.Format(format)
    }
}

func (lm *LocaleManager) FormatNumber(value float64) string {
    str := fmt.Sprintf("%.2f", value)
    
    // 替換小數點
    str = strings.Replace(str, ".", lm.numberFormat.DecimalSeparator, 1)
    
    // 添加千位分隔符
    parts := strings.Split(str, lm.numberFormat.DecimalSeparator)
    intPart := parts[0]
    
    var result []string
    for i, r := range intPart {
        if i > 0 && (len(intPart)-i)%3 == 0 {
            result = append(result, lm.numberFormat.ThousandSeparator)
        }
        result = append(result, string(r))
    }
    
    formatted := strings.Join(result, "")
    if len(parts) > 1 {
        formatted += lm.numberFormat.DecimalSeparator + parts[1]
    }
    
    return formatted
}
```

## 8. CSV 輸出

### 8.1 CSV 格式化器

```go
type CSVFormatter struct {
    delimiter rune
    headers   bool
    quote     bool
}

func NewCSVFormatter() *CSVFormatter {
    return &CSVFormatter{
        delimiter: ',',
        headers:   true,
        quote:     true,
    }
}

func (cf *CSVFormatter) Format(data [][]string, headers []string) ([]byte, error) {
    var buf bytes.Buffer
    writer := csv.NewWriter(&buf)
    writer.Comma = cf.delimiter
    
    // 寫入標題
    if cf.headers && len(headers) > 0 {
        if err := writer.Write(headers); err != nil {
            return nil, err
        }
    }
    
    // 寫入數據
    for _, row := range data {
        if err := writer.Write(row); err != nil {
            return nil, err
        }
    }
    
    writer.Flush()
    if err := writer.Error(); err != nil {
        return nil, err
    }
    
    return buf.Bytes(), nil
}

func (cf *CSVFormatter) FormatReports(reports interface{}) ([]byte, error) {
    // 使用反射提取欄位
    headers, rows := cf.extractTableData(reports)
    return cf.Format(rows, headers)
}
```

## 9. 格式化管道

### 9.1 格式化管道

```go
type FormatterPipeline struct {
    formatters []Formatter
}

type Formatter interface {
    Format(data interface{}) (interface{}, error)
}

func NewFormatterPipeline() *FormatterPipeline {
    return &FormatterPipeline{
        formatters: []Formatter{},
    }
}

func (fp *FormatterPipeline) Add(formatter Formatter) *FormatterPipeline {
    fp.formatters = append(fp.formatters, formatter)
    return fp
}

func (fp *FormatterPipeline) Execute(data interface{}) (interface{}, error) {
    result := data
    
    for _, formatter := range fp.formatters {
        formatted, err := formatter.Format(result)
        if err != nil {
            return nil, fmt.Errorf("formatter pipeline error: %w", err)
        }
        result = formatted
    }
    
    return result, nil
}

// 預設管道配置
func DefaultPipeline() *FormatterPipeline {
    return NewFormatterPipeline().
        Add(&DataValidator{}).
        Add(&DataTransformer{}).
        Add(&DataFormatter{})
}
```

## 10. 效能優化

### 10.1 輸出緩衝

```go
type BufferedOutput struct {
    buffer    *bytes.Buffer
    writer    io.Writer
    flushSize int
    mutex     sync.Mutex
}

func NewBufferedOutput(writer io.Writer) *BufferedOutput {
    return &BufferedOutput{
        buffer:    bytes.NewBuffer(nil),
        writer:    writer,
        flushSize: 4096,
    }
}

func (bo *BufferedOutput) Write(p []byte) (n int, err error) {
    bo.mutex.Lock()
    defer bo.mutex.Unlock()
    
    n, err = bo.buffer.Write(p)
    
    if bo.buffer.Len() >= bo.flushSize {
        return n, bo.flush()
    }
    
    return n, err
}

func (bo *BufferedOutput) flush() error {
    if bo.buffer.Len() == 0 {
        return nil
    }
    
    _, err := bo.writer.Write(bo.buffer.Bytes())
    bo.buffer.Reset()
    return err
}

func (bo *BufferedOutput) Flush() error {
    bo.mutex.Lock()
    defer bo.mutex.Unlock()
    
    return bo.flush()
}
```

## 11. 測試策略

### 11.1 單元測試

```go
func TestTableBuilder(t *testing.T) {
    builder := NewTableBuilder()
    table := builder.
        SetHeaders("Date", "Tokens", "Cost").
        AddRow("2024-01-01", "1000", "$10.00").
        AddRow("2024-01-02", "2000", "$20.00").
        Build()
    
    var buf bytes.Buffer
    err := table.Render(&buf)
    
    assert.NoError(t, err)
    assert.Contains(t, buf.String(), "Date")
    assert.Contains(t, buf.String(), "2024-01-01")
}

func TestJSONFormatter(t *testing.T) {
    formatter := NewJSONFormatter()
    
    data := map[string]interface{}{
        "date":   "2024-01-01",
        "tokens": 1000,
        "cost":   10.5,
    }
    
    result, err := formatter.Format(data)
    
    assert.NoError(t, err)
    assert.Contains(t, string(result), `"date": "2024-01-01"`)
    assert.Contains(t, string(result), `"tokens": 1000`)
}
```

## 12. 與 TypeScript 版本對照

| TypeScript 函數 | Go 函數 | 說明 |
|----------------|---------|------|
| ResponsiveTable | ResponsiveTable | 響應式表格 |
| formatCurrency() | FormatCurrency() | 貨幣格式化 |
| formatNumber() | FormatNumber() | 數字格式化 |
| formatModelsDisplayMultiline() | FormatModels() | 模型列表格式化 |
| getTerminalWidth() | GetTerminalWidth() | 獲取終端寬度 |