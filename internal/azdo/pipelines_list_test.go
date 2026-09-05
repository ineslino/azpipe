package azdo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/build"
)

func TestListPipelineDefinitionsPopulatesTagsWithBoundedConcurrency(t *testing.T) {
	definitions := make([]build.BuildDefinitionReference, 6)
	tags := make(map[int][]string, len(definitions))
	for index := range definitions {
		id := index + 1
		name := fmt.Sprintf("pipeline-%d", id)
		definitions[index] = build.BuildDefinitionReference{Id: &id, Name: &name}
		tags[id] = []string{fmt.Sprintf("team-%d", id)}
	}
	client := &fakePipelineBuildClient{definitions: definitions, tags: tags}

	pipelines, err := listPipelineDefinitions(context.Background(), client, "sample-project")
	if err != nil {
		t.Fatal(err)
	}
	if got := client.maxConcurrent.Load(); got < 2 || got > pipelineTagParallelism {
		t.Fatalf("concurrent tag requests = %d, want between 2 and %d", got, pipelineTagParallelism)
	}
	for index, pipeline := range pipelines {
		want := fmt.Sprintf("team-%d", index+1)
		if len(pipeline.Tags) != 1 || pipeline.Tags[0] != want {
			t.Errorf("pipeline %d tags = %v, want [%s]", pipeline.ID, pipeline.Tags, want)
		}
	}
}

func TestListPipelineDefinitionsReturnsTagErrorWithPipelineID(t *testing.T) {
	id := 22
	name := "deploy"
	client := &fakePipelineBuildClient{
		definitions: []build.BuildDefinitionReference{{Id: &id, Name: &name}},
		tagErrors:   map[int]error{id: errors.New("forbidden")},
	}

	pipelines, err := listPipelineDefinitions(context.Background(), client, "sample-project")
	if err != nil || len(pipelines) != 1 || !strings.Contains(pipelines[0].MetadataWarning, "forbidden") {
		t.Fatalf("catalog must survive tags failure: %#v, %v", pipelines, err)
	}
}

type fakePipelineBuildClient struct {
	definitions   []build.BuildDefinitionReference
	tags          map[int][]string
	tagErrors     map[int]error
	current       atomic.Int32
	maxConcurrent atomic.Int32
	pages         map[string]build.GetDefinitionsResponseValue
	tokens        []string
}

func (c *fakePipelineBuildClient) GetDefinitions(_ context.Context, args build.GetDefinitionsArgs) (*build.GetDefinitionsResponseValue, error) {
	if c.pages != nil {
		token := ""
		if args.ContinuationToken != nil {
			token = *args.ContinuationToken
		}
		c.tokens = append(c.tokens, token)
		page := c.pages[token]
		return &page, nil
	}
	return &build.GetDefinitionsResponseValue{Value: c.definitions}, nil
}

func TestPipelinePagination(t *testing.T) {
	one, two := 1, 2
	client := &fakePipelineBuildClient{pages: map[string]build.GetDefinitionsResponseValue{
		"":     {Value: []build.BuildDefinitionReference{{Id: &one}}, ContinuationToken: "next"},
		"next": {Value: []build.BuildDefinitionReference{{Id: &two}}},
	}}
	result, err := listPipelineDefinitions(context.Background(), client, "example")
	if err != nil || len(result) != 2 || result[1].ID != 2 || len(client.tokens) != 2 {
		t.Fatalf("pagination: %v %v", result, err)
	}
	client.pages["next"] = build.GetDefinitionsResponseValue{ContinuationToken: "next"}
	if _, err := listPipelineDefinitions(context.Background(), client, "example"); err == nil {
		t.Fatal("repeated token accepted")
	}
}

func (c *fakePipelineBuildClient) GetDefinitionTags(_ context.Context, args build.GetDefinitionTagsArgs) (*[]string, error) {
	current := c.current.Add(1)
	defer c.current.Add(-1)
	for {
		maximum := c.maxConcurrent.Load()
		if current <= maximum || c.maxConcurrent.CompareAndSwap(maximum, current) {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)

	id := *args.DefinitionId
	if err := c.tagErrors[id]; err != nil {
		return nil, err
	}
	tags := append([]string(nil), c.tags[id]...)
	return &tags, nil
}
