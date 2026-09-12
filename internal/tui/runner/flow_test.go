package runner

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
)

func TestAllGuidedScreensFit(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 32}} {
		check := func(name, view string) {
			t.Helper()
			if len(strings.Split(view, "\n")) > size.Height {
				t.Errorf("%s exceeds %dx%d (%d lines):\n%s", name, size.Width, size.Height, len(strings.Split(view, "\n")), ansi.Strip(view))
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > size.Width {
					t.Errorf("%s too wide", name)
				}
			}
		}
		m := NewDemoApp()
		u, _ := m.Update(size)
		m = u.(AppModel)
		check("catalog", m.View())
		m, _ = pressApp(t, m, "d")
		check("pipeline details", m.View())
		m, _ = pressApp(t, m, "esc")
		m, _ = pressApp(t, m, "?")
		check("actions", m.View())
		m, _ = pressApp(t, m, "esc")
		m, _ = pressApp(t, m, " ")
		m, cmd := pressApp(t, m, "enter")
		m, _ = runAppCmd(t, m, cmd)
		check("review", m.View())
		m, _ = pressApp(t, m, "esc")
		for _, key := range []string{"s", "l", "h"} {
			m, _ = pressApp(t, m, key)
			check(key, m.View())
			m, _ = pressApp(t, m, "esc")
		}
		b := NewBranchDemo()
		b.width = size.Width
		b.height = size.Height
		check("branches", b.View())
		b.stage = "review"
		b.reviewed = b.branches[1:]
		check("branch demo review", b.View())
		b.demo = false
		check("branch real review", b.View())
		c := newContextModel(ContextDefaults{Organization: "example-org"})
		c.width = size.Width - 4
		c.height = size.Height - 2
		check("login", section("LIGAÇÃO", c.view(), size.Width))
		c.err = strings.Repeat("Falha de autenticação. ", 20) + "RECUPERAR"
		check("login error", section("LIGAÇÃO", c.view(), size.Width))
		c.errorScroll = 1000
		if !strings.Contains(c.view(), "RECUPERAR") {
			t.Fatal("error tail inaccessible")
		}
		schema := azdo.ParameterSchema{Parameters: []azdo.Parameter{{Name: "environment", DisplayName: "Ambiente", Type: "string", DefaultValue: "dev", HasDefault: true}}}
		e, err := newSchemaEditor(schema, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		check("parameters", section("CONFIGURAR", e.view(size.Width-4, size.Height-2, "sample"), size.Width))
	}
}

func TestFinalUXRegressions(t *testing.T) {
	m := NewDemoApp()
	u, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	m = u.(AppModel)
	m, _ = pressApp(t, m, "B")
	u, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m = u.(AppModel)
	m, cmd := pressApp(t, m, "esc")
	m, _ = runAppCmd(t, m, cmd)
	for _, line := range strings.Split(m.View(), "\n") {
		if ansi.StringWidth(line) > 60 {
			t.Fatal("stale size after returning from branches")
		}
	}
	c := newContextModel(ContextDefaults{Organization: "example-org"})
	c.width = 76
	c.height = 22
	for i := 0; i < 10; i++ {
		c.projects = append(c.projects, azdo.Project{Name: strings.Repeat("Project-", 10)})
	}
	c.err = strings.Repeat("Failure ", 40) + "Recovery"
	if len(strings.Split(section("CONTEXT", c.view(), 80), "\n")) > 24 {
		t.Fatal("project error hides footer")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyCtrlC}, {Type: tea.KeyCtrlD}, {Type: tea.KeyRunes, Runes: []rune("q")}} {
		b := NewBranchDemo()
		b.showHelp = true
		_, quit := b.Update(key)
		if quit == nil {
			t.Fatal("help swallows exit")
		}
		if _, ok := quit().(tea.QuitMsg); !ok {
			t.Fatal("help did not quit")
		}
	}
	b := NewBranchDemo()
	b.stage = "review"
	b.demo = false
	b.reviewed = b.branches[1:2]
	if !strings.Contains(b.View(), b.reviewed[0].ObjectID) {
		t.Fatal("review hides SHA before confirmation")
	}
	m = NewDemoApp()
	m.screen = ScreenExecution
	m.demo = false
	m.execution = executionModel{width: 76, height: 22}
	view := m.View()
	if strings.Contains(view, "esc catálogo") || strings.Contains(view, "q sair") || !strings.Contains(view, "Submissão em curso") {
		t.Fatal("submitting view promises unavailable exits")
	}
	_, cmd = pressApp(t, m, "esc")
	if cmd != nil {
		t.Fatal("submission guard removed")
	}
	if quantity(1, "pipeline", "pipelines") != "1 pipeline" || quantity(2, "pipeline", "pipelines") != "2 pipelines" {
		t.Fatal("wrong plural")
	}
}

func TestMonitoringLayoutFitsAndKeepsNavigation(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 32}} {
		m := NewDemoApp()
		updated, _ := m.Update(size)
		m = updated.(AppModel)
		m, _ = pressApp(t, m, "h")
		m, _ = pressApp(t, m, "enter")
		view := ansi.Strip(m.View())
		if len(strings.Split(view, "\n")) > size.Height {
			t.Fatalf("monitor exceeds %dx%d: %s", size.Width, size.Height, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size.Width {
				t.Fatal("overflow", line)
			}
		}
		if !strings.Contains(view, "ESTADO") || !strings.Contains(view, "esc catálogo") {
			t.Fatal("missing hierarchy or recovery")
		}
		m, _ = pressApp(t, m, "down")
		if m.execution.offset != 1 || !strings.Contains(m.View(), "deploy infrastructure") {
			t.Fatal("run navigation failed")
		}
	}
}

func TestDemoReviewContinuesOfflineAndRetainsSelection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AZPIPE_DATA_DIR", root)
	m := NewDemoApp()
	m, _ = pressApp(t, m, " ")
	m, cmd := pressApp(t, m, "enter")
	m, _ = runAppCmd(t, m, cmd)
	if !strings.Contains(m.View(), "Revisão de exemplo") {
		t.Fatal("missing demo guidance")
	}
	m, cmd = pressApp(t, m, "enter")
	if cmd != nil || m.Screen() != ScreenExecution || !strings.Contains(m.View(), "Demonstração estática") {
		t.Fatal("demo did not continue locally to static example")
	}
	m, _ = pressApp(t, m, "esc")
	if m.Screen() != ScreenCatalog || len(m.catalog.Selected()) != 1 {
		t.Fatal("return lost selection")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("demo wrote files", err)
	}
}

func TestBranchesExplainConfirmationBeforeInvalidAttempt(t *testing.T) {
	m := NewBranchDemo()
	m.demo = false
	m.stage = "review"
	m.reviewed = m.branches[1:]
	view := m.View()
	for _, text := range []string{"Escreve ELIMINAR", "2 branches", "sample-repo"} {
		if !strings.Contains(view, text) {
			t.Fatal("missing upfront consequence", text)
		}
	}
}

func TestBranchesQuitDoesNotReturnToCatalog(t *testing.T) {
	m := NewBranchDemo()
	m.returnToCatalog = true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("missing quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q should quit")
	}
}
