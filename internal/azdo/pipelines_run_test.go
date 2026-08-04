package azdo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPreviewPipelineNormalizesBranchAndSendsParameters(t *testing.T) {
	t.Parallel()

	var received struct {
		Resources struct {
			Repositories map[string]struct {
				RefName string `json:"refName"`
			} `json:"repositories"`
		} `json:"resources"`
		TemplateParameters map[string]string `json:"templateParameters"`
	}
	server := newPipelineServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("preview request method = %s, want POST", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode preview request: %v", err)
		}
		_, _ = w.Write([]byte(`{"finalYaml":"stages: []"}`))
	})

	client := New(server.URL, "token")
	err := client.PreviewPipeline(context.Background(), "sample-project", RunRequest{
		PipelineID: 742,
		Branch:     "main",
		Parameters: map[string]string{"planOnly": "true"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := received.Resources.Repositories["self"].RefName; got != "refs/heads/main" {
		t.Fatalf("self ref = %q, want %q", got, "refs/heads/main")
	}
	if got := received.TemplateParameters["planOnly"]; got != "true" {
		t.Fatalf("planOnly = %q, want %q", got, "true")
	}
}

func TestQueuePipelineReturnsAcceptedRunWithoutSubsequentGet(t *testing.T) {
	t.Parallel()

	var getRequests atomic.Int32
	server := newPipelineServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"id":99,"state":"inProgress","result":"succeeded"}`))
		case http.MethodGet:
			getRequests.Add(1)
			http.Error(w, "unexpected GET", http.StatusInternalServerError)
		default:
			t.Fatalf("request method = %s, want POST or GET", r.Method)
		}
	})

	client := New(server.URL, "token")
	run, err := client.QueuePipeline(context.Background(), "sample-project", RunRequest{PipelineID: 742, Branch: "refs/heads/main"})
	if err != nil {
		t.Fatal(err)
	}

	if run.ID != 99 || run.State != "inProgress" || run.Result != "succeeded" {
		t.Fatalf("run = %#v, want ID 99, state inProgress, result succeeded", run)
	}
	if want := server.URL + "/sample-project/_build/results?buildId=99"; run.WebURL != want {
		t.Fatalf("web URL = %q, want %q", run.WebURL, want)
	}
	if got := getRequests.Load(); got != 0 {
		t.Fatalf("GET requests after accepted queue = %d, want 0", got)
	}
}

func TestGetPipelineRunMapsBuild(t *testing.T) {
	t.Parallel()

	server := newPipelineServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("request method = %s, want GET", r.Method)
		}
		_, _ = w.Write([]byte(`{"id":88,"status":"completed","result":"failed","_links":{"web":{"href":"https://dev.azure.com/org/sample-project/_build/results?buildId=88"}}}`))
	})

	run, err := New(server.URL, "token").GetPipelineRun(context.Background(), "sample-project", 88)
	if err != nil {
		t.Fatal(err)
	}
	if run.ID != 88 || run.State != "completed" || run.Result != "failed" {
		t.Fatalf("run = %#v, want ID 88, state completed, result failed", run)
	}
	if got, want := run.WebURL, "https://dev.azure.com/org/sample-project/_build/results?buildId=88"; got != want {
		t.Fatalf("web URL = %q, want %q", got, want)
	}
}

func TestPipelineType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		folder string
		want   string
	}{
		{name: "root", folder: "", want: "root"},
		{name: "slashes only", folder: "///", want: "root"},
		{name: "first folder", folder: "/platform/deploy", want: "platform"},
		{name: "without leading slash", folder: "services/api", want: "services"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Pipeline{Folder: tt.folder}).Type(); got != tt.want {
				t.Fatalf("Type() = %q, want %q", got, tt.want)
			}
		})
	}
}

func newPipelineServer(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodOptions && r.URL.Path == "/_apis" {
			_, _ = w.Write([]byte(`{"count":4,"value":[
				{"id":"53df2d18-29ea-46a9-bee0-933540f80abf","area":"pipelines","resourceName":"preview","minVersion":"7.1","maxVersion":"7.1","releasedVersion":"7.1","resourceVersion":1,"routeTemplate":"/{project}/_apis/{area}/{pipelineId}/{resource}"},
				{"id":"7859261e-d2e9-4a68-b820-a5d84cc5bb3d","area":"pipelines","resourceName":"runs","minVersion":"7.1","maxVersion":"7.1","releasedVersion":"7.1","resourceVersion":1,"routeTemplate":"/{project}/_apis/{area}/{pipelineId}/{resource}"},
				{"id":"0cd358e1-9217-4d94-8269-1c1ee6f93dcf","area":"build","resourceName":"builds","minVersion":"7.1","maxVersion":"7.1","releasedVersion":"7.1","resourceVersion":7,"routeTemplate":"/{project}/_apis/{area}/{resource}/{buildId}"},
				{"id":"e81700f7-3be2-46de-8624-2eb35882fcaa","area":"location","resourceName":"ResourceAreas","minVersion":"5.1","maxVersion":"7.1","releasedVersion":"7.1","resourceVersion":1,"routeTemplate":"/_apis/{resource}"}
			]}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/_apis/ResourceAreas" {
			_, _ = w.Write([]byte(`{"count":1,"value":[{"id":"965220d5-5bb9-42cf-8d67-9b146df2a5a4","locationUrl":"http://` + r.Host + `"}]}`))
			return
		}
		handler(w, r)
	}))
}
