package runner

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type catalogAction struct {
	label, key, description, blocked string
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
	if _, selected := m.catalog.selected[pipeline.ID]; active && !selected {
		modeReason = "Selecciona a pipeline activa com espaço."
	}
	if active && modeReason == "" && pipeline.PlanContract == nil {
		modeReason = "PLAN indisponível: falta contrato validado."
	}
	planReason := selection
	for _, p := range m.catalog.pipelines {
		if _, selected := m.catalog.selected[p.ID]; selected && p.PlanContract == nil {
			planReason = "PLAN indisponível: a selecção inclui pipelines sem contrato."
		}
	}
	return []catalogAction{
		{"Rever selecção", "enter", "Valida branch e parâmetros. Ainda não lança runs.", selection},
		{"Configurar parâmetros da pipeline activa", "e", pipeline.Name + ": abre campos tipados do YAML.", activeReason},
		{"Alternar RUN / PLAN da pipeline activa", "m", pipeline.Name + ": muda apenas esta pipeline seleccionada.", modeReason},
		{"Aplicar PLAN a toda a selecção", "P", "Usa o contrato revisto de cada pipeline seleccionada.", planReason},
		{"Aplicar RUN a toda a selecção", "R", "Execução normal de todas as pipelines seleccionadas.", selection},
		{"Alterar branch da selecção", "b", "Aplica a mesma branch a todas as pipelines.", ""},
		{"Guardar selecção como perfil", "s", "Guarda parâmetros não secretos após confirmação.", selection},
		{"Carregar perfil", "l", "Substitui a selecção. Exige uma nova revisão.", ""},
		{"Consultar lotes anteriores", "h", "Retoma monitorização sem submeter runs.", ""},
		{"Procurar pipelines", "/", "Filtra por nome, ID, tipo, pasta, repositório ou tag.", ""},
		{"Editar parâmetros JSON (avançado)", "J", "Não contorna a validação do schema. Nunca uses segredos.", activeReason},
	}
}

func (m AppModel) updateActions(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	index := *m.actions
	items := m.catalogActions()
	switch key.String() {
	case "esc", "q", "a", "?":
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
	}
	return m, nil
}

func (m AppModel) actionsView() string {
	items := m.catalogActions()
	index := *m.actions
	capacity := max(1, m.height-11)
	start := max(0, index-capacity+1)
	end := min(len(items), start+capacity)
	lines := []string{fmt.Sprintf("%d seleccionadas · escolhe com ↑/↓ e Enter", len(m.catalog.selected))}
	for i := start; i < end; i++ {
		item := items[i]
		label := fmt.Sprintf("  %-5s %s", item.key, item.label)
		if item.blocked != "" {
			label += " [indisponível]"
		}
		label = truncateWidth(label, max(1, m.width-4))
		if i == index {
			label = catalogActiveStyle.Render(">" + label[1:])
		}
		lines = append(lines, label)
	}
	item := items[index]
	detail := item.description
	if item.blocked != "" {
		detail = item.blocked
	}
	lines = append(lines, "", truncateWidth(detail, max(1, m.width-4)), fmt.Sprintf("%d/%d · Os atalhos continuam disponíveis na lista.", index+1, len(items)), shortcutBar(max(1, m.width-4), "↑/↓ escolher", "enter abrir", "esc voltar"))
	return section("ACÇÕES E AJUDA", strings.Join(lines, "\n"), m.width)
}
