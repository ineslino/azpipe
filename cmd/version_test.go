package cmd

import (
	"strings"
	"testing"
)

func TestVersionIncludesSuppliedBuildMetadata(t *testing.T) {
	oldVersion, oldCommit := buildVersion, buildCommit
	t.Cleanup(func() { buildVersion, buildCommit = oldVersion, oldCommit })
	buildVersion, buildCommit = "1.2.3", strings.Repeat("a", 40)
	got := versionString()
	if !strings.HasPrefix(got, "1.2.3 (commit "+buildCommit+"; modified=") {
		t.Fatalf("missing metadata: %s", got)
	}
}
