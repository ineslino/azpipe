package azdo

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMalformedContractRejected(t *testing.T) {
	for _, value := range []string{`[{"pipelineId":1}]`, `[] {}`, `[{"unknown":true}]`} {
		p := filepath.Join(t.TempDir(), "contracts.json")
		if err := os.WriteFile(p, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadContracts(p); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}

func TestChangedExpandedYamlNeverQueues(t *testing.T) {
	queued := false
	server := newPipelineServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/runs") {
			queued = true
		}
		w.Write([]byte(`{"finalYaml":"stages: []"}`))
	})
	_, err := New(server.URL, "test").QueuePipeline(context.Background(), "sample", RunRequest{PipelineID: 1, Branch: "main", PreviewHash: "different"})
	if err == nil || queued {
		t.Fatal("changed preview was queued")
	}
}
