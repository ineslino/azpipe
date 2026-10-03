package runner

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
	domain "github.com/ineslino/azpipe/internal/runner"
	"github.com/muesli/termenv"
)

func TestRemainingEmptyProjectFooterOffersAvailableActions(t *testing.T) {
	profile, dark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	t.Cleanup(func() {
		lipgloss.SetColorProfile(profile)
		lipgloss.SetHasDarkBackground(dark)
	})
	for _, theme := range []struct {
		profile termenv.Profile
		dark    bool
	}{{termenv.ANSI256, true}, {termenv.ANSI256, false}, {termenv.Ascii, true}} {
		lipgloss.SetColorProfile(theme.profile)
		lipgloss.SetHasDarkBackground(theme.dark)
		for _, size := range []tea.WindowSizeMsg{{Width: 60, Height: 24}, {Width: 80, Height: 24}, {Width: 100, Height: 32}, {Width: 120, Height: 40}} {
			for _, branchesOnly := range []bool{false, true} {
				m := NewBootstrapApp(nil, ContextDefaults{Organization: "example-org"})
				if branchesOnly {
					m = NewBranchesBootstrap(nil, ContextDefaults{Organization: "example-org"})
				}
				u, _ := m.Update(size)
				m = u.(AppModel)
				m.context.setProjects([]azdo.Project{{Name: "sample-project"}})
				m.context.setFocus(contextProjectFocus)
				m, _ = pressApp(t, m, "/")
				m, _ = pressApp(t, m, "inexistente")
				if !strings.Contains(ansi.Strip(m.View()), "enter terminar pesquisa") {
					t.Fatal("empty result hid the available search editing action")
				}
				m, _ = pressApp(t, m, "enter")
				view := ansi.Strip(m.View())
				footer := view[strings.LastIndex(view, "╭─ ACÇÕES"):]
				for _, action := range []string{"c limpar filtro", "/ alterar pesquisa", "esc mudar organização", ":q sair"} {
					if !strings.Contains(footer, action) {
						t.Fatalf("empty result missing action %q at %dx%d:\n%s", action, size.Width, size.Height, footer)
					}
				}
				if strings.Contains(footer, "enter abrir") || strings.Contains(footer, "↑/↓ escolher") || strings.Contains(footer, "página") {
					t.Fatalf("empty result advertised an unavailable action:\n%s", footer)
				}
				if lipgloss.Height(view) > size.Height || lipgloss.Width(view) > size.Width {
					t.Fatalf("empty result overflow at %dx%d", size.Width, size.Height)
				}
				m, cmd := pressApp(t, m, "enter")
				if cmd != nil || m.context.loading || m.branchBrowser != nil {
					t.Fatal("empty result opened a scope")
				}
				m, _ = pressApp(t, m, "/")
				if !m.context.projectSearching {
					t.Fatal("advertised edit action unavailable")
				}
				m, _ = pressApp(t, m, "esc")
				m, _ = pressApp(t, m, "c")
				primary := "enter abrir catálogo"
				if branchesOnly {
					primary = "enter abrir repositórios"
				}
				if !strings.Contains(ansi.Strip(m.View()), primary) || len(m.context.visibleProjectIndexes()) != 2 {
					t.Fatal("clearing search did not restore normal choices and footer")
				}
			}
		}
	}
}

func TestRemainingGlobalBranchCopyMatchesScopeAndProfiles(t *testing.T) {
	m := NewDemoApp()
	index := -1
	for i, action := range m.catalogActions() {
		if action.key == "b" {
			index = i
			if action.label != "Alterar branch global" || action.blocked != "" {
				t.Fatal("global branch action has an ambiguous label or requires a selection")
			}
			for _, meaning := range []string{"actuais", "futuras", "perfis"} {
				if !strings.Contains(action.description, meaning) {
					t.Fatalf("branch action omits consequence %q", meaning)
				}
			}
		}
	}
	if index < 0 {
		t.Fatal("branch action missing")
	}
	m.actions = &index
	m, _ = pressApp(t, m, "enter")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Branch global:") || !strings.Contains(view, "enter aplicar a todas") {
		t.Fatal("branch editor does not explain global scope before applying")
	}
	m, _ = pressApp(t, m, "esc")
	m.demoProfiles = []domain.Profile{{Version: 1, Name: "mixed", Organization: "demo", Project: m.project, Selections: []domain.ProfileSelection{
		{ID: 101, Project: m.project, Mode: domain.ModeRun, Branch: "profile-app"},
		{ID: 202, Project: m.project, Mode: domain.ModePlan, Branch: "profile-iac"},
	}}}
	m, _ = pressApp(t, m, "l")
	m, _ = pressApp(t, m, "enter")
	if m.library != nil || len(m.catalog.branches) != 2 {
		t.Fatal("profile did not load per-pipeline branches")
	}
	m, _ = pressApp(t, m, "b")
	m, _ = pressApp(t, m, "ctrl+u")
	m = typeApp(t, m, "discarded")
	m, _ = pressApp(t, m, "esc")
	if len(m.catalog.branches) != 2 || m.catalog.branch.Value() != "main" {
		t.Fatal("cancelling global edit discarded profile branches")
	}
	m, _ = pressApp(t, m, "b")
	m, _ = pressApp(t, m, "ctrl+u")
	m = typeApp(t, m, "release/global")
	m, _ = pressApp(t, m, "enter")
	if len(m.catalog.branches) != 0 || len(m.catalog.Selected()) != 2 {
		t.Fatal("global branch did not replace profile branches while retaining selection")
	}
	for _, selection := range m.catalog.Selected() {
		if selection.Branch != "release/global" {
			t.Fatal("current selection did not use global branch")
		}
	}
	m, _ = pressApp(t, m, "down")
	m, _ = pressApp(t, m, "down")
	m, _ = pressApp(t, m, " ")
	if len(m.catalog.Selected()) != 3 {
		t.Fatal("future selection was not added")
	}
	for _, selection := range m.catalog.Selected() {
		if selection.Branch != "release/global" || (selection.ID() == 202 && selection.Mode != domain.ModePlan) {
			t.Fatal("global branch did not apply to future selections or changed mode")
		}
	}
}
