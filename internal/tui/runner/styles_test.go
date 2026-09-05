package runner

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ineslino/azpipe/internal/azdo"
	"strings"
	"testing"
)

func TestShortcutBarFitsTerminal(t *testing.T) {
	for _, width := range []int{40, 60, 80, 120} {
		m := catalogFixture()
		m.width = width
		for _, line := range strings.Split(m.helpView(), "\n") {
			if lipgloss.Width(line) > width {
				t.Fatalf("help exceeds %d columns: %s", width, line)
			}
		}
	}
}

func TestBorderedCatalogSectionsAndReviewPaging(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 32}} {
		pipelines := make([]azdo.Pipeline, 30)
		for i := range pipelines {
			pipelines[i] = azdo.Pipeline{ID: i + 1, Name: fmt.Sprintf("pipeline-%02d", i+1)}
		}
		m := NewApp(nil, "test", pipelines)
		m.demo = true
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = updated.(AppModel)
		for _, title := range []string{"╭─ PIPELINES", "╭─ DETALHE", "╭─ ACÇÕES"} {
			if !strings.Contains(m.View(), title) {
				t.Fatal("missing section", title)
			}
		}
		for _, p := range pipelines {
			m.catalog.selected[p.ID] = "RUN"
		}
		m, cmd := pressApp(t, m, "enter")
		m, _ = runAppCmd(t, m, cmd)
		seen := map[int]bool{}
		for page := 0; page < 30; page++ {
			view := m.View()
			if lipgloss.Height(view) > size[1] {
				t.Fatal("frame exceeds terminal height")
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size[0] {
					t.Fatal("frame exceeds terminal width", line)
				}
			}
			for _, p := range pipelines {
				if strings.Contains(view, p.Name) {
					seen[p.ID] = true
				}
			}
			old := m.review.offset
			m, _ = pressApp(t, m, "pgdown")
			if m.review.offset == old {
				break
			}
		}
		if len(seen) != 30 {
			t.Fatalf("framed review skips pipelines: %d/30", len(seen))
		}
	}
}

func TestSelectedRowKeepsFocusMarkerWithoutColour(t *testing.T) {
	m := catalogFixture()
	m.selected[101] = "PLAN"
	row := m.pipelineRow(m.pipelines[0], true)
	if !strings.HasPrefix(row, ">[x]") || !strings.Contains(row, "PLAN") {
		t.Fatalf("missing independent selection/focus/mode: %s", row)
	}
}

func TestCatalogFitsStandardTerminal(t *testing.T) {
	for _, width := range []int{60, 80, 120} {
		m := catalogFixture()
		m.width = width
		for _, line := range strings.Split(m.View(), "\n") {
			if lipgloss.Width(line) > width {
				t.Fatalf("catalog exceeds %d columns: %s", width, line)
			}
		}
		if lipgloss.Height(m.View()) > m.height-2 {
			t.Fatal("catalog overlaps application header")
		}
	}
}
