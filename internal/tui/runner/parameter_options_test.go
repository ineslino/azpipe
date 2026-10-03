package runner

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
)

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
