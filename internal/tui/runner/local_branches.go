package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ineslino/azpipe/internal/azdo"
)

type branchOrigin string

const (
	branchOriginRemote   branchOrigin = "remote"
	branchOriginLocal    branchOrigin = "local"
	branchOriginWorktree branchOrigin = "worktree"
)

type branchEntry struct {
	azdoBranch azdo.Branch
	origin     branchOrigin
	location   string
}

type localBranchSnapshot struct {
	entries   []branchEntry
	remoteURL []string
	root      string
}

func (e branchEntry) branch() azdo.Branch {
	return e.azdoBranch
}

func (e branchEntry) key() string {
	return string(e.origin) + "\x00" + e.name()
}

func (e branchEntry) name() string {
	return strings.TrimPrefix(e.azdoBranch.Name, "refs/heads/")
}

func (e branchEntry) selectable() bool {
	return e.origin == branchOriginRemote
}

func (e branchEntry) originLabel() string {
	switch e.origin {
	case branchOriginLocal:
		return "LOCAL"
	case branchOriginWorktree:
		return "WORKTREE"
	default:
		return "REMOTE"
	}
}

func (e branchEntry) locationLabel() string {
	if e.location == "" {
		return "Azure DevOps"
	}
	return e.location
}

func remoteBranchEntry(branch azdo.Branch) branchEntry {
	return branchEntry{azdoBranch: branch, origin: branchOriginRemote}
}

func localBranchEntry(name, objectID, location string, origin branchOrigin) branchEntry {
	branch := azdo.Branch{Name: "refs/heads/" + strings.TrimPrefix(name, "refs/heads/"), ObjectID: objectID}
	branch.Creator.DisplayName = "Git local"
	return branchEntry{azdoBranch: branch, origin: origin, location: location}
}

func discoverLocalBranches(ctx context.Context, workingDirectory string) localBranchSnapshot {
	if strings.TrimSpace(workingDirectory) == "" {
		workingDirectory = "."
	}
	rootOutput, err := gitOutput(ctx, workingDirectory, "rev-parse", "--show-toplevel")
	if err != nil {
		return localBranchSnapshot{}
	}
	root := strings.TrimSpace(string(rootOutput))
	if root == "" {
		return localBranchSnapshot{}
	}

	localOutput, err := gitOutput(ctx, root, "for-each-ref", "--format=%(refname:short)\t%(objectname)", "refs/heads/")
	if err != nil {
		return localBranchSnapshot{}
	}
	worktreePaths := discoverWorktreePaths(ctx, root)
	snapshot := localBranchSnapshot{root: root, remoteURL: discoverRemoteURLs(ctx, root)}
	for _, line := range strings.Split(strings.TrimSpace(string(localOutput)), "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
			continue
		}
		origin := branchOriginLocal
		location := shortenPath(root)
		if worktreePath, ok := worktreePaths[fields[0]]; ok {
			origin = branchOriginWorktree
			location = shortenPath(worktreePath)
		}
		snapshot.entries = append(snapshot.entries, localBranchEntry(fields[0], fields[1], location, origin))
	}
	return snapshot
}

func gitOutput(ctx context.Context, workingDirectory string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", workingDirectory}, args...)
	return exec.CommandContext(ctx, "git", commandArgs...).Output()
}

func discoverWorktreePaths(ctx context.Context, root string) map[string]string {
	output, err := gitOutput(ctx, root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil
	}
	paths := map[string]string{}
	var path, branch string
	flush := func() {
		if branch != "" && path != "" && filepath.Clean(path) != filepath.Clean(root) {
			paths[strings.TrimPrefix(branch, "refs/heads/")] = path
		}
		path, branch = "", ""
	}
	for _, line := range strings.Split(string(output), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch "):
			branch = strings.TrimPrefix(line, "branch ")
		case strings.TrimSpace(line) == "":
			flush()
		}
	}
	flush()
	return paths
}

func discoverRemoteURLs(ctx context.Context, root string) []string {
	remotesOutput, err := gitOutput(ctx, root, "remote")
	if err != nil {
		return nil
	}
	var urls []string
	for _, remote := range strings.Fields(string(remotesOutput)) {
		output, err := gitOutput(ctx, root, "remote", "get-url", "--all", remote)
		if err != nil {
			continue
		}
		for _, url := range strings.Fields(string(output)) {
			if url != "" {
				urls = append(urls, url)
			}
		}
	}
	return urls
}

func repositoriesMatch(first string, others ...string) bool {
	firstKey := repositoryURLKey(first)
	if firstKey == "" {
		return false
	}
	for _, other := range others {
		if firstKey == repositoryURLKey(other) || azureRepositoryPath(first) != "" && azureRepositoryPath(first) == azureRepositoryPath(other) {
			return true
		}
	}
	return false
}

func repositoryURLKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, ".git")
	value = strings.TrimSuffix(value, "/")
	value = strings.TrimPrefix(value, "ssh://")
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	if strings.HasPrefix(value, "git@") {
		value = strings.TrimPrefix(value, "git@")
		value = strings.Replace(value, ":", "/", 1)
	}
	return strings.TrimPrefix(value, "/")
}

func azureRepositoryPath(value string) string {
	value = repositoryURLKey(value)
	if value == "" {
		return ""
	}
	host := value
	if separator := strings.IndexByte(host, '/'); separator >= 0 {
		host = host[:separator]
	}
	if host != "dev.azure.com" && host != "ssh.dev.azure.com" && !strings.HasSuffix(host, ".visualstudio.com") {
		return ""
	}
	if marker := strings.Index(value, "/_git/"); marker >= 0 {
		value = value[:marker] + "/" + strings.TrimPrefix(value[marker+len("/_git/"):], "/")
	}
	if marker := strings.Index(value, "/v3/"); marker >= 0 {
		value = value[:marker] + "/" + strings.TrimPrefix(value[marker+len("/v3/"):], "/")
	}
	parts := strings.Split(value, "/")
	if strings.HasSuffix(host, ".visualstudio.com") {
		if len(parts) < 3 {
			return ""
		}
		organization := strings.TrimSuffix(host, ".visualstudio.com")
		return strings.Join([]string{organization, parts[len(parts)-2], parts[len(parts)-1]}, "/")
	}
	if len(parts) < 4 {
		return ""
	}
	return strings.Join(parts[len(parts)-3:], "/")
}

func shortenPath(path string) string {
	if path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
