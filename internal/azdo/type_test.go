package azdo

import "testing"

func TestPipelineTypeSeparators(t *testing.T) {
	for _, path := range []string{"/AWS/IaC", "\\AWS\\IaC", "AWS/IaC"} {
		if got := (Pipeline{Folder: path}).Type(); got != "AWS" {
			t.Errorf("%s: %s", path, got)
		}
	}
}
