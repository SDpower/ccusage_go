# Changelog

All notable changes to this project will be documented in this file.

## [v0.12.0] - 2026-03-24

### ✨ Features

- **feat**: 新增 Session Name 顯示與查詢功能
  - 從 JSONL 中解析 `custom-title` 和 `agent-name` 條目取得 session name
  - `session` 命令新增 `--session-id` flag，依 UUID 精確查詢特定 session
  - `session` 命令新增 `--session-name` flag，依 session name 精確查詢
  - Session 報表優先顯示 session name（無 name 時回退為 project path）
  - 跨檔案全局回填 SessionName（subagent 檔案也能正確顯示）
- **feat**: Session 過濾模式新增 Source File 明細
  - `--session-id` / `--session-name` 查詢時，以 Session 為大區塊，逐行列出每個 Source File
  - 每個 Source File 獨立顯示 Models、Input、Output、Cache、Cost、Last Activity
  - 一般模式維持 Files 數字欄不變
- **feat**: Daily/Monthly 報表新增 Sessions 數量欄
  - 統計每個時間區間內的唯一 session 數量
  - Footer 顯示總計唯一 session 數量
- **feat**: Session 報表新增 Session IDs 和 Source Files 追蹤
  - SessionInfo 收集所有唯一 Session UUID 和 Source File 路徑
  - CSV 輸出新增 `session_name`、`session_ids`、`source_files` 欄位
- **feat**: Last Activity 欄顯示日期+時間（當地時區）
- **feat**: 所有報表新增 CC Cost / CR Cost / API Cost 獨立欄位
  - Cache Create Cost 和 Cache Read Cost 分別顯示
  - API Cost 只含 input + output 費用
  - 舊版資料無 cache 時欄位顯示 `-`
- **feat**: Blocks 報表新增 token 和費用明細
  - 從單一 Tokens + Cost 欄拆分為 Input / Output / Cache Create / CC Cost / Cache Read / CR Cost / Total Tokens / API Cost / Cost
  - Gap 行和 REMAINING/PROJECTED 特殊行同步更新

### 📁 Files Changed

- `internal/types/usage.go` — 新增 SessionName、SessionIDs、SourceFiles、SourceFile、SourceFileStat
- `internal/loader/loader.go` — 攔截 custom-title/agent-name、設定 SourceFile、全局 sessionNameMap 回填
- `internal/commands/session.go` — 新增 --session-id/--session-name flags、過濾模式調用 detail 報表
- `internal/commands/shared.go` — 新增 filterEntriesBySessionID/Name 輔助函式
- `internal/calculator/calculator.go` — GenerateSessionReport 收集 SessionName/SessionIDs/SourceFiles、新增 AggregateBySourceFile
- `internal/output/tablewriter_formatter.go` — FormatSessionDetailReport、Session name 顯示、Sessions 欄、Last Activity 含時間和時區
- `internal/output/formatter.go` — CSV 格式加入 session_name/session_ids/source_files
- `internal/types/usage.go` — UsageEntry、SessionInfo、SourceFileStat 新增 CacheCreateCost / CacheReadCost 欄位
- `internal/calculator/calculator.go` — 分別計算 cache create / cache read 費用
- `internal/output/tablewriter_formatter.go` — 報表新增 CC Cost (USD) / CR Cost (USD) / API Cost (USD) 欄位

### 🧪 Tests Added

- `internal/loader/session_name_test.go` — 6 tests (custom-title, agent-name, priority, empty, cross-file, source-file)
- `internal/calculator/session_report_test.go` — 6 tests (session name, multiple, empty, session IDs, source files, aggregate)
- `internal/commands/session_filter_test.go` — 5 tests (ID/name filter, no match, empty)
- `internal/output/session_count_test.go` — 7 tests (session name, session IDs, files column, detail report, CSV, daily/monthly sessions)

---

## [v0.11.1] - 2026-03-15

### ⚡ Performance

- **perf**: Add project-level incremental cache for `blocks --live` mode
  - Tracks file state (ModTime+Size) per project directory
  - Only reloads changed files; skips entirely when no changes detected
  - Per-project deduplication for efficient append-only JSONL handling
  - Full project reload on file deletion for cache consistency

### 📊 Performance Metrics

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| CPU avg (30s idle) | 26.7% | 8.5% | **-68%** |

### 📁 Files Changed

- `internal/loader/project_cache.go` — New incremental cache implementation
- `internal/loader/project_cache_test.go` — 9 unit tests
- `internal/monitor/blocks_live.go` — Integrated incremental cache into tick handler

---

## [v0.11.0] - 2026-03-11

### ✨ Features

- **feat**: Add LIMITS section to `blocks --live` showing Claude API usage quota in real-time
  - Displays session (5-hour) and weekly limits with color-coded progress bars
  - Green (≤60%), yellow (60-90%), red (>90%) indicators
  - Shows reset times in local timezone
  - Graceful degradation when no OAuth token or API unavailable
- **feat**: Add `internal/usage` package for Claude OAuth Usage API integration
  - Cross-platform OAuth token reading (env var, credentials file, macOS Keychain)
  - 5-minute in-memory cache with concurrent access protection

### 🐛 Bug Fixes

- **fix**: Fix model name display for dateless model IDs
  - `claude-opus-4-6` now correctly shows as `Opus-4.6` (was `claude-opus-`)
  - `claude-sonnet-4-6` now correctly shows as `Sonnet-4.6` (was `claude-sonne`)
  - Tightened date regex to require 8-digit dates, preventing false matches

### 📚 Documentation

- Updated README and README_ZH_TW for v0.11.0
- Updated blocks-live implementation docs with LIMITS section
- Updated API integration docs with Claude OAuth Usage API
- Updated live monitor screenshot

---

## [v0.10.1] - 2025-10-16

### ✨ Features

- **feat**: Add support for Haiku 4.5 model
- **feat**: Update pricing for Sonnet 4.5

### 🎨 Style

- **style**: Standardize model display names to use hyphens (e.g., `Sonnet-4.5`)



## [v0.9.0] - 2024-08-24

### 🎉 Major Performance Improvements

#### Memory Optimization
- **90% reduction** in peak memory usage during live monitoring (446MB → 46MB)
- **83% reduction** in steady-state memory usage (263MB → 45MB)
- Implemented single-worker architecture for reduced resource consumption
- Stream processing with immediate memory release after cost calculation
- Smart retention of only essential cache token data

#### Build System Overhaul
- Default static binary compilation with `CGO_ENABLED=0`
- Binary size optimization with `-ldflags="-s -w"` (14MB → 9.6MB)
- Unified naming convention: all binaries now use `ccusage_go` prefix
- Simplified deployment with zero runtime dependencies

### 🔧 Technical Improvements

#### Core Optimizations
- Changed default `maxWorkers` from 5 to 1 for lower CPU usage
- Implemented progressive memory release during file processing
- Fixed token accumulation in live mode with proper cache token handling
- Aligned with TypeScript version's 24-hour retention window

#### Build and CI/CD
- Updated Makefile with static compilation as default
- Enhanced GitHub Actions workflows for new naming convention
- Added `make dynamic` target for optional dynamic linking
- Improved release automation with proper binary naming

### 📊 Performance Metrics

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| Peak Memory | 446 MB | 46 MB | **-90%** |
| Steady Memory | 263 MB | 45 MB | **-83%** |
| Binary Size | 14 MB | 9.6 MB | **-31%** |
| Process Count | 3 | 2 | **-33%** |

### 🐛 Bug Fixes
- Fixed memory leak in stream processing mode
- Corrected cache token accumulation in `IdentifySessionBlocks`
- Fixed process statistics in monitoring script for npm processes
- Resolved Raw data clearing issue that affected cost calculations

### 📚 Documentation
- Updated performance comparison tables with latest measurements
- Added detailed testing methodology documentation
- Enhanced installation instructions for v0.9.0
- Added monitoring script usage examples

### 🔄 Breaking Changes
- Binary output name changed from `ccusage` to `ccusage_go`
- Release artifacts now use underscore naming (e.g., `ccusage_go-linux-amd64`)

### 📦 Dependencies
- No changes to external dependencies
- Maintained compatibility with Go 1.22+

---

## [v0.8.0] - Previous Release

Initial public release with core functionality:
- Daily, monthly, weekly, and session reports
- 5-hour billing blocks tracking
- Live monitoring mode with gradient progress bars
- JSON/CSV/table output formats
- Cross-platform support