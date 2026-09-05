package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ineslino/azpipe/internal/azdo"
)

type tickMsg struct{}

type stagesUpdateMsg struct {
	stages []stageRow
	runID  int
}

type runCompleteMsg struct{ result string }

type watchErrMsg struct{ err error }

type stageRow struct {
	name   string
	state  string
	result string
	order  int
}

// WatchModel is a bubbletea model that live-polls a pipeline's active run.
type WatchModel struct {
	client      azdo.Client
	project     string
	pipelineID  int
	interval    time.Duration
	sp          spinner.Model
	stages      []stageRow
	runID       int
	finalResult string
	done        bool
	err         error
	width       int
}

// NewWatchModel creates a WatchModel ready to pass to tea.NewProgram.
func NewWatchModel(client azdo.Client, project string, pipelineID int, interval time.Duration) WatchModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return WatchModel{
		client:     client,
		project:    project,
		pipelineID: pipelineID,
		interval:   interval,
		sp:         s,
	}
}

// Err returns any terminal error after the program exits.
func (m WatchModel) Err() error { return m.err }

// FinalResult returns the run result string after the run completes.
func (m WatchModel) FinalResult() string { return m.finalResult }

func (m WatchModel) Init() tea.Cmd {
	return tea.Batch(m.sp.Tick, m.fetchStatus())
}

func (m WatchModel) fetchStatus() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		run, err := m.client.GetActiveRun(ctx, m.project, m.pipelineID)
		if err != nil {
			return watchErrMsg{err}
		}
		if run == nil {
			// No active run — check if the last run is complete.
			runs, err := m.client.GetPipelineRuns(ctx, m.project, m.pipelineID, 1)
			if err != nil || len(runs) == 0 {
				return watchErrMsg{fmt.Errorf("no active or recent runs found for pipeline %d", m.pipelineID)}
			}
			if runs[0].State == "completed" {
				return runCompleteMsg{result: runs[0].Result}
			}
			run = &runs[0]
		}

		stages, _ := m.client.GetBuildTimeline(ctx, m.project, run.ID)
		return stagesUpdateMsg{stages: toStageRows(stages), runID: run.ID}
	}
}

func (m WatchModel) schedulePoll() tea.Cmd {
	return tea.Tick(m.interval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m WatchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		return m, cmd

	case stagesUpdateMsg:
		m.stages = msg.stages
		m.runID = msg.runID
		return m, tea.Batch(m.sp.Tick, m.schedulePoll())

	case tickMsg:
		return m, m.fetchStatus()

	case runCompleteMsg:
		m.done = true
		m.finalResult = msg.result
		return m, tea.Quit

	case watchErrMsg:
		m.err = msg.err
		return m, tea.Quit

	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
	}
	return m, nil
}

func (m WatchModel) View() string {
	if m.err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("error: "+m.err.Error()) + "\n"
	}

	if m.done {
		icon, color := resultIconColor(m.finalResult)
		return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).
			Render(fmt.Sprintf("%s Run completed: %s", icon, m.finalResult)) + "\n"
	}

	var sb strings.Builder
	header := m.sp.View() + " Watching pipeline run"
	if m.runID > 0 {
		header += fmt.Sprintf(" #%d", m.runID)
	}
	sb.WriteString(header + "...\n\n")

	if len(m.stages) == 0 {
		sb.WriteString("  Loading stages...\n")
	} else {
		for _, s := range m.stages {
			icon, color := stageIconColor(s.state, s.result)
			iconStr := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(icon)
			name := lipgloss.NewStyle().Width(32).Render(s.name)
			label := lipgloss.NewStyle().Faint(true).Render(stageLabel(s.state, s.result))
			sb.WriteString(fmt.Sprintf("  %s  %s  %s\n", iconStr, name, label))
		}
	}

	sb.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render("  q / ctrl+c  stop watching") + "\n")
	return sb.String()
}

func toStageRows(stages []azdo.StageResult) []stageRow {
	var rows []stageRow
	for _, s := range stages {
		if s.RecordType == "Stage" {
			rows = append(rows, stageRow{
				name:   s.Name,
				state:  s.State,
				result: s.Result,
				order:  s.Order,
			})
		}
	}
	return rows
}

func stageIconColor(state, result string) (icon, color string) {
	switch {
	case state == "completed" && result == "succeeded":
		return "✓", "2"
	case state == "completed" && result == "failed":
		return "✗", "9"
	case state == "completed" && result == "partiallySucceeded":
		return "⚠", "3"
	case state == "completed" && (result == "canceled" || result == "abandoned"):
		return "○", "8"
	case result == "skipped":
		return "─", "8"
	case state == "inProgress":
		return "●", "3"
	default:
		return "○", "240"
	}
}

func stageLabel(state, result string) string {
	if state == "completed" {
		return result
	}
	return state
}

func resultIconColor(result string) (icon, color string) {
	switch result {
	case "succeeded":
		return "✓", "2"
	case "failed":
		return "✗", "9"
	case "partiallySucceeded":
		return "⚠", "3"
	case "canceled":
		return "○", "8"
	default:
		return "?", "240"
	}
}
