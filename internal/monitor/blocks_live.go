package monitor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/mattn/go-isatty"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/sdpower/ccusage-go/internal/calculator"
	"github.com/sdpower/ccusage-go/internal/loader"
	"github.com/sdpower/ccusage-go/internal/pricing"
	"github.com/sdpower/ccusage-go/internal/types"
)

// Burn rate thresholds for indicators
const (
	BurnRateHigh     = 1000 // tokens per minute
	BurnRateModerate = 500  // tokens per minute
)

// BlocksLiveConfig contains configuration for live monitoring
type BlocksLiveConfig struct {
	DataPath         string
	TokenLimit       int
	RefreshInterval  time.Duration
	SessionLength    int
	NoColor          bool
	Timezone         *time.Location
	UseGradient      bool  // Enable gradient progress bars
	OptimizeMemory   bool  // Enable memory optimization (only load recent data)
}

// BlocksLiveModel represents the state of the live monitor
type BlocksLiveModel struct {
	config        BlocksLiveConfig
	activeBlock   *types.SessionBlock
	lastUpdate    time.Time
	err           error
	width         int
	height        int
	quitting      bool
	loader        *loader.Loader
	calculator    *calculator.Calculator
	allEntries    []types.UsageEntry
	gradientCache map[string][]string // Cache for gradient colors
}

// blocksTickMsg is sent periodically to update the display
type blocksTickMsg time.Time

// Init initializes the model
func (m *BlocksLiveModel) Init() tea.Cmd {
	return tea.Batch(
		blocksTickCmd(m.config.RefreshInterval),
		tea.WindowSize(),
	)
}

// Update handles messages
func (m *BlocksLiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case blocksTickMsg:
		// Reload data and find active block
		ctx := context.Background()
		
		// Use optimized loading if enabled
		var entries []types.UsageEntry
		var err error
		if m.config.OptimizeMemory {
			// Only load recent data (last 24 hours) for live monitoring
			// Matches TypeScript version's RETENTION_HOURS = 24
			// Enable stream processing to calculate costs during loading
			options := &loader.LoaderOptions{
				OnlyActiveSession: true,
				ModifiedWithin:    24 * time.Hour,
				MaxFiles:          100, // Limit to most recent 100 files
				StreamProcessing:  true, // Calculate costs immediately after reading each file
				Calculator:        m.calculator, // Pass calculator for stream processing
			}
			entries, err = m.loader.LoadFromPathWithOptions(ctx, m.config.DataPath, options)
		} else {
			entries, err = m.loader.LoadFromPath(ctx, m.config.DataPath)
		}
		
		if err != nil {
			m.err = err
			return m, blocksTickCmd(m.config.RefreshInterval)
		}

		// Calculate costs only if stream processing was not used
		if !m.config.OptimizeMemory {
			entries, err = m.calculator.CalculateCosts(ctx, entries)
			if err != nil {
				m.err = err
				return m, blocksTickCmd(m.config.RefreshInterval)
			}
		}

		// Identify session blocks
		blocks := m.calculator.IdentifySessionBlocks(entries, m.config.SessionLength)
		
		// Find active block
		m.activeBlock = nil
		for i := range blocks {
			if blocks[i].IsActive {
				m.activeBlock = &blocks[i]
				break
			}
		}

		m.lastUpdate = time.Now()
		m.err = nil
		
		return m, blocksTickCmd(m.config.RefreshInterval)
	}

	return m, nil
}

// View renders the display
func (m *BlocksLiveModel) View() string {
	if m.quitting {
		return ""
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress 'q' to quit.", m.err)
	}

	if m.activeBlock == nil {
		waitingStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Bold(true)
		return waitingStyle.Render("No active session block found. Waiting...") + 
			"\n\nPress 'q' to quit."
	}

	// Render active block display
	return m.renderActiveBlock()
}

// renderActiveBlock renders the active block display
func (m *BlocksLiveModel) renderActiveBlock() string {
	block := m.activeBlock
	now := time.Now()

	// Calculate metrics
	totalTokens := block.TokenCounts.GetTotal()
	elapsed := now.Sub(block.StartTime)
	remaining := block.EndTime.Sub(now)
	sessionDuration := elapsed + remaining
	sessionPercent := float64(elapsed) / float64(sessionDuration) * 100
	
	// Calculate burn rate
	burnRate := calculator.CalculateBurnRate(*block)
	
	// Calculate projection
	projection := calculator.ProjectBlockUsage(*block)

	// Create a buffer for the table
	var buf bytes.Buffer
	
	// Create table with tablewriter v1.0.9 API
	table := tablewriter.NewTable(&buf,
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Settings: tw.Settings{
				Separators: tw.Separators{
					BetweenRows: tw.On,
				},
			},
		})),
		tablewriter.WithConfig(tablewriter.Config{
			Header: tw.CellConfig{
				Alignment: tw.CellAlignment{Global: tw.AlignCenter}, // 標題置中
			},
			Row: tw.CellConfig{
				Alignment: tw.CellAlignment{Global: tw.AlignLeft}, // 內容左對齊
				Padding: tw.CellPadding{
					Global: tw.Padding{
						Bottom: " ",   // 在儲存格下方增加一個空格
						Left:   " ",   // 左側保持一個空格
						Right:  " ",   // 右側保持一個空格
					},
				},
			},
			Footer: tw.CellConfig{
				Alignment: tw.CellAlignment{Global: tw.AlignCenter}, // Footer 置中
			},
		}),
		tablewriter.WithHeaderAutoFormat(tw.Off),
	)
	
	// Title row - use Header for center alignment
	title := "CLAUDE CODE - LIVE TOKEN USAGE MONITOR (WITH GO)"
	titleStyle := lipgloss.NewStyle().Bold(true)
	table.Header([]string{titleStyle.Render(title)})
	
	// SESSION section
	sessionLine := m.renderCompactSectionAsString(
		"⏱️", "SESSION", 
		sessionPercent,
		fmt.Sprintf("Started: %s  Elapsed: %s  Remaining: %s (%s)",
			block.StartTime.In(m.config.Timezone).Format("03:04:05 PM"),
			formatDuration(elapsed),
			formatDuration(remaining),
			block.EndTime.In(m.config.Timezone).Format("03:04:05 PM")),
		"cyan",
		fmt.Sprintf("%.1f%%", sessionPercent),
	)
	table.Append([]string{sessionLine})
	
	// USAGE section
	usagePercent := 0.0
	if m.config.TokenLimit > 0 {
		usagePercent = float64(totalTokens) / float64(m.config.TokenLimit) * 100
	}
	
	burnRateIndicator := ""
	burnRateValue := 0
	if burnRate != nil {
		burnRateValue = int(burnRate.TokensPerMinute)
		if burnRate.TokensPerMinuteForIndicator > BurnRateHigh {
			burnRateIndicator = " ⚡ HIGH"
		} else if burnRate.TokensPerMinuteForIndicator > BurnRateModerate {
			burnRateIndicator = " ⚡ MODERATE"
		} else {
			burnRateIndicator = " ✓ NORMAL"
		}
	}
	
	usageInfo := fmt.Sprintf("Tokens: %s (Burn Rate: %s token/min%s)  Limit: %s  Cost: $%.2f",
		formatNumberWithCommas(totalTokens),
		formatNumberWithCommas(burnRateValue),
		burnRateIndicator,
		formatNumberWithCommas(m.config.TokenLimit),
		block.CostUSD)
	
	usageRightText := fmt.Sprintf("%.1f%% (%s/%s)",
		usagePercent,
		formatTokensShort(totalTokens),
		formatTokensShort(m.config.TokenLimit))
	
	// Determine usage color
	usageColor := "green"
	if usagePercent > 80 {
		usageColor = "yellow"
	}
	if usagePercent > 95 {
		usageColor = "red"
	}
	
	usageLine := m.renderCompactSectionAsString(
		"🔥", "USAGE",
		usagePercent,
		usageInfo,
		usageColor,
		usageRightText,
	)
	table.Append([]string{usageLine})
	
	// PROJECTION section
	if projection != nil && m.config.TokenLimit > 0 {
		projPercent := float64(projection.TotalTokens) / float64(m.config.TokenLimit) * 100
		
		// Determine status
		var statusText string
		if projPercent > 100 {
			statusText = "🚨 EXCEEDS LIMIT"
		} else if projPercent > 90 {
			statusText = "⚠️  APPROACHING LIMIT"
		} else {
			statusText = "✅ WITHIN LIMIT"
		}
		
		projInfo := fmt.Sprintf("Status: %s  Tokens: %s  Cost: $%.2f",
			statusText,
			formatNumberWithCommas(projection.TotalTokens),
			projection.TotalCost)
		
		projRightText := fmt.Sprintf("%.1f%% (%s/%s)",
			projPercent,
			formatTokensShort(projection.TotalTokens),
			formatTokensShort(m.config.TokenLimit))
		
		// Determine projection color
		projColor := "green"
		if projPercent > 80 {
			projColor = "yellow"
		}
		if projPercent > 95 {
			projColor = "red"
		}
		
		projectionLine := m.renderCompactSectionAsString(
			"📈", "PROJECTION",
			projPercent,
			projInfo,
			projColor,
			projRightText,
		)
		table.Append([]string{projectionLine})
	}
	
	// Models section
	modelsText := "⚙️  Models: "
	if len(block.Models) > 0 {
		// Simplify model names
		simplifiedModels := []string{}
		for _, model := range block.Models {
			parts := strings.Split(model, "-")
			if len(parts) >= 3 {
				// Extract model type and version
				simplifiedModels = append(simplifiedModels, fmt.Sprintf("%s-%s", parts[1], parts[2]))
			} else {
				simplifiedModels = append(simplifiedModels, model)
			}
		}
		modelsText += strings.Join(simplifiedModels, ", ")
	} else {
		modelsText += "none"
	}
	table.Append([]string{modelsText})
	
	// Footer (inside the box) - use Footer for center alignment
	footerText := fmt.Sprintf("↻ Refreshing every %ds  •  Press Ctrl+C to stop",
		int(m.config.RefreshInterval.Seconds()))
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
	table.Footer([]string{footerStyle.Render(footerText)})
	
	// Render the table
	table.Render()
	
	// If terminal is wider than max width, center the entire table
	if m.width > 120 {
		tableOutput := buf.String()
		lines := strings.Split(tableOutput, "\n")
		var centeredOutput strings.Builder
		
		// Calculate left padding for centering
		leftPadding := (m.width - 120) / 2
		paddingStr := strings.Repeat(" ", leftPadding)
		
		// Add padding to each line
		for i, line := range lines {
			if line != "" {
				centeredOutput.WriteString(paddingStr + line)
			}
			if i < len(lines)-1 {
				centeredOutput.WriteString("\n")
			}
		}
		
		return centeredOutput.String()
	}
	
	return buf.String()
}

// renderCompactSectionAsString renders a compact section as a single string for table cell
func (m *BlocksLiveModel) renderCompactSectionAsString(icon, title string, percent float64, info, barColor, rightText string) string {
	// Build left part (icon + title)
	leftPart := fmt.Sprintf("%s %-9s", icon, title)
	
	// Determine progress bar width based on terminal width
	// Min width: 95, Max width: 120
	progressBarWidth := 40 // Default for minimum width
	if m.width > 0 {
		availableWidth := m.width - 2
		if availableWidth >= 120 {
			progressBarWidth = 50 // Use wider bar for max width
		} else if availableWidth >= 100 {
			progressBarWidth = 45 // Medium width
		}
	}
	
	// Build progress bar
	progressBar := m.renderEnhancedProgressBar(percent, progressBarWidth, barColor)
	
	// Build the complete line with dynamic spacing
	// Adjust spacing based on progress bar width
	rightPadding := 20
	if progressBarWidth == 50 {
		rightPadding = 20
	} else if progressBarWidth == 45 {
		rightPadding = 15
	} else {
		rightPadding = 10
	}
	topLine := fmt.Sprintf("%-12s %s %*s", leftPart, progressBar, rightPadding, rightText)
	
	// Add spacing above and below for better readability
	return fmt.Sprintf("\n%s\n%s\n", topLine, info)
}

// renderCompactSection renders a compact single-line section with progress bar
func (m *BlocksLiveModel) renderCompactSection(icon, title string, percent float64, info, barColor, rightText string, boxWidth int) string {
	// Calculate layout widths
	leftPartWidth := 12  // Icon + title
	progressBarWidth := 50 // Progress bar
	rightPartWidth := len(rightText) + 2
	
	// Build left part (icon + title)
	leftPart := fmt.Sprintf("%s %-9s", icon, title)
	
	// Build progress bar
	progressBar := m.renderEnhancedProgressBar(percent, progressBarWidth, barColor)
	
	// Build the line
	line := fmt.Sprintf("│ %-*s %s %*s │\n",
		leftPartWidth, leftPart,
		progressBar,
		rightPartWidth, rightText)
	
	// Add info line below
	infoLine := fmt.Sprintf("│ %-*s │\n", boxWidth-4, info)
	
	return line + infoLine
}

// renderEnhancedProgressBar renders an enhanced progress bar with gradient colors
func (m *BlocksLiveModel) renderEnhancedProgressBar(percent float64, width int, colorName string) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	
	filled := int(percent * float64(width) / 100)
	if filled > width {
		filled = width
	}
	
	// Use gradient or solid color based on configuration
	if m.config.UseGradient && !m.config.NoColor {
		return m.renderGradientProgressBar(percent, width, colorName)
	}
	return m.renderSolidProgressBar(percent, width, colorName)
}

// renderGradientProgressBar renders a progress bar with smooth color gradient
func (m *BlocksLiveModel) renderGradientProgressBar(percent float64, width int, colorName string) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	
	filled := int(percent * float64(width) / 100)
	if filled > width {
		filled = width
	}
	
	// Create cache key
	cacheKey := fmt.Sprintf("%s-%d-%d", colorName, width, filled)
	
	// Check cache first
	if m.gradientCache == nil {
		m.gradientCache = make(map[string][]string)
	}
	
	// Define gradient colors based on type
	var startColor, endColor string
	switch colorName {
	case "cyan":
		// SESSION: Deep blue to light cyan
		startColor = "#1e40af"
		endColor = "#06b6d4"
	case "green":
		// USAGE: Green gradient
		startColor = "#16a34a"
		endColor = "#4ade80"
	case "yellow":
		// USAGE: Yellow gradient (for warning)
		startColor = "#ca8a04"
		endColor = "#fbbf24"
	case "red":
		// USAGE/PROJECTION: Red gradient (for danger)
		startColor = "#dc2626"
		endColor = "#f87171"
	default:
		// Default: Blue gradient
		startColor = "#3b82f6"
		endColor = "#60a5fa"
	}
	
	// Check if we have cached colors for this configuration
	if cachedColors, ok := m.gradientCache[cacheKey]; ok && len(cachedColors) == filled {
		// Use cached colors
		var bar strings.Builder
		bar.WriteString("[")
		
		for _, hexColor := range cachedColors {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(hexColor))
			bar.WriteString(style.Render("█"))
		}
		
		// Add empty portion
		emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("239"))
		bar.WriteString(emptyStyle.Render(strings.Repeat("░", width-filled)))
		bar.WriteString("]")
		
		return bar.String()
	}
	
	// Parse colors
	c1, err1 := colorful.Hex(startColor)
	c2, err2 := colorful.Hex(endColor)
	
	// Fallback to solid color if parsing fails
	if err1 != nil || err2 != nil {
		return m.renderSolidProgressBar(percent, width, colorName)
	}
	
	// Calculate and cache gradient colors
	gradientColors := make([]string, filled)
	if filled > 0 {
		for i := 0; i < filled; i++ {
			// Calculate blend ratio for this position
			blend := float64(i) / float64(filled-1)
			if filled == 1 {
				blend = 0.5 // Middle color if only one character
			}
			
			// Blend colors in LUV space for smooth transitions
			blendedColor := c1.BlendLuv(c2, blend)
			gradientColors[i] = blendedColor.Hex()
		}
		
		// Cache the calculated colors
		m.gradientCache[cacheKey] = gradientColors
	}
	
	// Build gradient progress bar
	var bar strings.Builder
	bar.WriteString("[")
	
	// Render filled portion with gradient
	for _, hexColor := range gradientColors {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(hexColor))
		bar.WriteString(style.Render("█"))
	}
	
	// Add empty portion
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("239"))
	bar.WriteString(emptyStyle.Render(strings.Repeat("░", width-filled)))
	
	bar.WriteString("]")
	
	return bar.String()
}

// renderSolidProgressBar renders a progress bar with solid color (fallback)
func (m *BlocksLiveModel) renderSolidProgressBar(percent float64, width int, colorName string) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	
	filled := int(percent * float64(width) / 100)
	if filled > width {
		filled = width
	}
	
	// Select color based on name
	var color lipgloss.Color
	switch colorName {
	case "cyan":
		color = lipgloss.Color("51")  // Cyan
	case "green":
		color = lipgloss.Color("46")  // Green
	case "yellow":
		color = lipgloss.Color("226") // Yellow
	case "red":
		color = lipgloss.Color("196") // Red
	default:
		color = lipgloss.Color("252") // Default white
	}
	
	// Build the progress bar
	filledStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("239"))
	
	bar := "["
	bar += filledStyle.Render(strings.Repeat("█", filled))
	bar += emptyStyle.Render(strings.Repeat("░", width-filled))
	bar += "]"
	
	return bar
}

// formatTokensShort formats tokens with k/M suffix
func formatTokensShort(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// formatNumberWithCommas formats a number with comma separators
func formatNumberWithCommas(n int) string {
	if n < 0 {
		return "-" + formatNumberWithCommas(-n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return formatNumberWithCommas(n/1000) + "," + fmt.Sprintf("%03d", n%1000)
}

// renderProgressBar renders a progress bar
func (m *BlocksLiveModel) renderProgressBar(current, total time.Duration, width int) string {
	if total == 0 {
		return ""
	}
	
	percent := float64(current) / float64(total)
	filled := int(percent * float64(width))
	
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	
	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	percentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	
	return fmt.Sprintf("[%s] %s", 
		barStyle.Render(bar),
		percentStyle.Render(fmt.Sprintf("%.1f%%", percent*100)))
}

// getBurnRateIndicator returns the burn rate indicator
func (m *BlocksLiveModel) getBurnRateIndicator(tokensPerMinute float64) string {
	if tokensPerMinute > BurnRateHigh {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			Render("⚡ HIGH")
	}
	if tokensPerMinute > BurnRateModerate {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")).
			Bold(true).
			Render("⚡ MODERATE")
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("46")).
		Render("✓ NORMAL")
}

// blocksTickCmd returns a command that sends a tick message after the given duration
func blocksTickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return blocksTickMsg(t)
	})
}

// formatDuration formats a duration for display
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// formatNumber formats a number with thousand separators
func formatNumber(n int) string {
	if n < 0 {
		return "-" + formatNumber(-n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return formatNumber(n/1000) + "," + fmt.Sprintf("%03d", n%1000)
}

// StartBlocksLiveMonitoring starts the live monitoring for blocks
func StartBlocksLiveMonitoring(config BlocksLiveConfig) error {
	// Check if we're in a TTY environment
	if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("live monitoring requires an interactive terminal (TTY)")
	}

	// Initialize services
	pricingService := pricing.NewService()
	calc := calculator.New(pricingService)
	dataLoader := loader.New()
	
	// Optimize for live mode: reduce concurrent file reads to minimize CPU usage
	dataLoader.SetMaxWorkers(3) // Even more conservative for live monitoring
	
	// Enable debug mode if DEBUG env var is set
	if os.Getenv("DEBUG") != "" {
		dataLoader.SetDebug(true)
	}

	// Create initial model
	model := &BlocksLiveModel{
		config:     config,
		lastUpdate: time.Now(),
		loader:     dataLoader,
		calculator: calc,
		gradientCache: make(map[string][]string),
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create and run the program
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
	)

	// Run in a goroutine to handle signals
	go func() {
		<-sigChan
		p.Quit()
	}()

	fmt.Println("ℹ Live monitoring started. Press 'q' or Ctrl+C to quit.")
	_, err := p.Run()
	fmt.Println("ℹ Live monitoring stopped.")
	return err
}