package runner

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ineslino/azpipe/internal/azdo"
	domainrunner "github.com/ineslino/azpipe/internal/runner"
)

type branchFake struct {
	azdo.MockClient
	deleted  []string
	blocked  bool
	fail     bool
	onDelete func()
}

func (f *branchFake) ListBranches(context.Context, string, string) ([]azdo.Branch, error) {
	if f.fail {
		return nil, fmt.Errorf("listing failed")
	}
	return NewBranchDemo().branches, nil
}
func (f *branchFake) ReviewBranch(_ context.Context, _, _ string, b azdo.Branch) (azdo.Branch, error) {
	if f.blocked {
		b.Blocked = "protected"
	}
	return b, nil
}
func (f *branchFake) DeleteBranch(_ context.Context, _, _ string, b azdo.Branch) error {
	f.deleted = append(f.deleted, b.Name)
	if f.onDelete != nil {
		f.onDelete()
	}
	if f.fail {
		return fmt.Errorf("uncertain")
	}
	return nil
}
func branchKey(m BranchModel, key tea.KeyMsg) (BranchModel, tea.Cmd) {
	u, c := m.Update(key)
	return u.(BranchModel), c
}

func TestBranchCancellationStopsRemainingDeletes(t *testing.T) {
	f := &branchFake{}
	m := NewBranchDemo()
	m.demo, m.api, m.stage = false, f, "review"
	m.reviewed = m.branches[1:]
	m.confirmation.SetValue("ELIMINAR")
	m, command := branchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	f.onDelete = m.cancel
	u, _ := m.Update(command())
	m = u.(BranchModel)
	if len(f.deleted) != 1 || m.stage != "results" || len(m.results) != 2 || !strings.Contains(m.results[1], "não iniciada") {
		t.Fatalf("deletes=%v results=%v", f.deleted, m.results)
	}
}

func TestBranchBusyEscapeCancelsAndCatalogExitReturns(t *testing.T) {
	m := NewBranchDemo()
	m.busy = true
	m, command := branchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if command != nil || m.opCtx.Err() == nil {
		t.Fatal("busy escape did not cancel")
	}
	m.busy, m.returnToCatalog = false, true
	m, command = branchKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if command == nil {
		t.Fatal("missing return command")
	}
	app := AppModel{branchBrowser: &m, screen: ScreenCatalog}
	u, _ := app.Update(command())
	if u.(AppModel).branchBrowser != nil || u.(AppModel).screen != ScreenCatalog {
		t.Fatal("catalog not restored")
	}
}

func TestBranchReviewConfirmationAndPartialResults(t *testing.T) {
	f := &branchFake{}
	m := NewBranchDemo()
	m.demo = false
	m.api = f
	m.selected[m.branches[1].Name] = true
	m.selected[m.branches[2].Name] = true
	m, c := branchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if c == nil {
		t.Fatal("missing review")
	}
	u, _ := m.Update(c())
	m = u.(BranchModel)
	if len(f.deleted) != 0 || m.stage != "review" {
		t.Fatal("review mutated")
	}
	m, c = branchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if c != nil {
		t.Fatal("unconfirmed deletion")
	}
	m.confirmation.SetValue("eliminar")
	m, c = branchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if c != nil {
		t.Fatal("loose confirmation")
	}
	m.confirmation.SetValue("ELIMINAR")
	f.fail = true
	m, c = branchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if c == nil {
		t.Fatal("missing deletion")
	}
	u, _ = m.Update(c())
	m = u.(BranchModel)
	if len(f.deleted) != 2 || m.stage != "results" || !strings.Contains(m.View(), "uncertain") {
		t.Fatal("results missing")
	}
	_, c = branchKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if c != nil {
		t.Fatal("automatic retry")
	}
}

func TestBranchesBlockedDemoAndCancelNeverDelete(t *testing.T) {
	for _, scenario := range []string{"blocked", "demo", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			f := &branchFake{blocked: scenario == "blocked"}
			m := NewBranchDemo()
			m.demo = scenario == "demo"
			m.api = f
			m.stage = "review"
			m.reviewed = []azdo.Branch{m.branches[1]}
			if f.blocked {
				m.reviewed[0].Blocked = "protected"
			}
			m.confirmation.SetValue("ELIMINAR")
			key := tea.KeyMsg{Type: tea.KeyEnter}
			if scenario == "cancel" {
				key.Type = tea.KeyEsc
			}
			m, c := branchKey(m, key)
			if c != nil {
				c()
			}
			if len(f.deleted) != 0 {
				t.Fatal("unexpected delete")
			}
			if scenario == "cancel" && m.stage != "list" {
				t.Fatal("cancel failed")
			}
		})
	}
}

func TestBranchFiltersRetainSelectionAndFramesFit(t *testing.T) {
	for _, size := range [][2]int{{60, 24}, {80, 24}, {100, 40}} {
		m := NewBranchDemo()
		m.width, m.height = size[0], size[1]
		m.selected[m.branches[1].Name] = true
		m.creator.SetValue("USER@EXAMPLE")
		m.filter.SetValue("catalog")
		if len(m.visible()) != 1 {
			t.Fatal("filters")
		}
		m.creator.SetValue("missing")
		if len(m.visible()) != 0 || !m.selected[m.branches[1].Name] {
			t.Fatal("filter changed selection")
		}
		m.filter.SetValue("")
		m.creator.SetValue("")
		for _, stage := range []string{"list", "review", "results"} {
			m.stage = stage
			m.reviewed = m.branches
			m.results = []string{strings.Repeat("long-result ", 30)}
			view := m.View()
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("overflow %v %s: %dx%d\n%s", size, stage, lipgloss.Width(view), lipgloss.Height(view), view)
			}
		}
	}
	m := NewBranchDemo()
	view := m.View()
	if !strings.Contains(view, "│ BRANCH") || !strings.Contains(view, ">[ ]") {
		t.Fatalf("branch list is not rendered as the catalog table:\n%s", view)
	}
}

func TestBranchListingFailureClearsPreviousRepository(t *testing.T) {
	f := &branchFake{fail: true}
	m := NewBranchDemo()
	m.demo = false
	m.api = f
	m.repo.ID = "other"
	m, c := m.load()
	u, _ := m.Update(c())
	m = u.(BranchModel)
	if len(m.branches) != 0 || len(m.selected) != 0 || m.err == "" {
		t.Fatal("stale branches retained after failure")
	}
}

func TestCatalogBranchBrowserDemoReturnsToCatalog(t *testing.T) {
	m := NewDemoApp()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("B")})
	m = updated.(AppModel)
	if m.branchBrowser == nil || cmd != nil || !strings.Contains(m.View(), "BRANCHES") {
		t.Fatalf("branch browser not opened:\n%s", m.View())
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(AppModel)
	if cmd == nil {
		t.Fatal("branch browser did not return an exit command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(AppModel)
	if m.branchBrowser != nil || m.Screen() != ScreenCatalog {
		t.Fatal("catalog was not restored")
	}
}

func TestBranchesBootstrapUsesSelectedProjectAndRepositories(t *testing.T) {
	f := &branchFake{}
	f.Repos = []azdo.Repository{{ID: "repo-id", Name: "sample-repo"}}
	m := NewBranchesBootstrap(func(string) (azdo.Client, error) { return f, nil }, ContextDefaults{})
	m.context.organization.SetValue("example-org")
	m.context.projects = []azdo.Project{{ID: "project-id", Name: "sample-project"}}
	m.contextClient = f

	updated, _ := m.Update(contextSubmitMsg{organization: "example-org", project: domainrunner.AllProjects})
	m = updated.(AppModel)
	if m.branchBrowser != nil || !strings.Contains(m.context.err, "projecto específico") {
		t.Fatalf("all-project selection must be rejected: browser=%v error=%q", m.branchBrowser != nil, m.context.err)
	}

	m.context.projectCursor = 1
	updated, cmd := m.Update(contextSubmitMsg{organization: "example-org", project: "sample-project"})
	m = updated.(AppModel)
	if m.branchBrowser == nil || cmd == nil {
		t.Fatal("selected project did not open branch browser")
	}
	updated, _ = m.Update(cmd())
	m = updated.(AppModel)
	if m.branchBrowser == nil || len(m.branchBrowser.repos) != 1 || m.branchBrowser.project != "sample-project" {
		t.Fatalf("repository selection was not loaded: %#v", m.branchBrowser)
	}
}

func TestBranchBrowserIgnoresStaleOperationResults(t *testing.T) {
	m := NewBranchDemo()
	m.demo = false
	m.stage = "list"
	m.branches = []azdo.Branch{{Name: "refs/heads/current"}}
	m.generation = 2

	updated, cmd := m.Update(branchLoaded{
		token:    1,
		branches: []azdo.Branch{{Name: "refs/heads/old"}},
	})
	m = updated.(BranchModel)
	if cmd != nil || len(m.branches) != 1 || m.branches[0].Name != "refs/heads/current" {
		t.Fatalf("stale result changed the browser: branches=%#v cmd=%v", m.branches, cmd != nil)
	}
}
