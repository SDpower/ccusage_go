# Changelog

All notable changes to this project will be documented in this file.

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