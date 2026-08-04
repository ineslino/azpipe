package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ineslino/azpipe/internal/azdo"
	"github.com/ineslino/azpipe/internal/config"
	"github.com/spf13/cobra"
)

var (
	flagOrg     string
	flagProject string
	flagOutput  string
	flagDebug   bool
)

var rootCmd = &cobra.Command{
	Use:   "azpipe",
	Short: "Azure DevOps pipeline visibility and analysis",
	Long: `azpipe is a terminal tool for DevOps engineers to inspect pipeline health,
run history trends, stage/job breakdowns, and repo-to-pipeline mappings
across Azure DevOps projects.

Auth: set AZDO_PAT and AZDO_ORG, or run 'azpipe auth set'.`,
	Args: cobra.NoArgs,
	RunE: runRootTUI,
}

// Execute runs the root command. Called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(config.Init)

	rootCmd.PersistentFlags().StringVar(&flagOrg, "org", "", "Azure DevOps org name or URL (overrides AZDO_ORG)")
	rootCmd.PersistentFlags().StringVar(&flagProject, "project", "", "Azure DevOps project name")
	rootCmd.PersistentFlags().StringVarP(&flagOutput, "output", "o", "table", "Output format: table|json|plain")
	rootCmd.PersistentFlags().BoolVar(&flagDebug, "debug", false, "Enable debug output")
}

// resolveOrg returns the org URL, checking flag → env/config → error.
func resolveOrg() (string, error) {
	o := flagOrg
	if o == "" {
		o = config.Org()
	}
	if o == "" {
		return "", fmt.Errorf("org is required: set --org, AZDO_ORG, or run 'azpipe auth set --org <name>'")
	}
	return toOrgURL(o), nil
}

// resolveProject returns the project name, checking flag → config → error.
func resolveProject() (string, error) {
	p := flagProject
	if p == "" {
		p = config.Project()
	}
	if p == "" {
		return "", fmt.Errorf("project is required: set --project or run 'azpipe auth set --project <name>'")
	}
	return p, nil
}

// resolvePAT returns the PAT from config, or errors.
func resolvePAT() (string, error) {
	t := config.PAT()
	if t == "" {
		return "", fmt.Errorf("PAT not set: run 'azpipe auth set --pat <token>' or export AZDO_PAT=<token>")
	}
	return t, nil
}

// newClient builds an azdo.Client from the current flag/config state.
func newClient() (azdo.Client, error) {
	orgURL, err := resolveOrg()
	if err != nil {
		return nil, err
	}
	pat, err := resolvePAT()
	if err != nil {
		return nil, err
	}
	return azdo.New(orgURL, pat), nil
}

// toOrgURL ensures the org value is a full Azure DevOps URL.
func toOrgURL(org string) string {
	if strings.HasPrefix(org, "http://") || strings.HasPrefix(org, "https://") {
		return org
	}
	return "https://dev.azure.com/" + org
}

// formatDuration formats a duration for display (e.g. "3m 45s").
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "—"
	}
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// formatTime formats a time.Time for display.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04")
}
