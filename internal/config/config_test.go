package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ineslino/azpipe/internal/localfile"
	"github.com/spf13/viper"
)

func TestSetPAT_WritesConfigWithRestrictedPermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	viper.Reset()
	t.Cleanup(viper.Reset)

	if err := SetPAT("test-pat"); err != nil {
		t.Fatalf("SetPAT: %v", err)
	}

	configPath := filepath.Join(home, configDir, configFile+"."+configType)
	_, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := localfile.Check(configPath); err != nil {
		t.Fatal(err)
	}
}

func TestSetAuthPersistsAdapterMetadataWithoutPAT(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	viper.Reset()
	t.Cleanup(viper.Reset)

	if err := SetAuth("/bin/adapter", "profile", "user@example.com"); err != nil {
		t.Fatalf("SetAuth: %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(home, configDir, configFile+"."+configType))
	if err != nil {
		t.Fatal(err)
	}
	value := string(contents)
	for _, expected := range []string{"auth_executable", "auth_profile", "expected_identity", "/bin/adapter", "profile", "user@example.com"} {
		if !strings.Contains(value, expected) {
			t.Fatalf("config missing %q:\n%s", expected, value)
		}
	}
	if strings.Contains(value, "pat:") {
		t.Fatalf("adapter settings unexpectedly persisted a PAT:\n%s", value)
	}
}

func TestSetDefaults_DoNotPersistEnvironmentPAT(t *testing.T) {
	for _, test := range []struct {
		name string
		set  func(string) error
	}{
		{name: "organization", set: SetOrg},
		{name: "project", set: SetProject},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("AZDO_PAT", "environment-secret")
			viper.Reset()
			t.Cleanup(viper.Reset)
			Init()

			if err := test.set("example-value"); err != nil {
				t.Fatalf("set default: %v", err)
			}

			contents, err := os.ReadFile(filepath.Join(home, configDir, configFile+"."+configType))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(contents), "environment-secret") || strings.Contains(string(contents), "pat:") {
				t.Fatalf("environment PAT was persisted:\n%s", contents)
			}
		})
	}
}

func TestSetOrg_PreservesPersistedPATInsteadOfEnvironmentPATAndRestrictsExistingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AZDO_PAT", "environment-secret")
	dir := filepath.Join(home, configDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, configFile+"."+configType)
	if err := os.WriteFile(path, []byte("pat: persisted-secret\nproject: existing-project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	viper.Reset()
	t.Cleanup(viper.Reset)
	Init()

	if err := SetOrg("example-org"); err != nil {
		t.Fatalf("SetOrg: %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "persisted-secret") || strings.Contains(string(contents), "environment-secret") {
		t.Fatalf("persisted config used the wrong PAT:\n%s", contents)
	}
	if err := localfile.Check(path); err != nil {
		t.Fatal(err)
	}
}
