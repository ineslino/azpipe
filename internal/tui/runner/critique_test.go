package runner

import (
	"errors"
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
