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

func TestManyParameterOptionsFitTheAppAndRetainValues(t *testing.T) {
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
		for _, count := range []int{9, 12, 50} {
			for _, size := range []tea.WindowSizeMsg{{Width: 60, Height: 24}, {Width: 80, Height: 24}, {Width: 120, Height: 40}} {
				t.Run(fmt.Sprintf("%v-dark%v-%d-%dx%d", theme.profile, theme.dark, count, size.Width, size.Height), func(t *testing.T) {
					options := make([]string, count)
					for i := range options {
						options[i] = strings.Repeat("destino-", 40) + fmt.Sprintf("fim-%02d", i)
					}
					editor, err := newSchemaEditor(azdo.ParameterSchema{Parameters: []azdo.Parameter{{
						Name: "environment", DisplayName: "Ambiente · " + strings.Repeat("produção 東京 ", 10), Type: "string",
						DefaultValue: options[0], HasDefault: true, Values: options,
					}}}, nil, "")
					if err != nil {
						t.Fatal(err)
					}
					m := NewBootstrapApp(func(string) (azdo.Client, error) {
						return &azdo.MockClient{Pipelines: appFixtures()}, nil
					}, ContextDefaults{Organization: "fixture-org", Project: "fixture"})
					updated, _ := m.Update(size)
					m = updated.(AppModel)
					updated, load := m.Update(contextSubmitMsg{organization: "fixture-org", project: "fixture"})
					m, _ = runAppCmd(t, updated.(AppModel), load)
					if m.screen != ScreenCatalog || m.catalog.height != size.Height-2 {
						t.Fatal("loaded catalog lost the header's reserved height")
					}
					m.catalog.input, m.catalog.editor = inputParameterForm, editor
					updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyF2})
					m = updated.(AppModel)
					check := func() string {
						t.Helper()
						view := m.View()
						if lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
							t.Fatalf("options overflow %dx%d: %dx%d\n%s", size.Width, size.Height, lipgloss.Width(view), lipgloss.Height(view), ansi.Strip(view))
						}
						plain := ansi.Strip(view)
						for _, label := range []string{"AZPIPE", "Valor completo", "PgUp/PgDn detalhe", "esc voltar"} {
							if !strings.Contains(plain, label) {
								t.Fatalf("missing %q", label)
							}
						}
						return plain
					}
					check()
					for i := 1; i < count; i++ {
						m.catalog.editor.update(tea.KeyMsg{Type: tea.KeyDown})
					}
					m.catalog.editor.optionScroll = 100
					if !strings.Contains(check(), fmt.Sprintf("fim-%02d", count-1)) {
						t.Fatal("the complete last option cannot be inspected")
					}
					m.catalog.editor.update(tea.KeyMsg{Type: tea.KeyEsc})
					if m.catalog.editor.rows[0].value.Value() != options[0] || !m.catalog.editor.useDefault[0] {
						t.Fatal("inspection changed the default")
					}
					m.catalog.editor.update(tea.KeyMsg{Type: tea.KeyF2})
					m.catalog.editor.optionCursor = count - 1
					m.catalog.editor.update(tea.KeyMsg{Type: tea.KeyEnter})
					values, err := m.catalog.editor.values()
					if err != nil || values["environment"] != options[count-1] {
						t.Fatalf("selection lost the complete option: %v %v", values, err)
					}
				})
			}
		}
	}
}

func TestLongParameterOptionsCanBeInspectedBeforeChoosing(t *testing.T) {
	prefix := strings.Repeat("destino-", 40)
	options := []string{prefix + "test", prefix + "prod"}
	for _, size := range [][2]int{{56, 20}, {76, 20}, {116, 36}} {
		e, err := newSchemaEditor(azdo.ParameterSchema{Parameters: []azdo.Parameter{{
			Name: "environment", DisplayName: "Ambiente", Type: "string",
			DefaultValue: options[0], HasDefault: true, Values: options,
		}}}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		e.update(tea.KeyMsg{Type: tea.KeyF2})
		check := func() string {
			t.Helper()
			view := e.view(size[0], size[1], "fixture")
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("option detail exceeds %dx%d: %dx%d", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
			}
			return ansi.Strip(view)
		}
		if !strings.Contains(check(), "PgUp/PgDn detalhe") {
			t.Fatal("long option detail has no inspection control")
		}
		for _, suffix := range []string{"test", "prod"} {
			for page := 0; page < 12 && !strings.Contains(check(), suffix); page++ {
				e.update(tea.KeyMsg{Type: tea.KeyPgDown})
			}
			if !strings.Contains(check(), suffix) {
				t.Fatalf("cannot inspect distinct %q suffix before choosing", suffix)
			}
			if e.rows[0].value.Value() != options[0] || !e.useDefault[0] {
				t.Fatal("inspection mutated the parameter")
			}
			if suffix == "test" {
				e.update(tea.KeyMsg{Type: tea.KeyDown})
				if e.optionScroll != 0 {
					t.Fatal("changing focus retained the previous option's scroll")
				}
			}
		}
		e.update(tea.KeyMsg{Type: tea.KeyEsc})
		if e.rows[0].value.Value() != options[0] || !e.useDefault[0] {
			t.Fatal("cancelling option inspection changed the default")
		}
		e.update(tea.KeyMsg{Type: tea.KeyF2})
		e.update(tea.KeyMsg{Type: tea.KeyDown})
		e.update(tea.KeyMsg{Type: tea.KeyEnter})
		values, err := e.values()
		if err != nil || values["environment"] != options[1] {
			t.Fatalf("confirmation lost the complete option: %v %v", values, err)
		}
	}
}
