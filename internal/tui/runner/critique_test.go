package runner

import (
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
	domain "github.com/ineslino/azpipe/internal/runner"
	"strings"
	"testing"
)

func TestHiddenSelectionRemainsVisibleAndCanBeCleared(t *testing.T) {
	m := NewDemoApp()
	m, _ = pressApp(t, m, " ")
	m, _ = pressApp(t, m, "down")
	m, _ = pressApp(t, m, " ")
	m, _ = pressApp(t, m, "/")
	m, _ = pressApp(t, m, "infrastructure")
	if !strings.Contains(ansi.Strip(m.View()), "1 oculta") {
		t.Fatal("hidden selection missing during search")
	}
	m, _ = pressApp(t, m, "enter")
	m, _ = pressApp(t, m, "x")
	selected := m.catalog.Selected()
	if len(selected) != 1 || selected[0].Pipeline.ID != 202 {
		t.Fatalf("visible selection changed: %#v", selected)
	}
	m, _ = pressApp(t, m, "/")
	m, _ = pressApp(t, m, "no-match")
	if !strings.Contains(ansi.Strip(m.View()), "1 oculta") {
		t.Fatal("hidden selection missing with no results")
	}
}

func TestReviewFocusesFirstBlockedPipeline(t *testing.T) {
	m := NewDemoApp()
	m.demo = false
	m, _ = pressApp(t, m, " ")
	m, _ = pressApp(t, m, "down")
	m, _ = pressApp(t, m, " ")
	m, cmd := pressApp(t, m, "enter")
	m, _ = runAppCmd(t, m, cmd)
	reviews := append([]domain.Review(nil), m.review.reviews...)
	reviews[0].State = domain.ReviewReady
	reviews[1].State = domain.ReviewError
	reviews[1].Err = errors.New("branch indisponível")
	updated, _ := m.Update(previewFinishedMsg{token: m.active, reviews: reviews})
	m = updated.(AppModel)
	if m.review.offset != 1 || m.review.canExecute() {
		t.Fatal("blocked preview must focus error and prevent execution")
	}
	for _, text := range []string{"1 pronta", "1 bloqueada", "0 pendentes", "branch indisponível"} {
		if !strings.Contains(ansi.Strip(m.View()), text) {
			t.Fatalf("missing %q", text)
		}
	}
	m, _ = pressApp(t, m, "enter")
	active, _ := m.catalog.active()
	if m.screen != ScreenCatalog || active.ID != 202 {
		t.Fatal("recovery did not return to blocked pipeline")
	}
}

func TestUnknownRunRecoveryAlongsideRunningRun(t *testing.T) {
	m := executionModel{queued: true, width: 76, height: 22, runs: []domain.RunResult{
		{Err: errors.New("resposta perdida")},
		{Run: azdo.PipelineRun{ID: 123, State: "inProgress"}},
	}}
	view := ansi.Strip(m.view())
	for _, text := range []string{"confirma no Azure DevOps antes de repetir", "Esc → h", "resposta perdida"} {
		if !strings.Contains(view, text) {
			t.Fatalf("missing recovery instruction %q", text)
		}
	}
}

func TestCompactCatalogKeepsListAndDetailAccessible(t *testing.T) {
	pipelines := make([]azdo.Pipeline, 20)
	for i := range pipelines {
		pipelines[i] = azdo.Pipeline{ID: i + 1, Name: fmt.Sprintf("pipeline-%02d", i), RepoName: "sample-repo"}
	}
	m := NewApp(nil, "test", pipelines)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(AppModel)
	start, end := m.catalog.displayRange()
	if end-start < 6 {
		t.Fatalf("only %d rows visible", end-start)
	}
	if lipgloss.Height(m.View()) > 24 {
		t.Fatal("catalog exceeds terminal")
	}
	if !strings.Contains(m.View(), "a/? abre acções, perfis e histórico") {
		t.Fatal("menu missing from initial prompt")
	}
	m, _ = pressApp(t, m, "d")
	if !strings.Contains(m.View(), "sample-repo") {
		t.Fatal("full detail unavailable")
	}
	m, _ = pressApp(t, m, "esc")
	if m.catalog.showDetails {
		t.Fatal("detail did not close")
	}
}

func TestBatchOutcomeSummary(t *testing.T) {
	for _, tc := range []struct {
		result        string
		id            int
		title, action string
	}{
		{"succeeded", 1, "Lote concluído com sucesso", "Todas as runs terminaram"},
		{"failed", 1, "Lote terminado · existem runs sem sucesso", "seus logs antes de repetir"},
		{"", 0, "Lote por confirmar", "confirma no Azure DevOps antes de repetir"},
	} {
		m := executionModel{queued: true, width: 76, height: 22, runs: []domain.RunResult{
			{Run: azdo.PipelineRun{ID: 10, State: "completed", Result: "succeeded"}},
			{Run: azdo.PipelineRun{ID: tc.id, State: "completed", Result: tc.result}},
		}}
		view := ansi.Strip(m.view())
		if !strings.Contains(view, tc.title) || !strings.Contains(view, tc.action) {
			t.Fatalf("incorrect outcome: %s", view)
		}
		if lipgloss.Height(view) > 22 {
			t.Fatal("summary exceeds terminal")
		}
	}
}
