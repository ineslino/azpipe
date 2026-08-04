package runner

import "github.com/ineslino/azpipe/internal/azdo"

const defaultBranch = "main"

// Mode defines whether a selected pipeline is queued normally or as a plan.
type Mode string

const (
	ModeRun  Mode = "RUN"
	ModePlan Mode = "PLAN"
)

// Selection is a pipeline and its effective execution choices.
type Selection struct {
	Pipeline azdo.Pipeline
	Mode     Mode
	Branch   string
}

// ID is the stable selection identity.
func (s Selection) ID() int {
	return s.Pipeline.ID
}

// Parameters returns only parameters required by the selected mode.
func (s Selection) Parameters() map[string]string {
	if s.Mode == ModePlan {
		return map[string]string{"planOnly": "true"}
	}
	return map[string]string{}
}

// Request builds the Azure DevOps request for this selection.
func (s Selection) Request() azdo.RunRequest {
	branch := s.Branch
	if branch == "" {
		branch = defaultBranch
	}
	return azdo.RunRequest{
		PipelineID: s.Pipeline.ID,
		Branch:     branch,
		Parameters: s.Parameters(),
	}
}

// ReviewState represents the preview gate state for a selection.
type ReviewState string

const (
	ReviewPending ReviewState = "CHECK"
	ReviewReady   ReviewState = "READY"
	ReviewError   ReviewState = "ERROR"
)

// Review is the result of previewing a selection.
type Review struct {
	Selection Selection
	State     ReviewState
	Err       error
}

// RunResult is the remote run, or the error that prevented it being queued or refreshed.
type RunResult struct {
	Review Review
	Run    azdo.PipelineRun
	Err    error
}
