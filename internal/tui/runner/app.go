package runner

import (
	"encoding/json"
	"errors"
	"fmt"
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
	screen       Screen
	factory      ClientFactory
	service      domainrunner.Service
	context      contextModel
	catalog      CatalogModel
	review       reviewModel
	execution    executionModel
	demo         bool
	generation   uint64
	active       operationToken
	height       int
	width        int
	organization string
	project      string
	library      *libraryModel
	demoProfiles []domainrunner.Profile
	actions      *int
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
		width: defaultWidth, height: defaultHeight,
		screen:  ScreenCatalog,
		service: domainrunner.NewService(client, project),
		catalog: NewCatalogModel(pipelines),
		project: project,
	}
}

// NewBootstrapApp starts at a non-secret organization and project context screen.
func NewBootstrapApp(factory ClientFactory, defaults ContextDefaults) AppModel {
	return AppModel{
		width: defaultWidth, height: defaultHeight,
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
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = size.Height
		m.width = size.Width
		m.review.height = max(1, size.Height-2)
		m.review.width = max(1, size.Width-4)
		m.execution.height = max(1, size.Height-2)
		m.execution.width = max(1, size.Width-4)
	}
	if key, ok := msg.(tea.KeyMsg); ok && m.screen == ScreenExecution && !m.execution.queued {
		if key.String() == "q" || key.Type == tea.KeyEsc || key.Type == tea.KeyCtrlC || key.Type == tea.KeyCtrlD {
			return m, nil
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok && (key.Type == tea.KeyCtrlC || key.Type == tea.KeyCtrlD) {
		return m, tea.Quit
	}
	if m.library != nil {
		return m.libraryUpdate(msg)
	}
	if m.actions != nil {
		if key, ok := msg.(tea.KeyMsg); ok {
			return m.updateActions(key)
		}
	}

	switch typed := msg.(type) {
	case schemaLoadedMsg:
		if !m.accepts(ScreenCatalog, typed.token) || m.catalog.input != inputNone {
			return m, nil
		}
		pipeline, ok := m.catalog.active()
		if !ok || pipeline.ID != typed.id || m.catalog.branchFor(pipeline.ID) != typed.branch {
			return m, nil
		}
		m.catalog.notice = ""
		if typed.err != nil {
			m.catalog.warning = "Não foi possível ler parâmetros: " + typed.err.Error()
			return m, nil
		}
		modeParameter := ""
		if pipeline.PlanContract != nil {
			modeParameter = pipeline.PlanContract.Parameter
		}
		editor, err := newSchemaEditor(typed.schema, m.catalog.parameters[pipeline.ID], modeParameter)
		if err != nil {
			m.catalog.warning = err.Error()
			return m, nil
		}
		m.catalog.editor = editor
		m.catalog.input = inputParameterForm
		m.catalog.warning = ""
		return m, nil
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
		if m.width > 0 {
			m.catalog.width = m.width
		}
		if m.height > 0 {
			m.catalog.height = m.height
		}
		m.organization, m.project = typed.organization, typed.project
		m.screen = ScreenCatalog
		return m, nil
	case CatalogReviewMsg:
		if m.screen != ScreenCatalog || selectionsOperationTarget(typed.Selections) != selectionsOperationTarget(m.catalog.Selected()) {
			return m, nil
		}
		token := m.startOperation(selectionsOperationTarget(typed.Selections))
		m.review = newReviewModel(typed.Selections, m.demo, token)
		m.review.height = max(1, m.height-2)
		m.review.width = max(1, m.width-4)
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
		m.execution.runs = make([]domainrunner.RunResult, len(typed.reviews))
		for i, r := range typed.reviews {
			m.execution.runs[i].Review = r
		}
		m.execution.height = max(1, m.height-2)
		m.execution.width = max(1, m.width-4)
		m.screen = ScreenExecution
		if m.organization != "" {
			return m, queueReviews(m.service, typed.reviews, token, m.organization, m.project)
		}
		return m, queueReviews(m.service, typed.reviews, token)
	case queueProgressMsg:
		if !m.accepts(ScreenExecution, typed.token) {
			return m, nil
		}
		m.execution.journal = typed.journal
		for len(m.execution.runs) <= typed.index {
			m.execution.runs = append(m.execution.runs, domainrunner.RunResult{})
		}
		m.execution.runs[typed.index] = typed.result
		return m, func() tea.Msg { return <-typed.next }
	case queueFinishedMsg:
		if !m.accepts(ScreenExecution, typed.token) || (typed.err == nil && typed.token.target != runsOperationTarget(typed.runs)) {
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
			return m, refreshRuns(m.service, m.execution.runs, m.active, m.execution.journal, m.organization, m.project)
		}
		return m, nil
	case refreshFinishedMsg:
		if !m.accepts(ScreenExecution, typed.token) || typed.token.target != runsOperationTarget(typed.runs) {
			return m, nil
		}
		m.execution.runs = typed.runs
		m.execution.persistErr = typed.err
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
			m.screen = ScreenCatalog
			return m, nil
		}
	}

	switch m.screen {
	case ScreenContext:
		var cmd tea.Cmd
		m.context, cmd = m.context.update(msg)
		return m, cmd
	case ScreenCatalog:
		if key, ok := msg.(tea.KeyMsg); ok && m.catalog.input == inputNone {
			if key.String() == "a" || key.String() == "?" {
				index := 0
				m.actions = &index
				return m, nil
			}
			kind := ""
			switch key.String() {
			case "s":
				kind = "save"
			case "l":
				kind = "profiles"
			case "h":
				kind = "history"
			}
			if kind != "" {
				m.openLibrary(kind)
				return m, nil
			}
		}
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "e" && m.catalog.input == inputNone {
			if pipeline, ok := m.catalog.active(); ok {
				token := m.startOperation("schema")
				m.catalog.warning = ""
				m.catalog.notice = "A ler parâmetros do YAML..."
				return m, m.loadSchema(pipeline.ID, m.catalog.branchFor(pipeline.ID), token)
			}
		}
		updated, cmd := m.catalog.Update(msg)
		m.catalog = updated.(CatalogModel)
		return m, cmd
	case ScreenReview:
		if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter && len(m.review.reviews) > 0 {
			r := m.review.reviews[m.review.offset]
			if r.Err != nil {
				m.invalidateOperation()
				m.screen = ScreenCatalog
				m.catalog.search.SetValue("")
				m.catalog.filter()
				for i, p := range m.catalog.visible {
					if p.ID == r.Selection.ID() {
						m.catalog.cursor = i
						break
					}
				}
				m.catalog.notice = "Corrige com a (acções), depois Enter para uma nova revisão."
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.review, cmd = m.review.update(msg)
		return m, cmd
	case ScreenExecution:
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.String() == "right" {
				m.execution.horizontal += 20
			}
			if key.String() == "left" {
				m.execution.horizontal = max(0, m.execution.horizontal-20)
			}
			if key.String() == "pgdown" {
				m.execution.offset = min(max(0, len(m.execution.runs)-1), m.execution.offset+m.execution.pageSize())
			}
			if key.String() == "pgup" {
				m.execution.offset = max(0, m.execution.offset-m.execution.pageSize())
			}
		}
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "q" {
			return m, tea.Quit
		}
	}
	return m, nil
}

// ExecutionError reports queue failures retained by the final execution model.
func (m AppModel) ExecutionError() error {
	if m.demo {
		return nil
	}
	errs := []error{m.execution.err, m.execution.persistErr}
	for _, result := range m.execution.runs {
		if result.Err != nil {
			errs = append(errs, result.Err)
		} else if result.Run.State == "completed" && result.Run.Result != "succeeded" {
			errs = append(errs, fmt.Errorf("run %d terminou com resultado %s", result.Run.ID, result.Run.Result))
		} else if result.Run.ID != 0 && result.Run.State != "completed" {
			errs = append(errs, fmt.Errorf("run %d ainda não terminou", result.Run.ID))
		}
	}
	return errors.Join(errs...)
}

// View renders the active screen.
func (m AppModel) View() string {
	if m.actions != nil {
		return m.contextHeader() + m.actionsView()
	}
	if m.library != nil {
		return m.contextHeader() + section("PERFIS E LOTES", m.library.view(max(1, m.width-4), max(1, m.height-2)), m.width)
	}
	switch m.screen {
	case ScreenContext:
		return section("LIGAÇÃO AO AZURE DEVOPS", m.context.view(), m.width)
	case ScreenCatalog:
		catalog := m.catalog
		banner := ""
		if catalog.input == inputNone {
			banner = brandWhiteStyle.Render("As tuas pipelines. ") + brandLimeStyle.Render("Um só terminal.") + "\n"
			if m.height >= 32 && m.width >= 60 {
				parts := strings.Split(welcomeBrand(), "\n")
				banner = strings.Join(parts[2:9], "\n") + "\n"
			}
			catalog.height -= strings.Count(banner, "\n")
		}
		return banner + m.contextHeader() + catalog.View()
	case ScreenReview:
		return m.contextHeader() + section("VALIDAÇÃO DO LOTE", m.review.view(), m.width)
	case ScreenExecution:
		return m.contextHeader() + section("MONITORIZAÇÃO DO LOTE", m.execution.view(), m.width)
	default:
		return ""
	}
}

func (m AppModel) contextHeader() string {
	active := 0
	if m.screen == ScreenReview {
		active = 1
	}
	if m.screen == ScreenExecution {
		active = 2
	}
	header := pipelineBrand(m.width, active, m.demo) + "\n"
	if m.demo {
		return header + catalogDetailStyle.Render("Contexto fictício: example-org / sample-project") + "\n"
	}
	return header + catalogDetailStyle.Render(truncateWidth(fmt.Sprintf("Organização: %s | Projecto: %s", m.organization, m.project), m.width)) + "\n"
}

func demoPipelines() []azdo.Pipeline {
	return []azdo.Pipeline{
		{ID: 101, Name: "build application", Folder: "/apps", RepoName: "sample-api", Tags: []string{"demo"}},
		{ID: 202, Name: "deploy infrastructure", Folder: "/platform", RepoName: "sample-iac", Tags: []string{"demo"}, PlanContract: &azdo.PlanContract{Parameter: "planOnly", Type: "boolean", PlanValue: "true", RunValue: "false", Evidence: "fixture offline"}},
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
