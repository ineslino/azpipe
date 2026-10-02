package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDiscoverLocalBranchesClassifiesWorktreeAndLocalBranches(t *testing.T) {
	repository := t.TempDir()
	runGit(t, repository, "init", "-b", "main")
	runGit(t, repository, "config", "user.name", "azpipe test")
	runGit(t, repository, "config", "user.email", "azpipe@example.com")
	if err := os.WriteFile(filepath.Join(repository, "README"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "README")
	runGit(t, repository, "commit", "-m", "initial")
	runGit(t, repository, "remote", "add", "origin", "https://dev.azure.com/example/sample/_git/sample-repo")
	runGit(t, repository, "branch", "feature/local")
	worktree := filepath.Join(t.TempDir(), "feature-worktree")
	runGit(t, repository, "worktree", "add", worktree, "-b", "feature/worktree")

	snapshot := discoverLocalBranches(context.Background(), repository)
	if !repositoriesMatch("https://dev.azure.com/example/sample/_git/sample-repo", snapshot.remoteURL...) {
		t.Fatalf("remote URLs = %v", snapshot.remoteURL)
	}
	origins := map[branchOrigin]map[string]bool{}
	for _, entry := range snapshot.entries {
		if origins[entry.origin] == nil {
			origins[entry.origin] = map[string]bool{}
		}
		origins[entry.origin][entry.name()] = true
	}
	if !origins[branchOriginLocal]["feature/local"] {
		t.Fatalf("local branches = %v, want feature/local", origins[branchOriginLocal])
	}
	if !origins[branchOriginWorktree]["feature/worktree"] {
		t.Fatalf("worktree branches = %v, want feature/worktree", origins[branchOriginWorktree])
	}
}

func TestRepositoriesMatchAzureHTTPSAndSSHRemotes(t *testing.T) {
	if !repositoriesMatch(
		"https://dev.azure.com/example/sample/_git/sample-repo",
		"git@ssh.dev.azure.com:v3/example/sample/sample-repo",
	) {
		t.Fatal("equivalent Azure DevOps HTTPS and SSH remotes did not match")
	}
}

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
