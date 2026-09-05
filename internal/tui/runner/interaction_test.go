package runner

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ineslino/azpipe/internal/azdo"
	domain "github.com/ineslino/azpipe/internal/runner"
)

func TestModeNeverSelectsImplicitly(t *testing.T) {
	m := catalogFixture()
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if len(m.Selected()) != 0 || m.warning == "" {
		t.Fatal("mode implicitly selected a pipeline")
	}
}

func TestParameterFormSaveAndDiscard(t *testing.T) {
	m := catalogFixture()
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m.editor.rows[0].name.SetValue("environment")
	m.editor.rows[0].value.SetValue("dev")
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.parameters[101]["environment"] != "dev" || m.input != inputNone {
		t.Fatal("form not saved")
	}
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m.editor.rows[0].value.SetValue("prod")
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.parameters[101]["environment"] != "dev" {
		t.Fatal("discard changed saved parameters")
	}
}

func TestParameterFormRejectsDuplicateNames(t *testing.T) {
	e := newParameterEditor(map[string]string{"env": "dev"})
	e.rows = append(e.rows, newParameterField("env", "prod"))
	if _, err := e.values(); err == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestReviewPagesNeverSkipAndFit(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 32}} {
		selections := make([]domain.Selection, 30)
		for i := range selections {
			selections[i] = domain.Selection{Pipeline: azdo.Pipeline{ID: i + 1, Name: fmt.Sprintf("pipeline-%02d", i+1)}, Mode: domain.ModeRun, Branch: "main"}
		}
		m := newReviewModel(selections, true, operationToken{})
		m.width, m.height = size[0], size[1]
		seen := map[int]bool{}
		for page := 0; page < 30; page++ {
			view := m.view()
			if lipgloss.Height(view) > m.height-2 {
				t.Fatalf("height %d > %d\n%s", lipgloss.Height(view), m.height-2, view)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > m.width {
					t.Fatalf("line overflow: %s", line)
				}
			}
			for i := range selections {
				if strings.Contains(view, selections[i].Pipeline.Name) {
					seen[i] = true
				}
			}
			old := m.offset
			m, _ = m.update(tea.KeyMsg{Type: tea.KeyPgDown})
			if old == m.offset {
				break
			}
		}
		if len(seen) != 30 {
			t.Fatalf("only saw %d of 30", len(seen))
		}
	}
}
