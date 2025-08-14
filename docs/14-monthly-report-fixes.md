# Monthly 報表修正文檔

## 修正概述

本次修正解決了 monthly 命令與 daily 命令在時區處理、表格格式、模型去重複等方面的不一致問題，確保兩個命令的輸出格式和行為完全一致。

## 修正項目

### 1. 時區處理統一

**問題**：monthly 命令缺乏時區支援，無法正確處理不同時區的資料分組。

**解決方案**：
- 在 `internal/commands/monthly.go` 中添加 `--timezone` 參數
- 在資料載入前設置時區：`dataLoader.SetTimezone(loc)`
- 使用與 daily 命令相同的時區處理邏輯

**程式碼變更**：
```go
// 在 monthly.go 中添加時區參數
var timezone string
cmd.Flags().StringVarP(&timezone, "timezone", "z", "", "Timezone for date grouping")

// 載入時區並設置到資料載入器
var loc *time.Location
if timezone != "" {
    loc, err = time.LoadLocation(timezone)
    if err != nil {
        return fmt.Errorf("invalid timezone %s: %w", timezone, err)
    }
} else {
    loc = time.Local
}
dataLoader.SetTimezone(loc) // 在資料載入前設置時區
```

### 2. 表格格式一致性

**問題**：monthly 表格的邊框顏色不統一，與 daily 表格格式不匹配。

**解決方案**：
- 實現完整的顏色處理邏輯，與 daily 格式保持一致
- 使用相同的 ANSI 顏色代碼：
  - 灰色 (`\033[90m`) 用於邊框
  - 青色 (`\033[36m`) 用於標題
  - 黃色 (`\033[33m`) 用於總計行

**程式碼變更**：
```go
// 在 FormatMonthlyReportWithFilter 中添加完整的顏色處理
if !f.noColor {
    gray := "\033[90m"     // Gray color for borders
    cyan := "\033[36m"     // Cyan color for headers
    yellow := "\033[33m"   // Yellow color for Total row
    reset := "\033[0m"     // Reset color
    
    // 實現與 daily 相同的顏色渲染邏輯
}
```

### 3. 模型去重複處理

**問題**：MODELS 欄位顯示重複的模型名稱（如 `opus-4` 出現兩次），即使應該去重複。

**解決方案**：
- 使用與 daily 報表相同的模型處理邏輯
- 實現 `shortenModelName` 方法進行模型名稱簡化
- 使用 map 進行去重複處理

**程式碼變更**：
```go
// 修正模型處理邏輯
for _, entry := range monthEntries {
    // 跳過合成模型，但仍計算其代幣和成本
    if entry.Model != "" && entry.Model != "<synthetic>" {
        modelMap[entry.Model] = true
    }
}

// 格式化模型列表（與 daily 格式相同的邏輯）
simplifiedModels := make(map[string]bool)
for model := range modelMap {
    shortModel := f.shortenModelName(model)
    simplifiedModels[shortModel] = true
}

var models []string
for model := range simplifiedModels {
    models = append(models, model)
}
sort.Strings(models)
modelsStr := "- " + strings.Join(models, "\n- ")
```

### 4. 日期格式修正

**問題**：月份顯示格式錯誤，應該顯示為 `2025-07` 而不是分兩行顯示。

**解決方案**：
- 保持月份的原始 `YYYY-MM` 格式
- 移除不必要的換行符分割

**程式碼變更**：
```go
// 格式化月份為 YYYY-MM（保持原始格式）
formattedMonth := month
```

### 5. 變數名稱修正

**問題**：在月份處理迴圈中，變數名稱衝突導致邏輯錯誤。

**解決方案**：
- 將 `entries` 變更為 `monthEntries` 避免與外部變數衝突
- 確保正確處理每個月份的資料集

## 測試驗證

### 執行測試
```bash
# 編譯程式
go build -o ccusage_go ./cmd/ccusage

# 測試 monthly 命令
./ccusage_go monthly --timezone Asia/Taipei
```

### 驗證結果
- ✅ 月份格式正確顯示為 `2025-07`、`2025-08`
- ✅ MODELS 欄位正確去重複，不再顯示重複的 `opus-4`
- ✅ 表格邊框顏色統一為灰色
- ✅ 標題使用青色，總計行使用黃色
- ✅ 時區處理與 daily 命令一致

## 影響範圍

### 修改的檔案
1. `internal/commands/monthly.go` - 添加時區支援
2. `internal/output/tablewriter_formatter.go` - 修正表格格式和模型處理

### 功能影響
- Monthly 報表現在完全符合 daily 報表的格式標準
- 支援時區參數，確保資料分組的準確性
- 模型顯示去重複，提高報表的可讀性
- 顏色渲染一致，提供更好的視覺體驗

## 相容性

本次修正保持完全向後相容：
- 現有的 monthly 命令參數不變
- 新增的 `--timezone` 參數為可選
- 預設行為與修正前一致（使用系統時區）

## 後續維護

- 確保 daily 和 monthly 命令的任何格式變更都同步更新
- 定期驗證時區處理的準確性
- 監控模型名稱簡化邏輯的正確性