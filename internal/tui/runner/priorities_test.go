package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/muesli/termenv"
)

func TestPrioritiesBranchCommandBurstAndPaste(t *testing.T) {
	for _, pasted := range []bool{false, true} {
		for _, command := range []string{":q", ":quit"} {
			for _, busy := range []bool{false, true} {
				m := NewBranchDemo()
				m.busy = busy
				m, _ = branchKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(command), Paste: pasted})
				if !m.command.active || m.command.input.Value() != command[1:] {
					t.Fatalf("command %q paste=%v busy=%v did not open", command, pasted, busy)
				}
				m, cmd := branchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
				if busy {
					if cmd != nil || m.opCtx.Err() == nil || !strings.Contains(m.err, "Cancelamento pedido") {
						t.Fatal("busy command must cancel without quitting")
					}
				} else {
					assertQuit(t, cmd)
				}
			}
		}
	}
}

func TestPrioritiesBranchPanelsUseAvailableRows(t *testing.T) {
	m := NewBranchDemo()
	m.width, m.height = 80, 24
	view := ansi.Strip(m.View())
	for _, label := range []string{"SELECÇÃO E FILTROS", "ACÇÕES", "feat/catalog", "fix/layout"} {
		if !strings.Contains(view, label) {
			t.Fatalf("missing %q in compact branch view:\n%s", label, view)
		}
	}
	if lipgloss.Height(view) > 24 || lipgloss.Width(view) > 80 {
		t.Fatal("compact branch panels overflow")
	}
}

func TestPrioritiesProjectSearchOpensOnlyMatchingScope(t *testing.T) {
	mock := &azdo.MockClient{Pipelines: appFixtures()}
	for i := 0; i < 35; i++ {
		mock.Projects = append(mock.Projects, azdo.Project{ID: fmt.Sprintf("id-%02d", i), Name: fmt.Sprintf("project-%02d", i)})
	}
	m := NewBootstrapApp(func(string) (azdo.Client, error) { return mock, nil }, ContextDefaults{Organization: "example-org"})
	m, cmd := pressApp(t, m, "enter")
	m, cmd = runAppCmd(t, m, cmd)
	m, _ = runAppCmd(t, m, cmd)
	m, _ = pressApp(t, m, "/")
	m, _ = pressApp(t, m, "id-34")
	m, cmd = pressApp(t, m, "enter")
	if cmd != nil || m.Screen() != ScreenContext || m.context.selectedProject() != "project-34" {
		t.Fatal("ending search must retain the matching project without loading")
	}
	if strings.Contains(ansi.Strip(m.View()), "project-00") {
		t.Fatal("unmatched project remained visible")
	}
	m, cmd = pressApp(t, m, "enter")
	m, cmd = runAppCmd(t, m, cmd)
	m, _ = runAppCmd(t, m, cmd)
	if m.Screen() != ScreenCatalog || m.project != "project-34" || len(mock.ListPipelineProjects) != 1 || mock.ListPipelineProjects[0] != "project-34" {
		t.Fatal("search opened the wrong project or whole organization")
	}
}

func TestPrioritiesProjectSearchBurstAndPaste(t *testing.T) {
	for _, pasted := range []bool{false, true} {
		m := NewBootstrapApp(nil, ContextDefaults{Organization: "example-org"})
		m.context.setProjects([]azdo.Project{{ID: "id-00", Name: "first"}, {ID: "id-34", Name: "last"}})
		m.context.setFocus(contextProjectFocus)
		u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/id-34"), Paste: pasted})
		m = u.(AppModel)
		if !m.context.projectSearching || m.context.projectSearch.Value() != "id-34" || m.context.selectedProject() != "last" {
			t.Fatalf("burst search paste=%v did not open the matching scope", pasted)
		}
		m, cmd := pressApp(t, m, "enter")
		if cmd != nil || m.context.loading {
			t.Fatal("finishing burst search loaded a scope")
		}
	}
}

func TestPrioritiesEmptyProjectSearchCannotOpenAllProjects(t *testing.T) {
	m := NewBootstrapApp(nil, ContextDefaults{Organization: "example-org"})
	m.context.setProjects([]azdo.Project{{Name: "sample-project"}})
	m.context.setFocus(contextProjectFocus)
	m, _ = pressApp(t, m, "/")
	m, _ = pressApp(t, m, "inexistente")
	m, _ = pressApp(t, m, "esc")
	m, cmd := pressApp(t, m, "enter")
	if cmd != nil || m.context.loading || !strings.Contains(m.View(), "Nenhum projecto") {
		t.Fatal("empty search opened a scope")
	}
	m, _ = pressApp(t, m, "c")
	if !strings.Contains(m.View(), "Todos os projectos") || !strings.Contains(m.View(), "sample-project") {
		t.Fatal("clearing search did not restore project choices")
	}
}

type cancellableContextClient struct {
	azdo.MockClient
	started chan struct{}
	release chan struct{}
}

func (c *cancellableContextClient) ListPipelines(ctx context.Context, project string) ([]azdo.Pipeline, error) {
	close(c.started)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.release:
		return []azdo.Pipeline{{ID: 1, Name: "late pipeline", Project: project}}, nil
	}
}

func TestPrioritiesCancelQueuedContextSubmit(t *testing.T) {
	m := NewBootstrapApp(nil, ContextDefaults{Organization: "example-org"})
	m, queued := pressApp(t, m, "enter")
	if queued == nil || !m.context.loading {
		t.Fatal("Enter did not queue the context submission")
	}
	m, _ = pressApp(t, m, "esc")
	m, load := runAppCmd(t, m, queued)
	if load != nil || m.context.loading {
		t.Fatal("a cancelled queued submission restarted the read")
	}
	m, retry := pressApp(t, m, "enter")
	m, load = runAppCmd(t, m, queued)
	if load != nil {
		t.Fatal("old submission replaced the retry")
	}
	_, load = runAppCmd(t, m, retry)
	if load == nil {
		t.Fatal("current retry was rejected")
	}
}

type projectCountingClient struct {
	azdo.MockClient
	reads int
}

func (c *projectCountingClient) ListProjects(ctx context.Context) ([]azdo.Project, error) {
	c.reads++
	return c.MockClient.ListProjects(ctx)
}

func TestPrioritiesCancelledFactorySkipsProjectRead(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	mock := &projectCountingClient{MockClient: azdo.MockClient{Projects: []azdo.Project{{Name: "sample-project"}}}}
	m := NewBootstrapApp(func(string) (azdo.Client, error) {
		close(started)
		<-release
		return mock, nil
	}, ContextDefaults{Organization: "example-org"})
	m, cmd := pressApp(t, m, "enter")
	m, cmd = runAppCmd(t, m, cmd)
	token := m.active
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("factory did not start")
	}
	m, _ = pressApp(t, m, "esc")
	close(release)
	select {
	case message := <-result:
		u, _ := m.Update(message)
		m = u.(AppModel)
		if mock.reads != 0 || m.context.loading || len(m.context.projects) != 0 {
			t.Fatal("cancelled factory continued into project reading")
		}
	case <-time.After(time.Second):
		t.Fatal("released factory did not finish")
	}
	u, _ := m.Update(projectsLoadedMsg{token: token, organization: "example-org", client: mock, projects: mock.Projects})
	m = u.(AppModel)
	if len(m.context.projects) != 0 || m.context.focus != contextOrganizationFocus {
		t.Fatal("successful late project result reopened the selector")
	}
}

func TestPrioritiesCancelledAllProjectsSkipsReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mock := &azdo.MockClient{}
	_, err := listAllProjectPipelines(ctx, mock, []azdo.Project{{Name: "first"}, {Name: "second"}})
	if !errors.Is(err, context.Canceled) || len(mock.ListPipelineProjects) != 0 {
		t.Fatal("cancelled organization read invoked another project")
	}
}

func TestPrioritiesCancelContextReadAndIgnoreLateResult(t *testing.T) {
	client := &cancellableContextClient{started: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() { close(client.release) })
	m := NewBootstrapApp(nil, ContextDefaults{Organization: "example-org"})
	m.contextClient = client
	m.context.setProjects([]azdo.Project{{Name: "sample-project"}})
	m.context.projectCursor = 1
	m.context.setFocus(contextProjectFocus)
	m, cmd := pressApp(t, m, "enter")
	m, cmd = runAppCmd(t, m, cmd)
	token := m.active
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("read did not start")
	}
	m, _ = pressApp(t, m, "esc")
	if m.context.loading || m.context.selectedProject() != "sample-project" || !strings.Contains(m.View(), "Leitura cancelada") {
		t.Fatal("cancel did not restore the chosen scope")
	}
	select {
	case message := <-result:
		u, _ := m.Update(message)
		m = u.(AppModel)
		if m.Screen() != ScreenContext || m.context.err != "" || strings.Contains(m.View(), "late pipeline") {
			t.Fatal("cancelled result overwrote context")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not stop the read context")
	}
	u, _ := m.Update(contextLoadedMsg{token: token, organization: "example-org", project: "sample-project", client: client, pipelines: []azdo.Pipeline{{ID: 1, Name: "late pipeline"}}})
	m = u.(AppModel)
	if m.Screen() != ScreenContext || strings.Contains(m.View(), "late pipeline") {
		t.Fatal("successful late pipeline result reopened the catalog")
	}
}

func TestPrioritiesRecoveryPreservesTypedAndGenericDiagnostics(t *testing.T) {
	status, message := 403, "original diagnostic"
	for _, test := range []struct {
		err  error
		hint string
	}{
		{fmt.Errorf("wrapped: %w", azuredevops.WrappedError{StatusCode: &status, Message: &message}), "permissões"},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "rede"},
		{errors.New("adapter failed without HTTP status"), "login aprovado"},
	} {
		m := newContextModel(ContextDefaults{})
		m.setError(test.err)
		if m.err != test.err.Error() || !strings.Contains(m.recovery, test.hint) {
			t.Fatalf("recovery lost diagnostic or wrong cause: %q / %q", m.err, m.recovery)
		}
	}
}

func TestPrioritiesGroupedLayoutsAcrossThemes(t *testing.T) {
	profile, dark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	t.Cleanup(func() {
		lipgloss.SetColorProfile(profile)
		lipgloss.SetHasDarkBackground(dark)
	})
	for _, theme := range []struct {
		profile termenv.Profile
		dark    bool
	}{{termenv.ANSI256, true}, {termenv.ANSI256, false}, {termenv.Ascii, true}} {
		lipgloss.SetColorProfile(theme.profile)
		lipgloss.SetHasDarkBackground(theme.dark)
		for _, size := range []tea.WindowSizeMsg{{Width: 60, Height: 24}, {Width: 80, Height: 24}, {Width: 100, Height: 32}, {Width: 120, Height: 40}} {
			check := func(view string) {
				t.Helper()
				if lipgloss.Height(view) > size.Height || lipgloss.Width(view) > size.Width {
					t.Fatalf("overflow at %dx%d:\n%s", size.Width, size.Height, ansi.Strip(view))
				}
				if theme.profile == termenv.Ascii && strings.Contains(view, "\x1b[") {
					t.Fatal("plain view contains styling escapes")
				}
			}
			app := NewDemoApp()
			u, _ := app.Update(size)
			app = u.(AppModel)
			for i := range app.catalogActions() {
				index := i
				app.actions = &index
				check(app.View())
				if !strings.Contains(ansi.Strip(app.View()), fmt.Sprintf("%d/14", i+1)) {
					t.Fatal("active action counter missing")
				}
			}
			branches := NewBranchDemo()
			branches.width, branches.height = size.Width, size.Height
			branches.branches[1].Name = "refs/heads/" + strings.Repeat("long-branch-", 10)
			for _, stage := range []string{"list", "review", "results"} {
				branches.stage = stage
				branches.reviewed = branches.branches
				branches.results = []string{strings.Repeat("long-result ", 30)}
				branches.err = "Original diagnostic"
				check(branches.View())
			}
			branches.stage = "list"
			branches.command.start(size.Width)
			check(branches.View())
		}
	}
}

func TestPrioritiesConnectionErrorsGuideRecovery(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			errorMessage := "diagnóstico original"
			apiErr := &azuredevops.WrappedError{StatusCode: &status, Message: &errorMessage}
			m := NewBootstrapApp(func(string) (azdo.Client, error) { return nil, fmt.Errorf("auth: %w", apiErr) }, ContextDefaults{Organization: "example-org"})
			m, cmd := pressApp(t, m, "enter")
			m, cmd = runAppCmd(t, m, cmd)
			m, _ = runAppCmd(t, m, cmd)
			view := ansi.Strip(m.View())
			if !strings.Contains(view, errorMessage) || !strings.Contains(view, "COMO RECUPERAR") {
				t.Fatalf("diagnostic or recovery missing: %s", view)
			}
			checks := map[int]string{401: "Renova", 403: "permissões", 404: "organização", 429: "Aguarda"}
			if !strings.Contains(view, checks[status]) {
				t.Fatalf("wrong recovery for HTTP %d: %s", status, view)
			}
		})
	}
}

func TestPrioritiesActionGroupsRemainNavigable(t *testing.T) {
	m := NewDemoApp()
	u, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = u.(AppModel)
	m, _ = pressApp(t, m, "a")
	view := ansi.Strip(m.View())
	for _, group := range []string{"SELECCIONAR", "PIPELINE", "LOTE", "AVANÇADO", "PERFIS E HISTÓRICO", "CONTEXTO"} {
		if !strings.Contains(view, "│ "+group) {
			t.Fatalf("missing real group heading %q", group)
		}
	}
	for range 13 {
		m, _ = pressApp(t, m, "down")
	}
	m, _ = pressApp(t, m, "enter")
	if m.branchBrowser == nil {
		t.Fatal("last grouped action cannot open branch management")
	}
}
