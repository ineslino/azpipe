package runner

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	domain "github.com/ineslino/azpipe/internal/runner"
)

func actionKey(m AppModel, key tea.KeyMsg) (AppModel, tea.Cmd) {
	u, cmd := m.Update(key)
	return u.(AppModel), cmd
}

func TestActionsBlockExplainAndCancel(t *testing.T) {
	m := NewDemoApp()
	m, _ = actionKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.actions == nil || !strings.Contains(m.View(), "Selecciona pelo menos") {
		t.Fatal("missing explanation")
	}
	m, cmd := actionKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.actions == nil || len(m.catalog.Selected()) != 0 {
		t.Fatal("blocked action dispatched")
	}
	m, _ = actionKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.actions != nil || m.screen != ScreenCatalog {
		t.Fatal("cancel did not restore catalog")
	}
}

func TestActionDispatchUsesExistingBranchEditor(t *testing.T) {
	m := NewDemoApp()
	index := 5
	m.actions = &index
	m, _ = actionKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.actions != nil || m.catalog.input != inputBranch {
		t.Fatal("branch action not routed")
	}
}

func TestContextFooterAndBannerFit(t *testing.T) {
	for _, size := range [][2]int{{60, 24}, {80, 24}, {120, 32}, {120, 40}} {
		m := NewDemoApp()
		u, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = u.(AppModel)
		if !strings.Contains(m.View(), "Um só terminal.") {
			t.Fatal("missing catalog description")
		}
		if size[1] >= 32 && !strings.Contains(m.View(), "█") {
			t.Fatal("missing large banner")
		}
		if strings.Contains(m.catalog.helpView(), "guardar perfil") {
			t.Fatal("secondary action leaked into footer")
		}
		for _, menu := range []bool{false, true} {
			if menu {
				index := 10
				m.actions = &index
			}
			if lipgloss.Height(m.View()) > size[1] || lipgloss.Width(m.View()) > size[0] {
				t.Fatalf("overflow %v menu=%v", size, menu)
			}
		}
	}
}

func TestReviewErrorReturnsToAffectedPipeline(t *testing.T) {
	m := NewDemoApp()
	m.screen = ScreenReview
	m.review.reviews = []domain.Review{{Selection: domain.Selection{Pipeline: demoPipelines()[1]}, Err: errors.New("required parameter")}}
	m, cmd := actionKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	p, _ := m.catalog.active()
	if cmd != nil || m.screen != ScreenCatalog || p.ID != 202 {
		t.Fatal("error recovery failed")
	}
}
