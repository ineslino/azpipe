package runner

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
	domain "github.com/ineslino/azpipe/internal/runner"
)

func TestContinuityParameterEditingSendsTheVisibleValue(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyCtrlW}, {Type: tea.KeyCtrlH}, {Type: tea.KeyBackspace, Alt: true}} {
		t.Run(key.String(), func(t *testing.T) {
			schema, err := azdo.ParseParameterSchema("parameters:\n- name: label\n  type: string\n  default: alpha beta\n")
			if err != nil {
				t.Fatal(err)
			}
			editor, err := newSchemaEditor(schema, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			m := NewDemoApp()
			m, _ = pressApp(t, m, " ")
			m.catalog.editor, m.catalog.input = editor, inputParameterForm
			u, _ := m.Update(key)
			m = u.(AppModel)
			visible := m.catalog.editor.rows[0].value.Value()
			if visible == "alpha beta" {
				t.Fatal("editing key did not change the fixture")
			}
			u, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
			m = u.(AppModel)
			if got, sent := m.catalog.Selected()[0].Request().Parameters["label"]; !sent || got != visible {
				t.Fatalf("visible value %q not sent: value=%q sent=%v", visible, got, sent)
			}
		})
	}
}

func TestContinuityCursorMovementKeepsThePipelineDefault(t *testing.T) {
	schema, err := azdo.ParseParameterSchema("parameters:\n- name: label\n  type: string\n  default: alpha beta\n")
	if err != nil {
		t.Fatal(err)
	}
	editor, err := newSchemaEditor(schema, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyLeft}, {Type: tea.KeyCtrlA}, {Type: tea.KeyCtrlE}} {
		editor.update(key)
	}
	values, err := editor.values()
	if err != nil || len(values) != 0 || !editor.useDefault[0] {
		t.Fatal("cursor movement created an override", values, err)
	}
}

func TestContinuityReopeningTheSameProjectPreservesPreparation(t *testing.T) {
	client := &azdo.MockClient{Projects: []azdo.Project{{Name: "sample-project"}}, Pipelines: appFixtures()}
	m := NewBootstrapApp(func(string) (azdo.Client, error) { return client, nil }, ContextDefaults{Organization: "example-org", Project: "sample-project"})
	m, cmd := pressApp(t, m, "enter")
	m, cmd = runAppCmd(t, m, cmd)
	m, _ = runAppCmd(t, m, cmd)
	m, cmd = pressApp(t, m, "enter")
	m, cmd = runAppCmd(t, m, cmd)
	m, _ = runAppCmd(t, m, cmd)
	m, _ = pressApp(t, m, " ")
	m, _ = pressApp(t, m, "down")
	m, _ = pressApp(t, m, " ")
	m.catalog.branch.SetValue("release/prepared")
	m.catalog.parameters[domain.PipelineKey(m.catalog.pipelines[0])] = map[string]string{"environment": "qa"}
	m.catalog.branches[domain.PipelineKey(m.catalog.pipelines[1])] = "release/orders"
	m.catalog.search.SetValue("billing")
	m.catalog.filter()
	before := m.catalog.Selected()
	reads := len(client.ListPipelineProjects)
	m, _ = pressApp(t, m, "c")
	m, cmd = pressApp(t, m, "enter")
	m, cmd = runAppCmd(t, m, cmd)
	if cmd != nil {
		m, _ = runAppCmd(t, m, cmd)
	}
	if m.screen != ScreenCatalog || !reflect.DeepEqual(before, m.catalog.Selected()) || m.catalog.search.Value() != "billing" || m.catalog.branch.Value() != "release/prepared" {
		t.Fatal("reopening the current context discarded preparation")
	}
	if len(client.ListPipelineProjects) != reads {
		t.Fatal("reopening the unchanged context needlessly reloaded the catalog")
	}
}

func TestContinuityInvalidProfileDoesNotBlockAValidProfile(t *testing.T) {
	m := NewDemoApp()
	m, _ = pressApp(t, m, " ")
	before := m.catalog.Selected()
	m.demoProfiles = []domain.Profile{
		{Version: 1, Name: "removed", Organization: "demo", Project: m.project, Selections: []domain.ProfileSelection{{ID: 9999, Mode: domain.ModeRun, Branch: "main"}}},
		{Version: 1, Name: "valid", Organization: "demo", Project: m.project, Selections: []domain.ProfileSelection{{ID: 202, Mode: domain.ModeRun, Branch: "release/valid"}}},
	}
	m, _ = pressApp(t, m, "l")
	m, _ = pressApp(t, m, "enter")
	if m.library == nil || m.library.err == "" || !reflect.DeepEqual(before, m.catalog.Selected()) {
		t.Fatal("invalid profile did not retain the current preparation with an error")
	}
	for _, label := range []string{"Escolhe outro perfil", "↑/↓", "enter carregar"} {
		if !strings.Contains(ansi.Strip(m.View()), label) {
			t.Fatalf("invalid profile hid its recovery action %q", label)
		}
	}
	m, _ = pressApp(t, m, "down")
	m, _ = pressApp(t, m, "enter")
	if m.library != nil || len(m.catalog.Selected()) != 1 || m.catalog.Selected()[0].ID() != 202 || m.catalog.Selected()[0].Branch != "release/valid" {
		t.Fatal("valid profile remained blocked after an invalid profile")
	}
}

func TestContinuityEscapeDiscardsALateParameterForm(t *testing.T) {
	m := NewDemoApp()
	m, late := pressApp(t, m, "e")
	if late == nil {
		t.Fatal("schema read was not queued")
	}
	m, _ = pressApp(t, m, "esc")
	u, _ := m.Update(late())
	m = u.(AppModel)
	if m.catalog.input != inputNone || !strings.Contains(m.catalog.notice, "cancelada") {
		t.Fatal("cancelled schema read opened a late form or omitted recovery")
	}
}

type continuitySlowSchema struct {
	azdo.MockClient
	started chan context.Context
	release chan struct{}
}

func (c *continuitySlowSchema) GetPipelineSchema(ctx context.Context, _ string, _ int, _ string) (azdo.ParameterSchema, error) {
	c.started <- ctx
	select {
	case <-ctx.Done():
		return azdo.ParameterSchema{}, ctx.Err()
	case <-c.release:
		return azdo.ParameterSchema{}, nil
	}
}

func TestContinuityEscapeStopsTheParameterRead(t *testing.T) {
	client := &continuitySlowSchema{started: make(chan context.Context, 1), release: make(chan struct{})}
	t.Cleanup(func() { close(client.release) })
	m := NewApp(client, "sample-project", appFixtures())
	m, cmd := pressApp(t, m, "e")
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	var ctx context.Context
	select {
	case ctx = <-client.started:
	case <-time.After(time.Second):
		t.Fatal("parameter read did not start")
	}
	m, _ = pressApp(t, m, "esc")
	if ctx.Err() != context.Canceled {
		t.Fatal("Escape did not cancel the active schema request")
	}
	select {
	case msg := <-result:
		u, _ := m.Update(msg)
		m = u.(AppModel)
		if m.catalog.input != inputNone || m.catalog.warning != "" {
			t.Fatal("cancelled request changed the current catalog")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled schema request did not finish")
	}
}

type continuitySession struct {
	azdo.MockClient
	label string
}

func (c *continuitySession) GetPipelineSchema(context.Context, string, int, string) (azdo.ParameterSchema, error) {
	return azdo.ParameterSchema{Parameters: []azdo.Parameter{{Name: "session", Type: "string", HasDefault: true, DefaultValue: c.label}}}, nil
}

func TestContinuityReauthenticationUsesTheCurrentClient(t *testing.T) {
	first := &continuitySession{MockClient: azdo.MockClient{Projects: []azdo.Project{{Name: "sample-project"}}, Pipelines: appFixtures()}, label: "first"}
	second := &continuitySession{MockClient: azdo.MockClient{Projects: first.Projects, Pipelines: appFixtures()}, label: "second"}
	calls := 0
	m := NewBootstrapApp(func(string) (azdo.Client, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return second, nil
	}, ContextDefaults{Organization: "example-org", Project: "sample-project"})
	m, cmd := pressApp(t, m, "enter")
	m, cmd = runAppCmd(t, m, cmd)
	m, _ = runAppCmd(t, m, cmd)
	m, cmd = pressApp(t, m, "enter")
	m, cmd = runAppCmd(t, m, cmd)
	m, _ = runAppCmd(t, m, cmd)
	m, _ = pressApp(t, m, " ")
	m.catalog.branch.SetValue("release/prepared")
	before := m.catalog.Selected()
	m, _ = pressApp(t, m, "c")
	m, _ = pressApp(t, m, "esc")
	m, cmd = pressApp(t, m, "enter")
	m, cmd = runAppCmd(t, m, cmd)
	m, _ = runAppCmd(t, m, cmd)
	m, cmd = pressApp(t, m, "enter")
	m, cmd = runAppCmd(t, m, cmd)
	if cmd != nil {
		m, _ = runAppCmd(t, m, cmd)
	}
	schema, err := m.service.Schema(context.Background(), 101, "main")
	if err != nil || schema.Parameters[0].DefaultValue != "second" || !reflect.DeepEqual(before, m.catalog.Selected()) {
		t.Fatal("reauthentication retained the old service or discarded preparation", schema, err)
	}
}

func TestContinuityOtherActionsCancelThePendingForm(t *testing.T) {
	for _, key := range []string{"a", "?", "B", "/", "b", "down", "e"} {
		t.Run(key, func(t *testing.T) {
			m := NewDemoApp()
			m, old := pressApp(t, m, "e")
			m, fresh := pressApp(t, m, key)
			msg := old().(schemaLoadedMsg)
			if msg.err != context.Canceled {
				t.Fatal("previous schema read was not cancelled")
			}
			u, _ := m.Update(msg)
			m = u.(AppModel)
			if m.catalog.input == inputParameterForm {
				t.Fatal("old schema response opened a form after another action")
			}
			if key == "e" {
				m, _ = runAppCmd(t, m, fresh)
				if m.catalog.input != inputParameterForm {
					t.Fatal("old response discarded the new request")
				}
			} else if m.schemaCancel != nil || strings.Contains(m.catalog.notice, "A ler parâmetros") {
				t.Fatal("cancelled read still appears pending")
			}
		})
	}
}

func TestContinuitySearchBurstAndPaste(t *testing.T) {
	for _, pasted := range []bool{false, true} {
		m := NewDemoApp()
		u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/build"), Paste: pasted})
		m = u.(AppModel)
		if m.catalog.input != inputSearch || m.catalog.search.Value() != "build" || len(m.catalog.visible) != 1 {
			t.Fatalf("burst search paste=%v was ignored", pasted)
		}
		m, _ = pressApp(t, m, "esc")
		if m.catalog.search.Value() != "build" || len(m.catalog.visible) != 1 {
			t.Fatal("Escape discarded the burst search")
		}
	}
}

func TestContinuityBatchDiagnosticsRemainFullyReadable(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 60, Height: 24}, {Width: 80, Height: 24}, {Width: 120, Height: 40}} {
		for _, persisted := range []bool{false, true} {
			m := NewApp(nil, "sample-project", nil)
			m.screen = ScreenExecution
			m.execution.queued = true
			u, _ := m.Update(size)
			m = u.(AppModel)
			diagnostic := strings.Repeat("directório de dados sem permissão ", 12) + "CORRIGIR_PERMISSOES"
			if persisted {
				m.execution.persistErr = errors.New(diagnostic)
			} else {
				m.execution.err = errors.New(diagnostic)
			}
			found := false
			for offset := 0; offset < len(diagnostic)+40; offset += 20 {
				m.execution.horizontal = offset
				view := ansi.Strip(m.View())
				if lipgloss.Height(view) > size.Height || lipgloss.Width(view) > size.Width || !strings.Contains(view, "esc catálogo") {
					t.Fatal("batch error hid navigation or overflowed the terminal")
				}
				found = found || strings.Contains(view, "CORRIGIR_PERMISSOES")
			}
			if !found {
				t.Fatalf("batch diagnostic tail inaccessible at %dx%d, persistence=%v", size.Width, size.Height, persisted)
			}
		}
	}
}

func TestContinuityReviewStatesUsePortugueseLabels(t *testing.T) {
	for state, label := range map[domain.ReviewState]string{domain.ReviewPending: "Pendente", domain.ReviewReady: "Pronta", domain.ReviewError: "Bloqueada"} {
		m := newReviewModel([]domain.Selection{appSelection(appFixtures()[0])}, false, operationToken{})
		m.reviews[0].State = state
		if state == domain.ReviewError {
			m.reviews[0].Err = errors.New("sem acesso")
		}
		if !strings.Contains(ansi.Strip(m.view()), label) {
			t.Fatalf("review state %s has no Portuguese label %q", state, label)
		}
	}
}
