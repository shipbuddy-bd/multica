# Developer Quick Start: Working with Issues and Agents

This guide helps developers understand how to work with Multica's issue and agent systems.

---

## Quick Facts

### Issue Creation → Agent Execution Path

```go
// 1. Create issue (server/internal/handler/issue.go)
POST /api/issues {
    title: "Fix OAuth bug",
    assignee_type: "agent",  // ← key: type must be "agent"
    assignee_id: "agent-123",
    status: "todo"           // ← key: NOT "backlog" triggers task
}

// 2. Server enqueues task (server/internal/handler/issue.go:UpdateIssue)
if issue.AssigneeType == "agent" && issue.Status != "backlog" {
    INSERT INTO agent_task_queue (
        agent_id: issue.assignee_id,
        issue_id: issue.id,
        status: "queued"
    )
    // Broadcast WebSocket event: daemon:task_available
}

// 3. Daemon wakes up (server/internal/daemon/daemon.go)
// Receives: daemon:task_available {runtime_id, task_id}
// Calls: GET /api/tasks/claim {runtime_id}

// 4. Server dispatches task
UPDATE agent_task_queue SET status = "dispatched"
BROADCAST: task:dispatch

// 5. Daemon executes
// - Clone repo to worktree
// - Spawn agent CLI: claude-agent --task-id=xxx
// - Stream: task:progress, task:message
// - POST /api/tasks/complete with result

// 6. Server broadcasts completion
BROADCAST: task:completed {task_id, result}
```

---

## Key Files by Use Case

### I want to understand issue lifecycle
**Primary:** `server/internal/handler/issue.go`  
**Secondary:** `server/pkg/db/queries/issue.sql`  
**Frontend:** `packages/core/types/issue.ts`

### I want to add a new issue field (e.g., priority_score)
1. **Migration:** `server/migrations/XXX_add_priority_score.up.sql`
   ```sql
   ALTER TABLE issue ADD COLUMN priority_score INT;
   ```

2. **Go models:** `server/pkg/db/generated/models.go` (auto-generated, don't edit)

3. **Frontend types:** `packages/core/types/issue.ts`
   ```typescript
   interface Issue {
     // ... existing fields
     priority_score: number;
   }
   ```

4. **Handler:** `server/internal/handler/issue.go`
   ```go
   // Add field to IssueResponse struct
   type IssueResponse struct {
     // ...
     PriorityScore int `json:"priority_score"`
   }
   ```

### I want to add agent metadata tracking
**Best practice:** Use `issue.metadata` JSONB map instead of new columns

```go
// In daemon agent execution:
metadata := map[string]interface{}{
    "pr_number": "1234",
    "pipeline_status": "running",
    "attempt": 1,
}
// Update issue.metadata (daemon sends this to server)
```

### I want to add a new WebSocket event
**File:** `server/pkg/protocol/events.go`

```go
const (
    // Add your event constant
    EventIssueWorkflowChanged = "issue:workflow_changed"
)
```

**Message definition:** `server/pkg/protocol/messages.go`

```go
type IssueWorkflowPayload struct {
    IssueID  string
    Status   string
    // ... fields
}
```

**Broadcast from handler:** `server/internal/handler/issue.go`

```go
// After issue update
d.hub.Broadcast(&protocol.Message{
    Type: protocol.EventIssueWorkflowChanged,
    Payload: jsonMarshal(payload),
})
```

### I want to trace a task through the system
**Key tables in order:**
1. `issue` — What the user sees
2. `agent_task_queue` — Task queue entry (created on assignment)
3. `agent_runtime` — Where it runs (heartbeat tracked)
4. Look at `agent_task_queue.context` — What the daemon received

**Common queries:**
```sql
-- Find all tasks for an issue
SELECT * FROM agent_task_queue WHERE issue_id = 'xxx' ORDER BY created_at;

-- Find all pending tasks for a runtime
SELECT * FROM agent_task_queue 
WHERE runtime_id = 'yyy' AND status IN ('queued', 'dispatched', 'running');

-- Find tasks that failed
SELECT * FROM agent_task_queue 
WHERE status = 'failed' AND failure_reason IS NOT NULL
ORDER BY completed_at DESC;
```

---

## Agent Status Understanding

```
Agent has THREE orthogonal status fields:

1. status (operational state)
   - idle: waiting for work
   - working: actively executing
   - blocked: waiting for external event
   - error: fatal failure
   - offline: unreachable (heartbeat timeout)

2. runtime_mode
   - local: desktop app on user's machine
   - cloud: hosted service

3. visibility
   - workspace: anyone can use
   - private: owner only

Query example:
SELECT * FROM agent 
WHERE status = 'offline' AND runtime_mode = 'local' 
AND visibility = 'workspace';
```

---

## Testing Issues & Tasks

### Create a test issue with agent
```go
// From tests
issue := &models.Issue{
    ID:           uuid.New().String(),
    WorkspaceID:  workspaceID,
    Title:        "Test issue",
    Status:       "todo",
    AssigneeType: "agent",
    AssigneeID:   agentID,
    CreatorType:  "member",
    CreatorID:    userID,
}
// Insert into DB
db.CreateIssue(ctx, issue)

// Check task was enqueued
tasks := db.GetAgentTasks(ctx, agentID)
assert.Len(t, tasks, 1)
assert.Equal(t, "queued", tasks[0].Status)
```

### Simulate daemon task completion
```go
// Daemon sends completion
result := map[string]interface{}{
    "output": "Fixed the bug!",
    "pr_url": "https://github.com/...",
}
client.CompleteTask(ctx, taskID, result)

// Verify status updated
task, _ := db.GetTask(ctx, taskID)
assert.Equal(t, "completed", task.Status)
```

---

## Common Pitfalls

### ❌ Pitfall 1: Creating issue in backlog with agent assignee
```go
// NO! Task won't be enqueued
issue := Issue{
    Status: "backlog",
    AssigneeType: "agent",
}

// YES! Task will be enqueued
issue := Issue{
    Status: "todo",  // or in_progress, etc.
    AssigneeType: "agent",
}
```

### ❌ Pitfall 2: Updating metadata without full object
```go
// NO! Will replace entire metadata
issue.Metadata = {new_field: "value"}  // Old fields lost!

// YES! Merge before updating
existing := json.Unmarshal(issue.Metadata)
existing["new_field"] = "value"
issue.Metadata = json.Marshal(existing)
```

### ❌ Pitfall 3: Assuming runtime and agent are the same
```go
// NO! One runtime ≠ one agent
agent := agent_table (id = agent-123)
agent.runtime_id = runtime-456 (this is a foreign key to agent_runtime table)

// A runtime can host multiple agents
// A daemon can have multiple runtimes
```

### ❌ Pitfall 4: Fetching on-demand in daemon
```go
// NO! Task context is already captured
while executing_task(taskID) {
    // Don't fetch fresh issue from server!
    // Issue may have been updated since task started
}

// YES! Use captured context
context := task.Context  // This is the JSONB snapshot
issue := context["issue"]  // Use this, not fresh from DB
```

---

## Debugging Checklist

**Issue not getting a task:**
- [ ] `status` != "backlog"?
- [ ] `assignee_type` == "agent"?
- [ ] Agent exists in workspace?
- [ ] Check `agent_task_queue` for errors?

**Daemon not claiming task:**
- [ ] Runtime registered? (`agent_runtime` table)
- [ ] Runtime online? (heartbeat recent?)
- [ ] Daemon process running? (check logs)
- [ ] Task status is "queued"? (not already claimed)

**Task stuck in "running":**
- [ ] Daemon process crashed? (check logs)
- [ ] Agent process hung? (timeout logic?)
- [ ] Network timeout between daemon and server?
- [ ] Manual cancel: `PATCH /api/tasks/{id} {status: "cancelled"}`

**WebSocket events not appearing:**
- [ ] Client subscribed to correct event type?
- [ ] Server has `hub.Broadcast()` call for event?
- [ ] Client WebSocket connected (not closed)?
- [ ] Check Chrome DevTools Network tab → WS tab

---

## Performance Notes

### Fast queries
- `SELECT * FROM issue WHERE workspace_id = ? AND status = ?` (indexed)
- `SELECT * FROM agent_task_queue WHERE status = 'queued'` (partial index)

### Slow queries (avoid)
- `SELECT * FROM agent_task_queue WHERE status LIKE '%ing'` (no index)
- `SELECT * FROM issue WHERE metadata @> '{"key": "value"}'` (full table scan)
- Filtering on metadata without index

### Optimization for large workspaces
- Use `status IN ('done', 'cancelled')` + pagination for archive
- Consider materialized view for "active tasks by runtime"
- Monitor `agent_task_queue` growth (auto-archive completed tasks after 30d)

---

## Resources

- **Full architecture:** See `COMPREHENSIVE_ARCHITECTURE.md`
- **Issue types:** `packages/core/types/issue.ts`
- **Agent types:** `packages/core/types/agent.ts`
- **Database schema:** `server/migrations/001_init.up.sql`
- **WebSocket protocol:** `server/pkg/protocol/events.go`, `messages.go`
