package runner

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ineslino/azpipe/internal/azdo"
	domain "github.com/ineslino/azpipe/internal/runner"
)

type libraryModel struct {
	kind         string
	cursor       int
	name         textinput.Model
	profiles     []domain.Profile
	journals     []*domain.Journal
	err          string
	errorScroll  int
	details      bool
	detailScroll int
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
	if l.details {
		switch key.String() {
		case "esc", "d":
			l.details = false
		case "pgdown", "down":
			l.detailScroll++
		case "pgup", "up":
			l.detailScroll = max(0, l.detailScroll-1)
		}
		return m, nil
	}
	if l.err != "" {
		if key.String() == "pgdown" {
			l.errorScroll++
			return m, nil
		}
		if key.String() == "pgup" {
			l.errorScroll = max(0, l.errorScroll-1)
			return m, nil
		}
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
			profile.Selections = append(profile.Selections, domain.ProfileSelection{ID: s.ID(), Project: s.Project(), Mode: s.Mode, Branch: s.Branch, Parameters: s.Inputs})
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
	if key.String() == "d" && l.kind == "history" && count > 0 {
		l.details, l.detailScroll = true, 0
		return m, nil
	}
	previousCursor := l.cursor
	switch key.String() {
	case "up", "k":
		l.cursor = max(0, l.cursor-1)
	case "down", "j":
		l.cursor = min(max(0, count-1), l.cursor+1)
	case "enter":
		if count == 0 {
			return m, nil
		}
		l.err, l.errorScroll = "", 0
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
			m.catalog.selected = map[string]domain.Mode{}
			m.catalog.parameters = map[string]map[string]string{}
			m.catalog.branches = map[string]string{}
			for _, s := range selections {
				key := s.Key()
				m.catalog.selected[key] = s.Mode
				m.catalog.parameters[key] = s.Inputs
				m.catalog.branches[key] = s.Branch
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
	if l.cursor != previousCursor {
		l.err, l.errorScroll = "", 0
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
	if l.details && len(l.journals) > 0 {
		j := l.journals[l.cursor]
		lines := []string{"Detalhe do lote · último estado guardado", journalUpdated(j), "Ficheiro: " + j.Path(), ""}
		for _, r := range j.Runs {
			lines = append(lines, fmt.Sprintf("%s · pipeline %d · run %d · %s", r.PipelineName, r.PipelineID, r.Run.ID, historyState(r)), r.Error, "")
		}
		return textPage(strings.Join(lines, "\n"), width, l.detailScroll, max(1, height-5)) + "\n\n" + shortcutBar(width, "pgup/pgdown detalhe", "esc voltar aos lotes")
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
		capacity := max(1, height-12)
		if l.kind == "profiles" && l.err != "" && count > 0 {
			capacity = max(1, capacity-1)
		}
		start := max(0, l.cursor-capacity+1)
		for i := start; i < min(count, start+capacity); i++ {
			line := ""
			if l.kind == "profiles" {
				p := l.profiles[i]
				line = p.Name + " · " + quantity(len(p.Selections), "pipeline", "pipelines")
			} else {
				j := l.journals[i]
				names := []string{}
				for k, r := range j.Runs {
					if k >= 2 {
						break
					}
					name := r.PipelineName
					if name == "" {
						name = fmt.Sprintf("pipeline %d", r.PipelineID)
					}
					names = append(names, name)
				}
				line = strings.Join(names, " / ")
				if len(j.Runs) > 2 {
					line += fmt.Sprintf(" +%d", len(j.Runs)-2)
				}
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
			if l.kind == "profiles" {
				lines = append(lines, "Volta ao catálogo, selecciona pipelines", "e usa s para guardar o primeiro perfil.")
			} else {
				lines = append(lines, "Volta ao catálogo e revê uma selecção para criar um lote.")
			}
		}
		if l.kind == "history" {
			if count > 0 {
				j := l.journals[l.cursor]
				counts := map[string]int{}
				for _, r := range j.Runs {
					counts[historyState(r)]++
				}
				states := []string{}
				for _, s := range []string{"incerta", "erro", "falhou", "cancelada", "concluída", "a correr", "em fila", "parcial"} {
					if counts[s] > 0 {
						states = append(states, fmt.Sprintf("%d %s", counts[s], s))
					}
				}
				lines = append(lines, "", catalogDetailStyle.Render(truncateWidth("Último estado: "+strings.Join(states, " · "), width)), catalogDetailStyle.Render(journalUpdated(j)))
			}
			lines = append(lines, "", catalogDetailStyle.Render("Retomar apenas consulta IDs conhecidos. Nunca volta a lançar runs."))
		} else {
			lines = append(lines, "", catalogDetailStyle.Render("Carregar substitui a selecção. Exige nova preview e confirmação."))
		}
	}
	if l.err != "" {
		lines = append(lines, catalogWarningStyle.Render(textPage(l.err, width, l.errorScroll, 3)), "PgUp/PgDn: percorrer erro completo")
		if l.kind == "profiles" && len(l.profiles) > 0 {
			hint := "Enter volta a tentar; Esc volta ao catálogo."
			if len(l.profiles) > 1 {
				hint = "Escolhe outro perfil; Enter volta a tentar."
			}
			lines = append(lines, catalogDetailStyle.Render(hint))
		}
	}
	action := "enter carregar selecção"
	if l.kind == "save" {
		action = "enter guardar perfil"
	}
	if l.kind == "history" {
		action = "enter acompanhar lote"
	}
	items := []string{"esc voltar"}
	if l.kind == "save" {
		items = append([]string{action}, items...)
	} else if (l.kind == "history" && len(l.journals) > 0) || (l.kind == "profiles" && len(l.profiles) > 0) {
		items = append([]string{"↑/↓ escolher", action}, items...)
		if l.kind == "history" {
			items = append(items, "d detalhe")
		}
	}
	lines = append(lines, "", shortcutBar(width, items...))
	return strings.Join(lines, "\n")
}

func journalUpdated(j *domain.Journal) string {
	if j.UpdatedAt.IsZero() {
		return "Data indisponível · demonstração offline"
	}
	return "Actualizado: " + j.UpdatedAt.Local().Format("02/01/2006 15:04")
}

func historyState(r domain.JournalRecord) string {
	if r.Run.ID == 0 {
		return "incerta"
	}
	if r.Error != "" {
		return "erro"
	}
	if r.Run.State == "completed" {
		switch r.Run.Result {
		case "succeeded":
			return "concluída"
		case "failed":
			return "falhou"
		case "canceled":
			return "cancelada"
		default:
			return "parcial"
		}
	}
	if r.Run.State == "inProgress" {
		return "a correr"
	}
	return "em fila"
}
