package runner

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ineslino/azpipe/internal/azdo"
	domain "github.com/ineslino/azpipe/internal/runner"
)

func TestSchemaFormChoicesDefaultsAndRequired(t *testing.T) {
	s, err := azdo.ParseParameterSchema("parameters:\n- name: env\n  type: string\n  default: dev\n  values: [dev, prod]\n- name: tests\n  type: boolean\n  default: false\n- name: count\n  type: number\n")
	if err != nil {
		t.Fatal(err)
	}
	e, err := newSchemaEditor(s, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.values(); err == nil {
		t.Fatal("missing required count accepted")
	}
	e.update(tea.KeyMsg{Type: tea.KeyRight})
	e.update(tea.KeyMsg{Type: tea.KeyTab})
	e.update(tea.KeyMsg{Type: tea.KeyRight})
	e.update(tea.KeyMsg{Type: tea.KeyTab})
	e.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	values, err := e.values()
	if err != nil || values["env"] != "prod" || values["tests"] != "true" || values["count"] != "3" {
		t.Fatal(values, err)
	}
	e.focus = 1
	e.focusField()
	e.update(tea.KeyMsg{Type: tea.KeyCtrlR})
	values, err = e.values()
	if err != nil {
		t.Fatal(err)
	}
	if _, sent := values["env"]; sent {
		t.Fatal("default should be omitted")
	}
}

func TestDemoProfilesAndHistoryNeverWriteOrQueue(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AZPIPE_DATA_DIR", root)
	m := NewDemoApp()
	m, _ = pressApp(t, m, " ")
	m, _ = pressApp(t, m, "s")
	m.library.name.SetValue("my-demo")
	m, _ = pressApp(t, m, "enter")
	if len(m.demoProfiles) != 1 || m.library != nil {
		t.Fatal("demo profile not saved")
	}
	m, _ = pressApp(t, m, " ")
	m, _ = pressApp(t, m, "l")
	m, _ = pressApp(t, m, "enter")
	if len(m.catalog.Selected()) != 1 || m.Screen() != ScreenCatalog {
		t.Fatal("profile queued or not loaded")
	}
	m, _ = pressApp(t, m, "h")
	m, cmd := pressApp(t, m, "enter")
	if cmd != nil || m.Screen() != ScreenExecution || len(m.execution.runs) != 3 {
		t.Fatal("demo resumed network operations")
	}
	if m.ExecutionError() != nil || !strings.Contains(m.View(), "Demonstração estática") {
		t.Fatal("demo claims live monitoring or fails on fixture states")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("demo wrote files")
	}
}

func TestHistoryResumeReadsOnlyAndPersistsResult(t *testing.T) {
	t.Setenv("AZPIPE_DATA_DIR", t.TempDir())
	j, err := domain.NewJournal("example", "project", []domain.Review{{Selection: domain.Selection{Pipeline: azdo.Pipeline{ID: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Record(0, domain.RunResult{Run: azdo.PipelineRun{ID: 42, State: "notStarted"}}); err != nil {
		t.Fatal(err)
	}
	c := &azdo.MockClient{RunByID: map[int]azdo.PipelineRun{42: {ID: 42, State: "completed", Result: "succeeded"}}}
	m := NewApp(c, "project", []azdo.Pipeline{{ID: 1}})
	m.organization = "example"
	m, _ = pressApp(t, m, "h")
	m, cmd := pressApp(t, m, "enter")
	if cmd == nil {
		t.Fatal("resume did not refresh")
	}
	m, _ = runAppCmd(t, m, cmd)
	if len(c.QueueRequests) != 0 || len(c.PreviewRequests) != 0 || m.execution.runs[0].Run.Result != "succeeded" {
		t.Fatal("resume performed wrong operation")
	}
	saved, err := domain.LoadJournal(j.Path(), "example", "project")
	if err != nil || saved.Runs[0].Run.Result != "succeeded" {
		t.Fatal(saved, err)
	}
}

func TestUpgradeViewsFitStandardSizes(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 32}} {
		m := NewDemoApp()
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = updated.(AppModel)
		assertView := func(view string) {
			if lipgloss.Height(view) > size[1] {
				t.Fatalf("height exceeds %v: %d\n%s", size, lipgloss.Height(view), view)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size[0] {
					t.Fatalf("width exceeds %v: %s", size, line)
				}
			}
		}
		assertView(m.View())
		m.demoProfiles = []domain.Profile{{Name: "test", Selections: []domain.ProfileSelection{{ID: 1}}}}
		m, _ = pressApp(t, m, "l")
		assertView(m.View())
		m, _ = pressApp(t, m, "esc")
		m, _ = pressApp(t, m, "s")
		assertView(m.View())
		m, _ = pressApp(t, m, "esc")
		m, cmd := pressApp(t, m, "e")
		m, _ = runAppCmd(t, m, cmd)
		assertView(m.View())
		m, _ = pressApp(t, m, "esc")
		m, _ = pressApp(t, m, "h")
		assertView(m.View())
		m, _ = pressApp(t, m, "enter")
		assertView(m.View())
	}
}
