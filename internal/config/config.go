package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	configFile = "config"
	configType = "yaml"
	configDir  = ".config/azpipe"
)

// Init configures viper. Called by cobra.OnInitialize.
func Init() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	viper.SetConfigName(configFile)
	viper.SetConfigType(configType)
	viper.AddConfigPath(filepath.Join(home, configDir))

	// Explicit env var bindings — no prefix, exact names.
	_ = viper.BindEnv("pat", "AZDO_PAT")
	_ = viper.BindEnv("org", "AZDO_ORG")

	_ = viper.ReadInConfig()
}

func PAT() string     { return viper.GetString("pat") }
func Org() string     { return viper.GetString("org") }
func Project() string { return viper.GetString("project") }

func SetPAT(pat string) error {
	viper.Set("pat", pat)
	return save()
}

func SetOrg(org string) error {
	viper.Set("org", org)
	return save()
}

func SetProject(project string) error {
	viper.Set("project", project)
	return save()
}

func save() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, configDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return viper.WriteConfigAs(filepath.Join(dir, configFile+"."+configType))
}
