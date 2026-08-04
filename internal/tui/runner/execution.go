package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	domainrunner "github.com/ineslino/azpipe/internal/runner"
)

const refreshInterval = 5 * time.Second

type queueFinishedMsg struct {
	token operationToken
	runs  []domainrunner.RunResult
	err   error
}

type refreshTickMsg struct {
	token operationToken
}

type refreshFinishedMsg struct {
	token operationToken
	runs  []domainrunner.RunResult
}

type executionModel struct {
	runs   []domainrunner.RunResult
	queued bool
	err    error
}

func newExecutionModel() executionModel {
	return executionModel{}
}

func queueReviews(service domainrunner.Service, reviews []domainrunner.Review, token operationToken) tea.Cmd {
	return func() tea.Msg {
		runs, err := service.QueueAll(context.Background(), reviews, 4)
		return queueFinishedMsg{token: token, runs: runs, err: err}
	}
}

func refreshRuns(service domainrunner.Service, runs []domainrunner.RunResult, token operationToken) tea.Cmd {
	return func() tea.Msg {
		retryable := append([]domainrunner.RunResult(nil), runs...)
		for index := range retryable {
			if retryable[index].Run.ID != 0 && retryable[index].Run.State != "completed" {
				retryable[index].Err = nil
			}
		}
		return refreshFinishedMsg{token: token, runs: service.Refresh(context.Background(), retryable, 4)}
	}
}

func scheduleRefresh(token operationToken) tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return refreshTickMsg{token: token} })
}

func hasNonTerminalRun(runs []domainrunner.RunResult) bool {
	for _, result := range runs {
		if result.Run.ID != 0 && result.Run.State != "completed" {
			return true
		}
	}
	return false
}

func (m executionModel) view() string {
	lines := []string{catalogTitleStyle.Render("Execução")}
	if !m.queued {
		lines = append(lines, "A colocar pipelines em execução...")
	}
	for _, result := range m.runs {
		name := result.Review.Selection.Pipeline.Name
		if result.Err != nil {
			line := fmt.Sprintf("ERROR %s: %v", name, result.Err)
			if result.Run.WebURL != "" {
				line += " " + result.Run.WebURL
			}
			lines = append(lines, line)
			continue
		}
		line := fmt.Sprintf("%s %s", strings.ToUpper(result.Run.State), name)
		if result.Run.Result != "" {
			line += " " + result.Run.Result
		}
		if result.Run.WebURL != "" {
			line += " " + result.Run.WebURL
		}
		lines = append(lines, line)
	}
	if m.err != nil && len(m.runs) == 0 {
		lines = append(lines, catalogWarningStyle.Render(m.err.Error()))
	}
	lines = append(lines, catalogFooterStyle.Render("actualização automática a cada 5s • q sair sem cancelar runs"))
	return strings.Join(lines, "\n")
}
