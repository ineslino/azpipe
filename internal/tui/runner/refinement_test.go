package runner

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
	domain "github.com/ineslino/azpipe/internal/runner"
)

func TestRefinementCatalogSearchUsesSharedAdaptiveStyles(t *testing.T) {
	m := NewCatalogModel(demoPipelines())
	if m.search.PlaceholderStyle.GetForeground() != mutedColor || m.search.TextStyle.GetForeground() != textColor {
		t.Fatal("catalog search does not use the shared adaptive text styles")
	}
}

func TestRefinementPrimitiveTypeLabelsPreserveSchemaTypes(t *testing.T) {
	for _, field := range []struct{ kind, label string }{{"number", "número"}, {"string", "texto"}} {
		t.Run(field.kind, func(t *testing.T) {
			schema := azdo.ParameterSchema{Parameters: []azdo.Parameter{{Name: "value", DisplayName: "Valor", Type: field.kind, HasDefault: true, DefaultValue: "1"}}}
			e, err := newSchemaEditor(schema, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(ansi.Strip(e.view(76, 22, "pipeline")), "Valor ["+field.label) || e.schema.Parameters[0].Type != field.kind {
				t.Fatal("type label is not localized or changed the schema type")
			}
		})
	}
}

func TestRefinementReviewUsesFreeHeightAndKeepsControls(t *testing.T) {
	for _, size := range [][2]int{{60, 24}, {80, 24}, {120, 40}} {
		for _, count := range []int{1, 30} {
			for _, state := range []string{"demo", "ready", "pending", "blocked"} {
				for _, command := range []bool{false, true} {
					t.Run(fmt.Sprintf("%dx%d/%d/%s/command=%t", size[0], size[1], count, state, command), func(t *testing.T) {
						m := NewDemoApp()
						selections := make([]domain.Selection, count)
						for i := range selections {
							selections[i] = domain.Selection{Pipeline: azdo.Pipeline{ID: i + 1, Name: "pipeline", Project: "sample-project"}, Mode: domain.ModeRun, Branch: "main"}
						}
						m.review = newReviewModel(selections, state == "demo", operationToken{})
						m.review.organization, m.review.project = "example-org", "sample-project"
						for i := range m.review.reviews {
							r := &m.review.reviews[i]
							r.Request = r.Selection.Request()
							r.Request.Commit, r.Request.DefinitionVersion = strings.Repeat("a", 40), 7
							if state == "ready" {
								r.State = domain.ReviewReady
							}
							if state == "blocked" {
								r.State, r.Err = domain.ReviewError, errors.New(strings.Repeat("Erro de preview. ", 30))
								r.Request.Parameters = map[string]string{"long": strings.Repeat("value ", 30)}
							}
						}
						m.screen = ScreenReview
						updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
						m = updated.(AppModel)
						if command {
							m, _ = pressApp(t, m, ":")
							if !m.command.active {
								t.Fatal("command bar did not open")
							}
						}
						view := m.View()
						text := ansi.Strip(view)
						if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || !strings.Contains(text, "esc voltar e editar") {
							t.Fatalf("review or recovery controls exceed %v: %dx%d\n%s", size, lipgloss.Width(view), lipgloss.Height(view), text)
						}
						if state == "ready" && !strings.Contains(text, "Confirmação:") {
							t.Fatal("review hid execution confirmation")
						}
						if size[1] == 40 && count == 1 && (state == "demo" || state == "ready") {
							for _, label := range []string{"Definição: 7", "Valores predefinidos:"} {
								if !strings.Contains(text, label) {
									t.Fatalf("tall review hid %q despite free height", label)
								}
							}
						}
						if state == "ready" && size[1] == 40 && count == 1 && !strings.Contains(text, "Organização: example-org") {
							t.Fatal("tall review hid ownership")
						}
					})
				}
			}
		}
	}
}

func TestRefinementProfileRecoveryFitsLongErrorsAndLists(t *testing.T) {
	for _, size := range [][2]int{{60, 24}, {80, 24}, {120, 40}} {
		for _, count := range []int{1, 30} {
			m := NewDemoApp()
			for i := range count {
				m.demoProfiles = append(m.demoProfiles, domain.Profile{Name: fmt.Sprintf("perfil-%d", i)})
			}
			m, _ = pressApp(t, m, "l")
			m.library.cursor = count - 1
			m.library.err = strings.Repeat("Pipeline já não está no catálogo. ", 20)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = updated.(AppModel)
			for _, command := range []bool{false, true} {
				if command {
					m, _ = pressApp(t, m, ":")
				}
				view := m.View()
				text := ansi.Strip(view)
				hint := "Enter volta a tentar"
				if count > 1 {
					hint = "Escolhe outro perfil"
				}
				if lipgloss.Height(view) > size[1] || lipgloss.Width(view) > size[0] || !strings.Contains(text, hint) || !strings.Contains(text, "enter carregar") {
					t.Fatalf("profile recovery exceeds %v or hides actions (command=%v)\n%s", size, command, text)
				}
			}
		}
	}
}
