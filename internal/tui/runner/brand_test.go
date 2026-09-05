package runner

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestPipelineBrandFitsAndIdentifiesCurrentStage(t *testing.T) {
	for _, width := range []int{60, 80, 120} {
		for active := 0; active < 3; active++ {
			line := pipelineBrand(width, active, true)
			plain := ansi.Strip(line)
			if lipgloss.Width(line) > width || !strings.Contains(plain, "AZPIPE") || strings.Count(plain, "◉") != 1 || strings.Count(plain, "○") != 2 {
				t.Fatalf("invalid brand at %d: %s", width, plain)
			}
		}
	}
}

func TestBrandKeepsTwoLineContextBudget(t *testing.T) {
	m := NewDemoApp()
	if lipgloss.Height(strings.TrimSuffix(m.contextHeader(), "\n")) != 2 {
		t.Fatal("brand steals table space")
	}
	if !strings.Contains(ansi.Strip(m.contextHeader()), "Contexto fictício") {
		t.Fatal("brand hides context")
	}
}

func TestWelcomeBannerFitsInitialScreen(t *testing.T) {
	brand := ansi.Strip(welcomeBrand())
	if !strings.Contains(brand, "AZPIPE") || !strings.Contains(brand, "Um só terminal.") || !strings.Contains(brand, "█") {
		t.Fatal("welcome banner missing name, description or lettering")
	}
	if lipgloss.Width(brand) > 76 || lipgloss.Height(brand) > 10 {
		t.Fatal("welcome banner exceeds initial screen budget")
	}
	for _, width := range []int{80, 120} {
		m := newContextModel(ContextDefaults{Organization: "example-org", Project: "sample-project"})
		m.err = "Organização e projecto são obrigatórios."
		view := section("LIGAÇÃO AO AZURE DEVOPS", m.view(), width)
		if lipgloss.Width(view) > width || lipgloss.Height(view) > 24 {
			t.Fatalf("welcome screen does not fit %dx24", width)
		}
		for _, text := range []string{"Organização:", "Projecto:", "enter", "esc"} {
			if !strings.Contains(ansi.Strip(view), text) {
				t.Fatalf("missing context control %q", text)
			}
		}
	}
}
