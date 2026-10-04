package runner

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ineslino/azpipe/internal/azdo"
)

type parameterField struct{ name, value textinput.Model }
type parameterEditor struct {
	rows           []parameterField
	focus          int
	warning        string
	warningScroll  int
	optionsOpen    bool
	optionSearch   textinput.Model
	optionCursor   int
	optionScroll   int
	optionPageSize int
	schema         *azdo.ParameterSchema
	useDefault     []bool
}

func newSchemaEditor(schema azdo.ParameterSchema, values map[string]string, modeParameter string) (parameterEditor, error) {
	e := parameterEditor{schema: &schema}
	e.optionSearch = textinput.New()
	e.optionSearch.Prompt = "Procurar: "
	e.optionSearch.Placeholder = "valor completo · / editar"
	e.optionSearch.PromptStyle = keyStyle
	e.optionSearch.TextStyle, e.optionSearch.PlaceholderStyle = catalogTextStyle, catalogDetailStyle
	e.optionSearch.CharLimit = 4096
	filtered := []azdo.Parameter{}
	known := map[string]bool{}
	for _, p := range schema.Parameters {
		known[p.Name] = true
		if p.Name == modeParameter {
			continue
		}
		filtered = append(filtered, p)
		value, sent := values[p.Name]
		if sent && !p.Editable() {
			return e, fmt.Errorf("%s usa %s: remova o override avançado para usar o default", p.Name, p.Type)
		}
		if !sent {
			value = p.DefaultValue
		}
		e.rows = append(e.rows, newParameterField(p.Name, value))
		e.useDefault = append(e.useDefault, !sent)
	}
	for name := range values {
		if !known[name] {
			return e, fmt.Errorf("parâmetro removido da pipeline: %s; corrija em J", name)
		}
	}
	e.schema.Parameters = filtered
	if len(e.rows) > 0 {
		e.focus = 1
		e.focusField()
	}
	return e, nil
}

func newParameterField(name, value string) parameterField {
	n, v := textinput.New(), textinput.New()
	n.Prompt, v.Prompt = "Nome:  ", "Valor: "
	n.PromptStyle, v.PromptStyle = keyStyle, keyStyle
	n.CharLimit, v.CharLimit = 128, 4096
	n.SetValue(name)
	v.SetValue(value)
	return parameterField{n, v}
}

func newParameterEditor(values map[string]string) parameterEditor {
	e := parameterEditor{}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		e.rows = append(e.rows, newParameterField(key, values[key]))
	}
	if len(e.rows) == 0 {
		e.rows = append(e.rows, newParameterField("", ""))
	}
	e.focusField()
	return e
}

func (e *parameterEditor) focusField() {
	for i := range e.rows {
		e.rows[i].name.Blur()
		e.rows[i].value.Blur()
	}
	if e.focus%2 == 0 {
		e.rows[e.focus/2].name.Focus()
	} else {
		e.rows[e.focus/2].value.Focus()
	}
}

func (e *parameterEditor) update(msg tea.Msg) tea.Cmd {
	if e.optionsOpen {
		return e.updateOptions(msg)
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if key.String() == "pgdown" || key.String() == "pgup" {
			if key.String() == "pgdown" {
				e.warningScroll++
			} else {
				e.warningScroll = max(0, e.warningScroll-1)
			}
			return nil
		}
		if key.String() == "f2" && e.schema != nil && len(e.rows) > 0 {
			options := e.options(e.focus / 2)
			if len(options) > 0 {
				e.optionsOpen, e.optionCursor = true, 0
				e.optionScroll = 0
				e.optionSearch.SetValue("")
				e.optionSearch.Blur()
				for i, value := range options {
					if value == e.rows[e.focus/2].value.Value() {
						e.optionCursor = i
					}
				}
			}
			return nil
		}
	}
	if e.schema != nil {
		if len(e.rows) == 0 {
			return nil
		}
		i := e.focus / 2
		p := e.schema.Parameters[i]
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "tab", "enter", "down":
				e.focus = ((i+1)%len(e.rows))*2 + 1
				e.focusField()
				return nil
			case "shift+tab", "up":
				e.focus = ((i+len(e.rows)-1)%len(e.rows))*2 + 1
				e.focusField()
				return nil
			case "ctrl+n", "ctrl+x":
				return nil
			case "ctrl+r":
				e.useDefault[i] = true
				e.rows[i].value.SetValue(p.DefaultValue)
				return nil
			}
			if !p.Editable() {
				return nil
			}
			options := e.options(i)
			if len(options) > 0 {
				if key.String() == "left" || key.String() == "right" || key.String() == " " {
					index := 0
					for j, v := range options {
						if e.rows[i].value.Value() == v {
							index = j
						}
					}
					step := 1
					if key.String() == "left" {
						step = len(options) - 1
					}
					e.rows[i].value.SetValue(options[(index+step)%len(options)])
					e.useDefault[i] = false
				}
				return nil
			}
		}
		var cmd tea.Cmd
		before := e.rows[i].value.Value()
		e.rows[i].value, cmd = e.rows[i].value.Update(msg)
		if e.rows[i].value.Value() != before {
			e.useDefault[i] = false
		}
		return cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "tab", "enter":
			e.focus = (e.focus + 1) % (2 * len(e.rows))
			e.focusField()
			return nil
		case "shift+tab":
			e.focus = (e.focus + 2*len(e.rows) - 1) % (2 * len(e.rows))
			e.focusField()
			return nil
		case "ctrl+n":
			e.rows = append(e.rows, newParameterField("", ""))
			e.focus = 2 * (len(e.rows) - 1)
			e.focusField()
			return nil
		case "ctrl+x":
			i := e.focus / 2
			e.rows = append(e.rows[:i], e.rows[i+1:]...)
			if len(e.rows) == 0 {
				e.rows = append(e.rows, newParameterField("", ""))
			}
			e.focus = min(e.focus, 2*len(e.rows)-1)
			e.focusField()
			return nil
		}
	}
	var cmd tea.Cmd
	if e.focus%2 == 0 {
		e.rows[e.focus/2].name, cmd = e.rows[e.focus/2].name.Update(msg)
	} else {
		e.rows[e.focus/2].value, cmd = e.rows[e.focus/2].value.Update(msg)
	}
	return cmd
}

func (e *parameterEditor) updateOptions(msg tea.Msg) tea.Cmd {
	key, isKey := msg.(tea.KeyMsg)
	if e.optionSearch.Focused() {
		if isKey && (key.String() == "esc" || key.String() == "enter") {
			e.optionSearch.Blur()
			return nil
		}
		before := e.optionSearch.Value()
		var cmd tea.Cmd
		if isKey && key.String() == "ctrl+u" {
			e.optionSearch.SetValue("")
		} else {
			e.optionSearch, cmd = e.optionSearch.Update(msg)
		}
		if before != e.optionSearch.Value() {
			e.optionCursor, e.optionScroll = 0, 0
		}
		return cmd
	}
	if !isKey {
		return nil
	}
	// A pasted /query follows the same search path as individual keystrokes.
	if key.Type == tea.KeyRunes && !key.Alt && len(key.Runes) > 1 && key.Runes[0] == '/' {
		focus := e.optionSearch.Focus()
		key.Runes = key.Runes[1:]
		return tea.Batch(focus, e.updateOptions(key))
	}
	options := e.filteredOptions()
	switch key.String() {
	case "esc", "f2":
		e.optionsOpen = false
	case "/":
		return e.optionSearch.Focus()
	case "ctrl+u":
		e.optionSearch.SetValue("")
		e.optionCursor, e.optionScroll = 0, 0
	case "up", "ctrl+pgup":
		step := 1
		if key.String() == "ctrl+pgup" {
			step = max(1, e.optionPageSize)
		}
		e.optionCursor = max(0, e.optionCursor-step)
		e.optionScroll = 0
	case "down", "ctrl+pgdown":
		step := 1
		if key.String() == "ctrl+pgdown" {
			step = max(1, e.optionPageSize)
		}
		e.optionCursor = min(max(0, len(options)-1), e.optionCursor+step)
		e.optionScroll = 0
	case "home":
		e.optionCursor, e.optionScroll = 0, 0
	case "end":
		e.optionCursor, e.optionScroll = max(0, len(options)-1), 0
	case "pgup":
		e.optionScroll = max(0, e.optionScroll-1)
	case "pgdown":
		e.optionScroll++
	case "enter":
		if len(options) > 0 {
			e.rows[e.focus/2].value.SetValue(options[e.optionCursor])
			e.useDefault[e.focus/2] = false
			e.optionsOpen = false
		}
	}
	return nil
}

func (e parameterEditor) filteredOptions() []string {
	options := e.options(e.focus / 2)
	query := strings.ToLower(strings.TrimSpace(e.optionSearch.Value()))
	if query == "" {
		return options
	}
	filtered := make([]string, 0, len(options))
	for _, value := range options {
		if strings.Contains(strings.ToLower(value), query) {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func (e parameterEditor) options(i int) []string {
	if e.schema == nil || !e.schema.Parameters[i].Editable() {
		return nil
	}
	p := e.schema.Parameters[i]
	if p.Type == "boolean" && len(p.Values) == 0 {
		return []string{"false", "true"}
	}
	return p.Values
}

func (e *parameterEditor) setWarning(err error) {
	e.warning, e.warningScroll = err.Error(), 0
	var invalid *azdo.ParameterValidationError
	if e.schema != nil && errors.As(err, &invalid) {
		for i, p := range e.schema.Parameters {
			if p.Name == invalid.Parameter {
				e.focus = i*2 + 1
				e.focusField()
				e.warning = strings.Replace(e.warning, p.Name, p.DisplayName, 1)
				break
			}
		}
	}
}

func (e parameterEditor) values() (map[string]string, error) {
	values := map[string]string{}
	if e.schema != nil {
		for i, row := range e.rows {
			if !e.useDefault[i] {
				values[row.name.Value()] = row.value.Value()
			}
		}
		return values, e.schema.Validate(values)
	}
	for _, row := range e.rows {
		name, value := strings.TrimSpace(row.name.Value()), row.value.Value()
		if name == "" && value == "" {
			continue
		}
		if name == "" {
			return nil, fmt.Errorf("Preencha o nome do parâmetro.")
		}
		if _, ok := values[name]; ok {
			return nil, fmt.Errorf("Nome repetido: %s", name)
		}
		values[name] = value
	}
	return values, nil
}

func (e parameterEditor) view(width, height int, name string) string {
	if e.schema != nil {
		return e.schemaView(width, height, name)
	}
	lines := []string{catalogTitleStyle.Render("Parâmetros · " + name), catalogDetailStyle.Render("Campos manuais. Não introduza segredos. Preview valida os valores."), ""}
	count := max(1, (height-11)/3)
	start := max(0, e.focus/2-count+1)
	for i := start; i < min(len(e.rows), start+count); i++ {
		row := e.rows[i]
		row.name.Width = max(8, width-10)
		row.value.Width = max(8, width-10)
		lines = append(lines, row.name.View(), row.value.View(), "")
	}
	lines = append(lines, catalogDetailStyle.Render(fmt.Sprintf("Parâmetro %d de %d", e.focus/2+1, len(e.rows))))
	if e.warning != "" {
		lines = append(lines, catalogWarningStyle.Render(e.warning))
	}
	lines = append(lines, shortcutBar(width, "tab próximo campo", "ctrl+n adicionar", "ctrl+x remover", "ctrl+s guardar", "esc descartar"))
	return strings.Join(lines, "\n")
}

func (e parameterEditor) schemaView(width, height int, name string) string {
	if e.optionsOpen {
		options := e.filteredOptions()
		lines, footer, count := e.optionsLayout(width, height, options)
		start := max(0, e.optionCursor-count+1)
		for i := start; i < min(len(options), start+count); i++ {
			style, marker := catalogTextStyle, "  "
			if i == e.optionCursor {
				style, marker = catalogActiveStyle.Width(width), "> "
			}
			lines = append(lines, style.Render(truncateWidth(marker+options[i], width)))
		}
		return strings.Join(append(lines, footer...), "\n")
	}
	position := "Sem campos editáveis."
	if len(e.rows) > 0 {
		position = fmt.Sprintf("Campo %d de %d · Tab percorre todos", e.focus/2+1, len(e.rows))
	}
	lines := []string{catalogTitleStyle.Render(truncateWidth("Configurar · "+name, width)), catalogDetailStyle.Render(position), "Ctrl+S aplica apenas à pipeline activa.", "Não introduzas segredos.", ""}
	count := max(1, (height-13)/4)
	start := max(0, e.focus/2-count+1)
	for i := start; i < min(len(e.rows), start+count); i++ {
		p := e.schema.Parameters[i]
		row := e.rows[i]
		row.value.Width = max(8, width-10)
		status := "obrigatório"
		if p.HasDefault {
			status = "predefinido pela pipeline"
		}
		if !e.useDefault[i] {
			status = "valor personalizado"
		}
		kind := p.Type
		switch kind {
		case "number":
			kind = "número"
		case "string":
			kind = "texto"
		}
		if len(e.options(i)) > 0 {
			kind = "escolha"
		}
		label := fmt.Sprintf("%s [%s · %s]", p.DisplayName, kind, status)
		style := catalogDetailStyle
		if i == e.focus/2 {
			style = catalogHeaderStyle
			label = "> " + label
		}
		lines = append(lines, style.Render(truncateWidth(label, width)))
		if !p.Editable() {
			lines = append(lines, catalogDetailStyle.Render("  Tipo complexo: apenas default YAML; não editável aqui."))
		} else {
			lines = append(lines, row.value.View())
		}
		if i == e.focus/2 && len(e.options(i)) > 0 {
			options := e.options(i)
			if len(options) <= 4 {
				lines = append(lines, catalogDetailStyle.Render(truncateWidth("Opções: "+strings.Join(options, " | "), width)))
			} else {
				lines = append(lines, catalogDetailStyle.Render(fmt.Sprintf("%d opções · F2 consulta a lista completa", len(options))))
			}
		} else {
			lines = append(lines, "")
		}
		lines = append(lines, "")
	}
	if len(e.rows) == 0 {
		lines = append(lines, "Sem parâmetros editáveis. RUN/PLAN é controlado no catálogo.")
	}
	if e.warning != "" {
		lines = append(lines, catalogWarningStyle.Render(textPage(e.warning, width, e.warningScroll, 2)), "PgUp/PgDn: percorrer erro completo")
	}
	lines = append(lines, "", shortcutBar(width, "tab próximo", "←/→ escolher opção", "f2 opções e detalhe", "ctrl+r repor predefinido", "ctrl+s aplicar", "esc descartar"))
	return strings.Join(lines, "\n")
}

func (e parameterEditor) optionsLayout(width, height int, options []string) ([]string, []string, int) {
	position := fmt.Sprintf("Opção %d de %d", min(e.optionCursor+1, len(options)), len(options))
	if len(options) == 0 {
		position = "0 opções encontradas"
	}
	if len(options) != len(e.options(e.focus/2)) {
		position += fmt.Sprintf(" · %d no total", len(e.options(e.focus/2)))
	}
	e.optionSearch.Width = max(1, width-lipgloss.Width(e.optionSearch.Prompt)-1)
	instruction := "Enter escolhe; Esc conserva o valor actual."
	if len(options) == 0 {
		instruction = "Altera a pesquisa; Ctrl+U limpa o filtro."
	}
	if e.optionSearch.Focused() {
		instruction = "Enter/Esc termina a pesquisa; o filtro mantém-se."
	}
	lines := []string{catalogTitleStyle.Render(truncateWidth("Opções · "+e.schema.Parameters[e.focus/2].DisplayName, width)), catalogDetailStyle.Render(position), e.optionSearch.View(), instruction, ""}
	shortcuts := shortcutBar(width, "↑/↓ escolher", "home/end extremos", "/ procurar", "Ctrl+PgUp/PgDn página", "PgUp/PgDn detalhe", "enter aplicar opção", "esc voltar")
	footer := []string{""}
	if len(options) > 0 {
		footer = append(footer, catalogHeaderStyle.Render("Valor completo da opção focada:"), catalogTextStyle.Render(textPage(options[e.optionCursor], width, e.optionScroll, 3)))
	} else {
		footer = append(footer, catalogWarningStyle.Render("Nenhuma opção encontrada."), "Ctrl+U limpa a pesquisa; / permite editá-la.")
		shortcuts = shortcutBar(width, "/ procurar", "ctrl+u limpar", "esc voltar")
	}
	if e.optionSearch.Focused() {
		shortcuts = shortcutBar(width, "enter/esc terminar pesquisa", "ctrl+u limpar")
	}
	footer = append(footer, "", shortcuts)
	// Use the same measured capacity for rendering and page navigation.
	count := max(1, height-lipgloss.Height(strings.Join(lines, "\n"))-lipgloss.Height(strings.Join(footer, "\n")))
	return lines, footer, count
}
