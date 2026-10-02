package runner

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ineslino/azpipe/internal/azdo"
	domain "github.com/ineslino/azpipe/internal/runner"
)

func TestModeNeverSelectsImplicitly(t *testing.T) {
	m := catalogFixture()
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if len(m.Selected()) != 0 || m.warning == "" {
		t.Fatal("mode implicitly selected a pipeline")
	}
}

func TestCatalogSelectAllVisibleAndEscapeStaysInCatalog(t *testing.T) {
	m := catalogFixture()
	m.search.SetValue("api")
	m.filter()
	m.selected[domain.PipelineKey(m.visible[0])] = domain.ModePlan
	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	m = updated.(CatalogModel)
	if command != nil || len(m.Selected()) != len(m.visible) {
		t.Fatalf("select all selected %d of %d visible pipelines", len(m.Selected()), len(m.visible))
	}
	if m.selected[domain.PipelineKey(m.visible[0])] != domain.ModePlan {
		t.Fatal("select all changed an existing PLAN selection to RUN")
	}
	updated, command = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if command != nil || updated.(CatalogModel).input != inputNone {
		t.Fatal("escape unexpectedly quit or changed catalog input")
	}
}

func TestAppCommandQuitAndPlainQ(t *testing.T) {
	model := NewDemoApp()
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	model = updated.(AppModel)
	if command != nil || model.command.active {
		t.Fatal("plain q must not exit the TUI")
	}

	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	model = updated.(AppModel)
	if command == nil || !model.command.active {
		t.Fatal("colon did not open the command line")
	}
	for _, r := range "q" {
		updated, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(AppModel)
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(AppModel)
	if command == nil {
		t.Fatal("missing :q quit command")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf(":q returned %T, want tea.QuitMsg", command())
	}
}

func TestEscapeNeverQuitsAtRootOrWhileCommandIsOpen(t *testing.T) {
	model := NewBootstrapApp(nil, ContextDefaults{})
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(AppModel)
	if command != nil || model.Screen() != ScreenContext {
		t.Fatal("escape at the root context must not quit")
	}

	model = NewDemoApp()
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	model = updated.(AppModel)
	if command == nil || !model.command.active {
		t.Fatal("colon did not open the command line")
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(AppModel)
	if command != nil || model.command.active || model.Screen() != ScreenCatalog {
		t.Fatal("escape from the command line must return without quitting")
	}
}

func TestCommandLineFitsTerminal(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 100, Height: 32}} {
		model := NewDemoApp()
		updated, _ := model.Update(size)
		model = updated.(AppModel)
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
		model = updated.(AppModel)
		if got := lipgloss.Height(model.View()); got > size.Height {
			t.Fatalf("command view exceeds %dx%d: %d lines", size.Width, size.Height, got)
		}

		branches := NewBranchDemo()
		branches.width, branches.height = size.Width, size.Height
		updatedBranch, _ := branches.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
		branches = updatedBranch.(BranchModel)
		if got := lipgloss.Height(branches.View()); got > size.Height {
			t.Fatalf("branch command view exceeds %dx%d: %d lines", size.Width, size.Height, got)
		}

		workflow := NewDemoApp()
		updatedWorkflow, _ := workflow.Update(size)
		workflow = updatedWorkflow.(AppModel)
		workflow, _ = pressApp(t, workflow, " ")
		var command tea.Cmd
		workflow, command = pressApp(t, workflow, "enter")
		workflow, _ = runAppCmd(t, workflow, command)
		workflow, _ = pressApp(t, workflow, ":")
		if got := lipgloss.Height(workflow.View()); got > size.Height {
			t.Fatalf("review command view exceeds %dx%d: %d lines", size.Width, size.Height, got)
		}

		workflow = NewDemoApp()
		updatedWorkflow, _ = workflow.Update(size)
		workflow = updatedWorkflow.(AppModel)
		workflow.screen = ScreenExecution
		workflow.execution = executionModel{queued: true, width: size.Width - 4, height: size.Height - 2, demo: true}
		workflow, _ = pressApp(t, workflow, ":")
		if got := lipgloss.Height(workflow.View()); got > size.Height {
			t.Fatalf("execution command view exceeds %dx%d: %d lines", size.Width, size.Height, got)
		}

		workflow = NewDemoApp()
		updatedWorkflow, _ = workflow.Update(size)
		workflow = updatedWorkflow.(AppModel)
		workflow.openLibrary("history")
		workflow, _ = pressApp(t, workflow, ":")
		if got := lipgloss.Height(workflow.View()); got > size.Height {
			t.Fatalf("library command view exceeds %dx%d: %d lines", size.Width, size.Height, got)
		}
	}
}

func TestCommandPrefixDoesNotStealTextInput(t *testing.T) {
	model := NewDemoApp()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = updated.(AppModel)
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	model = updated.(AppModel)
	if command == nil || model.command.active || model.catalog.search.Value() != ":" {
		t.Fatalf("colon was intercepted in catalog input: active=%v value=%q command=%v", model.command.active, model.catalog.search.Value(), command != nil)
	}

	branches := NewBranchDemo()
	updatedBranch, _ := branches.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	branches = updatedBranch.(BranchModel)
	updatedBranch, command = branches.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	branches = updatedBranch.(BranchModel)
	if command == nil || branches.command.active || branches.filter.Value() != ":" {
		t.Fatalf("colon was intercepted in branch input: active=%v value=%q command=%v", branches.command.active, branches.filter.Value(), command != nil)
	}
}

func TestParameterFormSaveAndDiscard(t *testing.T) {
	m := catalogFixture()
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m.editor.rows[0].name.SetValue("environment")
	m.editor.rows[0].value.SetValue("dev")
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.parameters[domain.PipelineKey(azdo.Pipeline{ID: 101})]["environment"] != "dev" || m.input != inputNone {
		t.Fatal("form not saved")
	}
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m.editor.rows[0].value.SetValue("prod")
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.parameters[domain.PipelineKey(azdo.Pipeline{ID: 101})]["environment"] != "dev" {
		t.Fatal("discard changed saved parameters")
	}
}

func TestParameterFormRejectsDuplicateNames(t *testing.T) {
	e := newParameterEditor(map[string]string{"env": "dev"})
	e.rows = append(e.rows, newParameterField("env", "prod"))
	if _, err := e.values(); err == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestReviewPagesNeverSkipAndFit(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 32}} {
		selections := make([]domain.Selection, 30)
		for i := range selections {
			selections[i] = domain.Selection{Pipeline: azdo.Pipeline{ID: i + 1, Name: fmt.Sprintf("pipeline-%02d", i+1)}, Mode: domain.ModeRun, Branch: "main"}
		}
		m := newReviewModel(selections, true, operationToken{})
		m.width, m.height = size[0], size[1]
		seen := map[int]bool{}
		for page := 0; page < 30; page++ {
			view := m.view()
			if lipgloss.Height(view) > m.height-2 {
				t.Fatalf("height %d > %d\n%s", lipgloss.Height(view), m.height-2, view)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > m.width {
					t.Fatalf("line overflow: %s", line)
				}
			}
			for i := range selections {
				if strings.Contains(view, selections[i].Pipeline.Name) {
					seen[i] = true
				}
			}
			old := m.offset
			m, _ = m.update(tea.KeyMsg{Type: tea.KeyPgDown})
			if old == m.offset {
				break
			}
		}
		if len(seen) != 30 {
			t.Fatalf("only saw %d of 30", len(seen))
		}
	}
}

func TestInitialScreenCommandQuitPreservesOrganization(t *testing.T) {
	m := NewBootstrapApp(nil, ContextDefaults{Organization: "example-org"})
	m, _ = pressApp(t, m, ":")
	if !m.command.active {
		t.Fatal("initial screen does not open : command")
	}
	m, _ = pressApp(t, m, "q")
	m, cmd := pressApp(t, m, "enter")
	if cmd == nil {
		t.Fatal("missing quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal(":q did not quit")
	}
	if m.context.organization.Value() != "example-org" {
		t.Fatal("command modified organization")
	}
}

func TestInitialScreenURLAndCommandCancel(t *testing.T) {
	m := NewBootstrapApp(nil, ContextDefaults{})
	for _, r := range "https://dev.azure.com/example-org" {
		m, _ = pressApp(t, m, string(r))
	}
	if m.command.active || m.context.organization.Value() != "https://dev.azure.com/example-org" {
		t.Fatal("URL typing was intercepted")
	}
	m, _ = pressApp(t, m, ":")
	m, _ = pressApp(t, m, "esc")
	if m.command.active || m.context.organization.Value() != "https://dev.azure.com/example-org" {
		t.Fatal("command cancellation changed input")
	}
}
