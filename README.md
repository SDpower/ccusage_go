# ccusage-go

<p align="center">
  <strong>🚀 A high-performance Go implementation of Claude Code usage analyzer</strong>
</p>

<p align="center">
  <a href="#installation">Installation</a> •
  <a href="#usage">Usage</a> •
  <a href="#features">Features</a> •
  <a href="#comparison">Comparison</a> •
  <a href="README_ZH_TW.md">繁體中文</a>
</p>

---

![Live Token Usage Monitor](docs/images/blocks-live-monitor.png)
*Real-time token usage monitoring with gradient progress bars*

## About

`ccusage-go` is a Go implementation of the popular [ccusage](https://github.com/ryoppippi/ccusage) tool by [@ryoppippi](https://github.com/ryoppippi). This version maintains compatibility with the original TypeScript version while offering significant performance improvements and reduced memory footprint.

## Why Go Version?

### 🎯 Performance Benefits (Real-world Measurements)

#### blocks --live Real-time Monitoring Performance

| Metric | TypeScript Version | Go Version | Improvement |
|--------|-------------------|------------|-------------|
| **Memory Usage** | 414,944 KB (~405 MB) | 55,248 KB (~54 MB) | **87% reduction** |
| **Memory Percentage** | 1.6% | 0.2% | **87.5% lower** |
| **CPU Usage** | 120.6% | 9.8% | **92% reduction** |
| **Executable Size** | Requires Node.js (~100MB) | Single binary (~25MB) | **75% smaller** |

*Test environment: macOS, Apple Silicon, monitoring 10+ projects*

#### Other Performance Metrics

| Metric | TypeScript Version | Go Version | Notes |
|--------|-------------------|------------|-------|
| Startup Time | ~300ms | ~50ms | **6x faster** |
| Resource Footprint | Node process + npm packages | Single binary | Cleaner |
| System Impact | Moderate | Minimal | Almost no system impact |

### 📦 Distribution Advantages

- **Single Binary**: No Node.js runtime required
- **Cross-Platform**: Easy compilation for Windows, macOS, Linux
- **Zero Dependencies**: Works out of the box

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/SDpower/ccusage_go.git
cd ccusage_go

# Build
make build

# Install to system
make install
```

### Pre-built Binaries

*Coming soon*

## Usage

### Basic Commands

```bash
# Daily usage report
./ccusage_go daily

# Monthly summary
./ccusage_go monthly

# Session-based analysis
./ccusage_go session

# 5-hour billing blocks
./ccusage_go blocks

# Live monitoring (with gradient progress bars!)
./ccusage_go blocks --live
```

### Advanced Options

```bash
# Filter by date range
./ccusage_go daily --since 2025-01-01 --until 2025-01-31

# Different output formats
./ccusage_go monthly --format json
./ccusage_go session --format csv

# Custom timezone
./ccusage_go daily --timezone America/New_York

# Show only recent activity
./ccusage_go blocks --recent
```

## Features

### ✅ Implemented Features

- 📊 **Daily Reports**: Token usage and costs per day
- 📈 **Monthly Reports**: Aggregated monthly statistics  
- 💬 **Session Analysis**: Usage by conversation session
- ⏱️ **Billing Blocks**: 5-hour billing window tracking
- 🔴 **Live Monitoring**: Real-time usage dashboard with gradient progress bars
- 🎨 **Multiple Output Formats**: Table (default), JSON, CSV
- 🌍 **Timezone Support**: Configurable timezone for reports
- 💾 **Offline Mode**: Works without internet connection
- 🚀 **Parallel Processing**: Fast data loading with goroutines
- 🎯 **Memory Efficient**: Streaming JSONL processing

### 🎨 Visual Enhancements (Go Exclusive)

- **Gradient Progress Bars**: Smooth color transitions in LUV color space
- **Enhanced TUI**: Built with Bubble Tea framework
- **Performance Caching**: Optimized rendering with color caching
- **"WITH GO" Branding**: All reports clearly marked as Go version

## Feature Comparison

| Feature | TypeScript Version | Go Version | Status |
|---------|-------------------|------------|--------|
| `daily` command | ✅ | ✅ | Complete |
| `monthly` command | ✅ | ✅ | Complete |
| `weekly` command | ✅ | ✅ | Complete |
| `session` command | ✅ | ✅ | Complete |
| `blocks` command | ✅ | ✅ | Complete |
| `blocks --live` | ✅ | ✅ | Enhanced with gradients |
| `monitor` command | ✅ | ✅ | Complete |
| `statusline` (Beta) | ✅ | ❌ | Not implemented |
| JSON output | ✅ | ✅ | Complete |
| CSV output | ✅ | ✅ | Complete |
| `--project` filter | ✅ | ❌ | Not implemented |
| `--instances` grouping | ✅ | ❌ | Not implemented |
| `--locale` option | ✅ | ❌ | Not implemented |
| MCP integration | ✅ | 🚧 | Partial |
| Offline mode | ✅ | ✅ | Complete |

## Technical Stack

- **Language**: Go 1.23+
- **CLI Framework**: [Cobra](https://github.com/spf13/cobra)
- **TUI Framework**: [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- **Table Rendering**: [tablewriter](https://github.com/olekukonko/tablewriter)
- **Styling**: [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- **Color Gradients**: [go-colorful](https://github.com/lucasb-eyer/go-colorful)

## Development

### Prerequisites

- Go 1.23 or higher
- Make (optional, for convenience)

### Building

```bash
# Basic build
make build

# Build for all platforms
make build-all

# Run tests
make test

# Run with profiling
ENABLE_PROFILING=1 go test -v ./...
```

### Project Structure

```
ccusage_go/
├── cmd/ccusage/        # CLI entry point
├── internal/           # Core implementation
│   ├── calculator/     # Cost calculation logic
│   ├── commands/       # CLI command handlers
│   ├── loader/         # Data loading and parsing
│   ├── monitor/        # Live monitoring features
│   ├── output/         # Formatting and display
│   ├── pricing/        # Price fetching and caching
│   └── types/          # Type definitions
├── docs/               # Documentation
└── test_data/          # Test fixtures
```

## Performance Tips

1. **Large Datasets**: The Go version uses streaming and parallel processing for optimal performance
2. **Memory Optimization**: Implements smart file filtering, only loading projects active within 12 hours
3. **Live Monitoring**: Gradient calculations are cached for smooth real-time updates
4. **Resource Usage**: blocks --live mode uses only ~54MB memory with minimal system impact

## Acknowledgments

- 🙏 Original [ccusage](https://github.com/ryoppippi/ccusage) by [@ryoppippi](https://github.com/ryoppippi)
- 🎨 [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the beautiful TUI framework
- 💙 All contributors and users

## License

[MIT](LICENSE) © [@SteveLuo](https://github.com/sdpower)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Roadmap

- [ ] Implement remaining TypeScript features
- [ ] Add pre-built binaries for major platforms
- [ ] Enhance MCP integration
- [ ] Add more customization options
- [ ] Implement `--project` and `--instances` filters
- [ ] Add internationalization support

---

<p align="center">
  Made with ❤️ in Go
</p>