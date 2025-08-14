# ccusage-go

<p align="center">
  <strong>🚀 高效能的 Claude Code 使用量分析工具 Go 實作版</strong>
</p>

<p align="center">
  <a href="#安裝">安裝</a> •
  <a href="#使用方法">使用方法</a> •
  <a href="#功能特色">功能特色</a> •
  <a href="#功能比較">功能比較</a> •
  <a href="README.md">English</a>
</p>

---

![即時 Token 使用量監控](docs/images/blocks-live-monitor.png)
*即時 token 使用量監控與漸變進度條*

## 關於專案

`ccusage-go` 是 [@ryoppippi](https://github.com/ryoppippi) 開發的熱門工具 [ccusage](https://github.com/ryoppippi/ccusage) 的 Go 語言實作版本。此版本保持與原始 TypeScript 版本的相容性，同時提供顯著的效能改進和更低的記憶體使用量。

## 為什麼選擇 Go 版本？

### 🎯 效能優勢（實測數據）

#### blocks --live 即時監控模式效能比較

| 指標 | TypeScript 版本 | Go 版本 | 改善幅度 |
|------|----------------|---------|----------|
| **記憶體使用量** | 414,944 KB (~405 MB) | 55,248 KB (~54 MB) | **減少 87%** |
| **記憶體百分比** | 1.6% | 0.2% | **降低 87.5%** |
| **CPU 使用率** | 120.6% | 9.8% | **降低 92%** |
| **執行檔大小** | 需要 Node.js (~100MB) | 單一執行檔 (~25MB) | **小 75%** |

*實測環境：macOS, Apple Silicon, 監控 10+ 個專案*

#### 其他效能指標

| 指標 | TypeScript 版本 | Go 版本 | 說明 |
|------|----------------|---------|------|
| 啟動時間 | ~300ms | ~50ms | **快 6 倍** |
| 資源占用 | Node 進程 + npm 套件 | 單一執行檔 | 更簡潔 |
| 系統影響 | 中等 | 極低 | 幾乎不影響系統 |

### 📦 發布優勢

- **單一執行檔**：無需 Node.js 執行環境
- **跨平台**：輕鬆編譯為 Windows、macOS、Linux 版本
- **零依賴**：開箱即用

## 安裝

### 從原始碼編譯

```bash
# 複製儲存庫
git clone https://github.com/SDpower/ccusage_go.git
cd ccusage_go

# 建置
make build

# 安裝到系統
make install
```

### 預編譯版本

*即將推出*

## 使用方法

### 基本指令

```bash
# 每日使用報告
./ccusage_go daily

# 月度總結
./ccusage_go monthly

# 依對話分析
./ccusage_go session

# 5 小時計費區塊
./ccusage_go blocks

# 即時監控（含漸變進度條！）
./ccusage_go blocks --live
```

### 進階選項

```bash
# 依日期範圍過濾
./ccusage_go daily --since 2025-01-01 --until 2025-01-31

# 不同輸出格式
./ccusage_go monthly --format json
./ccusage_go session --format csv

# 自訂時區
./ccusage_go daily --timezone Asia/Taipei

# 只顯示最近活動
./ccusage_go blocks --recent
```

## 功能特色

### ✅ 已實作功能

- 📊 **每日報告**：每天的 token 使用量和成本
- 📈 **月度報告**：彙總的月度統計資料  
- 💬 **對話分析**：依對話階段的使用量
- ⏱️ **計費區塊**：5 小時計費視窗追蹤
- 🔴 **即時監控**：具有漸變進度條的即時使用量儀表板
- 🎨 **多種輸出格式**：表格（預設）、JSON、CSV
- 🌍 **時區支援**：可配置報告時區
- 💾 **離線模式**：無需網路連線即可運作
- 🚀 **並行處理**：使用 goroutines 快速載入資料
- 🎯 **記憶體效率**：串流式 JSONL 處理

### 🎨 視覺增強（Go 版獨有）

- **漸變進度條**：在 LUV 色彩空間中平滑的顏色過渡
- **增強的 TUI**：使用 Bubble Tea 框架建置
- **效能快取**：透過顏色快取優化渲染
- **"WITH GO" 標記**：所有報告都清楚標示為 Go 版本

## 功能比較

| 功能 | TypeScript 版本 | Go 版本 | 狀態 |
|------|----------------|---------|------|
| `daily` 指令 | ✅ | ✅ | 完成 |
| `monthly` 指令 | ✅ | ✅ | 完成 |
| `weekly` 指令 | ✅ | ✅ | 完成 |
| `session` 指令 | ✅ | ✅ | 完成 |
| `blocks` 指令 | ✅ | ✅ | 完成 |
| `blocks --live` | ✅ | ✅ | 增強漸變效果 |
| `monitor` 指令 | ✅ | ✅ | 完成 |
| `statusline` (Beta) | ✅ | ❌ | 未實作 |
| JSON 輸出 | ✅ | ✅ | 完成 |
| CSV 輸出 | ✅ | ✅ | 完成 |
| `--project` 過濾 | ✅ | ❌ | 未實作 |
| `--instances` 分組 | ✅ | ❌ | 未實作 |
| `--locale` 選項 | ✅ | ❌ | 未實作 |
| MCP 整合 | ✅ | 🚧 | 部分完成 |
| 離線模式 | ✅ | ✅ | 完成 |

## 技術堆疊

- **程式語言**：Go 1.23+
- **CLI 框架**：[Cobra](https://github.com/spf13/cobra)
- **TUI 框架**：[Bubble Tea](https://github.com/charmbracelet/bubbletea)
- **表格渲染**：[tablewriter](https://github.com/olekukonko/tablewriter)
- **樣式處理**：[Lip Gloss](https://github.com/charmbracelet/lipgloss)
- **顏色漸變**：[go-colorful](https://github.com/lucasb-eyer/go-colorful)

## 開發

### 先決條件

- Go 1.23 或更高版本
- Make（選擇性，為了方便）

### 建置

```bash
# 基本建置
make build

# 為所有平台建置
make build-all

# 執行測試
make test

# 啟用效能分析執行
ENABLE_PROFILING=1 go test -v ./...
```

### 專案結構

```
ccusage_go/
├── cmd/ccusage/        # CLI 進入點
├── internal/           # 核心實作
│   ├── calculator/     # 成本計算邏輯
│   ├── commands/       # CLI 指令處理器
│   ├── loader/         # 資料載入和解析
│   ├── monitor/        # 即時監控功能
│   ├── output/         # 格式化和顯示
│   ├── pricing/        # 價格取得和快取
│   └── types/          # 類型定義
├── docs/               # 文件
└── test_data/          # 測試資料
```

## 效能建議

1. **大型資料集**：Go 版本使用串流和並行處理以達到最佳效能
2. **記憶體優化**：實作智慧型檔案過濾，只載入 12 小時內活動的專案
3. **即時監控**：漸變計算已快取以確保流暢的即時更新
4. **資源使用**：blocks --live 模式僅使用 ~54MB 記憶體，對系統幾乎無影響

## 致謝

- 🙏 原始 [ccusage](https://github.com/ryoppippi/ccusage) 作者 [@ryoppippi](https://github.com/ryoppippi)
- 🎨 [Bubble Tea](https://github.com/charmbracelet/bubbletea) 提供的精美 TUI 框架
- 💙 所有貢獻者和使用者

## 授權

[MIT](LICENSE) © [@SteveLuo](https://github.com/sdpower)

## 貢獻

歡迎貢獻！請隨時提交 Pull Request。

## 開發路線圖

- [ ] 實作剩餘的 TypeScript 功能
- [ ] 為主要平台新增預編譯版本
- [ ] 增強 MCP 整合
- [ ] 新增更多自訂選項
- [ ] 實作 `--project` 和 `--instances` 過濾器
- [ ] 新增國際化支援

---

<p align="center">
  使用 Go 語言 ❤️ 打造
</p>