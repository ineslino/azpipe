package runner

import (
	contextpkg "context"
	"fmt"
	"net/url"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	domainrunner "github.com/ineslino/azpipe/internal/runner"
)

const refreshInterval = 5 * time.Second

type queueFinishedMsg struct {
	token operationToken
	runs  []domainrunner.RunResult
	err   error
}

type queueProgressMsg struct {
	token   operationToken
	index   int
	result  domainrunner.RunResult
	journal string
	next    <-chan tea.Msg
}

type refreshTickMsg struct {
	token operationToken
}

type refreshFinishedMsg struct {
	token operationToken
	runs  []domainrunner.RunResult
	err   error
}

type executionModel struct {
	width      int
	horizontal int
	runs       []domainrunner.RunResult
	queued     bool
	err        error
	persistErr error
	offset     int
	height     int
	journal    string
	demo       bool
}

func newExecutionModel() executionModel {
	return executionModel{}
}

func queueReviews(service domainrunner.Service, reviews []domainrunner.Review, token operationToken, context ...string) tea.Cmd {
	return func() tea.Msg {
		if len(context) == 2 {
			journal, err := domainrunner.NewJournal(context[0], context[1], reviews)
			if err != nil {
				return queueFinishedMsg{token: token, err: err}
			}
			events := make(chan tea.Msg, len(reviews)+1)
			service.OnResult = func(i int, r domainrunner.RunResult) error {
				err := journal.Record(i, r)
				events <- queueProgressMsg{token: token, index: i, result: r, journal: journal.Path(), next: events}
				return err
			}
			go func() {
				runs, err := service.QueueAll(contextpkg.Background(), reviews, 4)
				events <- queueFinishedMsg{token: token, runs: runs, err: err}
				close(events)
			}()
			return <-events
		}
		runs, err := service.QueueAll(contextpkg.Background(), reviews, 4)
		return queueFinishedMsg{token: token, runs: runs, err: err}
	}
}

func refreshRuns(service domainrunner.Service, runs []domainrunner.RunResult, token operationToken, saved ...string) tea.Cmd {
	return func() tea.Msg {
		retryable := append([]domainrunner.RunResult(nil), runs...)
		for index := range retryable {
			if retryable[index].Run.ID != 0 && retryable[index].Run.State != "completed" {
				retryable[index].Err = nil
			}
		}
		refreshed := service.Refresh(contextpkg.Background(), retryable, 4)
		var err error
		if len(saved) == 3 && saved[0] != "" {
			var journal *domainrunner.Journal
			journal, err = domainrunner.LoadJournal(saved[0], saved[1], saved[2])
			if err == nil {
				err = journal.UpdateRuns(refreshed)
			}
		}
		return refreshFinishedMsg{token: token, runs: refreshed, err: err}
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
	queued, running, succeeded, failed, unknown := 0, 0, 0, 0, 0
	for _, r := range m.runs {
		switch {
		case r.Run.ID == 0:
			unknown++
		case r.Run.State == "completed" && r.Run.Result == "succeeded":
			succeeded++
		case r.Run.State == "completed":
			failed++
		case r.Run.State == "inProgress":
			running++
		default:
			queued++
		}
	}
	lines := []string{catalogTitleStyle.Render("Execução · acompanhamento do lote"), catalogDetailStyle.Render(fmt.Sprintf("%d em fila · %d a correr · %d sucesso · %d falha · %d sem ID", queued, running, succeeded, failed, unknown))}
	if m.journal != "" {
		lines = append(lines, horizontalWindow("Retoma: azpipe resume "+m.journal, m.horizontal, m.width))
	}
	if !m.queued {
		lines = append(lines, "A colocar pipelines em execução...")
	}
	height := m.height
	if height == 0 {
		height = defaultHeight
	}
	footer := shortcutBar(m.width, "pgup/pgdown linhas", "←/→ detalhe", "esc catálogo", "q sair sem cancelar runs")
	available := m.pageSize()
	end := min(len(m.runs), m.offset+available)
	for _, result := range m.runs[m.offset:end] {
		name := result.Review.Selection.Pipeline.Name
		if result.Err != nil {
			line := fmt.Sprintf("ERROR %s: %v", name, result.Err)
			if result.Run.WebURL != "" {
				line += " " + result.Run.WebURL
			}
			lines = append(lines, runLink(result.Run.WebURL, catalogWarningStyle.Render(horizontalWindow(line, m.horizontal, m.width))))
			continue
		}
		line := fmt.Sprintf("%s %s", strings.ToUpper(result.Run.State), name)
		if result.Run.Result != "" {
			line += " " + result.Run.Result
		}
		if result.Run.WebURL != "" {
			line += " " + result.Run.WebURL
		}
		style := runStyle
		if result.Run.Result == "succeeded" {
			style = successStyle
		}
		if result.Run.Result == "failed" || result.Run.Result == "canceled" || result.Run.Result == "partiallySucceeded" {
			style = catalogWarningStyle
		}
		lines = append(lines, runLink(result.Run.WebURL, style.Render(horizontalWindow(line, m.horizontal, m.width))))
	}
	if m.err != nil {
		lines = append(lines, catalogWarningStyle.Render(m.err.Error()))
	}
	if m.persistErr != nil {
		lines = append(lines, catalogWarningStyle.Render(truncateWidth("Estado não guardado: "+m.persistErr.Error(), m.width)))
	}
	status := "Actualização automática a cada 5s"
	if m.demo {
		status = "Demonstração estática · sem pedidos Azure"
	} else if !hasNonTerminalRun(m.runs) && m.queued {
		status = "Acompanhamento terminado · sem runs conhecidas pendentes"
	}
	lines = append(lines, footer, catalogDetailStyle.Render(status))
	return strings.Join(lines, "\n")
}

// OSC 8 preserves the complete run URL even when its visible label is clipped.
func runLink(raw, label string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Hostname() != "dev.azure.com" && !strings.HasSuffix(u.Hostname(), ".visualstudio.com")) {
		return label
	}
	return ansi.SetHyperlink(raw) + label + ansi.ResetHyperlink()
}

func (m executionModel) pageSize() int {
	height := m.height
	if height == 0 {
		height = defaultHeight
	}
	prefix := 2
	if m.err != nil {
		prefix++
	}
	if m.persistErr != nil {
		prefix++
	}
	if m.journal != "" {
		prefix++
	}
	if !m.queued {
		prefix++
	}
	footer := shortcutBar(m.width, "pgup/pgdown linhas", "←/→ detalhe", "esc catálogo", "q sair sem cancelar runs")
	return max(1, height-2-prefix-strings.Count(footer, "\n")-2)
}
