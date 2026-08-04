package cmd

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ineslino/azpipe/internal/azdo"
	tuirunner "github.com/ineslino/azpipe/internal/tui/runner"
)

type failedFinalModel struct {
	err error
}

func (m failedFinalModel) Init() tea.Cmd                       { return nil }
func (m failedFinalModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m failedFinalModel) View() string                        { return "" }
func (m failedFinalModel) ExecutionError() error               { return m.err }

func TestDemoCommand_DoesNotCreateClient(t *testing.T) {
	oldFactory := tuiClientFactory
	oldRunner := runTUI
	t.Cleanup(func() {
		tuiClientFactory = oldFactory
		runTUI = oldRunner
		rootCmd.SetArgs(nil)
	})

	tuiClientFactory = func(string) (azdo.Client, error) {
		panic("demo must not create a client")
	}
	runTUI = func(model tea.Model) error {
		app, ok := model.(tuirunner.AppModel)
		if !ok {
			t.Fatalf("demo model = %T, want runner.AppModel", model)
		}
		if app.Screen() != tuirunner.ScreenCatalog {
			t.Fatalf("demo initial screen = %v, want catalog", app.Screen())
		}
		return nil
	}

	rootCmd.SetArgs([]string{"demo"})
	if _, err := rootCmd.ExecuteC(); err != nil {
		t.Fatalf("execute azpipe demo: %v", err)
	}
}

func TestRootCommand_DefersClientCreationToBootstrapContext(t *testing.T) {
	oldFactory := tuiClientFactory
	oldRunner := runTUI
	t.Cleanup(func() {
		tuiClientFactory = oldFactory
		runTUI = oldRunner
		rootCmd.SetArgs(nil)
	})

	factoryCalls := 0
	tuiClientFactory = func(string) (azdo.Client, error) {
		factoryCalls++
		return &azdo.MockClient{}, nil
	}
	runTUI = func(model tea.Model) error {
		app, ok := model.(tuirunner.AppModel)
		if !ok {
			t.Fatalf("root model = %T, want runner.AppModel", model)
		}
		if app.Screen() != tuirunner.ScreenContext {
			t.Fatalf("root initial screen = %v, want context", app.Screen())
		}
		return nil
	}

	rootCmd.SetArgs([]string{})
	if _, err := rootCmd.ExecuteC(); err != nil {
		t.Fatalf("execute azpipe: %v", err)
	}
	if factoryCalls != 0 {
		t.Fatalf("client factory called %d times before context submit, want 0", factoryCalls)
	}
}

func TestRunTUI_ReturnsPartialQueueFailureFromFinalModel(t *testing.T) {
	oldProgram := runProgram
	t.Cleanup(func() { runProgram = oldProgram })
	runProgram = func(tea.Model) (tea.Model, error) {
		return failedFinalModel{err: errors.New("queue billing: permission denied")}, nil
	}

	err := runTUI(tuirunner.NewDemoApp())
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("runTUI error = %v, want partial queue failure", err)
	}
}
