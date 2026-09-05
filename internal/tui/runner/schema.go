package runner

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ineslino/azpipe/internal/azdo"
)

type schemaLoadedMsg struct {
	token  operationToken
	id     int
	branch string
	schema azdo.ParameterSchema
	err    error
}

func (m AppModel) loadSchema(id int, branch string, token operationToken) tea.Cmd {
	return func() tea.Msg {
		if m.demo {
			s, _ := azdo.ParseParameterSchema("parameters:\n- name: environment\n  displayName: Ambiente\n  type: string\n  default: dev\n  values: [dev, staging, prod]\n- name: tests\n  displayName: Executar testes\n  type: boolean\n  default: true\n- name: replicas\n  displayName: Réplicas\n  type: number\n  default: 2\n")
			s.Commit = "0123456789012345678901234567890123456789"
			s.DefinitionVersion = 7
			return schemaLoadedMsg{token: token, id: id, branch: branch, schema: s}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s, err := m.service.Schema(ctx, id, branch)
		return schemaLoadedMsg{token: token, id: id, branch: branch, schema: s, err: err}
	}
}
