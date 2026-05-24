// Package orchestrator manages the pipeline lifecycle for the "Super Individual" system.
// It listens for task completions and advances pipelines through stages:
// clarify → plan → implement → validate → handoff.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Pipeline stages in execution order.
const (
	StageClarify    = "clarify"
	StagePlan       = "plan"
	StageImplement  = "implement"
	StageValidate   = "validate"
	StageHandoff    = "handoff"
)

// PipelineContext is stored in agent_task_queue.context JSONB
// to identify a task as part of a pipeline.
type PipelineContext struct {
	PipelineIssueID string   `json:"pipeline_issue_id"` // parent issue UUID string
	Stage           string   `json:"stage"`
	Variant         string   `json:"variant,omitempty"`      // e.g. "plan-a"
	BranchName      string   `json:"branch_name,omitempty"`
	SkillIDs        []string `json:"skill_ids,omitempty"`
	SpecContent     string   `json:"spec_content,omitempty"` // from clarify stage
	PlanContent     string   `json:"plan_content,omitempty"` // from plan stage
}

// PipelineMeta is stored in issue.metadata JSONB under the "pipeline" key.
type PipelineMeta struct {
	Role      string  `json:"role"`                 // "root" or "checkpoint"
	Stage     string  `json:"stage"`
	Variant   string  `json:"variant,omitempty"`
	BranchName string `json:"branch_name,omitempty"`
	Score     *Score  `json:"score,omitempty"`
	RepoURL   string  `json:"repo_url,omitempty"`   // only on root
	BaseBranch string `json:"base_branch,omitempty"` // only on root
}

// Score represents the validation score for a checkpoint.
type Score struct {
	Total         float64 `json:"total"`
	LintPass      bool    `json:"lint_pass"`
	LintErrors    int     `json:"lint_errors"`
	TestTotal     int     `json:"test_total"`
	TestPassed    int     `json:"test_passed"`
	TestPassRate  float64 `json:"test_pass_rate"`
	TokenConsumed int     `json:"token_consumed"`
	DurationMs    int64   `json:"generation_time_ms"`
	DiffAdded     int     `json:"diff_lines_added"`
	DiffRemoved   int     `json:"diff_lines_removed"`
	DiffFiles     int     `json:"diff_files_changed"`
}

// TaskEnqueuer abstracts the ability to enqueue a new agent task.
type TaskEnqueuer interface {
	EnqueuePipelineTask(ctx context.Context, params EnqueueParams) (*db.AgentTaskQueue, error)
}

// EnqueueParams contains everything needed to create a pipeline task.
type EnqueueParams struct {
	AgentID     pgtype.UUID
	RuntimeID   pgtype.UUID
	IssueID     pgtype.UUID
	WorkspaceID pgtype.UUID
	Context     PipelineContext
	Priority    int32
}

// Orchestrator manages pipeline lifecycle.
type Orchestrator struct {
	Queries  *db.Queries
	Enqueuer TaskEnqueuer
	Logger   *slog.Logger
}

// New creates a new Orchestrator.
func New(queries *db.Queries, enqueuer TaskEnqueuer, logger *slog.Logger) *Orchestrator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Orchestrator{
		Queries:  queries,
		Enqueuer: enqueuer,
		Logger:   logger,
	}
}

// OnTaskCompleted is called when any task finishes. It checks if the task
// belongs to a pipeline and advances to the next stage if so.
func (o *Orchestrator) OnTaskCompleted(ctx context.Context, task *db.AgentTaskQueue) {
	if task == nil || len(task.Context) == 0 {
		return
	}

	var pctx PipelineContext
	if err := json.Unmarshal(task.Context, &pctx); err != nil {
		return // not a pipeline task
	}
	if pctx.PipelineIssueID == "" {
		return // not a pipeline task
	}

	o.Logger.Info("pipeline task completed",
		"task_id", task.ID.Bytes,
		"stage", pctx.Stage,
		"pipeline_issue_id", pctx.PipelineIssueID,
	)

	if err := o.advanceToNextStage(ctx, task, &pctx); err != nil {
		o.Logger.Error("failed to advance pipeline",
			"error", err,
			"stage", pctx.Stage,
			"pipeline_issue_id", pctx.PipelineIssueID,
		)
	}
}

// advanceToNextStage determines the next stage and creates the appropriate
// sub-issue + edge + task.
func (o *Orchestrator) advanceToNextStage(ctx context.Context, task *db.AgentTaskQueue, pctx *PipelineContext) error {
	switch pctx.Stage {
	case StageClarify:
		return o.onClarifyCompleted(ctx, task, pctx)
	case StagePlan:
		return o.onPlanCompleted(ctx, task, pctx)
	case StageImplement:
		return o.onImplementCompleted(ctx, task, pctx)
	case StageValidate:
		return o.onValidateCompleted(ctx, task, pctx)
	case StageHandoff:
		// Pipeline complete — trigger skill distillation
		return o.onHandoffCompleted(ctx, task, pctx)
	default:
		return fmt.Errorf("unknown pipeline stage: %s", pctx.Stage)
	}
}

// CreatePipeline starts a new pipeline from a requirement string.
// It creates the root issue and the first clarify checkpoint.
func (o *Orchestrator) CreatePipeline(ctx context.Context, params CreatePipelineParams) (*db.Issue, error) {
	o.Logger.Info("creating pipeline", "title", params.Title, "workspace_id", params.WorkspaceID)

	// 1. Create root issue
	rootIssue, err := o.Queries.CreateIssue(ctx, db.CreateIssueParams{
		WorkspaceID:  params.WorkspaceID,
		Title:        params.Title,
		Description:  pgtype.Text{String: params.Description, Valid: params.Description != ""},
		Status:       "in_progress",
		Priority:     "medium",
		AssigneeType: pgtype.Text{String: "agent", Valid: true},
		AssigneeID:   params.AgentID,
		CreatorType:  params.CreatorType,
		CreatorID:    params.CreatorID,
		Position:     0,
		Number:       params.Number,
	})
	if err != nil {
		return nil, fmt.Errorf("create root issue: %w", err)
	}

	// Set pipeline metadata on root issue
	rootMeta := PipelineMeta{
		Role:       "root",
		Stage:      StageClarify,
		RepoURL:    params.RepoURL,
		BaseBranch: "main",
	}
	rootMetaJSON, _ := json.Marshal(rootMeta)
	o.Queries.SetIssueMetadataKey(ctx, db.SetIssueMetadataKeyParams{
		ID:          rootIssue.ID,
		WorkspaceID: params.WorkspaceID,
		Key:         "pipeline",
		Value:       rootMetaJSON,
	})

	// 2. Create clarify sub-issue
	clarifyBranch := fmt.Sprintf("feat/%s/clarify/v1", params.IssuePrefix)

	clarifyIssue, err := o.Queries.CreateIssue(ctx, db.CreateIssueParams{
		WorkspaceID:   params.WorkspaceID,
		Title:         fmt.Sprintf("[Clarify] %s", params.Title),
		Description:   pgtype.Text{String: "Analyze and structure the requirement", Valid: true},
		Status:        "todo",
		Priority:      "medium",
		AssigneeType:  pgtype.Text{String: "agent", Valid: true},
		AssigneeID:    params.AgentID,
		CreatorType:   "agent",
		CreatorID:     params.AgentID,
		ParentIssueID: rootIssue.ID,
		Position:      1,
		Number:        params.Number + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("create clarify issue: %w", err)
	}

	// Set pipeline metadata on clarify issue
	clarifyMeta := PipelineMeta{
		Role:       "checkpoint",
		Stage:      StageClarify,
		BranchName: clarifyBranch,
	}
	clarifyMetaJSON, _ := json.Marshal(clarifyMeta)
	o.Queries.SetIssueMetadataKey(ctx, db.SetIssueMetadataKeyParams{
		ID:          clarifyIssue.ID,
		WorkspaceID: params.WorkspaceID,
		Key:         "pipeline",
		Value:       clarifyMetaJSON,
	})

	// 3. Create edge: root → clarify
	_, err = o.Queries.CreatePipelineEdge(ctx, db.CreatePipelineEdgeParams{
		WorkspaceID:   params.WorkspaceID,
		SourceIssueID: rootIssue.ID,
		TargetIssueID: clarifyIssue.ID,
		EdgeType:      "flow",
		Metadata:      []byte(`{"transition": "start → clarify"}`),
	})
	if err != nil {
		return nil, fmt.Errorf("create edge: %w", err)
	}

	// 4. Enqueue clarify task
	pipelineCtx := PipelineContext{
		PipelineIssueID: uuidToString(rootIssue.ID),
		Stage:           StageClarify,
		BranchName:      clarifyBranch,
	}
	_, err = o.Enqueuer.EnqueuePipelineTask(ctx, EnqueueParams{
		AgentID:     params.AgentID,
		RuntimeID:   params.RuntimeID,
		IssueID:     clarifyIssue.ID,
		WorkspaceID: params.WorkspaceID,
		Context:     pipelineCtx,
		Priority:    5,
	})
	if err != nil {
		return nil, fmt.Errorf("enqueue clarify task: %w", err)
	}

	return &rootIssue, nil
}

// CreatePipelineParams holds parameters for creating a new pipeline.
type CreatePipelineParams struct {
	WorkspaceID pgtype.UUID
	AgentID     pgtype.UUID
	RuntimeID   pgtype.UUID
	Title       string
	Description string
	RepoURL     string
	IssuePrefix string // e.g. "MUL-42"
	Number      int32
	CreatorType string
	CreatorID   pgtype.UUID
}
