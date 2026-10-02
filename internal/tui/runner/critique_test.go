package runner

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestHiddenSelectionRemainsVisibleAndCanBeCleared(t *testing.T) {
	m := NewDemoApp()
	m, _ = pressApp(t, m, " ")
	m, _ = pressApp(t, m, "down")
	m, _ = pressApp(t, m, " ")
	m, _ = pressApp(t, m, "/")
	m, _ = pressApp(t, m, "infrastructure")
	if !strings.Contains(ansi.Strip(m.View()), "1 oculta") {
		t.Fatal("hidden selection missing during search")
	}
	m, _ = pressApp(t, m, "enter")
	m, _ = pressApp(t, m, "x")
	selected := m.catalog.Selected()
	if len(selected) != 1 || selected[0].Pipeline.ID != 202 {
		t.Fatalf("visible selection changed: %#v", selected)
	}
	m, _ = pressApp(t, m, "/")
	m, _ = pressApp(t, m, "no-match")
	if !strings.Contains(ansi.Strip(m.View()), "1 oculta") {
		t.Fatal("hidden selection missing with no results")
	}
}
