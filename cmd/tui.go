package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ineslino/azpipe/internal/azdo"
	"github.com/ineslino/azpipe/internal/config"
	tuirunner "github.com/ineslino/azpipe/internal/tui/runner"
	"github.com/spf13/cobra"
)

var tuiClientFactory tuirunner.ClientFactory = func(organization string) (azdo.Client, error) {
	pat, err := resolvePAT()
	if err != nil {
		return nil, err
	}
	return azdo.New(toOrgURL(organization), pat), nil
}

var runTUI = func(model tea.Model) error {
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Experimentar o fluxo de execução sem Azure DevOps",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runTUI(tuirunner.NewDemoApp())
	},
}

func init() {
	rootCmd.AddCommand(demoCmd)
}

func runRootTUI(_ *cobra.Command, _ []string) error {
	organization := flagOrg
	if organization == "" {
		organization = config.Org()
	}
	project := flagProject
	if project == "" {
		project = config.Project()
	}
	return runTUI(tuirunner.NewBootstrapApp(tuiClientFactory, tuirunner.ContextDefaults{
		Organization: organization,
		Project:      project,
	}))
}
