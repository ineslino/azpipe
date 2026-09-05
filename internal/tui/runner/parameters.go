package runner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ineslino/azpipe/internal/azdo"
)

type parameterField struct{ name, value textinput.Model }
type parameterEditor struct {
	rows       []parameterField
	focus      int
	warning    string
	schema     *azdo.ParameterSchema
	useDefault []bool
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
			options := p.Values
			if p.Type == "boolean" && len(options) == 0 {
				options = []string{"false", "true"}
			}
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
			if key.Type == tea.KeyRunes || key.Type == tea.KeyBackspace || key.Type == tea.KeyDelete || key.Type == tea.KeyCtrlK || key.Type == tea.KeyCtrlU {
				e.useDefault[i] = false
			}
		}
		var cmd tea.Cmd
		e.rows[i].value, cmd = e.rows[i].value.Update(msg)
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
	lines := []string{catalogTitleStyle.Render(truncateWidth("Parâmetros · "+name, width)), catalogDetailStyle.Render(truncateWidth(fmt.Sprintf("YAML · SHA %s · definição %d", e.schema.Commit, e.schema.DefinitionVersion), width)), catalogDetailStyle.Render("Sem segredos. Defaults são omitidos do pedido."), ""}
	count := max(1, (height-12)/3)
	start := max(0, e.focus/2-count+1)
	for i := start; i < min(len(e.rows), start+count); i++ {
		p := e.schema.Parameters[i]
		row := e.rows[i]
		row.value.Width = max(8, width-10)
		status := "obrigatório"
		if p.HasDefault {
			status = "default"
		}
		if !e.useDefault[i] {
			status = "override"
		}
		label := fmt.Sprintf("%s [%s · %s]", p.DisplayName, p.Type, status)
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
		lines = append(lines, "")
	}
	if len(e.rows) == 0 {
		lines = append(lines, "Sem parâmetros editáveis. RUN/PLAN é controlado no catálogo.")
	}
	if e.warning != "" {
		lines = append(lines, catalogWarningStyle.Render(truncateWidth(e.warning, width)))
	}
	lines = append(lines, shortcutBar(width, "tab próximo", "←/→ escolher opção", "ctrl+r default", "ctrl+s guardar", "esc descartar"))
	return strings.Join(lines, "\n")
}
