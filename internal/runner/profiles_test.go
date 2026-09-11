package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ineslino/azpipe/internal/azdo"
)

func TestProfilesRoundTripContextAndNoOverwrite(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AZPIPE_DATA_DIR", root)
	p := Profile{Version: 1, Name: "dev-stack", Organization: "example", Project: "project", Selections: []ProfileSelection{{ID: 1, Mode: ModeRun, Branch: "feature/demo", Parameters: map[string]string{"environment": "dev"}}}}
	if err := SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	if err := SaveProfile(p); err == nil {
		t.Fatal("overwrote profile")
	}
	info, _ := os.Stat(filepath.Join(root, "profiles", "dev-stack.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	profiles, err := ListProfiles("https://dev.azure.com/example", "project")
	if err != nil || len(profiles) != 1 {
		t.Fatal(profiles, err)
	}
	selections, err := profiles[0].Resolve("example", "project", []azdo.Pipeline{{ID: 1}})
	if err != nil || selections[0].Branch != "feature/demo" || selections[0].Inputs["environment"] != "dev" {
		t.Fatal(selections, err)
	}
	if _, err = p.Resolve("other", "project", []azdo.Pipeline{{ID: 1}}); err == nil {
		t.Fatal("wrong context allowed")
	}
	if _, err = p.Resolve("example", "project", nil); err == nil {
		t.Fatal("removed pipeline allowed")
	}
	p.Selections[0].Mode = ModePlan
	if _, err = p.Resolve("example", "project", []azdo.Pipeline{{ID: 1}}); err == nil {
		t.Fatal("PLAN without contract allowed")
	}
	p.Name = "../escape"
	if err = SaveProfile(p); err == nil {
		t.Fatal("path traversal")
	}
}

func TestJournalResumePersistsOnlyKnownIDs(t *testing.T) {
	t.Setenv("AZPIPE_DATA_DIR", t.TempDir())
	j, err := NewJournal("example", "project", []Review{{Selection: Selection{Pipeline: azdo.Pipeline{ID: 1, Name: "alpha"}}}, {Selection: Selection{Pipeline: azdo.Pipeline{ID: 2, Name: "beta"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Record(0, RunResult{Run: azdo.PipelineRun{ID: 42, State: "inProgress"}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadJournal(j.Path(), "example", "project")
	if err != nil {
		t.Fatal(err)
	}
	runs := loaded.Results()
	if runs[1].Err == nil || runs[0].Review.Selection.Pipeline.Name != "alpha" {
		t.Fatal(runs)
	}
	runs[0].Run.State = "completed"
	runs[0].Run.Result = "succeeded"
	if err = loaded.UpdateRuns(runs); err != nil {
		t.Fatal(err)
	}
	updated, _ := LoadJournal(j.Path(), "example", "project")
	if updated.Runs[0].Run.Result != "succeeded" {
		t.Fatal(updated)
	}
	if _, err = LoadJournal(j.Path(), "other", "project"); err == nil {
		t.Fatal("wrong context")
	}
	runs[0].Run.ID = 100
	if err = loaded.UpdateRuns(runs); err == nil {
		t.Fatal("changed accepted ID")
	}
}

func TestProfilesResolveProjectAndPipelineIDTogether(t *testing.T) {
	profile := Profile{
		Version:      1,
		Name:         "all-projects",
		Organization: "example",
		Project:      AllProjects,
		Selections: []ProfileSelection{
			{ID: 7, Project: "Alpha", Mode: ModeRun, Branch: "main"},
			{ID: 7, Project: "Beta", Mode: ModeRun, Branch: "main"},
		},
	}
	pipelines := []azdo.Pipeline{
		{ID: 7, Project: "Alpha", Name: "alpha deploy"},
		{ID: 7, Project: "Beta", Name: "beta deploy"},
	}

	selections, err := profile.Resolve("example", AllProjects, pipelines)
	if err != nil {
		t.Fatal(err)
	}
	if len(selections) != 2 || selections[0].Pipeline.Project != "Alpha" || selections[1].Pipeline.Project != "Beta" {
		t.Fatalf("resolved selections = %#v, want Alpha and Beta pipelines", selections)
	}
}

func TestJournalResultsKeepProjectForAllProjectsContext(t *testing.T) {
	t.Setenv("AZPIPE_DATA_DIR", t.TempDir())
	j, err := NewJournal("example", AllProjects, []Review{
		{Selection: Selection{Pipeline: azdo.Pipeline{ID: 7, Project: "Alpha", Name: "alpha deploy"}}},
		{Selection: Selection{Pipeline: azdo.Pipeline{ID: 7, Project: "Beta", Name: "beta deploy"}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	results := j.Results()
	if len(results) != 2 || results[0].Review.Selection.Pipeline.Project != "Alpha" || results[1].Review.Selection.Pipeline.Project != "Beta" {
		t.Fatalf("journal projects = %#v, want Alpha and Beta", results)
	}
}
