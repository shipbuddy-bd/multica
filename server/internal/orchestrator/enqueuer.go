package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// QueriesEnqueuer is a TaskEnqueuer implementation that creates pipeline
// tasks directly via sqlc. It also fires task:queued events on the bus so
// the daemon's WS poller wakes up and the frontend updates in real time.
type QueriesEnqueuer struct {
	Queries *db.Queries
	Bus     *events.Bus // optional; nil = no event emission (tests)
}

// EnqueuePipelineTask inserts an agent_task_queue row with pipeline context
// in the JSONB Context field. The Daemon reads task.Context and dispatches
// BuildPrompt → buildPipelineStagePrompt based on the stage.
func (e *QueriesEnqueuer) EnqueuePipelineTask(ctx context.Context, params EnqueueParams) (*db.AgentTaskQueue, error) {
	contextBytes, err := json.Marshal(params.Context)
	if err != nil {
		return nil, fmt.Errorf("marshal pipeline context: %w", err)
	}

	triggerSummary := pgtype.Text{
		String: fmt.Sprintf("Pipeline stage: %s", params.Context.Stage),
		Valid:  true,
	}

	task, err := e.Queries.CreatePipelineTask(ctx, db.CreatePipelineTaskParams{
		AgentID:        params.AgentID,
		RuntimeID:      params.RuntimeID,
		IssueID:        params.IssueID,
		Priority:       params.Priority,
		Context:        contextBytes,
		TriggerSummary: triggerSummary,
	})
	if err != nil {
		return nil, fmt.Errorf("create pipeline task: %w", err)
	}

	// Best-effort event emission; the daemon also polls so a missed event
	// just delays pickup by a few seconds.
	if e.Bus != nil {
		e.Bus.Publish(events.Event{
			Type:        "task.queued",
			WorkspaceID: uuidToString(params.WorkspaceID),
			Payload: map[string]any{
				"task_id":   uuidToString(task.ID),
				"agent_id":  uuidToString(params.AgentID),
				"issue_id":  uuidToString(params.IssueID),
				"stage":     params.Context.Stage,
			},
		})
	}

	return &task, nil
}
