# Orchestrator Integration - Quick Reference & Checklist

## Quick Facts

| Item | Location | Line Range |
|------|----------|-----------|
| **Endpoint** | `server/internal/handler/daemon.go::CompleteTask()` | 1604-1630 |
| **Service Layer** | `server/internal/service/task.go::CompleteTask()` | 963-1128 |
| **Event Type** | `server/pkg/protocol/events.go` | 36 |
| **Event Broadcast** | `server/internal/service/task.go::broadcastTaskEvent()` | 1758-1779 |
| **Event Bus** | `server/internal/events/bus.go::Publish()` | 61-88 |
| **WS Broadcast** | `server/cmd/server/listeners.go::registerListeners()` | 151-193 |
| **Router** | `server/cmd/server/router.go` | 286 |

---

## The Hook Point: Where to Trigger Orchestrator

### Option 1: Direct Handler (SIMPLER, RECOMMENDED)

**File:** `server/internal/handler/daemon.go`  
**After line 1625** (after task is returned from service):

```go
func (h *Handler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	// ... existing code ...
	
	task, err := h.TaskService.CompleteTask(r.Context(), parseUUID(taskID), result, req.SessionID, req.WorkDir)
	if err != nil {
		// error handling
		return
	}

	// ★ ADD THIS BLOCK:
	if h.Orchestrator != nil {
		var taskContext map[string]any
		if task.Context != nil {
			if err := json.Unmarshal(task.Context, &taskContext); err == nil {
				if pipelineID, ok := taskContext["pipeline_issue_id"]; ok && pipelineID != "" {
					// Non-blocking: trigger in background goroutine
					go h.Orchestrator.OnCheckpointCompleted(r.Context(), *task)
				}
			}
		}
	}

	h.emitIssueExecutedOnFirstCompletion(r, task)
	slog.Info("task completed", "task_id", taskID, "agent_id", uuidToString(task.AgentID))
	writeJSON(w, http.StatusOK, taskToResponse(*task))
}
```

### Option 2: Event Handler (CLEANER, LESS COUPLED)

**File:** `server/cmd/server/listeners.go` (new file or existing)  
**Add to registerListeners() function:**

```go
bus.Subscribe(protocol.EventTaskCompleted, func(e events.Event) {
	// Only process pipeline tasks
	payload, ok := e.Payload.(map[string]any)
	if !ok {
		return
	}
	
	taskID, _ := payload["task_id"].(string)
	if taskID == "" {
		return
	}
	
	// Fetch full task to check context
	task, err := queries.GetAgentTask(context.Background(), parseUUID(taskID))
	if err != nil {
		return
	}
	
	// Check for pipeline context
	var taskContext map[string]any
	if task.Context != nil {
		if err := json.Unmarshal(task.Context, &taskContext); err != nil {
			return
		}
	}
	
	// Only trigger for pipeline tasks
	if pipelineID, ok := taskContext["pipeline_issue_id"]; ok && pipelineID != "" {
		// Trigger orchestrator in background
		go orchestrator.OnCheckpointCompleted(context.Background(), task)
	}
})
```

---

## Data Flow Summary

```
1. Daemon POST /api/tasks/{id}/complete
   ↓
2. Handler.CompleteTask()
   ├─ Checks daemon auth
   ├─ Parses {pr_url, output, session_id, work_dir}
   └─ Calls TaskService.CompleteTask()
   ↓
3. TaskService.CompleteTask()
   ├─ Transaction: Mark task completed + update resume pointers
   ├─ Capture analytics
   ├─ Create comment (if needed)
   ├─ Reconcile agent status
   └─ broadcastTaskEvent("task:completed")
   ↓
4. Event Bus Publish
   ├─ Activity logger (creates log entry)
   ├─ Notification handlers
   ├─ ★ Orchestrator handler (NEW)
   └─ Global WS broadcaster
   ↓
5. WebSocket to Clients
   └─ {type: "task:completed", payload: {task_id, agent_id, ...}}
```

---

## What Orchestrator Receives

When `OnCheckpointCompleted()` is called, the `task` parameter contains:

```go
type AgentTaskQueue struct {
	ID            pgtype.UUID      // Task ID
	Status        string           // "completed"
	Result        []byte           // JSON: {pr_url, output, session_id, work_dir}
	Context       []byte           // JSONB: {pipeline_issue_id, ...}
	IssueID       pgtype.UUID      // The checkpoint issue
	AgentID       pgtype.UUID      // Which agent ran it
	RuntimeID     pgtype.UUID      // Which runtime executed it
	CompletedAt   pgtype.Timestamptz // When it finished
	Attempt       int32            // Retry attempt number
	MaxAttempts   int32            // Max retries allowed
	// ... other fields ...
}
```

**Key fields for orchestrator:**
- `task.Context` (JSONB) → Extract `pipeline_issue_id`, `stage`, branch info
- `task.Result` (JSON) → Parse output/pr_url
- `task.IssueID` → The completed checkpoint
- `task.CompletedAt` → Timing info

---

## Handler Constructor: Wiring Orchestrator

**File:** `server/cmd/server/main.go` or `router.go`

Add Orchestrator instance to Handler:

```go
type Handler struct {
	// ... existing fields ...
	Orchestrator *orchestrator.Orchestrator  // Add this
	// ... rest of fields ...
}

// During initialization:
h := &Handler{
	// ... other fields ...
	Orchestrator: orchestrator.New(
		queries,
		taskService,
		// other dependencies...
	),
	// ... rest ...
}
```

---

## Event Payload Structure

The WebSocket clients receive:

```json
{
  "type": "task:completed",
  "payload": {
    "task_id": "550e8400-e29b-41d4-a716-446655440000",
    "agent_id": "660e8400-e29b-41d4-a716-446655440001",
    "issue_id": "770e8400-e29b-41d4-a716-446655440002",
    "status": "completed",
    "chat_session_id": "optional-if-chat"
  },
  "actor_id": "",
  "actor_type": "system"
}
```

**Orchestrator sees the full task object (before JSON serialization).**

---

## Files You'll Modify

1. **`server/internal/handler/daemon.go`**  
   - Add orchestrator hook after line 1625

2. **`server/cmd/server/router.go`** (or main.go)  
   - Wire Orchestrator instance into Handler

3. **`server/internal/orchestrator/orchestrator.go`** (NEW FILE)  
   - Implement `OnCheckpointCompleted(ctx context.Context, task db.AgentTaskQueue) error`
   - Implement pipeline advancement logic

4. **Optional: `server/cmd/server/listeners.go`**  
   - Add event subscriber if using event handler pattern

---

## Testing the Integration

```bash
# 1. Create a pipeline issue with context["pipeline_issue_id"]
POST /api/issues
{
  "workspace_id": "...",
  "title": "Pipeline checkpoint",
  "context": {
    "pipeline_issue_id": "parent-issue-id",
    "stage": "clarify"
  }
}

# 2. Assign to agent, start task
POST /api/agents/{id}/tasks/claim
POST /api/tasks/{id}/start

# 3. Complete task (daemon would do this)
POST /api/tasks/{id}/complete
{
  "output": "Checkpoint completed",
  "session_id": "...",
  "work_dir": "/tmp/..."
}

# 4. Check if Orchestrator.OnCheckpointCompleted() was called
# Look in server logs for pipeline advancement
```

---

## Imports Needed

In `daemon.go`:
```go
import (
	"encoding/json"
	// ... existing ...
)
```

In `listeners.go` (if using event handler):
```go
import (
	"context"
	"encoding/json"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/pkg/protocol"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)
```

---

## Timing & Guarantees

- **Synchronous:** Event dispatch happens before HTTP response
- **Non-blocking:** Orchestrator runs in background goroutine
- **Atomicity:** Task completion is transactional; event fires after commit
- **Idempotent:** Multiple calls to OnCheckpointCompleted should be safe
- **Recoverable:** If Orchestrator crashes, task stays completed; retry via dashboard

---

## Common Pitfalls

1. **Blocking the daemon response:** Always use `go` goroutine for Orchestrator
2. **Parsing context:** Check `task.Context != nil` before Unmarshal
3. **Workspace ID:** Available via `task.IssueID` → lookup issue → get workspace
4. **Concurrency:** Event handlers run sequentially; no mutex needed for single bus
5. **Error handling:** Log failures in Orchestrator but don't block event bus

