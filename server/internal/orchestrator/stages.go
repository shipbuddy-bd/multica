package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// onClarifyCompleted handles the transition from clarify → plan stage.
func (o *Orchestrator) onClarifyCompleted(ctx context.Context, task *db.AgentTaskQueue, pctx *PipelineContext) error {
	// Extract spec content from task result
	specContent := extractOutputFromResult(task.Result)

	parentIssueID := parsePipelineIssueID(pctx.PipelineIssueID)
	if !parentIssueID.Valid {
		return fmt.Errorf("invalid pipeline_issue_id: %s", pctx.PipelineIssueID)
	}

	parentIssue, err := o.Queries.GetIssue(ctx, parentIssueID)
	if err != nil {
		return fmt.Errorf("get parent issue: %w", err)
	}

	// Create plan sub-issue (single variant for MVP)
	variant := "plan-a"
	branchName := fmt.Sprintf("feat/%s/%s", extractIssuePrefix(parentIssue), variant)

	planNumber, err := o.Queries.IncrementIssueCounter(ctx, parentIssue.WorkspaceID)
	if err != nil {
		return fmt.Errorf("allocate plan issue number: %w", err)
	}

	planIssue, err := o.Queries.CreateIssue(ctx, db.CreateIssueParams{
		WorkspaceID:   parentIssue.WorkspaceID,
		Title:         fmt.Sprintf("[Plan %s] %s", variant, parentIssue.Title),
		Description:   pgtype.Text{String: "Design implementation approach", Valid: true},
		Status:        "todo",
		Priority:      "medium",
		AssigneeType:  pgtype.Text{String: "agent", Valid: true},
		AssigneeID:    task.AgentID,
		CreatorType:   "agent",
		CreatorID:     task.AgentID,
		ParentIssueID: parentIssueID,
		Position:      2,
		Number:        planNumber,
	})
	if err != nil {
		return fmt.Errorf("create plan issue: %w", err)
	}

	// Set metadata
	setCheckpointMeta(ctx, o.Queries, planIssue.ID, parentIssue.WorkspaceID, StagePlan, variant, branchName)

	// Create edge: clarify → plan
	_, err = o.Queries.CreatePipelineEdge(ctx, db.CreatePipelineEdgeParams{
		WorkspaceID:   parentIssue.WorkspaceID,
		SourceIssueID: task.IssueID,
		TargetIssueID: planIssue.ID,
		EdgeType:      "flow",
		Metadata:      []byte(`{"transition": "clarify → plan"}`),
	})
	if err != nil {
		return fmt.Errorf("create edge: %w", err)
	}

	// Enqueue plan task
	nextCtx := PipelineContext{
		PipelineIssueID: pctx.PipelineIssueID,
		Stage:           StagePlan,
		Variant:         variant,
		BranchName:      branchName,
		SpecContent:     specContent,
	}
	_, err = o.Enqueuer.EnqueuePipelineTask(ctx, EnqueueParams{
		AgentID:     task.AgentID,
		RuntimeID:   task.RuntimeID,
		IssueID:     planIssue.ID,
		WorkspaceID: parentIssue.WorkspaceID,
		Context:     nextCtx,
		Priority:    5,
	})
	return err
}

// onPlanCompleted handles the transition from plan → implement stage.
func (o *Orchestrator) onPlanCompleted(ctx context.Context, task *db.AgentTaskQueue, pctx *PipelineContext) error {
	planContent := extractOutputFromResult(task.Result)

	parentIssueID := parsePipelineIssueID(pctx.PipelineIssueID)
	if !parentIssueID.Valid {
		return fmt.Errorf("invalid pipeline_issue_id")
	}

	parentIssue, err := o.Queries.GetIssue(ctx, parentIssueID)
	if err != nil {
		return fmt.Errorf("get parent issue: %w", err)
	}

	branchName := fmt.Sprintf("feat/%s/%s/impl", extractIssuePrefix(parentIssue), pctx.Variant)

	implNumber, err := o.Queries.IncrementIssueCounter(ctx, parentIssue.WorkspaceID)
	if err != nil {
		return fmt.Errorf("allocate impl issue number: %w", err)
	}

	implIssue, err := o.Queries.CreateIssue(ctx, db.CreateIssueParams{
		WorkspaceID:   parentIssue.WorkspaceID,
		Title:         fmt.Sprintf("[Impl %s] %s", pctx.Variant, parentIssue.Title),
		Description:   pgtype.Text{String: "Implement the planned changes", Valid: true},
		Status:        "todo",
		Priority:      "medium",
		AssigneeType:  pgtype.Text{String: "agent", Valid: true},
		AssigneeID:    task.AgentID,
		CreatorType:   "agent",
		CreatorID:     task.AgentID,
		ParentIssueID: parentIssueID,
		Position:      3,
		Number:        implNumber,
	})
	if err != nil {
		return fmt.Errorf("create impl issue: %w", err)
	}

	setCheckpointMeta(ctx, o.Queries, implIssue.ID, parentIssue.WorkspaceID, StageImplement, pctx.Variant, branchName)

	_, err = o.Queries.CreatePipelineEdge(ctx, db.CreatePipelineEdgeParams{
		WorkspaceID:   parentIssue.WorkspaceID,
		SourceIssueID: task.IssueID,
		TargetIssueID: implIssue.ID,
		EdgeType:      "flow",
		Metadata:      []byte(`{"transition": "plan → implement"}`),
	})
	if err != nil {
		return fmt.Errorf("create edge: %w", err)
	}

	nextCtx := PipelineContext{
		PipelineIssueID: pctx.PipelineIssueID,
		Stage:           StageImplement,
		Variant:         pctx.Variant,
		BranchName:      branchName,
		SpecContent:     pctx.SpecContent,
		PlanContent:     planContent,
	}
	_, err = o.Enqueuer.EnqueuePipelineTask(ctx, EnqueueParams{
		AgentID:     task.AgentID,
		RuntimeID:   task.RuntimeID,
		IssueID:     implIssue.ID,
		WorkspaceID: parentIssue.WorkspaceID,
		Context:     nextCtx,
		Priority:    5,
	})
	return err
}

// onImplementCompleted handles the transition from implement → validate stage.
func (o *Orchestrator) onImplementCompleted(ctx context.Context, task *db.AgentTaskQueue, pctx *PipelineContext) error {
	parentIssueID := parsePipelineIssueID(pctx.PipelineIssueID)
	if !parentIssueID.Valid {
		return fmt.Errorf("invalid pipeline_issue_id")
	}

	parentIssue, err := o.Queries.GetIssue(ctx, parentIssueID)
	if err != nil {
		return fmt.Errorf("get parent issue: %w", err)
	}

	// Validate runs on same branch as implement
	branchName := pctx.BranchName

	valNumber, err := o.Queries.IncrementIssueCounter(ctx, parentIssue.WorkspaceID)
	if err != nil {
		return fmt.Errorf("allocate validate issue number: %w", err)
	}

	valIssue, err := o.Queries.CreateIssue(ctx, db.CreateIssueParams{
		WorkspaceID:   parentIssue.WorkspaceID,
		Title:         fmt.Sprintf("[Validate %s] %s", pctx.Variant, parentIssue.Title),
		Description:   pgtype.Text{String: "Run lint and tests, compute score", Valid: true},
		Status:        "todo",
		Priority:      "medium",
		AssigneeType:  pgtype.Text{String: "agent", Valid: true},
		AssigneeID:    task.AgentID,
		CreatorType:   "agent",
		CreatorID:     task.AgentID,
		ParentIssueID: parentIssueID,
		Position:      4,
		Number:        valNumber,
	})
	if err != nil {
		return fmt.Errorf("create validate issue: %w", err)
	}

	setCheckpointMeta(ctx, o.Queries, valIssue.ID, parentIssue.WorkspaceID, StageValidate, pctx.Variant, branchName)

	_, err = o.Queries.CreatePipelineEdge(ctx, db.CreatePipelineEdgeParams{
		WorkspaceID:   parentIssue.WorkspaceID,
		SourceIssueID: task.IssueID,
		TargetIssueID: valIssue.ID,
		EdgeType:      "flow",
		Metadata:      []byte(`{"transition": "implement → validate"}`),
	})
	if err != nil {
		return fmt.Errorf("create edge: %w", err)
	}

	nextCtx := PipelineContext{
		PipelineIssueID: pctx.PipelineIssueID,
		Stage:           StageValidate,
		Variant:         pctx.Variant,
		BranchName:      branchName,
		SpecContent:     pctx.SpecContent,
		PlanContent:     pctx.PlanContent,
	}
	_, err = o.Enqueuer.EnqueuePipelineTask(ctx, EnqueueParams{
		AgentID:     task.AgentID,
		RuntimeID:   task.RuntimeID,
		IssueID:     valIssue.ID,
		WorkspaceID: parentIssue.WorkspaceID,
		Context:     nextCtx,
		Priority:    5,
	})
	return err
}

// onValidateCompleted handles the transition from validate → handoff stage.
func (o *Orchestrator) onValidateCompleted(ctx context.Context, task *db.AgentTaskQueue, pctx *PipelineContext) error {
	parentIssueID := parsePipelineIssueID(pctx.PipelineIssueID)
	if !parentIssueID.Valid {
		return fmt.Errorf("invalid pipeline_issue_id")
	}

	// Parse score from task result
	score := parseScoreFromResult(task.Result)

	// Update the validate issue's metadata with score
	if task.IssueID.Valid {
		scoreJSON, _ := json.Marshal(score)
		o.Queries.SetIssueMetadataKey(ctx, db.SetIssueMetadataKeyParams{
			ID:          task.IssueID,
			WorkspaceID: parsePipelineIssueID(pctx.PipelineIssueID), // reuse workspace from parent
			Key:         "score",
			Value:       scoreJSON,
		})
	}

	parentIssue, err := o.Queries.GetIssue(ctx, parentIssueID)
	if err != nil {
		return fmt.Errorf("get parent issue: %w", err)
	}

	// Create handoff issue
	handoffNumber, err := o.Queries.IncrementIssueCounter(ctx, parentIssue.WorkspaceID)
	if err != nil {
		return fmt.Errorf("allocate handoff issue number: %w", err)
	}

	handoffIssue, err := o.Queries.CreateIssue(ctx, db.CreateIssueParams{
		WorkspaceID:   parentIssue.WorkspaceID,
		Title:         fmt.Sprintf("[Handoff] %s", parentIssue.Title),
		Description:   pgtype.Text{String: "Create PR with the implementation", Valid: true},
		Status:        "todo",
		Priority:      "medium",
		AssigneeType:  pgtype.Text{String: "agent", Valid: true},
		AssigneeID:    task.AgentID,
		CreatorType:   "agent",
		CreatorID:     task.AgentID,
		ParentIssueID: parentIssueID,
		Position:      5,
		Number:        handoffNumber,
	})
	if err != nil {
		return fmt.Errorf("create handoff issue: %w", err)
	}

	setCheckpointMeta(ctx, o.Queries, handoffIssue.ID, parentIssue.WorkspaceID, StageHandoff, pctx.Variant, pctx.BranchName)

	_, err = o.Queries.CreatePipelineEdge(ctx, db.CreatePipelineEdgeParams{
		WorkspaceID:   parentIssue.WorkspaceID,
		SourceIssueID: task.IssueID,
		TargetIssueID: handoffIssue.ID,
		EdgeType:      "flow",
		Metadata:      marshalEdgeMeta(score, pctx),
	})
	if err != nil {
		return fmt.Errorf("create edge: %w", err)
	}

	nextCtx := PipelineContext{
		PipelineIssueID: pctx.PipelineIssueID,
		Stage:           StageHandoff,
		Variant:         pctx.Variant,
		BranchName:      pctx.BranchName,
	}
	_, err = o.Enqueuer.EnqueuePipelineTask(ctx, EnqueueParams{
		AgentID:     task.AgentID,
		RuntimeID:   task.RuntimeID,
		IssueID:     handoffIssue.ID,
		WorkspaceID: parentIssue.WorkspaceID,
		Context:     nextCtx,
		Priority:    5,
	})
	return err
}

// onHandoffCompleted marks the pipeline as complete.
func (o *Orchestrator) onHandoffCompleted(ctx context.Context, task *db.AgentTaskQueue, pctx *PipelineContext) error {
	parentIssueID := parsePipelineIssueID(pctx.PipelineIssueID)
	if !parentIssueID.Valid {
		return nil
	}

	// Update root issue status to done
	o.Queries.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{
		ID:     parentIssueID,
		Status: "done",
	})

	o.Logger.Info("pipeline completed",
		"pipeline_issue_id", pctx.PipelineIssueID,
		"variant", pctx.Variant,
	)

	// TODO: trigger skill distillation
	return nil
}

// --- helpers ---

// extractOutputFromResult extracts the "output" field from task result JSONB.
func extractOutputFromResult(result []byte) string {
	if result == nil {
		return ""
	}
	var res map[string]any
	if err := json.Unmarshal(result, &res); err != nil {
		return ""
	}
	if output, ok := res["output"].(string); ok {
		return output
	}
	return ""
}

// setCheckpointMeta sets the pipeline metadata on a checkpoint issue.
func setCheckpointMeta(ctx context.Context, queries *db.Queries, issueID, workspaceID pgtype.UUID, stage, variant, branchName string) {
	meta := PipelineMeta{
		Role:       "checkpoint",
		Stage:      stage,
		Variant:    variant,
		BranchName: branchName,
	}
	metaJSON, _ := json.Marshal(meta)
	queries.SetIssueMetadataKey(ctx, db.SetIssueMetadataKeyParams{
		ID:          issueID,
		WorkspaceID: workspaceID,
		Key:         "pipeline",
		Value:       metaJSON,
	})
}
