// Export a deterministic walkthrough of the real offline TUI model.
package main

import (
	"encoding/json"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/tui/runner"
)

type frame struct {
	Caption string `json:"caption"`
	View    string `json:"view"`
	Seconds int    `json:"seconds"`
}

func main() {
	var m tea.Model = runner.NewDemoApp()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	frames := []frame{}
	snap := func(caption, expected string, seconds int) {
		view := ansi.Strip(m.View())
		if !strings.Contains(view, expected) {
			panic("unexpected demo screen: " + caption)
		}
		frames = append(frames, frame{caption, view, seconds})
	}
	key := func(k tea.KeyType, text string, run bool) {
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyMsg{Type: k, Runes: []rune(text)})
		if run && cmd != nil {
			m, _ = m.Update(cmd())
		}
	}
	runeKey := func(s string) { key(tea.KeyRunes, s, false) }
	snap("01 / Pipelines num só terminal", "build application", 4)
	runeKey("/")
	runeKey("deploy")
	key(tea.KeyEnter, "", false)
	snap("02 / Filtra por nome, repo ou tag", "deploy infrastructure", 4)
	runeKey(" ")
	runeKey("m")
	snap("03 / Selecciona e escolhe PLAN ou RUN", "1 PLAN", 4)
	key(tea.KeyEnter, "", true)
	snap("04 / Revê branch, SHA e parâmetros antes de continuar", "VALIDAÇÃO", 5)
	key(tea.KeyEnter, "", false)
	snap("05 / Estados por run: exemplo estático de histórico", "Demonstração estática", 5)
	key(tea.KeyEsc, "", false)
	runeKey("B")
	snap("06 / Branches por repositório, com detalhe e protecções", "BLOQUEADA", 4)
	runeKey("u")
	runeKey("user@example.com")
	key(tea.KeyEnter, "", false)
	snap("07 / Filtra pelo criador da branch", "user@example.com", 4)
	key(tea.KeyDown, "", false)
	runeKey(" ")
	key(tea.KeyEnter, "", true)
	snap("08 / Revê a selecção antes de eliminar", "REVER ELIMINAÇÃO", 5)
	key(tea.KeyEsc, "", false)
	key(tea.KeyEsc, "", true)
	frames = append(frames, frame{Caption: "Experimenta o azpipe", View: "\n\n  AZPIPE\n\n  Pipelines e branches num só terminal.\n\n\n  Experimenta sem credenciais:\n\n      azpipe demo\n\n\n  Código e instruções:\n\n      github.com/ineslino/azpipe\n\n\n  Fim da demonstração offline.", Seconds: 4})
	if err := json.NewEncoder(os.Stdout).Encode(frames); err != nil {
		panic(err)
	}
}
