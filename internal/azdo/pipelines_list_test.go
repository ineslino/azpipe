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
	if err == nil || !strings.Contains(err.Error(), "pipeline 22") || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("tag error = %v, want pipeline 22 and forbidden", err)
	}
	if pipelines != nil {
		t.Fatalf("pipelines after tag error = %#v, want nil", pipelines)
	}
}

type fakePipelineBuildClient struct {
	definitions   []build.BuildDefinitionReference
	tags          map[int][]string
	tagErrors     map[int]error
	current       atomic.Int32
	maxConcurrent atomic.Int32
}

func (c *fakePipelineBuildClient) GetDefinitions(context.Context, build.GetDefinitionsArgs) (*build.GetDefinitionsResponseValue, error) {
	return &build.GetDefinitionsResponseValue{Value: c.definitions}, nil
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
