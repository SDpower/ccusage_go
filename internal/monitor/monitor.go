package monitor

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sdpower/ccusage-go/internal/calculator"
	"github.com/sdpower/ccusage-go/internal/loader"
	"github.com/sdpower/ccusage-go/internal/pricing"
	"github.com/sdpower/ccusage-go/internal/types"
)

type Monitor struct {
	options Options
}

type Options struct {
	DataPath   string
	Interval   time.Duration
	NoColor    bool
	Continuous bool
}

type model struct {
	options       Options
	lastUpdate    time.Time
	totalCost     float64
	totalTokens   int
	totalReqs     int
	recentEntries []types.UsageEntry
	err           error
}

type tickMsg time.Time

func New(opts Options) *Monitor {
	if opts.Interval == 0 {
		opts.Interval = 5 * time.Second
	}

	return &Monitor{
		options: opts,
	}
}

func (m *Monitor) Start(ctx context.Context) error {
	if m.options.Continuous {
		return m.startTUI(ctx)
	}
	return m.runOnce(ctx)
}

func (m *Monitor) startTUI(ctx context.Context) error {
	p := tea.NewProgram(
		initialModel(m.options),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)

	_, err := p.Run()
	return err
}

func (m *Monitor) runOnce(ctx context.Context) error {
	pricingService := pricing.NewService()
	calc := calculator.New(pricingService)
	dataLoader := loader.New()

	entries, err := dataLoader.LoadFromPath(ctx, m.options.DataPath)
	if err != nil {
		return fmt.Errorf("failed to load data: %w", err)
	}

	entries, err = calc.CalculateCosts(ctx, entries)
	if err != nil {
		return fmt.Errorf("failed to calculate costs: %w", err)
	}

	// Simple output for one-time run
	var totalCost float64
	var totalTokens int
	for _, entry := range entries {
		totalCost += entry.Cost
		totalTokens += entry.TotalTokens
	}

	fmt.Printf("Total Requests: %d\n", len(entries))
	fmt.Printf("Total Cost: $%.4f\n", totalCost)
	fmt.Printf("Total Tokens: %d\n", totalTokens)

	return nil
}

func initialModel(opts Options) model {
	return model{
		options:    opts,
		lastUpdate: time.Now(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(m.options.Interval),
		m.updateData(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "r":
			return m, m.updateData()
		}

	case tickMsg:
		m.lastUpdate = time.Time(msg)
		return m, tea.Batch(
			tickCmd(m.options.Interval),
			m.updateData(),
		)

	case updateDataMsg:
		m.totalCost = msg.totalCost
		m.totalTokens = msg.totalTokens
		m.totalReqs = msg.totalReqs
		m.recentEntries = msg.recentEntries
		m.err = msg.err
	}

	return m, nil
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress 'q' to quit, 'r' to retry", m.err)
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		MarginBottom(1)

	if m.options.NoColor {
		headerStyle = lipgloss.NewStyle()
	}

	content := headerStyle.Render("Claude Code Usage Monitor")
	content += "\n\n"

	// Summary section
	summaryStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1).
		MarginBottom(1)

	if m.options.NoColor {
		summaryStyle = lipgloss.NewStyle()
	}

	summary := fmt.Sprintf(
		"Total Requests: %d\nTotal Cost: $%.4f\nTotal Tokens: %d\nLast Update: %s",
		m.totalReqs,
		m.totalCost,
		m.totalTokens,
		m.lastUpdate.Format("15:04:05"),
	)

	content += summaryStyle.Render(summary)
	content += "\n\n"

	// Recent entries
	if len(m.recentEntries) > 0 {
		content += "Recent Activity:\n"
		for i, entry := range m.recentEntries {
			if i >= 5 { // Show only last 5
				break
			}
			content += fmt.Sprintf(
				"%s - %s - $%.4f\n",
				entry.Timestamp.Format("15:04:05"),
				entry.Model,
				entry.Cost,
			)
		}
	}

	content += "\n\nPress 'q' to quit, 'r' to refresh"
	return content
}

type updateDataMsg struct {
	totalCost     float64
	totalTokens   int
	totalReqs     int
	recentEntries []types.UsageEntry
	err           error
}

func (m model) updateData() tea.Cmd {
	return func() tea.Msg {
		pricingService := pricing.NewService()
		calc := calculator.New(pricingService)
		dataLoader := loader.New()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		entries, err := dataLoader.LoadFromPath(ctx, m.options.DataPath)
		if err != nil {
			return updateDataMsg{err: err}
		}

		entries, err = calc.CalculateCosts(ctx, entries)
		if err != nil {
			return updateDataMsg{err: err}
		}

		var totalCost float64
		var totalTokens int
		for _, entry := range entries {
			totalCost += entry.Cost
			totalTokens += entry.TotalTokens
		}

		// Get recent entries (last 10)
		recentEntries := entries
		if len(entries) > 10 {
			recentEntries = entries[len(entries)-10:]
		}

		return updateDataMsg{
			totalCost:     totalCost,
			totalTokens:   totalTokens,
			totalReqs:     len(entries),
			recentEntries: recentEntries,
		}
	}
}

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
