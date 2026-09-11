package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBranchDeleteRejectsIncompleteConfirmationBeforeAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name, confirm, sha, want string
	}{
		{"", "", "", "--branch"},
		{"feat/test", "eliminar", strings.Repeat("a", 40), "--confirm"},
		{"feat/test", "ELIMINAR", "", "--confirm"},
		{"feat/test", "", "not-a-sha", "--sha"},
	} {
		command := &cobra.Command{}
		command.Flags().String("branch", tc.name, "")
		command.Flags().String("confirm", tc.confirm, "")
		command.Flags().String("sha", tc.sha, "")
		if err := runBranchDelete(command, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("expected local validation error, got %v", err)
		}
	}
}
