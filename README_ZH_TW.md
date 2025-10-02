# ccusage_go

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

`ccusage_go` 是 [@ryoppippi](https://github.com/ryoppippi) 開發的熱門工具 [ccusage](https://github.com/ryoppippi/ccusage) 的 Go 語言實作版本。此版本保持與原始 TypeScript 版本的相容性，同時提供顯著的效能改進和更低的記憶體使用量。

## 為什麼選擇 Go 版本？

### 🎯 效能優勢（實測數據）

#### blocks --live 即時監控模式效能比較

| 指標 | ccusage (TypeScript) | ccusage_go | 改善幅度 |
|------|---------------------|------------|----------|
| **尖峰記憶體使用量** | ~446 MB | ~46 MB | **減少 90%** |
| **尖峰 CPU 使用率** | 40.0% | 142% (僅啟動時) | 見註解† |
| **穩定狀態記憶體** | ~263 MB | ~45 MB | **減少 83%** |
| **程序數量** | 3 個 (script+npm+node) | 2 個 (script+執行檔) | 更簡單 |
| **啟動記憶體** | ~240 MB (Node.js) | ~10 MB | **減少 96%** |
| **下載大小** | ~1 MB* | **3.5-4 MB** 壓縮檔 | 見下方說明 |
| **需要執行環境** | Node.js (~100MB) | 無（單一執行檔） | **無需執行環境** |

*實測環境：macOS, Apple Silicon, 監控 10+ 個專案，5 秒暖機後測量 15 秒*
†CPU：Go 版本在初始載入檔案時有較高尖峰，但監控期間降至 <10%

**關於下載大小的說明**：雖然 ccusage npm 套件只有 ~1 MB，但需要預先安裝 Node.js 執行環境（~100 MB）。ccusage_go 執行檔壓縮後為 3.5-4 MB，完全獨立運作，不需要任何執行環境或相依套件。

**效能測試方法**：以上測量數據使用我們的監控腳本（`docs/monitor_ccusage.sh`）取得，該腳本會追蹤包括 Node.js 執行環境在內的所有子程序。您可以使用以下指令重現這些測試：
```bash
# 監控 ccusage (TypeScript 版本)
./docs/monitor_ccusage.sh

# 監控 ccusage_go
./docs/monitor_ccusage.sh ccusage_go
```

#### 其他效能指標

| 指標 | TypeScript 版本 | Go 版本 | 說明 |
|------|----------------|---------|------|
| 啟動時間 | ~300ms | ~50ms | **快 6 倍** |
| 資源占用 | Node 進程 + npm 套件 | 單一執行檔 | 更簡潔 |
| 系統影響 | 中等 | 極低 | 幾乎不影響系統 |

### 📦 發布優勢

- **超級精簡**：僅 **3.5-4 MB** 下載大小（壓縮後）
- **單一執行檔**：~10 MB 執行檔，無需任何執行環境
- **零依賴**：不需要 Node.js、npm 或任何其他相依套件
- **即時啟動**：直接執行，無需安裝過程
- **跨平台支援**：為所有主要平台提供原生執行檔

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

從 [GitHub Releases](https://github.com/SDpower/ccusage_go/releases) 下載

#### 快速安裝 (macOS/Linux)

```bash
# macOS Apple Silicon
curl -L https://github.com/SDpower/ccusage_go/releases/download/v0.9.0/ccusage_go-darwin-arm64.tar.gz | tar xz
sudo mv ccusage_go-darwin-arm64 /usr/local/bin/ccusage_go

# macOS Intel
curl -L https://github.com/SDpower/ccusage_go/releases/download/v0.9.0/ccusage_go-darwin-amd64.tar.gz | tar xz
sudo mv ccusage_go-darwin-amd64 /usr/local/bin/ccusage_go

# Linux x64
curl -L https://github.com/SDpower/ccusage_go/releases/download/v0.9.0/ccusage_go-linux-amd64.tar.gz | tar xz
sudo mv ccusage_go-linux-amd64 /usr/local/bin/ccusage_go
```

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

## 為什麼選擇 ccusage_go？

### 💾 儲存空間與執行環境比較

| 面向 | ccusage (TypeScript) | ccusage_go |
|------|---------------------|------------|
| **套件下載** | ~1 MB (npm 套件) | **3.5-4 MB** (壓縮的執行檔) |
| **執行環境需求** | Node.js (~100 MB) | **無** |
| **總儲存需求** | ~101 MB (Node.js + 套件) | **~10 MB** (單一執行檔) |
| **相依性** | npm 套件 + Node.js 執行環境 | **零相依性** |
| **更新方式** | npm update (需要網路) | 替換單一檔案 |

### 🚀 實際運作效能影響

| 場景 | ccusage (TypeScript) | ccusage_go |
|------|---------------------|------------|
| **全新安裝** | 安裝 Node.js + npm install | 下載即可執行 |
| **啟動時記憶體** | ~240 MB (Node.js 初始化) | ~10 MB |
| **尖峰記憶體** | ~419 MB | ~54 MB |
| **CPU 使用率 (live 模式)** | 120.3% (多核心) | 9.8% |
| **系統影響** | 明顯 | 極小 |

*雖然 ccusage 的 npm 套件較小（1 MB），但需要 Node.js 執行環境。ccusage_go 在單一 3.5-4 MB 下載中提供完整解決方案，運作時**記憶體使用量減少 87%**，**CPU 使用量減少 92%**。*

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
- **統一 Model 標籤**：支援最新的 Claude 模型格式（Opus-4, Sonnet-4, Opus-4.1, Sonnet-4.5）

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

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=SDpower/ccusage_go&type=Date)](https://star-history.com/#SDpower/ccusage_go&Date)

---

<p align="center">
  使用 Go 語言 ❤️ 打造
</p>