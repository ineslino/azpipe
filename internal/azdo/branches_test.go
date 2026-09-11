package azdo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestBranchDeletionGuardsAndCAS(t *testing.T) {
	for _, scenario := range []string{"allowed", "default", "main", "locked", "policy", "policy-error", "policy-malformed", "policy-no-state", "active-source", "active-target", "stale", "missing", "race", "denied", "timeout", "invalid-sha"} {
		t.Run(scenario, func(t *testing.T) {
			sha := strings.Repeat("a", 40)
			name := "refs/heads/feat/test"
			if scenario == "main" {
				name = "refs/heads/main"
			}
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/repositories/repo"):
					def := "refs/heads/main"
					if scenario == "default" {
						def = name
					}
					json.NewEncoder(w).Encode(map[string]any{"id": "repo", "defaultBranch": def})
				case strings.HasSuffix(r.URL.Path, "/refs") && r.Method == "GET":
					current := sha
					if scenario == "stale" {
						current = strings.Repeat("b", 40)
					}
					if scenario == "missing" {
						fmt.Fprint(w, `{"value":[]}`)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{{"name": name, "objectId": current, "isLocked": scenario == "locked"}}})
				case strings.HasSuffix(r.URL.Path, "/policy/configurations"):
					if r.URL.Query().Get("repositoryId") != "repo" || r.URL.Query().Get("refName") != name {
						t.Error("missing policy scope")
					}
					if scenario == "policy-error" {
						w.WriteHeader(403)
						return
					}
					if scenario == "policy-malformed" {
						fmt.Fprint(w, `{}`)
						return
					}
					if scenario == "policy-no-state" {
						fmt.Fprint(w, `{"value":[{}]}`)
						return
					}
					if scenario == "policy" {
						fmt.Fprint(w, `{"value":[{"isEnabled":true}]}`)
					} else {
						fmt.Fprint(w, `{"value":[]}`)
					}
				case strings.HasSuffix(r.URL.Path, "/pullRequests"):
					if (scenario == "active-source" && r.URL.Query().Get("searchCriteria.sourceRefName") == name) || (scenario == "active-target" && r.URL.Query().Get("searchCriteria.targetRefName") == name) {
						fmt.Fprint(w, `{"value":[{}]}`)
					} else {
						fmt.Fprint(w, `{"value":[]}`)
					}
				case r.Method == "POST":
					posts++
					var body []map[string]string
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if len(body) != 1 || body[0]["oldObjectId"] != sha || body[0]["newObjectId"] != zeroSHA || body[0]["name"] != name {
						t.Errorf("unsafe delete: %#v", body)
					}
					if scenario == "timeout" {
						w.WriteHeader(504)
						return
					}
					status := "succeeded"
					if scenario == "race" {
						status = "staleOldObjectId"
					}
					if scenario == "denied" {
						status = "forcePushRequired"
					}
					json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{{"name": name, "success": status == "succeeded", "updateStatus": status}}})
				default:
					t.Errorf("unexpected request %s", r.URL)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			b := Branch{Name: name, ObjectID: sha}
			if scenario == "invalid-sha" {
				b.ObjectID = ""
			}
			err := New(server.URL, "fake").(BranchClient).DeleteBranch(context.Background(), "project", "repo", b)
			if (err == nil) != (scenario == "allowed") {
				t.Fatalf("error=%v", err)
			}
			expected := 0
			if scenario == "allowed" || scenario == "race" || scenario == "denied" || scenario == "timeout" {
				expected = 1
			}
			if posts != expected {
				t.Fatalf("POSTs=%d want %d", posts, expected)
			}
		})
	}
}

func TestBranchesPaginationAndCreator(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("filter") != "heads/" {
			t.Error("missing heads filter")
		}
		if calls == 1 {
			w.Header().Set("x-ms-continuationtoken", "next")
			fmt.Fprint(w, `{"value":[{"name":"refs/heads/z","creator":{"uniqueName":"one@example.com"}}]}`)
		} else {
			if r.URL.Query().Get("continuationToken") != "next" {
				t.Error("missing token")
			}
			fmt.Fprint(w, `{"value":[{"name":"refs/heads/a","creator":{"displayName":"Two"}}]}`)
		}
	}))
	defer server.Close()
	rows, err := New(server.URL, "fake").(BranchClient).ListBranches(context.Background(), "project", "repo")
	if err != nil || len(rows) != 2 || calls != 2 {
		t.Fatalf("rows=%v error=%v", rows, err)
	}
	if !rows[1].Matches("Z", "ONE@") || rows[0].Matches("", "one@") {
		t.Fatal("creator filter is incorrect")
	}
}

func TestReviewBranchFindsExactRefAcrossPages(t *testing.T) {
	sha := strings.Repeat("a", 40)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/repositories/repo"):
			fmt.Fprint(w, `{"id":"repo","defaultBranch":"refs/heads/main"}`)
		case strings.HasSuffix(r.URL.Path, "/refs"):
			calls++
			if calls == 1 {
				w.Header().Set("x-ms-continuationtoken", "next")
				fmt.Fprint(w, `{"value":[{"name":"refs/heads/feat/other","objectId":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}`)
				return
			}
			if r.URL.Query().Get("continuationToken") != "next" {
				t.Error("missing continuation token")
			}
			fmt.Fprintf(w, `{"value":[{"name":"refs/heads/feat/test","objectId":%q}]}`, sha)
		case strings.HasSuffix(r.URL.Path, "/policy/configurations"):
			fmt.Fprint(w, `{"value":[]}`)
		case strings.HasSuffix(r.URL.Path, "/pullRequests"):
			fmt.Fprint(w, `{"value":[]}`)
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	branch := Branch{Name: "refs/heads/feat/test", ObjectID: sha}
	reviewed, err := New(server.URL, "fake").(BranchClient).ReviewBranch(context.Background(), "project", "repo", branch)
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Blocked != "" || calls != 2 {
		t.Fatalf("reviewed=%#v refs calls=%d", reviewed, calls)
	}
}

func TestBranchesAdapterUsesIdentityAndReadOnlyList(t *testing.T) {
	c := CommandClient{Executable: "helper", Profile: "test", ExpectedIdentity: "user@example.com"}
	calls := 0
	c.output = func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		if args[1] == "whoami" {
			return []byte(`{"authenticatedUser":{"properties":{"Account":{"$value":"user@example.com"}}}}`), nil
		}
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--resource refs") || !strings.Contains(joined, "--http-method GET") || !strings.Contains(joined, "repositoryId=repo") {
			t.Fatal(joined)
		}
		return []byte(`{"value":[]}`), nil
	}
	if _, err := c.ListBranches(context.Background(), "project", "repo"); err != nil || calls != 2 {
		t.Fatal(err, calls)
	}
}

func TestBranchesAdapterDeletionUsesResolvedRepositoryAndExactSHA(t *testing.T) {
	c := CommandClient{Executable: "helper", Profile: "test", ExpectedIdentity: "user@example.com"}
	sha := strings.Repeat("a", 40)
	posts, identities, requests := 0, 0, 0
	var requestFile string
	c.output = func(_ context.Context, args ...string) ([]byte, error) {
		if args[1] == "whoami" {
			identities++
			return []byte(`{"authenticatedUser":{"properties":{"Account":{"$value":"user@example.com"}}}}`), nil
		}
		requests++
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "--http-method POST"):
			posts++
			if !strings.Contains(joined, "repositoryId=stable-id") {
				t.Fatal("delete not bound to repository ID", joined)
			}
			for i, arg := range args {
				if arg == "--in-file" {
					requestFile = args[i+1]
				}
			}
			data, err := os.ReadFile(requestFile)
			if err != nil {
				t.Fatal(err)
			}
			var body []map[string]string
			if json.Unmarshal(data, &body) != nil || len(body) != 1 || body[0]["oldObjectId"] != sha || body[0]["newObjectId"] != zeroSHA || body[0]["name"] != "refs/heads/feat/test" {
				t.Fatalf("unsafe body %s", data)
			}
			return []byte(`{"value":[{"name":"refs/heads/feat/test","success":true,"updateStatus":"succeeded"}]}`), nil
		case strings.Contains(joined, "--resource repositories"):
			return []byte(`{"id":"stable-id","defaultBranch":"refs/heads/main"}`), nil
		case strings.Contains(joined, "--resource refs"):
			return []byte(fmt.Sprintf(`{"value":[{"name":"refs/heads/feat/test","objectId":%q}]}`, sha)), nil
		case strings.Contains(joined, "--resource policyConfigurations"):
			if !strings.Contains(joined, "repositoryId=stable-id") || !strings.Contains(joined, "refName=refs/heads/feat/test") {
				t.Fatal("missing policy scope", joined)
			}
			return []byte(`{"value":[]}`), nil
		case strings.Contains(joined, "--resource pullRequests"):
			return []byte(`{"value":[]}`), nil
		default:
			t.Fatalf("unexpected request %s", joined)
		}
		return nil, nil
	}
	if err := c.DeleteBranch(context.Background(), "project", "repo-name", Branch{Name: "refs/heads/feat/test", ObjectID: sha}); err != nil {
		t.Fatal(err)
	}
	if posts != 1 || requests != 6 || identities != requests {
		t.Fatalf("posts=%d requests=%d identity=%d", posts, requests, identities)
	}
	if _, err := os.Stat(requestFile); !os.IsNotExist(err) {
		t.Fatal("request file not removed")
	}
}
