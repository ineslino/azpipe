// Render offline model views as portable documentation SVGs; never initializes an API client.
package main

import (
	"fmt"
	"html"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/tui/runner"
)

func main() {
	if _, err := os.Stat("go.mod"); err != nil {
		panic("run from the repository root")
	}
	models := []struct {
		name  string
		model runner.AppModel
	}{
		{"welcome", runner.NewBootstrapApp(nil, runner.ContextDefaults{Organization: "example-org", Project: "sample-project"})},
		{"catalog", runner.NewDemoApp()},
	}
	for _, item := range models {
		model, _ := item.model.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
		lines := strings.Split(ansi.Strip(model.View()), "\n")
		var svg strings.Builder
		fmt.Fprintf(&svg, `<svg xmlns="http://www.w3.org/2000/svg" width="1240" height="%d" viewBox="0 0 1240 %d"><title>AZPIPE %s: offline model view</title><rect width="100%%" height="100%%" rx="12" fill="#101416"/><g font-family="Menlo,DejaVu Sans Mono,monospace" font-size="19" xml:space="preserve">`, len(lines)*26+40, len(lines)*26+40, item.name)
		for i, line := range lines {
			color := "#e8e9e8"
			if strings.ContainsAny(line, "╭╰") {
				color = "#42caff"
			}
			if strings.Contains(line, "AZPIPE") || strings.Contains(line, "█") || strings.Contains(line, "▪") {
				color = "#c4ee24"
			}
			// Position cells explicitly: SVG viewers differ in whitespace preservation.
			column := 0
			for _, char := range line {
				if char != ' ' {
					fmt.Fprintf(&svg, `<text x="%d" y="%d" fill="%s">%s</text>`, 20+column*12, 30+i*26, color, html.EscapeString(string(char)))
				}
				column += ansi.StringWidth(string(char))
			}
		}
		svg.WriteString("</g></svg>\n")
		if err := os.MkdirAll("docs/assets", 0755); err != nil {
			panic(err)
		}
		if err := os.WriteFile("docs/assets/"+item.name+".svg", []byte(svg.String()), 0644); err != nil {
			panic(err)
		}
	}
}
