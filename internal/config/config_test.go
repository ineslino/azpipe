package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestSetPAT_WritesConfigWithRestrictedPermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	viper.Reset()
	t.Cleanup(viper.Reset)

	if err := SetPAT("test-pat"); err != nil {
		t.Fatalf("SetPAT: %v", err)
	}

	configPath := filepath.Join(home, configDir, configFile+"."+configType)
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode=%o, want 600", got)
	}
}
