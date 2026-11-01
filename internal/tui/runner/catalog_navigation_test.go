package runner

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ineslino/azpipe/internal/azdo"
)

func TestCatalogPageNavigationPreservesPreparation(t *testing.T) {
	pipelines := make([]azdo.Pipeline, 1000)
	for i := range pipelines {
		pipelines[i] = azdo.Pipeline{ID: i + 1, Name: fmt.Sprintf("pipeline-%04d", i+1)}
		if i%2 == 0 {
			pipelines[i].Tags = []string{"target"}
		}
	}
	pipelines[0].PlanContract = &azdo.PlanContract{Parameter: "planOnly", PlanValue: "true", RunValue: "false"}
	for _, size := range []tea.WindowSizeMsg{{Width: 60, Height: 24}, {Width: 80, Height: 24}, {Width: 120, Height: 40}} {
		t.Run(fmt.Sprintf("%dx%d", size.Width, size.Height), func(t *testing.T) {
			m := NewDemoApp()
			m.catalog = NewCatalogModel(pipelines)
			m, _ = actionKey(m, tea.KeyMsg{Type: tea.KeySpace})
			m, _ = pressApp(t, m, "m")
			m, _ = pressApp(t, m, "J")
			m, _ = pressApp(t, m, "ctrl+u")
			m, _ = pressApp(t, m, `{"environment":"dev"}`)
			m, _ = pressApp(t, m, "enter")
			m, _ = pressApp(t, m, "b")
			m, _ = pressApp(t, m, "ctrl+u")
			m, _ = pressApp(t, m, "release/prepared")
			m, _ = pressApp(t, m, "enter")
			prepared := m.catalog.Selected()
			pageSize := func() int {
				t.Helper()
				rangeLabel := regexp.MustCompile(`PIPELINES (\d+)–(\d+) /`).FindStringSubmatch(m.View())
				if len(rangeLabel) != 3 {
					t.Fatal("missing displayed pipeline range")
				}
				start, _ := strconv.Atoi(rangeLabel[1])
				end, _ := strconv.Atoi(rangeLabel[2])
				return end - start + 1
			}
			check := func(key tea.KeyType, want int) {
				t.Helper()
				var cmd tea.Cmd
				m, cmd = actionKey(m, tea.KeyMsg{Type: key})
				if cmd != nil || m.screen != ScreenCatalog || m.catalog.cursor != want {
					t.Fatalf("%s: cursor=%d, want %d; screen=%v, command=%v", tea.KeyMsg{Type: key}, m.catalog.cursor, want, m.screen, cmd != nil)
				}
				if !reflect.DeepEqual(m.catalog.Selected(), prepared) {
					t.Fatal("navigation changed the prepared selection")
				}
				if lipgloss.Width(m.View()) > m.width || lipgloss.Height(m.View()) > m.height {
					t.Fatal("navigation clipped the application frame")
				}
				if p, ok := m.catalog.active(); ok && !strings.Contains(m.View(), p.Name) {
					t.Fatal("focused pipeline is not rendered")
				}
			}
			updated, _ := m.Update(size)
			m = updated.(AppModel)
			m.catalog.notice = strings.Repeat("Contexto mantido. ", 12)
			check(tea.KeyPgDown, pageSize())
			check(tea.KeyPgUp, 0)
			check(tea.KeyEnd, len(pipelines)-1)
			check(tea.KeyPgDown, len(pipelines)-1)
			// Resizing changes the next page step, without changing the selection or focus.
			updated, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
			m = updated.(AppModel)
			check(tea.KeyPgUp, len(pipelines)-1-pageSize())
			check(tea.KeyHome, 0)
			check(tea.KeyPgUp, 0)
			m, _ = pressApp(t, m, "/target")
			m, _ = pressApp(t, m, "enter")
			check(tea.KeyEnd, len(m.catalog.visible)-1)
			if p, _ := m.catalog.active(); p.ID != 999 {
				t.Fatalf("End did not use filtered results: %#v", p)
			}
			m, _ = pressApp(t, m, "/")
			m, _ = pressApp(t, m, "ctrl+u")
			m, _ = pressApp(t, m, "missing")
			m, _ = pressApp(t, m, "esc")
			for _, key := range []tea.KeyType{tea.KeyHome, tea.KeyEnd, tea.KeyPgUp, tea.KeyPgDown} {
				check(key, 0)
			}
		})
	}
}

func TestCatalogNavigationKeysRetainInputEditing(t *testing.T) {
	m := catalogFixture()
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyDown})
	for _, input := range []string{"/", "b"} {
		m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(input)})
		for _, key := range []tea.KeyType{tea.KeyHome, tea.KeyEnd, tea.KeyPgUp, tea.KeyPgDown} {
			m = updateCatalog(t, m, tea.KeyMsg{Type: key})
			if m.cursor != 1 {
				t.Fatal("input editing moved the active pipeline")
			}
		}
		m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	}
}
