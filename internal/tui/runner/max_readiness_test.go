package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
	domain "github.com/ineslino/azpipe/internal/runner"
)

func TestMaxDemoBranchReviewAcceptsAdvertisedQuit(t *testing.T) {
	m := NewBranchDemo()
	u, _ := m.Update(branchReviewed{token: m.generation, branches: m.branches[1:]})
	m = u.(BranchModel)
	if m.confirmation.Focused() {
		t.Fatal("hidden demo confirmation has focus")
	}
	m, _ = branchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":q"), Paste: true})
	_, cmd := branchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("advertised :q did not exit demo review")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("demo review command did not quit")
	}
}

type maxSlowPreview struct {
	azdo.MockClient
	started chan struct{}
	stopped chan struct{}
}

func (c *maxSlowPreview) PreviewPipeline(ctx context.Context, _ string, request azdo.RunRequest) error {
	if request.PipelineID == 101 {
		return nil
	}
	close(c.started)
	<-ctx.Done()
	close(c.stopped)
	return ctx.Err()
}

func TestMaxIncrementalPreviewCancelsAndRejectsLateMessages(t *testing.T) {
	client := &maxSlowPreview{started: make(chan struct{}), stopped: make(chan struct{})}
	m := NewApp(client, "sample-project", appFixtures())
	m, _ = pressApp(t, m, " ")
	m, _ = pressApp(t, m, "down")
	m, _ = pressApp(t, m, " ")
	u, cmd := m.Update(CatalogReviewMsg{Selections: m.catalog.Selected()})
	m = u.(AppModel)
	event := cmd().(previewProgressMsg)
	u, next := m.Update(event)
	m = u.(AppModel)
	if m.review.canExecute() || m.review.confirmation.Focused() || !strings.Contains(ansi.Strip(m.View()), "1/2 concluídas") {
		t.Fatal("partial preview enabled confirmation or hid progress")
	}
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("slow preview did not start")
	}
	m = typeApp(t, m, ":q")
	m, cmd = pressApp(t, m, "enter")
	if cmd != nil || !m.review.cancelled {
		t.Fatal("busy quit failed to cancel in place")
	}
	select {
	case <-client.stopped:
	case <-time.After(time.Second):
		t.Fatal("preview context was not cancelled")
	}
	u, cmd = m.Update(next())
	m = u.(AppModel)
	if cmd != nil || m.review.reviews[1].State != domain.ReviewPending || m.review.canExecute() {
		t.Fatal("late preview modified cancelled review")
	}
	m = typeApp(t, m, ":q")
	_, cmd = pressApp(t, m, "enter")
	assertQuit(t, cmd)
}

func TestMaxAllProgressStillRequiresFinalMessage(t *testing.T) {
	m := NewApp(&azdo.MockClient{}, "sample-project", appFixtures())
	m, _ = pressApp(t, m, " ")
	u, cmd := m.Update(CatalogReviewMsg{Selections: m.catalog.Selected()})
	m = u.(AppModel)
	u, next := m.Update(cmd())
	m = u.(AppModel)
	if m.review.canExecute() || m.review.reviews[0].State != domain.ReviewReady {
		t.Fatal("progress must update row while retaining final gate")
	}
	u, _ = m.Update(next())
	m = u.(AppModel)
	if !m.review.canExecute() || !m.review.confirmation.Focused() {
		t.Fatal("valid final result did not enable exact confirmation")
	}
}

func TestMaxOptionsInspectCancelAndConfirm(t *testing.T) {
	e, err := newSchemaEditor(maxSchema(t), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	e.update(tea.KeyMsg{Type: tea.KeyF2})
	e.update(tea.KeyMsg{Type: tea.KeyDown})
	e.update(tea.KeyMsg{Type: tea.KeyEsc})
	if e.rows[0].value.Value() != "dev" || !e.useDefault[0] {
		t.Fatal("inspection changed the parameter")
	}
	e.update(tea.KeyMsg{Type: tea.KeyF2})
	e.update(tea.KeyMsg{Type: tea.KeyDown})
	e.update(tea.KeyMsg{Type: tea.KeyEnter})
	if e.rows[0].value.Value() != "test" || e.useDefault[0] {
		t.Fatal("choice confirmation did not set an override")
	}
}

func TestMaxCatalogEscapeKeepsSearchAndExplicitClearWorks(t *testing.T) {
	m := catalogFixture()
	m.search.SetValue("202")
	m.filter()
	m.input = inputSearch
	m.search.Focus()
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.search.Value() != "202" || len(m.visible) != 1 || m.input != inputNone {
		t.Fatal("Esc discarded the filter")
	}
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !strings.Contains(ansi.Strip(m.helpView()), "ctrl+u limpar") {
		t.Fatal("clear action is hidden")
	}
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.search.Value() != "" || len(m.visible) != 3 {
		t.Fatal("explicit clear did not restore pipelines")
	}
}

func maxSchema(t *testing.T) azdo.ParameterSchema {
	t.Helper()
	s, err := azdo.ParseParameterSchema("parameters:\n- name: environment\n  displayName: Ambiente\n  default: dev\n  values: [dev, test, staging, prod, other]\n- name: replicas\n  displayName: Réplicas\n  type: number\n  default: 1\n")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMaxParameterValidationFocusesInvalidField(t *testing.T) {
	m := catalogFixture()
	e, err := newSchemaEditor(maxSchema(t), map[string]string{"replicas": "wrong"}, "")
	if err != nil {
		t.Fatal(err)
	}
	m.editor, m.input = e, inputParameterForm
	m = updateCatalog(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.editor.focus != 3 || !m.editor.rows[1].value.Focused() {
		t.Fatal("validation did not focus Réplicas")
	}
	view := ansi.Strip(m.editor.view(76, 22, "active"))
	if !strings.Contains(view, "pipeline activa") || strings.Contains(view, "aplica à selecção") {
		t.Fatal("save copy does not match per-pipeline scope")
	}
}

func TestMaxLibraryEmptyActionsAndHumanHistory(t *testing.T) {
	view := ansi.Strip((libraryModel{kind: "profiles"}).view(76, 22))
	if strings.Contains(view, "enter carregar") || !strings.Contains(view, "selecciona") {
		t.Fatal("empty profiles advertises load or omits recovery")
	}
	l := libraryModel{kind: "history", journals: []*domain.Journal{{Runs: []domain.JournalRecord{
		{PipelineID: 42, PipelineName: "payment QA", Run: azdo.PipelineRun{ID: 10, State: "completed", Result: "succeeded"}},
		{PipelineID: 43, PipelineName: "verify QA", Error: "submissão incerta"},
	}}}}
	view = ansi.Strip(l.view(76, 22))
	if !strings.Contains(view, "payment QA") || !strings.Contains(view, "incerta") {
		t.Fatal("history hides pipelines or uncertain submissions")
	}
}

func TestPolishContextKeepsProjectVisibleWithLongOrganization(t *testing.T) {
	m := NewApp(&azdo.MockClient{}, "projecto-de-destino", appFixtures())
	m.organization, m.width = "https://dev.azure.com/"+strings.Repeat("organization", 12), 60
	if !strings.Contains(ansi.Strip(m.contextHeader()), "Projecto: projecto-de-destino") {
		t.Fatal("long organization hides execution project")
	}
}

func TestPolishActionsAcceptAdvertisedAccelerator(t *testing.T) {
	m := NewDemoApp()
	m, _ = pressApp(t, m, "a")
	m, _ = pressApp(t, m, "/")
	if m.actions != nil || m.catalog.input != inputSearch {
		t.Fatal("advertised menu accelerator was ignored")
	}
}

func TestPolishParametersExplainHiddenFields(t *testing.T) {
	e, err := newSchemaEditor(maxSchema(t), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	view := ansi.Strip(e.schemaView(76, 19, "fixture"))
	if !strings.Contains(view, "Campo 1 de 2") || !strings.Contains(view, "Tab percorre todos") {
		t.Fatal("hidden parameter position/total is missing")
	}
}

func TestPolishReviewFullContextRemainsAccessible(t *testing.T) {
	m := NewApp(&azdo.MockClient{}, "projecto-de-destino", appFixtures())
	m.organization = "https://dev.azure.com/" + strings.Repeat("organization", 12)
	m, _ = pressApp(t, m, " ")
	m, cmd := pressApp(t, m, "enter")
	m, _ = runAppCmd(t, m, cmd)
	m.review.width, m.review.height, m.review.horizontal = 56, 22, 1000
	view := strings.ReplaceAll(ansi.Strip(m.review.view()), "\n", "")
	if !strings.Contains(view, m.organization) || !strings.Contains(view, "Projecto: projecto-de-destino") {
		t.Fatal("complete execution context is inaccessible in review details")
	}
}
