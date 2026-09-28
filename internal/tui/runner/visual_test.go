package runner

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestStyledCellsPreserveTextAndColumnWidths(t *testing.T) {
	profile := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	lipgloss.SetColorProfile(termenv.ANSI256)
	row := tableCells([]int{4, 4, 6}, brandLimeStyle.Render(" [x]"), planStyle.Render("PLAN"), catalogTitleStyle.Render("构建程序e\u0301-long"))
	if got := ansi.Strip(row); got != " [x] │ PLAN │ 构建… " {
		t.Fatalf("styled cells lost content or padding: %q", got)
	}
	if ansi.StringWidth(row) != 20 {
		t.Fatalf("styled table width = %d, want 20", ansi.StringWidth(row))
	}
	text := catalogActiveStyle.Render("produção e\u0301 pipeline")
	if got := ansi.Strip(truncateWidth(text, 11)); got != "produção e\u0301…" {
		t.Fatalf("truncation broke ANSI or a grapheme: %q", got)
	}
	// Every visible cell, including separators and padding, must keep the focus background.
	active := renderTableRow([]int{4, 4, 6}, []string{">[x]", "PLAN", "deploy"}, 0, true, map[int]lipgloss.Style{1: planStyle})
	focusedSpan := regexp.MustCompile(`\x1b\[[0-9;]*48;5;24m[^\x1b]*\x1b\[0m`)
	if ansi.StringWidth(active) != 20 || focusedSpan.ReplaceAllString(active, "") != "" {
		t.Fatalf("focus background interrupted by nested styling: %q", active)
	}
}

func TestVisualFlowPreservesSelectionAcrossThemesAndResize(t *testing.T) {
	profile, dark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	t.Cleanup(func() {
		lipgloss.SetColorProfile(profile)
		lipgloss.SetHasDarkBackground(dark)
	})
	for _, theme := range []struct {
		name    string
		profile termenv.Profile
		dark    bool
	}{{"dark", termenv.ANSI256, true}, {"light", termenv.ANSI256, false}, {"plain", termenv.Ascii, true}} {
		t.Run(theme.name, func(t *testing.T) {
			lipgloss.SetColorProfile(theme.profile)
			lipgloss.SetHasDarkBackground(theme.dark)
			m := NewDemoApp()
			for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 32}, {Width: 80, Height: 24}, {Width: 120, Height: 40}} {
				u, _ := m.Update(size)
				m = u.(AppModel)
				check := func(screen string) {
					t.Helper()
					view := m.View()
					if lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
						t.Fatalf("%s exceeds %dx%d: %dx%d\n%s", screen, size.Width, size.Height, lipgloss.Width(view), lipgloss.Height(view), ansi.Strip(view))
					}
					if theme.profile == termenv.Ascii && strings.Contains(view, "\x1b[") {
						t.Fatal("plain view contains styling escapes")
					}
				}
				m.catalog.cursor = 1
				if len(m.catalog.Selected()) == 0 {
					m, _ = pressApp(t, m, " ")
					m, _ = pressApp(t, m, "m")
				}
				m, _ = pressApp(t, m, "down")
				check("selected row and active row")
				for _, label := range []string{"[x]", "PLAN", "Repositório:", "Pasta:", "Tags:"} {
					if !strings.Contains(ansi.Strip(m.View()), label) {
						t.Fatalf("missing visible context %q", label)
					}
				}
				m, _ = pressApp(t, m, "/")
				m, _ = pressApp(t, m, strings.Repeat("inexistente", 16))
				check("empty search with hidden selection")
				if !strings.Contains(ansi.Strip(m.View()), "Nenhuma pipeline encontrada") {
					t.Fatal("missing empty state")
				}
				m, _ = pressApp(t, m, "esc")
				m, _ = pressApp(t, m, "a")
				check("actions")
				m, _ = pressApp(t, m, "esc")
				m, cmd := pressApp(t, m, "enter")
				m, _ = runAppCmd(t, m, cmd)
				check("review")
				m, _ = pressApp(t, m, "enter")
				check("monitoring")
				m, _ = pressApp(t, m, "down")
				check("monitoring focus moved")
				m, _ = pressApp(t, m, "esc")
				selected := m.catalog.Selected()
				if len(selected) != 1 || selected[0].Mode != "PLAN" || selected[0].Pipeline.ID != 202 {
					t.Fatalf("selection changed after recovery: %#v", selected)
				}
				m, _ = pressApp(t, m, "B")
				check("branches")
				m, cmd = pressApp(t, m, "esc")
				m, _ = runAppCmd(t, m, cmd)
			}
		})
	}
}
