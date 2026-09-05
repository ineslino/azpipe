package runner

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ineslino/azpipe/internal/azdo"
	domain "github.com/ineslino/azpipe/internal/runner"
	"testing"
)

func TestSearchEnterPreservesFilterForSelection(t *testing.T) {
	m := NewCatalogModel([]azdo.Pipeline{{ID: 1, Name: "alpha"}, {ID: 2, Name: "beta"}})
	m.input = inputSearch
	m.search.SetValue("beta")
	m.filter()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(CatalogModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updated.(CatalogModel)
	if m.search.Value() != "beta" || len(m.Selected()) != 1 || m.Selected()[0].ID() != 2 {
		t.Fatal("filtered selection lost", m.Selected())
	}
}

func TestExecutionResultExitStatus(t *testing.T) {
	for _, result := range []string{"succeeded", "failed", "canceled", "partiallySucceeded", ""} {
		m := AppModel{execution: executionModel{runs: []domain.RunResult{{Run: azdo.PipelineRun{ID: 42, State: "completed", Result: result}}}}}
		if (m.ExecutionError() == nil) != (result == "succeeded") {
			t.Errorf("result %q incorrectly classified", result)
		}
	}
}
