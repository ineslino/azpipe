package runner

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ineslino/azpipe/internal/azdo"
)

type parameterField struct{ name, value textinput.Model }
type parameterEditor struct {
	rows          []parameterField
	focus         int
	warning       string
	warningScroll int
	optionsOpen   bool
	optionCursor  int
	optionScroll  int
	schema        *azdo.ParameterSchema
	useDefault    []bool
}

func newSchemaEditor(schema azdo.ParameterSchema, values map[string]string, modeParameter string) (parameterEditor, error) {
	e := parameterEditor{schema: &schema}
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
	if key, ok := msg.(tea.KeyMsg); ok {
		if e.optionsOpen {
			options := e.options(e.focus / 2)
			switch key.String() {
			case "esc", "f2":
				e.optionsOpen = false
			case "up":
				e.optionCursor = max(0, e.optionCursor-1)
				e.optionScroll = 0
			case "down":
				e.optionCursor = min(len(options)-1, e.optionCursor+1)
				e.optionScroll = 0
			case "pgup":
				e.optionScroll = max(0, e.optionScroll-1)
			case "pgdown":
				e.optionScroll++
			case "enter":
				e.rows[e.focus/2].value.SetValue(options[e.optionCursor])
				e.useDefault[e.focus/2] = false
				e.optionsOpen = false
			}
			return nil
		}
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
		options := e.options(e.focus / 2)
		lines := []string{catalogTitleStyle.Render(truncateWidth("Opções · "+e.schema.Parameters[e.focus/2].DisplayName, width)), "", "Enter escolhe; Esc conserva o valor actual.", ""}
		count := max(1, height-13)
		start := max(0, e.optionCursor-count+1)
		for i := start; i < min(len(options), start+count); i++ {
			style, marker := catalogTextStyle, "  "
			if i == e.optionCursor {
				style, marker = catalogActiveStyle.Width(width), "> "
			}
			lines = append(lines, style.Render(truncateWidth(marker+options[i], width)))
		}
		lines = append(lines, "", catalogHeaderStyle.Render("Valor completo da opção focada:"), catalogTextStyle.Render(textPage(options[e.optionCursor], width, e.optionScroll, 3)))
		return strings.Join(append(lines, "", shortcutBar(width, "↑/↓ escolher", "PgUp/PgDn detalhe", "enter aplicar opção", "esc voltar")), "\n")
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
