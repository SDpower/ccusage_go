# blocks --live 模式實作文件

## 概述

實作了 `blocks --live` 即時監控模式，使用 Bubble Tea TUI 框架提供即時更新的 token 使用狀態顯示。

## 主要功能

### 1. 即時監控
- 每秒自動更新（可配置 1-60 秒）
- 顯示當前活動的 session block
- 實時計算 burn rate 和預測

### 2. 視覺設計

#### 標題
- 文字：`CLAUDE CODE - LIVE TOKEN USAGE MONITOR (WITH GO)`
- 置中顯示（使用 Header 配置）

#### 主要區段
1. **SESSION**
   - 圖標：⏱️
   - 顯示開始時間、已用時間、剩餘時間
   - 進度條顯示 session 進度（cyan 色）

2. **USAGE**
   - 圖標：🔥
   - 顯示當前 token 使用量、burn rate、成本
   - 進度條顯示使用率（綠/黃/紅色根據百分比）
   - Burn rate 指標（NORMAL/MODERATE/HIGH）

3. **PROJECTION**
   - 圖標：📈
   - 預測 session 結束時的總使用量
   - 狀態指標（WITHIN LIMIT/APPROACHING LIMIT/EXCEEDS LIMIT）

4. **Models**
   - 圖標：⚙️
   - 顯示使用的模型列表

#### Footer
- 顯示刷新頻率和退出提示
- 置中顯示（使用 Footer 配置）

### 3. 自適應寬度
- 最小寬度：95 字符
- 最大寬度：120 字符
- 超過最大寬度時，整個表格在終端中置中

## 技術實作

### 使用的套件
- `github.com/charmbracelet/bubbletea` - TUI 框架
- `github.com/charmbracelet/lipgloss` - 樣式處理
- `github.com/olekukonko/tablewriter` - 表格繪製

### 表格配置
```go
tablewriter.WithConfig(tablewriter.Config{
    Header: tw.CellConfig{
        Alignment: tw.CellAlignment{Global: tw.AlignCenter}, // 標題置中
    },
    Row: tw.CellConfig{
        Alignment: tw.CellAlignment{Global: tw.AlignLeft}, // 內容左對齊
    },
    Footer: tw.CellConfig{
        Alignment: tw.CellAlignment{Global: tw.AlignCenter}, // Footer 置中
    },
})
```

### 進度條實作
- 使用 █ 字符表示已填充部分
- 使用 ░ 字符表示未填充部分
- 根據百分比動態計算填充長度
- 支援多種顏色（cyan、green、yellow、red）

### Burn Rate 計算
```go
const (
    BurnRateHigh     = 1000 // tokens per minute
    BurnRateModerate = 500  // tokens per minute
)
```

## 使用方式

### 基本使用
```bash
./ccusage_go blocks --live
```

### 參數選項
- `--refresh-interval`: 刷新間隔（1-60 秒，預設 1 秒）
- `--token-limit`: Token 限制（用於計算使用率）
- `--no-color`: 禁用顏色輸出

## 與 TypeScript 版本的一致性

- ✅ 相同的視覺佈局
- ✅ 相同的進度條樣式
- ✅ 相同的顏色方案
- ✅ 相同的 emoji 圖標
- ✅ 相同的資訊顯示
- ✅ 相同的刷新行為

## 改進點

1. **使用 tablewriter 套件功能**
   - 利用 Header/Footer 配置實現置中
   - 自動處理表格寬度和對齊

2. **視覺優化**
   - 在區段內容上下增加空白行提升可讀性
   - 動態調整進度條寬度

3. **終端適應**
   - 自動偵測終端寬度
   - 在超寬終端中置中顯示

## 注意事項

- 需要 TTY 環境才能執行 --live 模式
- 使用 Ctrl+C 或 q 鍵退出
- 自動處理非 TTY 環境的錯誤