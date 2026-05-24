# Go Backend Task Completion Flow - Complete Analysis

## Overview

This document maps the complete flow of task completion in the Multica Go backend, from when the Daemon calls the completion endpoint through event emission and service processing.

---

## 1. ENDPOINT: Daemon calls `/api/tasks/{taskId}/complete`

### Location
**File:** `server/internal/handler/daemon.go` (lines 1596-1630)

### Endpoint Definition
**Route:** `POST /api/tasks/{taskId}/complete`
**Router Entry:** `server/cmd/server/router.go` line 286

### Handler Function: `CompleteTask`

```go
// CompleteTask marks a running task as completed.
type TaskCompleteRequest struct {
	PRURL     string `json:"pr_url"`
	Output    string `json:"output"`
	SessionID string `json:"session_id"` // Claude session ID for future resumption
	WorkDir   string `json:"work_dir"`   // working directory used during execution
}

func (h *Handler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskId")

	// Verify the caller owns this task's workspace.
	if _, ok := h.requireDaemonTaskAccess(w, r, taskID); !ok {
		return
	}

	var req TaskCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, _ := json.Marshal(req)
	task, err := h.TaskService.CompleteTask(r.Context(), parseUUID(taskID), result, req.SessionID, req.WorkDir)
	if err != nil {
		slog.Warn("complete task failed", "task_id", taskID, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.emitIssueExecutedOnFirstCompletion(r, task)

	slog.Info("task completed", "task_id", taskID, "agent_id", uuidToString(task.AgentID))
	writeJSON(w, http.StatusOK, taskToResponse(*task))
}
```

**Key Points:**
- Requires daemon authentication via `requireDaemonTaskAccess()`
- Request payload includes: `pr_url`, `output`, `session_id`, `work_dir`
- Marshals request to JSON and passes to `TaskService.CompleteTask()`
- Emits analytics event via `emitIssueExecutedOnFirstCompletion()`
- Returns the updated task row

---

## 2. TASK SERVICE: `CompleteTask` Core Logic

### Location
**File:** `server/internal/service/task.go` (lines 955-1128)

### Function: `TaskService.CompleteTask`

```go
// CompleteTask marks a task as completed.
// Issue status is NOT changed here — the agent manages it via the CLI.
//
// For chat tasks, CompleteAgentTask and the chat_session resume-pointer
// update run in a single transaction. This closes a race where the next
// queued chat message could be claimed in the window between the task
// flipping to 'completed' and chat_session.session_id being refreshed,
// causing the new task to resume against a stale (or NULL) session.
func (s *TaskService) CompleteTask(ctx context.Context, taskID pgtype.UUID, result []byte, sessionID, workDir string) (*db.AgentTaskQueue, error) {
	var task db.AgentTaskQueue
	if err := s.runInTx(ctx, func(qtx *db.Queries) error {
		// 1. Mark task as completed in DB
		t, err := qtx.CompleteAgentTask(ctx, db.CompleteAgentTaskParams{
			ID:        taskID,
			Result:    result,
			SessionID: pgtype.Text{String: sessionID, Valid: sessionID != ""},
			WorkDir:   pgtype.Text{String: workDir, Valid: workDir != ""},
		})
		if err != nil {
			return err
		}
		task = t

		// 2. For chat tasks: update chat_session's resume pointer in same transaction
		if t.ChatSessionID.Valid {
			var sessionRuntimeID pgtype.UUID
			if sessionID != "" {
				sessionRuntimeID = t.RuntimeID
			}
			if err := qtx.UpdateChatSessionSession(ctx, db.UpdateChatSessionSessionParams{
				ID:        t.ChatSessionID,
				SessionID: pgtype.Text{String: sessionID, Valid: sessionID != ""},
				WorkDir:   pgtype.Text{String: workDir, Valid: workDir != ""},
				RuntimeID: sessionRuntimeID,
			}); err != nil {
				return fmt.Errorf("update chat session resume pointer: %w", err)
			}
		}
		return nil
	}); err != nil {
		// Idempotent on already-completed/failed/cancelled tasks
		if existing, lookupErr := s.Queries.GetAgentTask(ctx, taskID); lookupErr == nil {
			if errors.Is(err, pgx.ErrNoRows) {
				slog.Info("complete task: already finalized",
					"task_id", util.UUIDToString(taskID),
					"current_status", existing.Status,
					"agent_id", util.UUIDToString(existing.AgentID),
				)
				return &existing, nil
			}
		}
		return nil, fmt.Errorf("complete task: %w", err)
	}

	// 3. Capture analytics
	slog.Info("task completed", "task_id", util.UUIDToString(task.ID), "issue_id", util.UUIDToString(task.IssueID))
	s.captureTaskCompleted(ctx, task)

	// 4. Handle issue-bound tasks: ensure agent left a comment
	if task.IssueID.Valid {
		suppressNoActionComment, err := HasSquadLeaderNoActionEvaluationForTask(ctx, s.Queries, task)
		if err != nil {
			slog.Warn("checking squad leader no_action evaluation failed", ...)
		}
		agentCommented, _ := s.Queries.HasAgentCommentedSince(ctx, db.HasAgentCommentedSinceParams{
			IssueID:  task.IssueID,
			AuthorID: task.AgentID,
			Since:    task.StartedAt,
		})
		// If agent didn't comment, synthesize one from task output
		if !suppressNoActionComment && !agentCommented {
			var payload protocol.TaskCompletedPayload
			if err := json.Unmarshal(result, &payload); err == nil {
				if payload.Output != "" {
					body := util.UnescapeBackslashEscapes(payload.Output)
					if task.TriggerCommentID.Valid && isTrivialDoneOutput(body) {
						// Suppress trivial outputs like "done"
					} else {
						s.createAgentComment(ctx, task.IssueID, task.AgentID, redact.Text(body), "comment", task.TriggerCommentID)
					}
				}
			}
		}
	}

	// 5. Handle quick-create tasks: find the created issue, notify requester
	if qc, ok := s.parseQuickCreateContext(task); ok {
		s.notifyQuickCreateCompleted(ctx, task, qc)
	}

	// 6. Handle chat tasks: save assistant message, broadcast chat:done
	if task.ChatSessionID.Valid {
		var assistantMsg *db.ChatMessage
		var payload protocol.TaskCompletedPayload
		if err := json.Unmarshal(result, &payload); err == nil && payload.Output != "" {
			body := util.UnescapeBackslashEscapes(payload.Output)
			row, err := s.Queries.CreateChatMessage(ctx, db.CreateChatMessageParams{
				ChatSessionID: task.ChatSessionID,
				Role:          "assistant",
				Content:       redact.Text(body),
				TaskID:        task.ID,
				ElapsedMs:     computeChatElapsedMs(task),
			})
			if err != nil {
				slog.Error("failed to save assistant chat message", "task_id", util.UUIDToString(task.ID), "error", err)
			} else {
				assistantMsg = &row
				if err := s.Queries.SetUnreadSinceIfNull(ctx, task.ChatSessionID); err != nil {
					slog.Warn("failed to set unread_since", ...)
				}
			}
		}
		s.broadcastChatDone(ctx, task, assistantMsg)
	}

	// 7. Reconcile agent status (agent moves to idle when no more tasks)
	s.ReconcileAgentStatus(ctx, task.AgentID)

	// 8. ★ BROADCAST WEBSOCKET EVENT: task:completed
	s.broadcastTaskEvent(ctx, protocol.EventTaskCompleted, task)

	return &task, nil
}
```

### Critical Sections:

#### 2a. Database Transaction (lines 965-997)
- Updates `agent_task_queue` status to "completed"
- Stores result JSON: `{pr_url, output, session_id, work_dir}`
- For chat tasks: atomically updates `chat_session.session_id` and `chat_session.work_dir`

#### 2b. Agent Comment Synthesis (lines 1040-1076)
- Checks if agent posted any comments during execution
- If not, synthesizes one from task output to ensure user sees something
- Skips trivial outputs like "done" for comment-triggered tasks

#### 2c. Quick-Create Handling (lines 1084-1086)
- Special path for prompt-based task creation
- Finds the issue the agent just created
- Notifies requester via inbox

#### 2d. Chat Message & Broadcast (lines 1090-1119)
- Creates chat message for assistant reply
- Sets unread marker on chat session
- Calls `s.broadcastChatDone(ctx, task, assistantMsg)`

#### 2e. Agent Status Reconciliation (line 1122)
- Calls `s.ReconcileAgentStatus(ctx, task.AgentID)`
- Agent status transitions: if no more active tasks → "idle"

#### 2f. ★ EVENT BROADCAST (line 1125)
```go
s.broadcastTaskEvent(ctx, protocol.EventTaskCompleted, task)
```

---

## 3. EVENT BROADCAST: How `task:completed` Gets Emitted

### 3a. Event Broadcast Function
**Location:** `server/internal/service/task.go` lines 1758-1779

```go
func (s *TaskService) broadcastTaskEvent(ctx context.Context, eventType string, task db.AgentTaskQueue) {
	// Resolve workspace ID (from issue, chat session, or autopilot)
	workspaceID := s.ResolveTaskWorkspaceID(ctx, task)
	if workspaceID == "" {
		return
	}
	
	// Build event payload
	payload := map[string]any{
		"task_id":  util.UUIDToString(task.ID),
		"agent_id": util.UUIDToString(task.AgentID),
		"issue_id": util.UUIDToString(task.IssueID),
		"status":   task.Status,
	}
	if task.ChatSessionID.Valid {
		payload["chat_session_id"] = util.UUIDToString(task.ChatSessionID)
	}
	
	// Publish to event bus
	s.Bus.Publish(events.Event{
		Type:        eventType,  // "task:completed"
		WorkspaceID: workspaceID,
		ActorType:   "system",
		ActorID:     "",
		Payload:     payload,
	})
}
```

### 3b. Event Bus Publish
**Location:** `server/internal/events/bus.go`

The event bus is synchronous and in-memory:
```go
func (b *Bus) Publish(e Event) {
	// Call type-specific handlers (subscribers to "task:completed")
	for _, h := range b.listeners[e.Type] {
		h(e)
	}
	// Call global handlers (SubscribeAll)
	for _, h := range b.globalHandlers {
		h(e)
	}
}
```

### 3c. Event Structure

**Event Type Constant:**
```go
// server/pkg/protocol/events.go line 36
const EventTaskCompleted = "task:completed"
```

**Event Data Structure:**
```go
type Event struct {
	Type        string  // "task:completed"
	WorkspaceID string  // Routes to correct workspace room
	ActorType   string  // "system"
	ActorID     string  // Empty
	TaskID      string  // Optional: per-resource routing hint
	ChatSessionID string // Optional: per-resource routing hint
	Payload     any     // Map with task_id, agent_id, issue_id, status, chat_session_id
}
```

### 3d. WebSocket Broadcasting
**Location:** `server/cmd/server/listeners.go` lines 151-193

The global event listener broadcasts to WebSocket clients:

```go
bus.SubscribeAll(func(e events.Event) {
	// Skip personal events
	if personalEvents[e.Type] {
		return
	}

	msg := map[string]any{
		"type":       e.Type,        // "task:completed"
		"payload":    e.Payload,     // {task_id, agent_id, issue_id, status, ...}
		"actor_id":   e.ActorID,     // ""
		"actor_type": e.ActorType,   // "system"
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	if e.WorkspaceID != "" {
		// Broadcast to all clients in this workspace
		b.BroadcastToWorkspace(e.WorkspaceID, data)
	}
})
```

**Final WebSocket Message to Clients:**
```json
{
  "type": "task:completed",
  "payload": {
    "task_id": "123e4567-e89b-12d3-a456-426614174000",
    "agent_id": "987fcdeb-51a2-11ec-81d3-0242ac130003",
    "issue_id": "abcd1234-51a2-11ec-81d3-0242ac130003",
    "status": "completed",
    "chat_session_id": "optional-if-chat-task"
  },
  "actor_id": "",
  "actor_type": "system"
}
```

---

## 4. EVENT HANDLERS: What Happens to `task:completed`

The event bus triggers multiple handlers that subscribe to `task:completed`:

### 4a. Activity Logger
**Location:** `server/cmd/server/activity_listeners.go` line 244

```go
bus.Subscribe(protocol.EventTaskCompleted, func(e events.Event) {
	handleTaskActivity(ctx, bus, queries, e, "completed")
})
```

Creates an activity log entry:
- Records task completion in workspace activity stream
- Used for "recent activity" displays

### 4b. ★ Orchestrator Hook Point (NOT YET IMPLEMENTED)
**Planned Location:** `server/cmd/server/router.go` or new file

According to design doc (line 617-623), this is where the Orchestrator will hook:

```go
// FUTURE: After CompleteTask returns and broadcasts task:completed
bus.Subscribe(protocol.EventTaskCompleted, func(e events.Event) {
	// Parse task from event
	taskID := e.Payload["task_id"].(string)
	task, _ := queries.GetAgentTask(ctx, parseUUID(taskID))
	
	// Check if this is part of a pipeline
	var context map[string]any
	json.Unmarshal(task.Context, &context)
	
	if pipelineID, hasPipeline := context["pipeline_issue_id"]; hasPipeline {
		// Trigger orchestrator to advance to next stage
		go orchestrator.OnCheckpointCompleted(ctx, task)
	}
})
```

**Or in the handler directly (simpler approach):**

```go
// In handler.go CompleteTask()
task, err := h.TaskService.CompleteTask(...)

// Check if task is part of a pipeline
var context map[string]any
if task.Context != nil {
	json.Unmarshal(task.Context, &context)
	if pipelineID, ok := context["pipeline_issue_id"]; ok && pipelineID != "" {
		// Non-blocking trigger
		go h.Orchestrator.OnCheckpointCompleted(r.Context(), task)
	}
}
```

---

## 5. SUMMARY: Complete Task Completion Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│ 1. DAEMON CALLS                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ POST /api/tasks/{taskId}/complete                                   │
│ Body: {pr_url, output, session_id, work_dir}                        │
└──────────────────┬──────────────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 2. HANDLER: daemon.go::CompleteTask()                               │
├─────────────────────────────────────────────────────────────────────┤
│ • Verify daemon auth                                                │
│ • Parse request body                                                │
│ • Call TaskService.CompleteTask()                                   │
│ • Call emitIssueExecutedOnFirstCompletion()                         │
│ • Return task response (200 OK)                                     │
└──────────────────┬──────────────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 3. SERVICE: task.go::CompleteTask()                                 │
├─────────────────────────────────────────────────────────────────────┤
│ • Transaction:                                                      │
│   - Mark task as completed in DB                                    │
│   - Save result JSON to result column                               │
│   - Update chat_session resume pointer (if chat task)               │
│ • captureTaskCompleted() → Analytics                                │
│ • Create agent comment (if missing)                                 │
│ • Handle quick-create notifications                                 │
│ • Create chat message (if chat task)                                │
│ • ReconcileAgentStatus() → Update agent.status                      │
│ • broadcastTaskEvent("task:completed", task)  ← ★ KEY EVENT        │
└──────────────────┬──────────────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 4. EVENT BUS: events/bus.go::Publish()                              │
├─────────────────────────────────────────────────────────────────────┤
│ • Synchronous dispatch to all "task:completed" subscribers          │
│ • Also dispatch to SubscribeAll() global handlers                   │
│ • Handlers execute sequentially (panic-safe)                        │
└──────────────────┬──────────────────────────────────────────────────┘
                   │
        ┌──────────┼──────────┬──────────────┐
        │          │          │              │
        ▼          ▼          ▼              ▼
    ┌────────┐ ┌────────┐ ┌────────┐ ┌─────────────────┐
    │Activity│ │Notif   │ │(future)│ │Global Listener  │
    │Logger  │ │Handler │ │Orch*   │ │(WS Broadcast)   │
    └────────┘ └────────┘ └────────┘ └────────┬────────┘
                                               │
                                               ▼
                          ┌──────────────────────────────────────┐
                          │ 5. WEBSOCKET BROADCAST              │
                          ├──────────────────────────────────────┤
                          │ Route: workspace:{workspace_id}      │
                          │ Message:                             │
                          │ {                                    │
                          │   "type": "task:completed",          │
                          │   "payload": {                       │
                          │     "task_id": "...",                │
                          │     "agent_id": "...",               │
                          │     "issue_id": "...",               │
                          │     "status": "completed"            │
                          │   },                                 │
                          │   "actor_type": "system"             │
                          │ }                                    │
                          │                                      │
                          │ Delivered to all clients in room     │
                          └──────────────────────────────────────┘

(*) Orchestrator: WHERE TO HOOK
    Insert handler after CompleteTask returns:
    
    if pipelineID := task.Context["pipeline_issue_id"]; pipelineID != "" {
        go orchestrator.OnCheckpointCompleted(ctx, task)
    }
```

---

## 6. KEY FIELDS FOR ORCHESTRATOR INTEGRATION

When a task completes, the following fields are available:

```go
type AgentTaskQueue struct {
	ID                pgtype.UUID    // Task ID
	Status            string         // Now "completed"
	AgentID           pgtype.UUID    // Which agent ran this
	RuntimeID         pgtype.UUID    // Which runtime executed it
	
	// Links (exactly one is valid per task)
	IssueID           pgtype.UUID    // Issue task (can be NULL)
	ChatSessionID     pgtype.UUID    // Chat task (can be NULL)
	AutopilotRunID    pgtype.UUID    // Autopilot task (can be NULL)
	
	// ★ PIPELINE INTEGRATION: Custom data
	Context           []byte         // JSONB: custom data including pipeline_issue_id
	
	// Results
	Result            []byte         // Task output as JSON: {pr_url, output, session_id, work_dir}
	CompletedAt       pgtype.Timestamptz
	
	// Session resumption
	SessionID         pgtype.Text    // Claude session ID for next attempt
	WorkDir           pgtype.Text    // Working directory
	
	// Retry tracking
	Attempt           int32
	MaxAttempts       int32
}
```

---

## 7. ORCHESTRATOR INTEGRATION POINT (Design Doc Reference)

**File:** `docs/super-individual-design.md` lines 617-626

```go
// Recommended hook location: handler/daemon.go or new handler/orchestrator.go

func (h *Handler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	// ... existing code ...
	
	task, err := h.TaskService.CompleteTask(r.Context(), parseUUID(taskID), result, req.SessionID, req.WorkDir)
	if err != nil {
		// ... error handling ...
		return
	}

	// ★ NEW: Orchestrator hook
	if h.Orchestrator != nil {
		var context map[string]any
		if task.Context != nil {
			json.Unmarshal(task.Context, &context)
		}
		if pipelineID, ok := context["pipeline_issue_id"]; ok && pipelineID != "" {
			// Non-blocking: don't let slow orchestrator delay response to daemon
			go h.Orchestrator.OnCheckpointCompleted(r.Context(), *task)
		}
	}

	// ... rest of handler ...
}
```

**Or via event handler (cleaner, less coupled):**

```go
// In server/cmd/server/listeners.go or new file:

bus.Subscribe(protocol.EventTaskCompleted, func(e events.Event) {
	// Only process pipeline tasks
	payload, ok := e.Payload.(map[string]any)
	if !ok {
		return
	}
	
	taskID := payload["task_id"].(string)
	task, err := queries.GetAgentTask(ctx, parseUUID(taskID))
	if err != nil {
		return
	}
	
	// Check for pipeline context
	var taskContext map[string]any
	if task.Context != nil {
		json.Unmarshal(task.Context, &taskContext)
	}
	
	if pipelineID, ok := taskContext["pipeline_issue_id"]; ok && pipelineID != "" {
		// Trigger orchestrator in background
		go orchestrator.OnCheckpointCompleted(context.Background(), *task)
	}
})
```

---

## 8. VERIFICATION CHECKLIST

- [x] Endpoint: `POST /api/tasks/{taskId}/complete` → `daemon.go::CompleteTask()`
- [x] Request body: `{pr_url, output, session_id, work_dir}`
- [x] Service call: `TaskService.CompleteTask()`
- [x] DB transaction: Marks task completed + updates resume pointers
- [x] Event type: `"task:completed"` (constant in `protocol/events.go`)
- [x] Event data: `{task_id, agent_id, issue_id, status, chat_session_id}`
- [x] Event broadcast: Via `Bus.Publish()` → synchronous dispatch
- [x] WebSocket broadcast: Via workspace room → all connected clients
- [x] Orchestrator hook point: Check `task.Context["pipeline_issue_id"]` → call `OnCheckpointCompleted()`

