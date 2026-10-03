package runner

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	domainrunner "github.com/ineslino/azpipe/internal/runner"
)

type catalogAction struct {
	label, key, description, blocked string
}

func (a catalogAction) group() string {
	switch a.key {
	case "enter", "/", "x":
		return "Seleccionar"
	case "s", "l", "h":
		return "Perfis e histórico"
	case "c", "B":
		return "Contexto"
	case "e", "m":
		return "Pipeline"
	case "P", "R", "b":
		return "Lote"
	default:
		return "Avançado"
	}
}

func (m AppModel) catalogActions() []catalogAction {
	pipeline, active := m.catalog.active()
	selection := ""
	if len(m.catalog.selected) == 0 {
		selection = "Selecciona pelo menos uma pipeline."
	}
	activeReason := ""
	if !active {
		activeReason = "Altera a pesquisa para encontrar uma pipeline."
	}
	modeReason := activeReason
	if _, selected := m.catalog.selected[domainrunner.PipelineKey(pipeline)]; active && !selected {
		modeReason = "Selecciona a pipeline activa com espaço."
	}
	if active && modeReason == "" && pipeline.PlanContract == nil {
		modeReason = "PLAN indisponível: falta contrato validado."
	}
	planReason := selection
	for _, p := range m.catalog.pipelines {
		if _, selected := m.catalog.selected[domainrunner.PipelineKey(p)]; selected && p.PlanContract == nil {
			planReason = "PLAN indisponível: a selecção inclui pipelines sem contrato."
		}
	}
	contextReason := "Só está disponível depois de carregar a lista de projectos."
	if m.demo {
		contextReason = "A demonstração não tem uma organização ligada."
	} else if len(m.context.projects) > 0 {
		contextReason = "Escolhe outro projecto ou Todos os projectos; o catálogo actual será substituído."
	}
	return []catalogAction{
		{"Rever selecção", "enter", "Valida branch e parâmetros. Ainda não lança runs.", selection},
		{"Procurar pipelines", "/", "Filtra por projecto, nome, ID, tipo, pasta, repositório ou tag. Ctrl+U limpa; Esc mantém o filtro.", ""},
		{"Limpar selecções ocultas", "x", "Remove apenas selecções que a pesquisa actual esconde.", selection},
		{"Configurar parâmetros da pipeline activa", "e", truncateWidth(pipeline.Name, max(12, m.width-40)) + ": abre campos tipados do YAML.", activeReason},
		{"Alternar RUN / PLAN da pipeline activa", "m", truncateWidth(pipeline.Name, max(12, m.width-40)) + ": muda apenas esta pipeline seleccionada.", modeReason},
		{"Aplicar PLAN a toda a selecção", "P", "Usa o contrato revisto de cada pipeline seleccionada.", planReason},
		{"Aplicar RUN a toda a selecção", "R", "Execução normal de todas as pipelines seleccionadas.", selection},
		{"Alterar branch global", "b", "Aplica às pipelines actuais e futuras. Substitui branches específicas de perfis.", ""},
		{"Editar parâmetros JSON (avançado)", "J", "Não contorna a validação do schema. Nunca uses segredos.", activeReason},
		{"Guardar selecção como perfil", "s", "Guarda parâmetros não secretos após confirmação.", selection},
		{"Carregar perfil", "l", "Substitui a selecção. Exige uma nova revisão.", ""},
		{"Consultar lotes anteriores", "h", "Retoma monitorização sem submeter runs.", ""},
		{"Mudar projecto", "c", "Volta ao selector de projectos da organização.", func() string {
			if m.demo || len(m.context.projects) == 0 {
				return contextReason
			}
			return ""
		}()},
		{"Gerir branches", "B", "Escolhe um repositório, filtra por criador e revê antes de eliminar.", ""},
	}
}

func (m AppModel) updateActions(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	index := *m.actions
	items := m.catalogActions()
	switch key.String() {
	case "esc", "a", "?":
		m.actions = nil
	case "up", "k":
		index = max(0, index-1)
		m.actions = &index
	case "down", "j":
		index = min(len(items)-1, index+1)
		m.actions = &index
	case "enter":
		item := items[index]
		if item.blocked != "" {
			return m, nil
		}
		m.actions = nil
		if item.key == "enter" {
			return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		}
		return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(item.key)})
	default:
		for i, item := range items {
			if key.String() == item.key {
				m.actions = &i
				return m.updateActions(tea.KeyMsg{Type: tea.KeyEnter})
			}
		}
	}
	return m, nil
}

func (m AppModel) actionsView() string {
	items := m.catalogActions()
	index := *m.actions
	inner := max(1, m.width-4)
	render := func(start, end int) string {
		lines := []string{quantity(len(m.catalog.selected), "seleccionada", "seleccionadas") + " · tecla directa ou ↑/↓ e Enter"}
		lastGroup := ""
		for i := start; i < end; i++ {
			item := items[i]
			if group := item.group(); group != lastGroup {
				if lastGroup != "" {
					lines = append(lines, "")
				}
				lines = append(lines, catalogHeaderStyle.Width(inner).Render(strings.ToUpper(group)))
				lastGroup = group
			}
			label := fmt.Sprintf("  %-5s %s", item.key, item.label)
			if item.blocked != "" {
				label += " [indisponível]"
			}
			label = truncateWidth(label, inner)
			if i == index {
				label = catalogActiveStyle.Width(inner).Render(">" + label[1:])
			} else if item.blocked != "" {
				label = catalogDetailStyle.Render(label)
			} else {
				label = catalogTextStyle.Render(label)
			}
			lines = append(lines, label)
		}
		detail := items[index].description
		if items[index].blocked != "" {
			detail = items[index].blocked
		}
		lines = append(lines, "", ansi.Wrap(detail, inner, ""), fmt.Sprintf("%d/%d", index+1, len(items)), shortcutBar(inner, "↑/↓ escolher", "enter abrir", "esc voltar"))
		return section("ACÇÕES E AJUDA", strings.Join(lines, "\n"), m.width)
	}
	start, end := 0, len(items)
	for lipgloss.Height(render(start, end)) > m.height-2 && end-start > 1 {
		if end-1 > index {
			end--
		} else {
			start++
		}
	}
	return render(start, end)
}
