package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/ineslino/azpipe/internal/azdo"
)

// Reuse the test executable as a read-only adapter and CLI subprocess on every OS.
func TestMain(m *testing.M) {
	if os.Getenv("AZPIPE_TEST_PIPELINES_LIST") != "1" {
		os.Exit(m.Run())
	}
	if len(os.Args) > 1 && os.Args[1] == "fixture" {
		if len(os.Args) == 3 && os.Args[2] == "whoami" {
			fmt.Print("{\"authenticatedUser\":{\"properties\":{\"Account\":{\"$value\":\"fixture@example.test\"}}}}")
		} else if slices.Contains(os.Args, "--http-method") && slices.Contains(os.Args, "GET") && !slices.Contains(os.Args, "POST") {
			fmt.Print(os.Getenv("AZPIPE_TEST_PIPELINES_RESPONSE"))
		} else {
			os.Exit(2)
		}
		os.Exit(0)
	}
	rootCmd.SetArgs(os.Args[1:])
	Execute()
	os.Exit(0)
}

func TestPipelinesListOutput(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, response, format, want string
	}{
		{"empty JSON", "{\"value\":[]}", "json", "[]\n"},
		{"null JSON", "{\"value\":null}", "json", "[]\n"},
		{"missing JSON", "{}", "json", "[]\n"},
		{"empty table", "{\"value\":[]}", "table", "No pipelines found in project \"fixture-project\".\n"},
		{"empty plain", "{\"value\":[]}", "plain", "No pipelines found in project \"fixture-project\".\n"},
		{"populated JSON", "{\"value\":[{\"id\":202,\"name\":\"fixture pipeline\",\"path\":\"\\\\fixture\",\"repository\":{\"name\":\"fixture-repo\"}}]}", "json", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, executable, "pipelines", "list", "--org", "fixture-org", "--project", "fixture-project", "--output", tc.format)
			command.Env = append(os.Environ(),
				"AZPIPE_TEST_PIPELINES_LIST=1",
				"AZPIPE_TEST_PIPELINES_RESPONSE="+tc.response,
				"AZPIPE_AZDO_AS="+executable,
				"AZPIPE_AUTH_PROFILE=fixture",
				"AZPIPE_EXPECTED_IDENTITY=fixture@example.test",
				"AZPIPE_CONTRACTS=",
				"AZDO_PAT=",
			)
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("CLI: %v; stderr: %s", err, &stderr)
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", &stderr)
			}
			if tc.want != "" {
				if stdout.String() != tc.want {
					t.Fatalf("stdout = %q, want %q", &stdout, tc.want)
				}
				return
			}
			var pipelines []azdo.Pipeline
			if err := json.Unmarshal(stdout.Bytes(), &pipelines); err != nil {
				t.Fatalf("invalid JSON: %v; stdout: %s", err, &stdout)
			}
			if len(pipelines) != 1 || pipelines[0].ID != 202 || pipelines[0].Name != "fixture pipeline" || pipelines[0].RepoName != "fixture-repo" {
				t.Fatalf("populated output changed: %#v", pipelines)
			}
		})
	}
}
