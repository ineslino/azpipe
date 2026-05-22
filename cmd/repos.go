package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/ineslino/azpipe/internal/ui"
)

var reposCmd = &cobra.Command{
	Use:   "repos",
	Short: "Inspect Azure DevOps repositories",
}

var reposListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List repositories in a project",
	Example: "  azpipe repos list --org myorg --project myproject",
	RunE:    runReposList,
}

var reposPipelinesCmd = &cobra.Command{
	Use:     "pipelines <repo-name>",
	Short:   "Show pipelines linked to a repository",
	Args:    cobra.ExactArgs(1),
	Example: "  azpipe repos pipelines myrepo --org myorg --project myproject",
	RunE:    runReposPipelines,
}

func init() {
	reposCmd.AddCommand(reposListCmd)
	reposCmd.AddCommand(reposPipelinesCmd)
	rootCmd.AddCommand(reposCmd)
}

func runReposList(_ *cobra.Command, _ []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	proj, err := resolveProject()
	if err != nil {
		return err
	}

	repos, err := client.ListRepositories(context.Background(), proj)
	if err != nil {
		return fmt.Errorf("list repositories: %w", err)
	}

	if len(repos) == 0 {
		fmt.Printf("No repositories found in project %q.\n", proj)
		return nil
	}

	switch flagOutput {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(repos)
	case "plain":
		for _, r := range repos {
			fmt.Printf("%s\t%s\n", r.Name, r.DefaultBranch)
		}
	default:
		rows := make([][]string, len(repos))
		for i, r := range repos {
			rows[i] = []string{r.Name, r.DefaultBranch, r.RemoteURL}
		}
		fmt.Print(ui.RenderTable(
			[]string{"NAME", "DEFAULT BRANCH", "REMOTE URL"},
			rows,
			[]int{32, 22, 70},
		))
	}
	return nil
}

func runReposPipelines(_ *cobra.Command, args []string) error {
	repoName := args[0]
	client, err := newClient()
	if err != nil {
		return err
	}
	proj, err := resolveProject()
	if err != nil {
		return err
	}

	pipelines, err := client.GetRepoPipelines(context.Background(), proj, repoName)
	if err != nil {
		return fmt.Errorf("get repo pipelines: %w", err)
	}

	if len(pipelines) == 0 {
		fmt.Printf("No pipelines linked to repository %q.\n", repoName)
		return nil
	}

	switch flagOutput {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pipelines)
	case "plain":
		for _, p := range pipelines {
			fmt.Printf("%d\t%s\t%s\n", p.ID, p.Name, p.Folder)
		}
	default:
		rows := make([][]string, len(pipelines))
		for i, p := range pipelines {
			rows[i] = []string{fmt.Sprintf("%d", p.ID), p.Name, p.Folder}
		}
		fmt.Print(ui.RenderTable(
			[]string{"ID", "NAME", "FOLDER"},
			rows,
			[]int{8, 45, 30},
		))
	}
	return nil
}
