package runner

import (
	"encoding/json"
	"sort"
	"strings"

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
	screen     Screen
	factory    ClientFactory
	service    domainrunner.Service
	context    contextModel
	catalog    CatalogModel
	review     reviewModel
	execution  executionModel
	demo       bool
	generation uint64
	active     operationToken
}

type operationToken struct {
	generation uint64
	target     string
}

type selectionTarget struct {
	PipelineID int               `json:"pipelineId"`
	Mode       domainrunner.Mode `json:"mode"`
	Branch     string            `json:"branch"`
	Parameters []parameterTarget `json:"parameters"`
}

type parameterTarget struct {
	Name  string `json:"name"`
	Value string `json:"value"`
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
	if key, ok := msg.(tea.KeyMsg); ok && m.screen == ScreenExecution && !m.execution.queued {
		if key.String() == "q" || key.Type == tea.KeyEsc || key.Type == tea.KeyCtrlC || key.Type == tea.KeyCtrlD {
			return m, nil
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok && (key.Type == tea.KeyCtrlC || key.Type == tea.KeyCtrlD) {
		return m, tea.Quit
	}

	switch typed := msg.(type) {
	case contextSubmitMsg:
		currentContextTarget := contextOperationTarget(strings.TrimSpace(m.context.organization.Value()), strings.TrimSpace(m.context.project.Value()))
		if m.screen != ScreenContext || contextOperationTarget(typed.organization, typed.project) != currentContextTarget {
			return m, nil
		}
		token := m.startOperation(contextOperationTarget(typed.organization, typed.project))
		return m, loadContext(m.factory, typed.organization, typed.project, token)
	case contextLoadedMsg:
		currentContextTarget := contextOperationTarget(strings.TrimSpace(m.context.organization.Value()), strings.TrimSpace(m.context.project.Value()))
		if !m.accepts(ScreenContext, typed.token) || typed.token.target != contextOperationTarget(typed.organization, typed.project) || typed.token.target != currentContextTarget {
			return m, nil
		}
		if typed.err != nil {
			m.context.err = typed.err.Error()
			return m, nil
		}
		m.service = domainrunner.NewService(typed.client, typed.project)
		m.catalog = NewCatalogModel(typed.pipelines)
		m.screen = ScreenCatalog
		return m, nil
	case CatalogReviewMsg:
		if m.screen != ScreenCatalog || selectionsOperationTarget(typed.Selections) != selectionsOperationTarget(m.catalog.Selected()) {
			return m, nil
		}
		token := m.startOperation(selectionsOperationTarget(typed.Selections))
		m.review = newReviewModel(typed.Selections, m.demo, token)
		m.screen = ScreenReview
		if m.demo {
			return m, nil
		}
		return m, previewSelections(m.service, typed.Selections, token)
	case previewFinishedMsg:
		if !m.accepts(ScreenReview, typed.token) || typed.token.target != reviewsOperationTarget(typed.reviews) || typed.token.target != reviewsOperationTarget(m.review.reviews) {
			return m, nil
		}
		m.review.reviews = typed.reviews
		if m.review.canExecute() {
			return m, m.review.confirmation.Focus()
		}
		return m, nil
	case queueConfirmedMsg:
		if !m.accepts(ScreenReview, typed.token) || typed.token.target != reviewsOperationTarget(typed.reviews) || !allReviewsReady(typed.reviews) || !m.review.canExecute() || m.review.confirmation.Value() != confirmationValue {
			return m, nil
		}
		token := m.startOperation(typed.token.target)
		m.execution = newExecutionModel()
		m.screen = ScreenExecution
		return m, queueReviews(m.service, typed.reviews, token)
	case queueFinishedMsg:
		if !m.accepts(ScreenExecution, typed.token) || typed.token.target != runsOperationTarget(typed.runs) {
			return m, nil
		}
		m.execution.runs = typed.runs
		m.execution.err = typed.err
		m.execution.queued = true
		if hasNonTerminalRun(typed.runs) {
			return m, scheduleRefresh(m.active)
		}
		return m, nil
	case refreshTickMsg:
		if m.accepts(ScreenExecution, typed.token) && hasNonTerminalRun(m.execution.runs) {
			return m, refreshRuns(m.service, m.execution.runs, m.active)
		}
		return m, nil
	case refreshFinishedMsg:
		if !m.accepts(ScreenExecution, typed.token) || typed.token.target != runsOperationTarget(typed.runs) {
			return m, nil
		}
		m.execution.runs = typed.runs
		if hasNonTerminalRun(typed.runs) {
			return m, scheduleRefresh(m.active)
		}
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEsc {
		switch m.screen {
		case ScreenReview:
			m.invalidateOperation()
			m.screen = ScreenCatalog
			return m, nil
		case ScreenExecution:
			m.invalidateOperation()
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

// ExecutionError reports queue failures retained by the final execution model.
func (m AppModel) ExecutionError() error {
	return m.execution.err
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

func (m *AppModel) startOperation(target string) operationToken {
	m.generation++
	m.active = operationToken{generation: m.generation, target: target}
	return m.active
}

func (m *AppModel) invalidateOperation() {
	m.generation++
	m.active = operationToken{generation: m.generation}
}

func (m AppModel) accepts(screen Screen, token operationToken) bool {
	return m.screen == screen && token == m.active && token.target != ""
}

func contextOperationTarget(organization, project string) string {
	encoded, _ := json.Marshal([]string{"context", organization, project})
	return string(encoded)
}

func selectionsOperationTarget(selections []domainrunner.Selection) string {
	targets := make([]selectionTarget, len(selections))
	for index, selection := range selections {
		request := selection.Request()
		keys := make([]string, 0, len(request.Parameters))
		for key := range request.Parameters {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parameters := make([]parameterTarget, 0, len(keys))
		for _, key := range keys {
			parameters = append(parameters, parameterTarget{Name: key, Value: request.Parameters[key]})
		}
		targets[index] = selectionTarget{
			PipelineID: selection.Pipeline.ID,
			Mode:       selection.Mode,
			Branch:     request.Branch,
			Parameters: parameters,
		}
	}
	encoded, _ := json.Marshal(targets)
	return string(encoded)
}

func reviewsOperationTarget(reviews []domainrunner.Review) string {
	selections := make([]domainrunner.Selection, len(reviews))
	for index, review := range reviews {
		selections[index] = review.Selection
	}
	return selectionsOperationTarget(selections)
}

func runsOperationTarget(runs []domainrunner.RunResult) string {
	reviews := make([]domainrunner.Review, len(runs))
	for index, run := range runs {
		reviews[index] = run.Review
	}
	return reviewsOperationTarget(reviews)
}

func allReviewsReady(reviews []domainrunner.Review) bool {
	if len(reviews) == 0 {
		return false
	}
	for _, review := range reviews {
		if review.State != domainrunner.ReviewReady || review.Err != nil {
			return false
		}
	}
	return true
}
