package runner

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ineslino/azpipe/internal/azdo"
	domainrunner "github.com/ineslino/azpipe/internal/runner"
)

// Screen identifies the active workflow screen.
type Screen int

const (
	ScreenContext Screen = iota
	ScreenCatalog
	ScreenReview
	ScreenExecution
)

// AppModel integrates context, catalog, review, and execution into one Bubble Tea model.
type AppModel struct {
	screen    Screen
	factory   ClientFactory
	service   domainrunner.Service
	context   contextModel
	catalog   CatalogModel
	review    reviewModel
	execution executionModel
	demo      bool
}

// NewApp starts an authenticated workflow at the pipeline catalog.
func NewApp(client azdo.Client, project string, pipelines []azdo.Pipeline) AppModel {
	return AppModel{
		screen:  ScreenCatalog,
		service: domainrunner.NewService(client, project),
		catalog: NewCatalogModel(pipelines),
	}
}

// NewBootstrapApp starts at a non-secret organization and project context screen.
func NewBootstrapApp(factory ClientFactory, defaults ContextDefaults) AppModel {
	return AppModel{
		screen:  ScreenContext,
		factory: factory,
		context: newContextModel(defaults),
	}
}

// NewDemoApp creates a fully offline catalog and review workflow.
func NewDemoApp() AppModel {
	model := NewApp(nil, "DEMO", demoPipelines())
	model.demo = true
	return model
}

// Screen returns the active screen for integration and tests.
func (m AppModel) Screen() Screen {
	return m.screen
}

// Init starts the cursor for context inputs when required.
func (m AppModel) Init() tea.Cmd {
	if m.screen == ScreenContext {
		return m.context.organization.Cursor.BlinkCmd()
	}
	return nil
}

// Update routes messages through the active screen and workflow transitions.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && (key.Type == tea.KeyCtrlC || key.Type == tea.KeyCtrlD) {
		return m, tea.Quit
	}

	switch typed := msg.(type) {
	case contextSubmitMsg:
		return m, loadContext(m.factory, typed.organization, typed.project)
	case contextLoadedMsg:
		if typed.err != nil {
			m.context.err = typed.err.Error()
			return m, nil
		}
		m.service = domainrunner.NewService(typed.client, typed.project)
		m.catalog = NewCatalogModel(typed.pipelines)
		m.screen = ScreenCatalog
		return m, nil
	case CatalogReviewMsg:
		m.review = newReviewModel(typed.Selections, m.demo)
		m.screen = ScreenReview
		if m.demo {
			return m, nil
		}
		return m, previewSelections(m.service, typed.Selections)
	case previewFinishedMsg:
		m.review.reviews = typed.reviews
		if m.review.canExecute() {
			return m, m.review.confirmation.Focus()
		}
		return m, nil
	case queueConfirmedMsg:
		m.execution = newExecutionModel()
		m.screen = ScreenExecution
		return m, queueReviews(m.service, typed.reviews)
	case queueFinishedMsg:
		m.execution.runs = typed.runs
		m.execution.err = typed.err
		m.execution.queued = true
		if hasNonTerminalRun(typed.runs) {
			return m, scheduleRefresh()
		}
		return m, nil
	case refreshTickMsg:
		if m.screen == ScreenExecution && hasNonTerminalRun(m.execution.runs) {
			return m, refreshRuns(m.service, m.execution.runs)
		}
		return m, nil
	case refreshFinishedMsg:
		m.execution.runs = typed.runs
		if hasNonTerminalRun(typed.runs) {
			return m, scheduleRefresh()
		}
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEsc {
		switch m.screen {
		case ScreenReview:
			m.screen = ScreenCatalog
			return m, nil
		case ScreenExecution:
			return m, tea.Quit
		}
	}

	switch m.screen {
	case ScreenContext:
		var cmd tea.Cmd
		m.context, cmd = m.context.update(msg)
		return m, cmd
	case ScreenCatalog:
		updated, cmd := m.catalog.Update(msg)
		m.catalog = updated.(CatalogModel)
		return m, cmd
	case ScreenReview:
		var cmd tea.Cmd
		m.review, cmd = m.review.update(msg)
		return m, cmd
	case ScreenExecution:
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "q" {
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the active screen.
func (m AppModel) View() string {
	switch m.screen {
	case ScreenContext:
		return m.context.view()
	case ScreenCatalog:
		return m.catalog.View()
	case ScreenReview:
		return m.review.view()
	case ScreenExecution:
		return m.execution.view()
	default:
		return ""
	}
}

func demoPipelines() []azdo.Pipeline {
	return []azdo.Pipeline{
		{ID: 101, Name: "build application", Folder: "/apps", RepoName: "sample-api", Tags: []string{"demo"}},
		{ID: 202, Name: "deploy infrastructure", Folder: "/platform", RepoName: "sample-iac", Tags: []string{"demo"}},
		{ID: 303, Name: "release website", Folder: "/web", RepoName: "sample-web", Tags: []string{"demo"}},
	}
}
