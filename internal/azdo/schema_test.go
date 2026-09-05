package azdo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const schemaYAML = `parameters:
- name: environment
  displayName: Ambiente
  type: string
  default: dev
  values: [dev, prod]
- name: tests
  type: boolean
  default: false
- name: replicas
  type: number
- name: steps
  type: stepList
  default: []
steps: []
`

func TestSchemaTypesDefaultsAndValidation(t *testing.T) {
	s, err := ParseParameterSchema(schemaYAML)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Parameters) != 4 || !s.Parameters[1].HasDefault || s.Parameters[1].DefaultValue != "false" || s.Parameters[2].HasDefault {
		t.Fatal(s)
	}
	if err = s.Validate(map[string]string{"replicas": "2"}); err != nil {
		t.Fatal(err)
	}
	for _, input := range []map[string]string{{}, {"replicas": "NaN"}, {"replicas": "2", "environment": "test"}, {"replicas": "2", "tests": "yes"}, {"replicas": "2", "unknown": "x"}, {"replicas": "2", "steps": "[]"}} {
		if err = s.Validate(input); err == nil {
			t.Fatalf("invalid input accepted: %v", input)
		}
	}
	for _, source := range []string{"parameters: {foo: bar}", "parameters: [{name: foo}, {name: foo}]", "parameters: [{type: string}]", "parameters: ["} {
		if _, err = ParseParameterSchema(source); err == nil {
			t.Fatalf("invalid schema accepted: %s", source)
		}
	}
}

func TestCommandSchemaReadsPinnedYAML(t *testing.T) {
	c := CommandClient{Executable: "helper", ExpectedIdentity: "operator@example.com", Profile: "test", Organization: "https://dev.azure.com/example"}
	sha := strings.Repeat("a", 40)
	calls := 0
	c.output = func(_ context.Context, args ...string) ([]byte, error) {
		if args[1] == "whoami" {
			return []byte(`{"authenticatedUser":{"properties":{"Account":{"$value":"operator@example.com"}}}}`), nil
		}
		calls++
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "--resource definitions"):
			return []byte(`{"revision":7,"repository":{"id":"repo","type":"TfsGit"},"process":{"yamlFilename":"/azure-pipelines.yml"}}`), nil
		case strings.Contains(joined, "--resource refs"):
			if !strings.Contains(joined, "filter=heads/feature/demo") {
				t.Fatal(joined)
			}
			return []byte(`{"value":[{"name":"refs/heads/feature/demo","objectId":"` + sha + `"}]}`), nil
		case strings.Contains(joined, "--resource items"):
			if !strings.Contains(joined, "versionDescriptor.versionType=commit") || !strings.Contains(joined, "versionDescriptor.version="+sha) {
				t.Fatal("unpinned YAML", joined)
			}
			return json.Marshal(map[string]string{"content": schemaYAML})
		default:
			t.Fatalf("unexpected API: %s", joined)
			return nil, nil
		}
	}
	s, err := c.GetPipelineSchema(context.Background(), "project", 42, "feature/demo")
	if err != nil || calls != 3 || s.Commit != sha || s.DefinitionVersion != 7 || len(s.Parameters) != 4 {
		t.Fatal(s, err, calls)
	}
}

func TestNativeSchemaReadsPinnedYAML(t *testing.T) {
	sha := strings.Repeat("b", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "OPTIONS":
			json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{
				{"id": "dbeaf647-6167-421a-bda9-c9327b25e2e6", "area": "build", "resourceName": "definitions", "minVersion": "7.1", "maxVersion": "7.1", "releasedVersion": "7.1", "resourceVersion": 7, "routeTemplate": "/{project}/_apis/build/definitions/{definitionId}"},
				{"id": "2d874a60-a811-4f62-9c9f-963a6ea0a55b", "area": "git", "resourceName": "refs", "minVersion": "7.1", "maxVersion": "7.1", "releasedVersion": "7.1", "resourceVersion": 1, "routeTemplate": "/{project}/_apis/git/repositories/{repositoryId}/refs"},
				{"id": "fb93c0db-47ed-4a31-8c20-47552878fb44", "area": "git", "resourceName": "items", "minVersion": "7.1", "maxVersion": "7.1", "releasedVersion": "7.1", "resourceVersion": 1, "routeTemplate": "/{project}/_apis/git/repositories/{repositoryId}/items"},
				{"id": "e81700f7-3be2-46de-8624-2eb35882fcaa", "area": "location", "resourceName": "ResourceAreas", "minVersion": "5.1", "maxVersion": "7.1", "releasedVersion": "7.1", "resourceVersion": 1, "routeTemplate": "/_apis/ResourceAreas"},
			}})
		case strings.Contains(r.URL.Path, "ResourceAreas"):
			json.NewEncoder(w).Encode(map[string]any{"value": []map[string]string{{"id": "965220d5-5bb9-42cf-8d67-9b146df2a5a4", "locationUrl": "http://" + r.Host}, {"id": "4e080c62-fa21-4fbc-8fef-2a10a2b38049", "locationUrl": "http://" + r.Host}}})
		case strings.HasSuffix(r.URL.Path, "definitions/42"):
			w.Write([]byte(`{"revision":7,"repository":{"id":"repo","type":"TfsGit"},"process":{"yamlFilename":"/azure-pipelines.yml"}}`))
		case strings.HasSuffix(r.URL.Path, "/refs"):
			w.Write([]byte(`{"value":[{"name":"refs/heads/main","objectId":"` + sha + `"}]}`))
		case strings.HasSuffix(r.URL.Path, "/items"):
			if r.URL.Query().Get("versionDescriptor.version") != sha || r.URL.Query().Get("includeContent") != "true" {
				t.Error("YAML not pinned", r.URL)
			}
			json.NewEncoder(w).Encode(map[string]string{"content": schemaYAML})
		default:
			t.Error("unexpected API", r.URL)
			http.Error(w, "unexpected", 404)
		}
	}))
	defer server.Close()
	s, err := New(server.URL, "fake").(SchemaProvider).GetPipelineSchema(context.Background(), "project", 42, "main")
	if err != nil || s.Commit != sha || len(s.Parameters) != 4 {
		t.Fatal(s, err)
	}
}
