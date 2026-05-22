package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/ineslino/azpipe/internal/ui"
)

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "Manage Azure DevOps projects",
}

var projectsListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all projects in the org",
	Example: "  azpipe projects list --org myorg",
	RunE:    runProjectsList,
}

func init() {
	projectsCmd.AddCommand(projectsListCmd)
	rootCmd.AddCommand(projectsCmd)
}

func runProjectsList(_ *cobra.Command, _ []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}

	projects, err := client.ListProjects(context.Background())
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	if len(projects) == 0 {
		fmt.Println("No projects found.")
		return nil
	}

	switch flagOutput {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(projects)
	case "plain":
		for _, p := range projects {
			fmt.Printf("%s\t%s\n", p.Name, p.ID)
		}
	default:
		rows := make([][]string, len(projects))
		for i, p := range projects {
			rows[i] = []string{p.Name, p.ID}
		}
		fmt.Print(ui.RenderTable(
			[]string{"NAME", "ID"},
			rows,
			[]int{40, 36},
		))
	}
	return nil
}
