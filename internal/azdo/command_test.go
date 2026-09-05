package azdo

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCommandIdentityMismatchBlocksRequest(t *testing.T) {
	c := CommandClient{Executable: "helper", Profile: "test", ExpectedIdentity: "expected@example.com"}
	calls := 0
	c.output = func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		return []byte(`{"authenticatedUser":{"properties":{"Account":{"$value":"wrong@example.com"}}}}`), nil
	}
	if _, err := c.ListProjects(context.Background()); err == nil || calls != 1 {
		t.Fatal("identity gate failed")
	}
}

func TestCommandAdapterUsesPrivateBodyAndContinuationToken(t *testing.T) {
	c := CommandClient{Executable: "helper", Profile: "test", ExpectedIdentity: "operator@example.com", Organization: "https://dev.azure.com/example"}
	var bodyPath string
	c.output = func(_ context.Context, args ...string) ([]byte, error) {
		if args[1] == "whoami" {
			return []byte(`{"authenticatedUser":{"properties":{"Account":{"$value":"operator@example.com"}}}}`), nil
		}
		for i, arg := range args {
			if arg == "--in-file" {
				bodyPath = args[i+1]
				info, err := os.Stat(bodyPath)
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("body permissions")
				}
				data, _ := os.ReadFile(bodyPath)
				var body map[string]any
				if json.Unmarshal(data, &body) != nil {
					t.Fatal("body JSON")
				}
			}
		}
		return []byte(`{"value":[],"continuation_token":"next"}`), nil
	}
	var out map[string]any
	token, err := c.invoke(context.Background(), "pipelines", "preview", "sample", "POST", nil, nil, map[string]string{"test": "value"}, &out)
	if err != nil || token != "next" {
		t.Fatal(token, err)
	}
	if _, err := os.Stat(bodyPath); !os.IsNotExist(err) {
		t.Fatal("temporary request retained")
	}
	c.output = func(_ context.Context, args ...string) ([]byte, error) { return nil, errors.New("SECRET") }
	if err := c.VerifyIdentity(context.Background()); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatal("helper error exposed")
	}
}
