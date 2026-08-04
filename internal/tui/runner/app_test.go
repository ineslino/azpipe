package runner

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ineslino/azpipe/internal/azdo"
	domainrunner "github.com/ineslino/azpipe/internal/runner"
)

func TestAppWorkflow_ReviewsSelectionAndQueuesOnlyExactConfirmation(t *testing.T) {
	mock := &azdo.MockClient{
		QueuedRuns: []azdo.PipelineRun{{ID: 9001, State: "inProgress", WebURL: "https://example.test/runs/9001"}},
	}
	model := NewApp(mock, "sample-project", appFixtures())

	model, _ = pressApp(t, model, " ")
	model, cmd := pressApp(t, model, "enter")
	model, cmd = runAppCmd(t, model, cmd)
	model, _ = runAppCmd(t, model, cmd)
	if model.Screen() != ScreenReview {
		t.Fatalf("screen after preview = %v, want review", model.Screen())
	}
	if !strings.Contains(model.View(), "READY") {
		t.Fatalf("ready review not rendered:\n%s", model.View())
	}

	model = typeApp(t, model, "executar")
	model, _ = pressApp(t, model, "enter")
	if model.Screen() != ScreenReview {
		t.Fatalf("screen after wrong confirmation = %v, want review", model.Screen())
	}
	if len(mock.QueueRequests) != 0 {
		t.Fatalf("queue requests after wrong confirmation = %d, want 0", len(mock.QueueRequests))
	}

	model, _ = pressApp(t, model, "ctrl+u")
	model = typeApp(t, model, "EXECUTAR")
	model, cmd = pressApp(t, model, "enter")
	model, cmd = runAppCmd(t, model, cmd)
	if model.Screen() != ScreenExecution {
		t.Fatalf("screen after confirmation = %v, want execution", model.Screen())
	}
	model, _ = runAppCmd(t, model, cmd)
	if len(mock.QueueRequests) != 1 {
		t.Fatalf("queue requests = %d, want 1", len(mock.QueueRequests))
	}
	if !strings.Contains(model.View(), "https://example.test/runs/9001") {
		t.Fatalf("run URL not rendered:\n%s", model.View())
	}
}

func TestAppWorkflow_ReviewStartsPendingBeforePreviewCompletes(t *testing.T) {
	model := NewApp(&azdo.MockClient{}, "sample-project", appFixtures())
	model, _ = pressApp(t, model, " ")
	model, cmd := pressApp(t, model, "enter")
	model, _ = runAppCmd(t, model, cmd)

	if model.Screen() != ScreenReview || !strings.Contains(model.View(), "CHECK") {
		t.Fatalf("review before preview completion must show CHECK:\n%s", model.View())
	}
}

func TestAppWorkflow_RejectsStaleContextResultAndWrongTarget(t *testing.T) {
	clients := map[string]*azdo.MockClient{
		"first":  {Pipelines: []azdo.Pipeline{{ID: 1, Name: "first pipeline"}}},
		"second": {Pipelines: []azdo.Pipeline{{ID: 2, Name: "second pipeline"}}},
	}
	model := NewBootstrapApp(func(organization string) (azdo.Client, error) {
		return clients[organization], nil
	}, ContextDefaults{})

	model.context.organization.SetValue("first")
	model.context.project.SetValue("A")
	updated, firstCmd := model.Update(contextSubmitMsg{organization: "first", project: "A"})
	model = updated.(AppModel)
	model.context.organization.SetValue("second")
	model.context.project.SetValue("B")
	updated, secondCmd := model.Update(contextSubmitMsg{organization: "second", project: "B"})
	model = updated.(AppModel)

	updated, _ = model.Update(firstCmd())
	model = updated.(AppModel)
	if model.Screen() != ScreenContext {
		t.Fatalf("stale context result changed screen to %v", model.Screen())
	}

	secondMsg := secondCmd().(contextLoadedMsg)
	wrongTarget := secondMsg
	wrongTarget.token.target = "forged-context-target"
	updated, _ = model.Update(wrongTarget)
	model = updated.(AppModel)
	if model.Screen() != ScreenContext {
		t.Fatalf("wrong context target changed screen to %v", model.Screen())
	}

	model.context.project.SetValue("EDITED")
	updated, _ = model.Update(secondMsg)
	model = updated.(AppModel)
	if model.Screen() != ScreenContext {
		t.Fatalf("result for edited context changed screen to %v", model.Screen())
	}
	model.context.project.SetValue("B")
	updated, _ = model.Update(secondMsg)
	model = updated.(AppModel)
	if model.Screen() != ScreenCatalog || !strings.Contains(model.View(), "second pipeline") {
		t.Fatalf("active context result not applied:\n%s", model.View())
	}
}

func TestAppWorkflow_RejectsStalePreviewAndWrongTarget(t *testing.T) {
	model := NewApp(&azdo.MockClient{}, "sample-project", appFixtures())
	model, _ = pressApp(t, model, " ")
	model, cmd := pressApp(t, model, "enter")
	model, oldPreviewCmd := runAppCmd(t, model, cmd)
	model, _ = pressApp(t, model, "esc")
	model, cmd = pressApp(t, model, "enter")
	model, currentPreviewCmd := runAppCmd(t, model, cmd)

	updated, _ := model.Update(oldPreviewCmd())
	model = updated.(AppModel)
	if !strings.Contains(model.View(), "CHECK") {
		t.Fatalf("stale preview changed active review:\n%s", model.View())
	}

	currentMsg := currentPreviewCmd().(previewFinishedMsg)
	wrongTarget := currentMsg
	wrongTarget.token.target = "forged-preview-target"
	updated, _ = model.Update(wrongTarget)
	model = updated.(AppModel)
	if !strings.Contains(model.View(), "CHECK") {
		t.Fatalf("wrong preview target changed active review:\n%s", model.View())
	}

	model.review.reviews[0].Selection.Branch = "other"
	updated, _ = model.Update(currentMsg)
	model = updated.(AppModel)
	if !strings.Contains(model.View(), "CHECK") {
		t.Fatalf("preview changed a review whose selection target was edited:\n%s", model.View())
	}
	model.review.reviews[0].Selection.Branch = "main"
	updated, _ = model.Update(currentMsg)
	model = updated.(AppModel)
	if !strings.Contains(model.View(), "READY") {
		t.Fatalf("active preview result not applied:\n%s", model.View())
	}
}

func TestAppWorkflow_RejectsStaleQueueConfirmationAndWrongTarget(t *testing.T) {
	mock := &azdo.MockClient{}
	model := NewApp(mock, "sample-project", appFixtures())
	model, _ = pressApp(t, model, " ")
	model, cmd := pressApp(t, model, "enter")
	model, cmd = runAppCmd(t, model, cmd)
	model, _ = runAppCmd(t, model, cmd)
	model = typeApp(t, model, "EXECUTAR")
	model, confirmCmd := pressApp(t, model, "enter")

	confirmMsg := confirmCmd().(queueConfirmedMsg)
	wrongTarget := confirmMsg
	wrongTarget.token.target = "forged-queue-target"
	updated, queueCmd := model.Update(wrongTarget)
	model = updated.(AppModel)
	if model.Screen() != ScreenReview || queueCmd != nil {
		t.Fatalf("wrong queue target advanced to %v with command %T", model.Screen(), queueCmd)
	}

	model.review.confirmation.SetValue("")
	updated, queueCmd = model.Update(confirmMsg)
	model = updated.(AppModel)
	if model.Screen() != ScreenReview || queueCmd != nil {
		t.Fatalf("stale confirmation text advanced to %v with command %T", model.Screen(), queueCmd)
	}
	model.review.confirmation.SetValue(confirmationValue)
	model, _ = pressApp(t, model, "esc")
	updated, queueCmd = model.Update(confirmMsg)
	model = updated.(AppModel)
	if model.Screen() != ScreenCatalog || queueCmd != nil || len(mock.QueueRequests) != 0 {
		t.Fatalf("stale queue confirmation was accepted: screen=%v cmd=%T requests=%d", model.Screen(), queueCmd, len(mock.QueueRequests))
	}
}

func TestAppWorkflow_PreviewErrorHidesConfirmationAndQuitIsFailClosed(t *testing.T) {
	mock := &azdo.MockClient{PreviewErr: errors.New("invalid YAML")}
	model := NewApp(mock, "sample-project", appFixtures())

	model, _ = pressApp(t, model, " ")
	model, cmd := pressApp(t, model, "enter")
	model, cmd = runAppCmd(t, model, cmd)
	model, _ = runAppCmd(t, model, cmd)

	view := model.View()
	if !strings.Contains(view, "ERROR") || strings.Contains(view, "EXECUTAR") {
		t.Fatalf("failed review must show ERROR without confirmation:\n%s", view)
	}
	_, quit := pressApp(t, model, "q")
	assertQuit(t, quit)
	if len(mock.QueueRequests) != 0 {
		t.Fatalf("queue requests after quit = %d, want 0", len(mock.QueueRequests))
	}
}

func TestAppWorkflow_EscapeReturnsToCatalogAndPreservesSelection(t *testing.T) {
	mock := &azdo.MockClient{}
	model := NewApp(mock, "sample-project", appFixtures())

	model, _ = pressApp(t, model, " ")
	model, cmd := pressApp(t, model, "enter")
	model, cmd = runAppCmd(t, model, cmd)
	model, _ = runAppCmd(t, model, cmd)
	model, _ = pressApp(t, model, "esc")

	if model.Screen() != ScreenCatalog {
		t.Fatalf("screen after escape = %v, want catalog", model.Screen())
	}
	if got := model.catalog.Selected(); len(got) != 1 || got[0].Pipeline.ID != 101 {
		t.Fatalf("selection after escape = %#v, want pipeline 101", got)
	}
}

func TestBootstrapApp_AcceptsContextWithoutFlagsAndListsPipelines(t *testing.T) {
	mock := &azdo.MockClient{Pipelines: appFixtures()}
	var gotOrganization string
	model := NewBootstrapApp(func(organization string) (azdo.Client, error) {
		gotOrganization = organization
		return mock, nil
	}, ContextDefaults{})

	model = typeApp(t, model, "example-org")
	model, _ = pressApp(t, model, "enter")
	model = typeApp(t, model, "sample-project")
	model, cmd := pressApp(t, model, "enter")
	model, cmd = runAppCmd(t, model, cmd)
	model, _ = runAppCmd(t, model, cmd)

	if model.Screen() != ScreenCatalog {
		t.Fatalf("screen after context load = %v, want catalog; view:\n%s", model.Screen(), model.View())
	}
	if gotOrganization != "example-org" {
		t.Fatalf("factory organization = %q, want example-org", gotOrganization)
	}
}

func TestBootstrapApp_ContextErrorsStayActionableAndFailClosed(t *testing.T) {
	factoryCalls := 0
	model := NewBootstrapApp(func(string) (azdo.Client, error) {
		factoryCalls++
		return nil, errors.New("credential unavailable")
	}, ContextDefaults{})

	model, _ = pressApp(t, model, "enter")
	if factoryCalls != 0 || !strings.Contains(model.View(), "Organização e projecto são obrigatórios") {
		t.Fatalf("missing context state unexpected: calls=%d view=%q", factoryCalls, model.View())
	}

	model = typeApp(t, model, "example-org")
	model, _ = pressApp(t, model, "enter")
	model = typeApp(t, model, "sample-project")
	model, cmd := pressApp(t, model, "enter")
	model, cmd = runAppCmd(t, model, cmd)
	model, _ = runAppCmd(t, model, cmd)
	if model.Screen() != ScreenContext || !strings.Contains(model.View(), "credential unavailable") {
		t.Fatalf("factory error must remain actionable on context:\n%s", model.View())
	}
	_, quit := pressApp(t, model, "ctrl+d")
	assertQuit(t, quit)
}

func TestBootstrapApp_ListErrorStaysOnContext(t *testing.T) {
	mock := &azdo.MockClient{Err: errors.New("project not found")}
	model := NewBootstrapApp(func(string) (azdo.Client, error) { return mock, nil }, ContextDefaults{
		Organization: "example-org",
		Project:      "UNKNOWN",
	})

	model, cmd := pressApp(t, model, "enter")
	model, cmd = runAppCmd(t, model, cmd)
	model, _ = runAppCmd(t, model, cmd)

	if model.Screen() != ScreenContext || !strings.Contains(model.View(), "não foi possível listar pipelines: project not found") {
		t.Fatalf("list error must remain actionable on context:\n%s", model.View())
	}
}

func TestExecution_RefreshesNonTerminalRunsAndShowsPartialFailure(t *testing.T) {
	mock := &azdo.MockClient{RunByID: map[int]azdo.PipelineRun{
		9001: {ID: 9001, State: "completed", Result: "succeeded", WebURL: "https://example.test/runs/9001"},
	}}
	selectionA := appSelection(appFixtures()[0])
	selectionB := appSelection(appFixtures()[1])
	model := NewApp(mock, "sample-project", appFixtures())
	model.screen = ScreenExecution
	model.execution = executionModel{
		queued: true,
		runs: []domainrunner.RunResult{
			{Review: domainrunner.Review{Selection: selectionA, State: domainrunner.ReviewReady}, Run: azdo.PipelineRun{ID: 9001, State: "inProgress", WebURL: "https://example.test/runs/9001"}},
			{Review: domainrunner.Review{Selection: selectionB, State: domainrunner.ReviewReady}, Err: errors.New("permission denied")},
		},
	}
	model.startOperation(runsOperationTarget(model.execution.runs))

	before := model.View()
	if !strings.Contains(before, "https://example.test/runs/9001") || !strings.Contains(before, "ERROR orders deploy: permission denied") {
		t.Fatalf("execution must expose URL and partial queue failure:\n%s", before)
	}
	updated, cmd := model.Update(refreshTickMsg{token: model.active})
	model = updated.(AppModel)
	model, next := runAppCmd(t, model, cmd)
	if next != nil {
		t.Fatal("completed runs must not schedule another refresh")
	}
	if !strings.Contains(model.View(), "COMPLETED billing deploy succeeded") {
		t.Fatalf("refreshed terminal run not rendered:\n%s", model.View())
	}
}

func TestExecution_QueueCannotBeInterruptedBeforeQueueFinished(t *testing.T) {
	for _, key := range []string{"q", "esc", "ctrl+c", "ctrl+d"} {
		t.Run(key, func(t *testing.T) {
			model := NewApp(&azdo.MockClient{}, "sample-project", appFixtures())
			model.screen = ScreenExecution
			model.execution = newExecutionModel()

			updated, cmd := pressApp(t, model, key)

			if updated.Screen() != ScreenExecution {
				t.Fatalf("screen after %s during queue = %v, want execution", key, updated.Screen())
			}
			if cmd != nil {
				if _, quit := cmd().(tea.QuitMsg); quit {
					t.Fatalf("%s terminated before queueFinishedMsg", key)
				}
				t.Fatalf("%s returned command %T before queueFinishedMsg, want nil", key, cmd())
			}
		})
	}
}

func TestExecution_QuitAfterQueueFinishedStopsLocalMonitoring(t *testing.T) {
	model := NewApp(&azdo.MockClient{}, "sample-project", appFixtures())
	model.screen = ScreenExecution
	model.execution = executionModel{queued: true}

	_, cmd := pressApp(t, model, "q")

	assertQuit(t, cmd)
}

func TestExecution_RetriesAfterTransientRefreshErrorUntilCompleted(t *testing.T) {
	mock := &azdo.MockClient{
		QueuedRuns: []azdo.PipelineRun{{ID: 9001, State: "inProgress", WebURL: "https://example.test/runs/9001"}},
		RunByID: map[int]azdo.PipelineRun{
			9001: {ID: 9001, State: "completed", Result: "succeeded", WebURL: "https://example.test/runs/9001"},
		},
	}
	model := NewApp(mock, "sample-project", appFixtures())
	model, _ = pressApp(t, model, " ")
	model, cmd := pressApp(t, model, "enter")
	model, cmd = runAppCmd(t, model, cmd)
	model, _ = runAppCmd(t, model, cmd)
	model = typeApp(t, model, "EXECUTAR")
	model, cmd = pressApp(t, model, "enter")
	model, cmd = runAppCmd(t, model, cmd)
	model, _ = runAppCmd(t, model, cmd)

	mock.GetRunErr = errors.New("temporary timeout")
	updated, refreshCmd := model.Update(refreshTickMsg{token: model.active})
	model = updated.(AppModel)
	model, retryTick := runAppCmd(t, model, refreshCmd)
	if retryTick == nil || !strings.Contains(model.View(), "temporary timeout") {
		t.Fatalf("transient refresh error must remain visible and schedule retry:\n%s", model.View())
	}

	mock.GetRunErr = nil
	updated, refreshCmd = model.Update(refreshTickMsg{token: model.active})
	model = updated.(AppModel)
	model, next := runAppCmd(t, model, refreshCmd)
	if next != nil || !strings.Contains(model.View(), "COMPLETED billing deploy succeeded") {
		t.Fatalf("retry did not reach terminal run:\n%s", model.View())
	}
}

func TestReview_RendersPipelineIDAndEffectiveParameters(t *testing.T) {
	model := NewApp(&azdo.MockClient{}, "sample-project", appFixtures())
	model, _ = pressApp(t, model, "p")
	model, cmd := pressApp(t, model, "enter")
	model, cmd = runAppCmd(t, model, cmd)
	model, _ = runAppCmd(t, model, cmd)

	view := model.View()
	for _, want := range []string{"101", "PLAN", "main", "planOnly=true"} {
		if !strings.Contains(view, want) {
			t.Fatalf("review missing %q:\n%s", want, view)
		}
	}
}

func TestDemoApp_LabelsReviewsAndNeverOffersExecution(t *testing.T) {
	model := NewDemoApp()
	model, _ = pressApp(t, model, " ")
	model, _ = pressApp(t, model, "down")
	model, _ = pressApp(t, model, " ")
	model, cmd := pressApp(t, model, "enter")
	model, _ = runAppCmd(t, model, cmd)

	if model.Screen() != ScreenReview {
		t.Fatalf("demo screen = %v, want review", model.Screen())
	}
	view := model.View()
	if strings.Count(view, "DEMO") != 2 || strings.Contains(view, "EXECUTAR") {
		t.Fatalf("demo review must show DEMO without execution confirmation:\n%s", view)
	}
}

func appFixtures() []azdo.Pipeline {
	return []azdo.Pipeline{
		{ID: 101, Name: "billing deploy", Folder: "/apps/billing", RepoName: "billing-api", Tags: []string{"owner:billing"}},
		{ID: 202, Name: "orders deploy", Folder: "/apps/orders", RepoName: "orders-api", Tags: []string{"owner:orders"}},
	}
}

func appSelection(pipeline azdo.Pipeline) domainrunner.Selection {
	return domainrunner.Selection{Pipeline: pipeline, Mode: domainrunner.ModeRun, Branch: "main"}
}

func pressApp(t *testing.T, model AppModel, key string) (AppModel, tea.Cmd) {
	t.Helper()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case " ":
		msg = tea.KeyMsg{Type: tea.KeySpace}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+u":
		msg = tea.KeyMsg{Type: tea.KeyCtrlU}
	case "ctrl+c":
		msg = tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+d":
		msg = tea.KeyMsg{Type: tea.KeyCtrlD}
	}
	updated, cmd := model.Update(msg)
	app, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("Update() model = %T, want AppModel", updated)
	}
	return app, cmd
}

func typeApp(t *testing.T, model AppModel, value string) AppModel {
	t.Helper()
	for _, r := range value {
		model, _ = pressApp(t, model, string(r))
	}
	return model
}

func runAppCmd(t *testing.T, model AppModel, cmd tea.Cmd) (AppModel, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected command")
	}
	updated, next := model.Update(cmd())
	app, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("command update model = %T, want AppModel", updated)
	}
	return app, next
}

func assertQuit(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("command message = %T, want tea.QuitMsg", cmd())
	}
}
