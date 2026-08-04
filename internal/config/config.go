package config

import (
	"bytes"
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
	return save(map[string]string{"pat": pat})
}

func SetOrg(org string) error {
	viper.Set("org", org)
	return save(map[string]string{"org": org})
}

func SetProject(project string) error {
	viper.Set("project", project)
	return save(map[string]string{"project": project})
}

func save(changes map[string]string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, configDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, configFile+"."+configType)
	values := make(map[string]string, 3)
	if contents, readErr := os.ReadFile(path); readErr == nil && len(contents) > 0 {
		existing := viper.New()
		existing.SetConfigType(configType)
		if err := existing.ReadConfig(bytes.NewReader(contents)); err != nil {
			return err
		}
		for _, key := range []string{"pat", "org", "project"} {
			if existing.IsSet(key) {
				values[key] = existing.GetString(key)
			}
		}
	} else if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	for key, value := range changes {
		values[key] = value
	}

	persisted := viper.New()
	persisted.SetConfigType(configType)
	persisted.SetConfigPermissions(0o600)
	for key, value := range values {
		persisted.Set(key, value)
	}
	if err := persisted.WriteConfigAs(path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
