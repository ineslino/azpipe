package runner

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ineslino/azpipe/internal/azdo"
	domainrunner "github.com/ineslino/azpipe/internal/runner"
)

func TestCatalogNavigation_ClampsCursorWithinVisiblePipelines(t *testing.T) {
	model := catalogFixture()

	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyDown})
	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyDown})
	if got := model.cursor; got != 2 {
		t.Fatalf("cursor after moving down = %d, want 2", got)
	}

	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyUp})
	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyUp})
	if got := model.cursor; got != 0 {
		t.Fatalf("cursor after moving up = %d, want 0", got)
	}
}

func TestCatalogSelection_SpaceTogglesOnlyActivePipeline(t *testing.T) {
	model := catalogFixture()
	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyDown})
	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeySpace})

	if got := model.Selected(); len(got) != 1 || got[0].Pipeline.ID != 202 || got[0].Mode != domainrunner.ModeRun {
		t.Fatalf("selected pipelines = %#v, want only pipeline 202 in RUN", got)
	}

	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeySpace})
	if got := model.Selected(); len(got) != 0 {
		t.Fatalf("selected pipelines after toggle = %#v, want none", got)
	}
}

func TestCatalogPlan_TogglesActivePipelineBetweenPlanAndRun(t *testing.T) {
	model := catalogFixture()
	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})

	got := model.Selected()
	if len(got) != 1 || got[0].Pipeline.ID != 101 || got[0].Mode != domainrunner.ModePlan {
		t.Fatalf("selected pipelines = %#v, want pipeline 101 in PLAN", got)
	}

	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if got := model.Selected(); len(got) != 1 || got[0].Mode != domainrunner.ModeRun {
		t.Fatalf("selected pipelines after second p = %#v, want pipeline 101 in RUN", got)
	}

	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if got := model.Selected(); len(got) != 1 || got[0].Mode != domainrunner.ModePlan {
		t.Fatalf("selected pipelines after third p = %#v, want pipeline 101 in PLAN", got)
	}
}

func TestCatalogSearch_FiltersEveryPipelineAttribute(t *testing.T) {
	queries := map[string]int{
		"alpha deploy": 101,
		"202":          202,
		"/platform":    202,
		"release":      303,
		"orders-api":   202,
		"critical":     303,
	}

	for query, wantID := range queries {
		t.Run(query, func(t *testing.T) {
			model := catalogFixture()
			model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
			for _, r := range query {
				model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}

			if len(model.visible) != 1 || model.visible[0].ID != wantID {
				t.Fatalf("visible pipelines for %q = %#v, want pipeline %d", query, model.visible, wantID)
			}
		})
	}
}

func TestCatalogEscape_ClearsSearchBeforeQuitting(t *testing.T) {
	model := catalogFixture()
	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	model, cmd := updateCatalogWithCmd(t, model, tea.KeyMsg{Type: tea.KeyEsc})
	if got := model.search.Value(); got != "" {
		t.Fatalf("search after escape = %q, want empty", got)
	}
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("escape with a search query must not quit")
		}
	}

	_, cmd = updateCatalogWithCmd(t, model, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("escape after clearing search must quit")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Fatalf("escape after clearing search message = %T, want tea.QuitMsg", cmd())
	}
}

func TestCatalogEnterWithoutSelection_ShowsWarningAndStaysInCatalog(t *testing.T) {
	model := catalogFixture()
	model, cmd := updateCatalogWithCmd(t, model, tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		t.Fatalf("enter without a selection returned command %T, want none", cmd)
	}
	if !strings.Contains(model.View(), "Selecione pelo menos uma pipeline") {
		t.Fatalf("catalog warning not visible:\n%s", model.View())
	}
}

func TestCatalogEnterWithSelection_ProducesReviewMessage(t *testing.T) {
	model := catalogFixture()
	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeySpace})
	_, cmd := updateCatalogWithCmd(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter with a selection must produce a review message")
	}

	msg, ok := cmd().(CatalogReviewMsg)
	if !ok {
		t.Fatalf("enter message = %T, want CatalogReviewMsg", cmd())
	}
	if len(msg.Selections) != 1 || msg.Selections[0].ID() != 101 {
		t.Fatalf("review selections = %#v, want pipeline 101", msg.Selections)
	}
}

func TestCatalogView_RendersProgressiveDetailOnlyForActivePipeline(t *testing.T) {
	model := catalogFixture()
	model = updateCatalog(t, model, tea.KeyMsg{Type: tea.KeyDown})

	view := model.View()
	for _, detail := range []string{"orders-api", "/platform/orders", "owner:orders"} {
		if got := strings.Count(view, detail); got != 1 {
			t.Fatalf("active detail %q rendered %d times, want once:\n%s", detail, got, view)
		}
	}
	for _, detail := range []string{"billing-api", "/apps/billing", "owner:billing", "identity-api", "/release/identity", "critical"} {
		if strings.Contains(view, detail) {
			t.Fatalf("inactive detail %q rendered:\n%s", detail, view)
		}
	}
}

func catalogFixture() CatalogModel {
	return NewCatalogModel([]azdo.Pipeline{
		{ID: 101, Name: "alpha deploy", Folder: "/apps/billing", RepoName: "billing-api", Tags: []string{"owner:billing"}},
		{ID: 202, Name: "orders deploy", Folder: "/platform/orders", RepoName: "orders-api", Tags: []string{"owner:orders"}},
		{ID: 303, Name: "identity deploy", Folder: "/release/identity", RepoName: "identity-api", Tags: []string{"critical"}},
	})
}

func updateCatalog(t *testing.T, model CatalogModel, msg tea.Msg) CatalogModel {
	t.Helper()
	updated, _ := updateCatalogWithCmd(t, model, msg)
	return updated
}

func updateCatalogWithCmd(t *testing.T, model CatalogModel, msg tea.Msg) (CatalogModel, tea.Cmd) {
	t.Helper()
	updated, cmd := model.Update(msg)
	catalog, ok := updated.(CatalogModel)
	if !ok {
		t.Fatalf("Update() model = %T, want CatalogModel", updated)
	}
	return catalog, cmd
}
