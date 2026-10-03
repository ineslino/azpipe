package runner

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestCommandEscapeRetainsHistoryUntilNextEscape(t *testing.T) {
	m, _ := pressApp(t, NewDemoApp(), "h")
	m, _ = pressApp(t, m, ":")
	before := lipgloss.Height(m.View())
	m, cmd := pressApp(t, m, "esc")
	if cmd != nil || m.command.active || m.library == nil || m.library.kind != "history" {
		t.Fatal("closing the command bar also closed history")
	}
	if lipgloss.Height(m.View()) != before {
		t.Fatal("closing commands changed the frame height")
	}
	m, _ = pressApp(t, m, "esc")
	if m.library != nil || m.screen != ScreenCatalog {
		t.Fatal("the next Escape did not return to the catalog")
	}
}

func TestCommandEscapePreservesAppOperations(t *testing.T) {
	for _, operation := range []string{"context", "preview", "schema"} {
		t.Run(operation, func(t *testing.T) {
			m := NewApp(nil, "fixture", appFixtures())
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			switch operation {
			case "context":
				m = NewBootstrapApp(nil, ContextDefaults{})
				m.context.loading, m.contextCancel = true, cancel
			case "preview":
				m.screen, m.review.previewing, m.previewCancel = ScreenReview, true, cancel
			case "schema":
				m.schemaCancel = cancel
			}
			generation, screen := m.generation, m.screen
			m, _ = pressApp(t, m, ":")
			if !m.command.active {
				t.Fatal("command bar did not open during the operation")
			}
			m, cmd := pressApp(t, m, "esc")
			if cmd != nil || m.command.active || ctx.Err() != nil || m.generation != generation || m.screen != screen {
				t.Fatal("closing the command bar cancelled or left the underlying operation")
			}
			m, _ = pressApp(t, m, "esc")
			if ctx.Err() == nil {
				t.Fatal("the next Escape did not cancel the operation")
			}
			if operation == "preview" && m.screen != ScreenCatalog {
				t.Fatal("the next Escape did not leave preview")
			}
		})
	}
}

func TestCommandEscapeRetainsBranchStageUntilNextEscape(t *testing.T) {
	for _, stage := range []string{"list", "review", "help", "busy"} {
		t.Run(stage, func(t *testing.T) {
			m := NewBranchDemo()
			t.Cleanup(m.cancel)
			switch stage {
			case "list":
				m.repos = append(m.repos, m.repo)
			case "review":
				m.stage = "review"
			case "help":
				m.showHelp = true
			case "busy":
				m.busy = true
			}
			before := m.stage
			m, _ = branchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
			if !m.command.active {
				t.Fatal("command bar did not open")
			}
			height := lipgloss.Height(m.View())
			m, cmd := branchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
			if cmd != nil || m.command.active || m.stage != before || m.opCtx.Err() != nil || (stage == "help" && !m.showHelp) {
				t.Fatal("closing the command bar also changed the branch screen or operation")
			}
			if lipgloss.Height(m.View()) != height {
				t.Fatal("closing commands changed the branch frame height")
			}
			m, _ = branchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
			switch stage {
			case "busy":
				if m.opCtx.Err() == nil {
					t.Fatal("the next Escape did not cancel")
				}
			case "help":
				if m.showHelp {
					t.Fatal("the next Escape did not close help")
				}
			case "list", "review":
				if m.stage == before {
					t.Fatal("the next Escape did not return one stage")
				}
			}
		})
	}
}
