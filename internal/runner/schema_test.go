package runner

import (
	"context"
	"github.com/ineslino/azpipe/internal/azdo"
	"testing"
)

type schemaClient struct {
	azdo.MockClient
	schema azdo.ParameterSchema
}

func (c *schemaClient) GetPipelineSchema(context.Context, string, int, string) (azdo.ParameterSchema, error) {
	return c.schema, nil
}

func TestSchemaBlocksInvalidProfileBeforePreview(t *testing.T) {
	schema, err := azdo.ParseParameterSchema("parameters:\n- name: environment\n  type: string\n  values: [dev, prod]\n")
	if err != nil {
		t.Fatal(err)
	}
	c := &schemaClient{schema: schema}
	s := NewService(c, "project")
	for _, input := range []map[string]string{nil, {"environment": "removed"}, {"renamed": "dev"}} {
		r := s.PreviewAll(context.Background(), []Selection{{Pipeline: azdo.Pipeline{ID: 1}, Mode: ModeRun, Inputs: input}}, 1)
		if r[0].Err == nil {
			t.Fatal("invalid profile accepted")
		}
		if _, err = s.QueueAll(context.Background(), r, 1); err == nil {
			t.Fatal("invalid profile queued")
		}
	}
	if len(c.PreviewRequests) != 0 || len(c.QueueRequests) != 0 {
		t.Fatal("invalid inputs reached API")
	}
}
