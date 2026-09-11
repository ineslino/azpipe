package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ineslino/azpipe/internal/azdo"
	"github.com/ineslino/azpipe/internal/config"
	tuirunner "github.com/ineslino/azpipe/internal/tui/runner"
	"github.com/spf13/cobra"
)

func init() {
	command := &cobra.Command{Use: "branches", Short: "Listar, filtrar e rever a eliminação de branches", Args: cobra.NoArgs, Example: `  azpipe branches --demo
  azpipe branches list --org myorg --project myproject --repo myrepo --creator 'user@example.com'
  azpipe branches delete --org myorg --project myproject --repo myrepo --branch 'feature/example'`, RunE: func(cmd *cobra.Command, _ []string) error {
		demo, _ := cmd.Flags().GetBool("demo")
		if demo {
			return runTUI(tuirunner.NewBranchDemo())
		}
		return runTUI(tuirunner.NewBranchesBootstrap(tuiClientFactory, tuirunner.ContextDefaults{Organization: firstNonEmpty(flagOrg, config.Org()), Project: firstNonEmpty(flagProject, config.Project())}))
	}}
	command.Flags().Bool("demo", false, "Experimentar offline sem eliminar branches")
	list := &cobra.Command{Use: "list", Short: "Listar branches; --creator filtra o criador, não o autor do commit", Args: cobra.NoArgs, Example: "  azpipe branches list --org myorg --project myproject --repo myrepo --creator 'user@example.com' --filter 'feature/'", RunE: runBranchesList}
	list.Flags().String("repo", "", "Nome ou ID do repositório (obrigatório)")
	list.Flags().String("creator", "", "Filtrar por nome, email ou ID do criador")
	list.Flags().String("filter", "", "Filtrar pelo nome da branch")
	command.AddCommand(list)
	del := &cobra.Command{Use: "delete", Short: "Rever uma branch; só elimina com --confirm ELIMINAR e SHA revisto", Args: cobra.NoArgs, Example: `  # Primeiro rever; este comando não elimina
  azpipe branches delete --org myorg --project myproject --repo myrepo --branch 'feature/example'
  # Só depois de confirmar o SHA apresentado
  azpipe branches delete --org myorg --project myproject --repo myrepo --branch 'feature/example' --sha '<sha-da-revisão>' --confirm ELIMINAR`, RunE: runBranchDelete}
	del.Flags().String("repo", "", "Nome ou ID do repositório (obrigatório)")
	del.Flags().String("branch", "", "Nome exacto da branch")
	del.Flags().String("sha", "", "SHA de 40 caracteres mostrado na revisão")
	del.Flags().String("confirm", "", "ELIMINAR para confirmar; omitir para rever")
	command.AddCommand(del)
	rootCmd.AddCommand(command)
}

func branchContext(cmd *cobra.Command) (azdo.BranchClient, string, string, error) {
	project, err := resolveProject()
	if err != nil {
		return nil, "", "", err
	}
	repo, _ := cmd.Flags().GetString("repo")
	if strings.TrimSpace(repo) == "" {
		return nil, "", "", fmt.Errorf("--repo é obrigatório")
	}
	client, err := newClient()
	if err != nil {
		return nil, "", "", err
	}
	api, ok := client.(azdo.BranchClient)
	if !ok {
		return nil, "", "", fmt.Errorf("cliente sem suporte de branches")
	}
	return api, project, repo, nil
}

func runBranchesList(cmd *cobra.Command, _ []string) error {
	api, project, repo, err := branchContext(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()
	branches, err := api.ListBranches(ctx, project, repo)
	if err != nil {
		return err
	}
	creator, _ := cmd.Flags().GetString("creator")
	filter, _ := cmd.Flags().GetString("filter")
	rows := []azdo.Branch{}
	for _, b := range branches {
		if b.Matches(filter, creator) {
			rows = append(rows, b)
		}
	}
	if flagOutput == "json" {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(rows)
	}
	if flagOutput == "plain" {
		for _, b := range rows {
			creator := b.Creator.UniqueName
			if creator == "" {
				creator = b.Creator.DisplayName
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%t\n", b.Name, creator, b.ObjectID, b.IsLocked)
		}
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "BRANCH\tCRIADOR\tSHA\tIS_LOCKED")
	for _, b := range rows {
		creator := b.Creator.UniqueName
		if creator == "" {
			creator = b.Creator.DisplayName
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%t\n", b.Name, creator, b.ObjectID, b.IsLocked)
	}
	return nil
}

func runBranchDelete(cmd *cobra.Command, _ []string) error {
	name, _ := cmd.Flags().GetString("branch")
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("--branch é obrigatório")
	}
	if !strings.HasPrefix(name, "refs/") {
		name = "refs/heads/" + name
	}
	confirm, _ := cmd.Flags().GetString("confirm")
	sha, _ := cmd.Flags().GetString("sha")
	if sha != "" && !branchSHAValue.MatchString(sha) {
		return fmt.Errorf("--sha deve ter 40 caracteres hexadecimais")
	}
	if confirm != "" && (confirm != "ELIMINAR" || sha == "") {
		return fmt.Errorf("é necessário --confirm ELIMINAR e --sha da revisão")
	}
	api, project, repo, err := branchContext(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()
	branches, err := api.ListBranches(ctx, project, repo)
	if err != nil {
		return err
	}
	for _, b := range branches {
		if b.Name != name {
			continue
		}
		if sha != "" && sha != b.ObjectID {
			return fmt.Errorf("SHA diferente; revê novamente")
		}
		reviewed, err := api.ReviewBranch(ctx, project, repo, b)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Projecto: %s\nRepositório: %s\nBranch: %s\nSHA: %s\n", project, repo, b.Name, b.ObjectID)
		if reviewed.Blocked != "" {
			return fmt.Errorf("bloqueada: %s", reviewed.Blocked)
		}
		if confirm == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "Revisão concluída. Não eliminada. Confirma com --sha e --confirm ELIMINAR.")
			return nil
		}
		if err := api.DeleteBranch(ctx, project, repo, reviewed); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Branch eliminada.")
		return nil
	}
	return fmt.Errorf("branch não encontrada")
}

var branchSHAValue = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
