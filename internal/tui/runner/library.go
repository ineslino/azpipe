package runner

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ineslino/azpipe/internal/azdo"
	domain "github.com/ineslino/azpipe/internal/runner"
)

type libraryModel struct {
	kind     string
	cursor   int
	name     textinput.Model
	profiles []domain.Profile
	journals []*domain.Journal
	err      string
}

func (m *AppModel) openLibrary(kind string) {
	m.invalidateOperation()
	l := &libraryModel{kind: kind, name: textinput.New()}
	l.name.Prompt = "Nome: "
	l.name.CharLimit = 64
	l.name.Width = 40
	l.name.PromptStyle = keyStyle
	m.library = l
	if kind == "save" {
		l.name.Focus()
		if len(m.catalog.Selected()) == 0 {
			l.err = "Seleccione pipelines antes de guardar um perfil."
		}
		return
	}
	var err error
	if kind == "profiles" {
		if m.demo {
			l.profiles = append([]domain.Profile(nil), m.demoProfiles...)
		} else {
			l.profiles, err = domain.ListProfiles(m.organization, m.project)
		}
	} else {
		if m.demo {
			l.journals = []*domain.Journal{{Organization: "demo", Project: m.project, Runs: []domain.JournalRecord{
				{PipelineID: 101, PipelineName: "build application", Run: azdo.PipelineRun{ID: 7001, State: "completed", Result: "succeeded", WebURL: "https://dev.azure.com/example-org/sample-project/_build/results?buildId=7001"}},
				{PipelineID: 202, PipelineName: "deploy infrastructure", Run: azdo.PipelineRun{ID: 7002, State: "inProgress", StartTime: time.Now().Add(-2 * time.Minute)}},
				{PipelineID: 303, PipelineName: "release website", Run: azdo.PipelineRun{ID: 7003, State: "notStarted"}},
			}}}
		} else {
			l.journals, err = domain.ListJournals(m.organization, m.project)
		}
	}
	if err != nil {
		l.err = err.Error()
	}
}

func (m AppModel) libraryUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	l := m.library
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.Type == tea.KeyEsc {
		m.library = nil
		return m, nil
	}
	if l.kind == "save" {
		if key.Type != tea.KeyEnter {
			var cmd tea.Cmd
			l.name, cmd = l.name.Update(msg)
			return m, cmd
		}
		profile := domain.Profile{Version: 1, Name: strings.TrimSpace(l.name.Value()), Organization: m.organization, Project: m.project}
		if m.demo {
			profile.Organization = "demo"
		}
		for _, s := range m.catalog.Selected() {
			profile.Selections = append(profile.Selections, domain.ProfileSelection{ID: s.ID(), Mode: s.Mode, Branch: s.Branch, Parameters: s.Inputs})
		}
		var err error
		if m.demo {
			if profile.Name == "" || len(profile.Selections) == 0 {
				err = fmt.Errorf("preencha nome e selecção")
			} else {
				for _, p := range m.demoProfiles {
					if p.Name == profile.Name {
						err = fmt.Errorf("nome já existe")
					}
				}
				if err == nil {
					m.demoProfiles = append(m.demoProfiles, profile)
				}
			}
		} else {
			err = domain.SaveProfile(profile)
		}
		if err != nil {
			l.err = err.Error()
			return m, nil
		}
		m.library = nil
		m.catalog.warning = ""
		m.catalog.notice = "Perfil guardado. Carregue com l; exige sempre nova revisão."
		return m, nil
	}
	count := len(l.profiles)
	if l.kind == "history" {
		count = len(l.journals)
	}
	switch key.String() {
	case "up", "k":
		l.cursor = max(0, l.cursor-1)
	case "down", "j":
		l.cursor = min(max(0, count-1), l.cursor+1)
	case "enter":
		if count == 0 || l.err != "" {
			return m, nil
		}
		if l.kind == "profiles" {
			org := m.organization
			if m.demo {
				org = "demo"
			}
			selections, err := l.profiles[l.cursor].Resolve(org, m.project, m.catalog.pipelines)
			if err != nil {
				l.err = err.Error()
				return m, nil
			}
			m.catalog.selected = map[int]domain.Mode{}
			m.catalog.parameters = map[int]map[string]string{}
			m.catalog.branches = map[int]string{}
			for _, s := range selections {
				m.catalog.selected[s.ID()] = s.Mode
				m.catalog.parameters[s.ID()] = s.Inputs
				m.catalog.branches[s.ID()] = s.Branch
			}
			m.catalog.warning = ""
			m.catalog.notice = "Perfil carregado; confirme selecção, branches e parâmetros antes de rever."
			m.library = nil
		} else {
			journal := l.journals[l.cursor]
			m.execution = executionModel{runs: journal.Results(), queued: true, journal: journal.Path(), height: max(1, m.height-2), width: max(1, m.width-4), demo: m.demo}
			m.screen = ScreenExecution
			m.library = nil
			token := m.startOperation(runsOperationTarget(m.execution.runs))
			if !m.demo {
				return m, refreshRuns(m.service, m.execution.runs, token, m.execution.journal, m.organization, m.project)
			}
		}
	}
	return m, nil
}

func (l libraryModel) view(width, height int) string {
	if width == 0 {
		width = defaultWidth
	}
	if height == 0 {
		height = defaultHeight
	}
	title := "Perfis guardados"
	if l.kind == "history" {
		title = "Lotes · retomar acompanhamento"
	}
	if l.kind == "save" {
		title = "Guardar perfil de execução"
	}
	lines := []string{catalogTitleStyle.Render(title), ""}
	if l.kind == "save" {
		lines = append(lines, "Guarda pipelines, modos, branches e parâmetros localmente.", "Não guarde segredos. Enter confirma a escrita; não executa pipelines.", "Um nome existente nunca é substituído.", "", l.name.View())
	} else {
		count := len(l.profiles)
		if l.kind == "history" {
			count = len(l.journals)
		}
		start := max(0, l.cursor-max(1, height-9)+1)
		for i := start; i < min(count, start+max(1, height-9)); i++ {
			line := ""
			if l.kind == "profiles" {
				p := l.profiles[i]
				line = fmt.Sprintf("%s · %d pipelines", p.Name, len(p.Selections))
			} else {
				j := l.journals[i]
				name := filepath.Base(j.Path())
				if j.Path() == "" {
					name = "lote de demonstração"
				}
				line = fmt.Sprintf("%s · %d runs", name, len(j.Runs))
			}
			style := catalogDetailStyle
			marker := "  "
			if i == l.cursor {
				style = catalogActiveStyle.Width(width)
				marker = "> "
			}
			lines = append(lines, style.Render(truncateWidth(marker+line, width)))
		}
		if count == 0 {
			lines = append(lines, "Nenhum registo neste contexto.")
		}
		if l.kind == "history" {
			lines = append(lines, "", catalogDetailStyle.Render("Retomar apenas consulta IDs conhecidos. Nunca volta a lançar runs."))
		} else {
			lines = append(lines, "", catalogDetailStyle.Render("Carregar substitui a selecção. Exige nova preview e confirmação."))
		}
	}
	if l.err != "" {
		lines = append(lines, catalogWarningStyle.Render(truncateWidth(l.err, width)))
	}
	lines = append(lines, "", shortcutBar(width, "↑/↓ escolher", "enter confirmar", "esc voltar"))
	return strings.Join(lines, "\n")
}
