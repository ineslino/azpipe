package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle  = lipgloss.NewStyle().Bold(true)
	dividerStyle = lipgloss.NewStyle().Faint(true)
)

// RenderTable renders headers and rows into a plain padded table string.
// widths is the visual column width for each column; values are truncated or
// padded accordingly. Accepts pre-styled (ANSI-escaped) cell values.
func RenderTable(headers []string, rows [][]string, widths []int) string {
	var sb strings.Builder

	// Header row
	for i, h := range headers {
		if i > 0 {
			sb.WriteString("  ")
		}
		w := colWidth(widths, i)
		sb.WriteString(headerStyle.Width(w).MaxWidth(w).Render(h))
	}
	sb.WriteString("\n")

	// Divider
	total := 0
	for i, w := range widths {
		total += w
		if i < len(widths)-1 {
			total += 2
		}
	}
	sb.WriteString(dividerStyle.Render(strings.Repeat("─", total)) + "\n")

	// Data rows
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(headers) {
				break
			}
			if i > 0 {
				sb.WriteString("  ")
			}
			w := colWidth(widths, i)
			sb.WriteString(lipgloss.NewStyle().Width(w).MaxWidth(w).Render(cell))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ResultBadge returns a coloured string representing a pipeline run result.
func ResultBadge(result string) string {
	switch result {
	case "succeeded":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓ succeeded")
	case "failed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("✗ failed")
	case "partiallySucceeded":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("⚠ partial")
	case "canceled":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○ canceled")
	case "none", "":
		return "—"
	default:
		return result
	}
}

func colWidth(widths []int, i int) int {
	if i < len(widths) {
		return widths[i]
	}
	return 20
}
