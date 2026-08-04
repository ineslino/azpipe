package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ineslino/azpipe/internal/config"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication credentials",
}

var authSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Store legacy PAT config and optional default org/project",
	Example: `  azpipe auth set --pat mytoken123 --org myorg --project myproject

  # Preferred: keep the PAT outside the config file
  export AZDO_PAT=mytoken123
  export AZDO_ORG=myorg`,
	RunE: runAuthSet,
}

var (
	authFlagPAT     string
	authFlagOrg     string
	authFlagProject string
)

func init() {
	authSetCmd.Flags().StringVar(&authFlagPAT, "pat", "", "Personal Access Token (legacy persisted config; prefer AZDO_PAT)")
	authSetCmd.Flags().StringVar(&authFlagOrg, "org", "", "Default Azure DevOps org name")
	authSetCmd.Flags().StringVar(&authFlagProject, "project", "", "Default Azure DevOps project")
	authCmd.AddCommand(authSetCmd)
	rootCmd.AddCommand(authCmd)
}

func runAuthSet(_ *cobra.Command, _ []string) error {
	if authFlagPAT == "" && authFlagOrg == "" && authFlagProject == "" {
		return fmt.Errorf("nothing to set: provide at least one of --pat, --org, --project")
	}

	if authFlagPAT != "" {
		if err := config.SetPAT(authFlagPAT); err != nil {
			return fmt.Errorf("save PAT: %w", err)
		}
		fmt.Println("PAT saved to legacy config. Prefer AZDO_PAT or external credential injection.")
	}
	if authFlagOrg != "" {
		if err := config.SetOrg(authFlagOrg); err != nil {
			return fmt.Errorf("save org: %w", err)
		}
		fmt.Printf("Default org set to %q.\n", authFlagOrg)
	}
	if authFlagProject != "" {
		if err := config.SetProject(authFlagProject); err != nil {
			return fmt.Errorf("save project: %w", err)
		}
		fmt.Printf("Default project set to %q.\n", authFlagProject)
	}
	return nil
}
