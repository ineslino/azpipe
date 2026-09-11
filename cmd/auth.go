package cmd

import (
	"fmt"

	"github.com/ineslino/azpipe/internal/config"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication credentials",
}

var authSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Store authentication settings and optional defaults",
	Example: `  azpipe auth set --pat mytoken123 --org myorg --project myproject

  # Preferred: keep the PAT outside the config file
  export AZDO_PAT=mytoken123
  export AZDO_ORG=myorg`,
	RunE: runAuthSet,
}

var (
	authFlagPAT        string
	authFlagOrg        string
	authFlagProject    string
	authFlagExecutable string
	authFlagProfile    string
	authFlagIdentity   string
)

func init() {
	authSetCmd.Flags().StringVar(&authFlagPAT, "pat", "", "Personal Access Token (legacy persisted config; prefer AZDO_PAT)")
	authSetCmd.Flags().StringVar(&authFlagOrg, "org", "", "Default Azure DevOps org name")
	authSetCmd.Flags().StringVar(&authFlagProject, "project", "", "Default Azure DevOps project")
	authSetCmd.Flags().StringVar(&authFlagExecutable, "auth-executable", "", "Optional credential adapter executable")
	authSetCmd.Flags().StringVar(&authFlagProfile, "auth-profile", "", "Credential adapter profile")
	authSetCmd.Flags().StringVar(&authFlagIdentity, "expected-identity", "", "Expected identity returned by the adapter")
	authCmd.AddCommand(authSetCmd)
	rootCmd.AddCommand(authCmd)
}

func runAuthSet(_ *cobra.Command, _ []string) error {
	if authFlagPAT == "" && authFlagOrg == "" && authFlagProject == "" && authFlagExecutable == "" && authFlagProfile == "" && authFlagIdentity == "" {
		return fmt.Errorf("nothing to set: provide authentication settings or --org/--project")
	}
	if authFlagExecutable != "" || authFlagProfile != "" || authFlagIdentity != "" {
		if authFlagExecutable == "" || authFlagProfile == "" || authFlagIdentity == "" {
			return fmt.Errorf("a configuração do adaptador requer --auth-executable, --auth-profile e --expected-identity")
		}
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
	if authFlagExecutable != "" || authFlagProfile != "" || authFlagIdentity != "" {
		if err := config.SetAuth(authFlagExecutable, authFlagProfile, authFlagIdentity); err != nil {
			return fmt.Errorf("save authentication settings: %w", err)
		}
		fmt.Println("Adaptador de credenciais configurado. Nenhum segredo foi guardado.")
	}
	return nil
}
