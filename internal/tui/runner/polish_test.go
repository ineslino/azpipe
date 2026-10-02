package runner

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
	"github.com/muesli/termenv"
)

func TestPolishedContextAndCatalogLayout(t *testing.T) {
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
			check := func(view string, labels ...string) {
				t.Helper()
				if lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
					t.Fatalf("overflow at %dx%d: %dx%d\n%s", size.Width, size.Height, lipgloss.Width(view), lipgloss.Height(view), ansi.Strip(view))
				}
				for _, label := range labels {
					if !strings.Contains(ansi.Strip(view), label) {
						t.Fatalf("missing %q\n%s", label, ansi.Strip(view))
					}
				}
				if theme.profile == termenv.Ascii && strings.Contains(view, "\x1b[") {
					t.Fatal("plain layout contains styling escapes")
				}
			}
			m := NewBootstrapApp(nil, ContextDefaults{Organization: "example-org"})
			u, _ := m.Update(size)
			m = u.(AppModel)
			check(m.View(), "AZPIPE", "█", "1 · LIGAR", "╭─ ACÇÕES", ":q sair")
			m.context.err = strings.Repeat("Falha ao ligar. ", 30) + "RECUPERAR"
			check(m.View(), "╭─ ERRO", "PgUp/PgDn", "enter ligar")
			m.context.errorScroll = 1000
			check(m.View(), "RECUPERAR")
			m.context.err = ""
			m.context.loading = true
			check(m.View(), "A validar")
			m.context.loading = false
			for i := 0; i < 35; i++ {
				m.context.projects = append(m.context.projects, azdo.Project{Name: fmt.Sprintf("project-%02d", i)})
			}
			m.context.setFocus(contextProjectFocus)
			check(m.View(), "2 · ESCOLHER PROJECTO", "SEL", "ÂMBITO", "ORGANIZAÇÃO", "Catálogo de toda a organização.", "PgUp/PgDn")
			m.context.projectCursor = len(m.context.projects)
			m.context.projects[len(m.context.projects)-1].Name = strings.Repeat("Projecto-", 12)
			m.context.err = strings.Repeat("Falha ao carregar. ", 15)
			check(m.View(), "╭─ ERRO", "enter abrir catálogo", "esc mudar organização")
			m, _ = pressApp(t, m, ":")
			check(m.View(), "Comando :", "╭─ ERRO")
			m.command.close()
			pipelines := make([]azdo.Pipeline, 20)
			for i := range pipelines {
				pipelines[i] = azdo.Pipeline{ID: i + 1, Name: fmt.Sprintf("pipeline-%02d", i), RepoName: "sample-repo"}
			}
			m = NewApp(nil, "sample-project", pipelines)
			u, _ = m.Update(size)
			m = u.(AppModel)
			check(m.View(), "╭─ SELECÇÃO E PESQUISA", "Procurar:", "Branch:", "╭─ PIPELINES", "╭─ ACÇÕES")
			if size.Width == 80 && size.Height == 24 {
				start, end := m.catalog.displayRange()
				if end-start < 6 {
					t.Fatalf("only %d pipeline rows", end-start)
				}
			}
		}
	}
}

func TestProjectPagingAndOpeningScope(t *testing.T) {
	mock := &azdo.MockClient{Pipelines: appFixtures()}
	for i := 0; i < 35; i++ {
		mock.Projects = append(mock.Projects, azdo.Project{Name: fmt.Sprintf("project-%02d", i)})
	}
	m := NewBootstrapApp(func(string) (azdo.Client, error) { return mock, nil }, ContextDefaults{Organization: "example-org"})
	u, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = u.(AppModel)
	m, cmd := pressApp(t, m, "enter")
	m, cmd = runAppCmd(t, m, cmd)
	m, _ = runAppCmd(t, m, cmd)
	for _, key := range []tea.KeyType{tea.KeyPgDown, tea.KeyEnd, tea.KeyHome, tea.KeyPgUp} {
		u, _ = m.Update(tea.KeyMsg{Type: key})
		m = u.(AppModel)
		switch key {
		case tea.KeyPgDown:
			if m.context.projectCursor <= 1 {
				t.Fatal("project page did not advance")
			}
		case tea.KeyEnd:
			if m.context.projectCursor != len(mock.Projects) || !strings.Contains(m.View(), "project-34") {
				t.Fatal("last project inaccessible")
			}
		case tea.KeyHome, tea.KeyPgUp:
			if m.context.projectCursor != 0 {
				t.Fatal("first project scope inaccessible")
			}
		}
	}
	m, _ = pressApp(t, m, "down")
	m.context.err = "Falha anterior"
	m, cmd = pressApp(t, m, "enter")
	if m.context.err != "" || !m.context.loading {
		t.Fatal("retry leaves stale error or lacks loading state")
	}
	m, cmd = runAppCmd(t, m, cmd)
	m, _ = runAppCmd(t, m, cmd)
	if m.Screen() != ScreenCatalog || m.project != "project-00" {
		t.Fatal("selected project did not open")
	}
}

func TestInitialCommandBurstPreservesOrganization(t *testing.T) {
	for _, pasted := range []bool{false, true} {
		m := NewBootstrapApp(nil, ContextDefaults{Organization: "example-org"})
		u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":q"), Paste: pasted})
		m = u.(AppModel)
		if !m.command.active || m.command.input.Value() != "q" || m.context.organization.Value() != "example-org" {
			t.Fatal("burst command changed organization or failed to open")
		}
		_, cmd := pressApp(t, m, "enter")
		assertQuit(t, cmd)
		m = NewBootstrapApp(nil, ContextDefaults{Organization: "https"})
		u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("://dev.azure.com/example-org"), Paste: pasted})
		m = u.(AppModel)
		if m.command.active || m.context.organization.Value() != "https://dev.azure.com/example-org" {
			t.Fatal("URL scheme burst was intercepted")
		}
	}
}

func TestBranchesBootstrapUsesProjectPanels(t *testing.T) {
	m := NewBranchesBootstrap(nil, ContextDefaults{Organization: "example-org"})
	u, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m = u.(AppModel)
	m.context.projects = []azdo.Project{{Name: "sample-project"}}
	m.context.setFocus(contextProjectFocus)
	view := ansi.Strip(m.View())
	if lipgloss.Width(view) > 60 || lipgloss.Height(view) > 24 || !strings.Contains(view, "abrir repositórios") || !strings.Contains(view, "Escolhe um projecto para gerir branches") || strings.Contains(view, "Catálogo de toda") {
		t.Fatalf("branch context is misleading or overflows:\n%s", view)
	}
	m.context.projectCursor = 1
	m.contextClient = &azdo.MockClient{}
	m, cmd := pressApp(t, m, "enter")
	m, _ = runAppCmd(t, m, cmd)
	if m.branchBrowser == nil || m.context.loading {
		t.Fatal("branch entry leaves context locked in loading")
	}
	u, _ = m.Update(branchExitMsg{})
	m = u.(AppModel)
	m, _ = pressApp(t, m, "up")
	if m.context.projectCursor != 0 {
		t.Fatal("cannot choose scope after returning from branches")
	}
}
