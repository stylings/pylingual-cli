package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cold/pylingual-cli/internal/runner"
)

type eventMsg runner.Event
type doneMsg struct{}

type rowState struct {
	input     string
	output    string
	status    runner.Status
	stage     string
	err       error
	startedAt time.Time
	updatedAt time.Time
}

type model struct {
	spinner   spinner.Model
	cancel    context.CancelFunc
	events    <-chan runner.Event
	rows      []rowState
	summary   runner.Summary
	done      bool
	cancelled bool
	width     int
	height    int
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	failStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	activeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	outputStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	summaryStyle = lipgloss.NewStyle().Bold(true)
)

func RunRich(ctx context.Context, cancel context.CancelFunc, jobs []runner.Job, events <-chan runner.Event) (runner.Summary, error) {
	spin := spinner.New()
	spin.Spinner = spinner.Dot

	rows := make([]rowState, len(jobs))
	for _, planned := range jobs {
		rows[planned.ID] = rowState{
			input:  planned.InputPath,
			output: planned.OutputPath,
			status: runner.StatusQueued,
			stage:  "queued",
		}
	}

	program := tea.NewProgram(model{
		spinner: spin,
		cancel:  cancel,
		events:  events,
		rows:    rows,
		summary: runner.Summary{Total: len(jobs)},
	}, tea.WithContext(ctx), tea.WithoutSignalHandler())

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			program.Quit()
		case <-done:
		}
	}()

	final, err := program.Run()
	if m, ok := final.(model); ok {
		if m.cancelled || ctx.Err() != nil {
			return m.summary, context.Canceled
		}
		if err != nil {
			return m.summary, err
		}
		return m.summary, nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return runner.Summary{}, context.Canceled
		}
		return runner.Summary{}, err
	}
	return runner.Summary{}, fmt.Errorf("unexpected UI model")
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, waitEvent(m.events))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.cancelled = true
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.done {
			return m, nil
		}
		return m, cmd
	case eventMsg:
		event := runner.Event(msg)
		if event.JobID >= 0 && event.JobID < len(m.rows) {
			row := m.rows[event.JobID]
			if row.startedAt.IsZero() {
				row.startedAt = event.At
			}
			row.updatedAt = event.At
			row.status = event.Status
			row.stage = event.Stage
			row.err = event.Err
			m.rows[event.JobID] = row
		}
		m.summary = runner.Summarize(m.summary, event)
		return m, waitEvent(m.events)
	case doneMsg:
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	done := m.summary.Succeeded + m.summary.Warnings + m.summary.Failed
	fmt.Fprintf(&b, "%s\n", titleStyle.Render("Pylingual decompile"))
	fmt.Fprintf(
		&b,
		"%s\n\n",
		summaryStyle.Render(fmt.Sprintf("%d/%d complete  %d ok  %d warnings  %d failed", done, m.summary.Total, m.summary.Succeeded, m.summary.Warnings, m.summary.Failed)),
	)

	rows, hidden := m.visibleRows()
	for _, row := range rows {
		fmt.Fprintf(&b, "%s\n", m.renderRow(row))
	}
	if hidden > 0 {
		fmt.Fprintf(&b, "%s\n", mutedStyle.Render(fmt.Sprintf("... %d more queued or completed files hidden", hidden)))
	}

	if m.cancelled {
		fmt.Fprintf(&b, "\n%s\n", failStyle.Render("Cancelled."))
	} else if m.done {
		fmt.Fprintf(&b, "\n%s\n", mutedStyle.Render("Finished."))
	}
	return b.String()
}

func (m model) visibleRows() ([]rowState, int) {
	maxRows := m.height - 5
	if maxRows <= 0 {
		maxRows = 20
	}
	if maxRows >= len(m.rows) {
		return m.rows, 0
	}

	var active []rowState
	var finished []rowState
	var queued []rowState
	for _, row := range m.rows {
		switch row.status {
		case runner.StatusQueued:
			queued = append(queued, row)
		case runner.StatusSucceeded, runner.StatusWarning, runner.StatusFailed:
			finished = append(finished, row)
		default:
			active = append(active, row)
		}
	}

	var rows []rowState
	rows = append(rows, active...)
	remaining := maxRows - len(rows)
	if remaining > 0 && len(finished) > 0 {
		start := len(finished) - remaining
		if start < 0 {
			start = 0
		}
		rows = append(rows, finished[start:]...)
	}
	remaining = maxRows - len(rows)
	if remaining > 0 {
		rows = append(rows, queued[:min(remaining, len(queued))]...)
	}

	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	return rows, len(m.rows) - len(rows)
}

func (m model) renderRow(row rowState) string {
	icon := m.spinner.View()
	style := activeStyle
	switch row.status {
	case runner.StatusQueued:
		icon = " "
		style = mutedStyle
	case runner.StatusSucceeded:
		icon = "✓"
		style = okStyle
	case runner.StatusWarning:
		icon = "!"
		style = warnStyle
	case runner.StatusFailed:
		icon = "x"
		style = failStyle
	}

	elapsed := ""
	if !row.startedAt.IsZero() {
		elapsed = fmt.Sprintf(" %s", mutedStyle.Render(time.Since(row.startedAt).Round(time.Second).String()))
	}

	detail := row.stage
	if row.status == runner.StatusSucceeded || row.status == runner.StatusWarning {
		detail = outputStyle.Render(filepath.Clean(row.output))
	}
	if row.status == runner.StatusFailed && row.err != nil {
		detail = failStyle.Render(row.err.Error())
	}

	input := filepath.Clean(row.input)
	if m.width > 0 {
		maxInput := m.width - 34
		if maxInput < 20 {
			maxInput = 20
		}
		input = truncateMiddle(input, maxInput)
	}

	return fmt.Sprintf("%s %s %s%s", style.Render(icon), input, detail, elapsed)
}

func waitEvent(events <-chan runner.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return doneMsg{}
		}
		return eventMsg(event)
	}
}

func truncateMiddle(value string, max int) string {
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	left := (max - 3) / 2
	right := max - 3 - left
	return value[:left] + "..." + value[len(value)-right:]
}
