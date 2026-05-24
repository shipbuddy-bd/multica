# Multica Architecture: Issue & Agent Integration

## Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            FRONTEND (React + TypeScript)                     │
│  packages/core/types/{issue.ts, agent.ts, squad.ts, events.ts}              │
│                                                                               │
│  Issue Display → Metadata Renderer → Agent Status Indicator                 │
│       ↑              ↓                      ↓                                 │
│  WebSocket Listener (event:type → reducer → cache update)                   │
└────────────────────────┬─────────────────────────────────────────────────────┘
                         │ WebSocket: ws://...
                         │ Subscribe: issue:*, task:*, agent:*
                         ↓
┌─────────────────────────────────────────────────────────────────────────────┐
│                          HTTP + WebSocket Server                             │
│  server/internal/handler/{issue.go, agent.go}                               │
│                                                                               │
│  POST /api/issues         → CreateIssue + queue agent_task_queue            │
│  PATCH /api/issues/:id    → UpdateIssue (NOT metadata)                      │
│  PUT /api/issues/:id/metadata/:key → SetMetadataKey (atomic)                │
│  GET /api/issues          → ListIssues (with filters)                       │
│  POST /api/agents         → CreateAgent + register runtime_id               │
│  POST /api/tasks/claim    → ClaimAgentTask (daemon calls this)              │
│  PATCH /api/tasks/:id     → UpdateTaskStatus (daemon reports progress)      │
│                                                                               │
│  Broadcast Events:                                                           │
│  - issue:created, issue:updated, issue:deleted                             │
│  - issue_metadata:changed (single key)                                      │
│  - task:queued, task:dispatch, task:progress, task:completed               │
│  - agent:status, agent:created                                             │
│                                                                               │
│  Message Broker: server/internal/events/bus.go                             │
│    ↓ Publishes to all connected WebSocket clients                          │
└─────────────┬───────────────────────┬──────────────────────┬────────────────┘
              │                       │                      │
    ┌─────────↓─────────┐   ┌────────↓──────────┐  ┌────────↓──────────┐
    │                   │   │                   │  │                   │
    ↓                   ↓   ↓                   ↓  ↓                   ↓
┌──────────────────┐ ┌──────────────────────────────────────────────────────┐
│   PostgreSQL     │ │   Daemon (Local or Cloud)                            │
│   Database       │ │   server/internal/daemon/                            │
│                  │ │                                                       │
│ issue (1)        │ │   1. HTTP POST /claim → agent_task_queue.status=     │
│  ├─ id           │ │      'dispatched'                                     │
│  ├─ status       │ │                                                       │
│  ├─ assignee_id  │ │   2. WebSocket listen task:queued, task:available   │
│  ├─ metadata {}  │ │                                                       │
│  ├─ labels       │ │   3. Execute agent/skill against issue               │
│  └─ ...          │ │                                                       │
│                  │ │   4. POST /complete + messages via WS                │
│ agent (1)        │ │      → agent_task_queue.status = 'completed'         │
│  ├─ id           │ │                                                       │
│  ├─ status       │ │   5. Listen for issue:updated, update metadata       │
│  ├─ runtime_id   │ │      via PUT /api/issues/:id/metadata/:key           │
│  └─ ...          │ │                                                       │
│                  │ │   Agent execution flow:                               │
│ agent_task_queue │ │   - Receive issue title + description                │
│ (N:N join)       │ │   - Run agent with Claude API or local LLM          │
│  ├─ agent_id     │ │   - Agent emits tool_use (e.g., create PR)          │
│  ├─ issue_id     │ │   - Tool result updates issue.metadata              │
│  ├─ status       │ │   - task:message events stream to frontend           │
│  ├─ priority     │ │                                                       │
│  ├─ dispatched_at│ │   Alternative: Squad Dispatch                       │
│  ├─ started_at   │ │   - Issue assigned to squad (assignee_type=squad)   │
│  ├─ completed_at │ │   - Squad leader receives task                       │
│  ├─ result {}    │ │   - Leader creates sub-tasks for members             │
│  └─ error TEXT   │ │   - Members execute in parallel/serial               │
│                  │ │   - Leader aggregates results                        │
│ squad (1)        │ │                                                       │
│  ├─ id           │ │                                                       │
│  ├─ name         │ │                                                       │
│  ├─ leader_id    │ │                                                       │
│  └─ members []   │ │                                                       │
│                  │ │                                                       │
│ issue_metadata   │ │   Runtime Info Exchange:                             │
│  (JSONB store)   │ │   - agent.runtime_id → daemon binding                │
│  ├─ pr_number    │ │   - daemon reports via heartbeat endpoint           │
│  ├─ pipeline_stat│ │   - agent.status transitions                         │
│  └─ waiting_on   │ │     (idle → working → completed/error)              │
│                  │ │                                                       │
└──────────────────┘ └──────────────────────────────────────────────────────┘
```

---

## Issue State Machine

```
                        ┌─────────────────────────────────────┐
                        │   Issue Created                     │
                        │   (Default status: backlog)         │
                        └────────────────┬────────────────────┘
                                         │
                                         ↓
                        ┌────────────────────────────────────┐
                        │         [backlog]                  │
                        │  (Unscheduled, not started)        │
                        └────────┬──────────────────────────┘
                                 │
                                 ↓
                        ┌────────────────────────────────────┐
                        │         [todo]                     │
                        │  (Scheduled, ready to work)        │
                        └────────┬──────────────────────────┘
                                 │
                    ┌────────────┘
                    ↓
        ┌───────────────────────┐
        │   [in_progress]       │
        │ (Agent/user working)  │
        └───────────┬───────────┘
                    │
        ┌───────────┴────────────┐
        ↓                        ↓
    ┌─────────────┐      ┌──────────────────┐
    │ [in_review] │      │   [blocked]      │
    │ (PR/review) │      │ (Waiting for...)  │
    └──────┬──────┘      └──────┬───────────┘
           │                    │
           ↓                    ↓ (after unblock)
    ┌─────────────┐      ┌──────────────────┐
    │  [done]     │      │   [in_progress]  │
    │ (Completed) │      │ (Resume work)    │
    └─────────────┘      └──────────────────┘
           ↑
           │
           └──────────────────────
                    or
    ┌─────────────────────────────────────────────┐
    │            [cancelled]                       │
    │  (Won't complete - from any status)         │
    └─────────────────────────────────────────────┘
```

---

## Agent Task Queue Lifecycle

```
┌─────────────────────────────────────────────────────────────────┐
│ ISSUE ASSIGNED TO AGENT                                         │
│ CreateIssue(..., assignee_type='agent', assignee_id=UUID)      │
└────────────────────┬────────────────────────────────────────────┘
                     │ Query: CountCreatedIssueAssignees
                     │        (for assignment suggestion)
                     ↓
┌────────────────────────────────────────────────────────────────┐
│ CREATE agent_task_queue ROW                                     │
│ {                                                               │
│   agent_id: UUID                                                │
│   issue_id: UUID                                                │
│   status: 'queued'    ← Not yet claimed by daemon              │
│   priority: 0 (default)                                         │
│   created_at: NOW()                                             │
│ }                                                               │
│                                                                  │
│ Broadcast: task:queued → Frontend subscribes, sees task added  │
└────────────────────┬───────────────────────────────────────────┘
                     │
                     ↓
┌────────────────────────────────────────────────────────────────┐
│ DAEMON CONNECTS VIA WEBSOCKET                                   │
│ Event: daemon:register { daemon_id, agent_id, runtimes[] }     │
│        → agent.status = 'idle'                                  │
│        → issue display shows agent avatar with "idle" badge    │
└────────────────────┬───────────────────────────────────────────┘
                     │
                     ↓
┌────────────────────────────────────────────────────────────────┐
│ DAEMON CLAIMS TASK                                              │
│ POST /api/tasks/claim { runtime_id, limit=N }                 │
│ Query: SELECT * FROM agent_task_queue                          │
│        WHERE runtime_id = $1 AND status = 'queued'             │
│        ORDER BY priority DESC, created_at ASC                  │
│        LIMIT $2                                                 │
│                                                                  │
│ Response: [{ id, agent_id, issue_id, ... }]  (batch)          │
│                                                                  │
│ Update: status = 'dispatched', dispatched_at = NOW()           │
└────────────────────┬───────────────────────────────────────────┘
                     │
                     ↓
┌────────────────────────────────────────────────────────────────┐
│ BROADCAST: task:dispatch                                        │
│ Payload: { task_id, agent_id, issue_id, runtime_id }          │
│ Frontend: Update task in cache, show "dispatched" status       │
│ Agent status: 'idle' → 'working' (if task is running)          │
└────────────────────┬───────────────────────────────────────────┘
                     │
                     ↓
┌────────────────────────────────────────────────────────────────┐
│ DAEMON EXECUTES AGENT                                           │
│                                                                  │
│ 1. Fetch issue details (title, description, metadata, etc.)   │
│ 2. Pass to Claude API or local LLM with agent instructions    │
│ 3. Agent makes tool calls (e.g., create PR, update status)    │
│                                                                  │
│ Update: status = 'running', started_at = NOW()                │
└────────────────────┬───────────────────────────────────────────┘
                     │
                     ├─────────────────────────────┐
                     ↓                             ↓
┌──────────────────────────────────┐  ┌────────────────────────────┐
│ STREAM MESSAGES (WS)             │  │ UPDATE METADATA (HTTP)     │
│                                   │  │                             │
│ Events: task:message             │  │ PUT /api/issues/:id/      │
│ Payload: {                        │  │     metadata/pr_number    │
│   seq: 1,                         │  │ Body: { value: 1234 }    │
│   type: 'text|tool_use|...',     │  │                             │
│   content: '...',                 │  │ Update: issue.metadata =  │
│   tool: 'tool_name' (optional)   │  │   jsonb_set(..., pr_number) │
│ }                                 │  │                             │
│                                   │  │ Broadcast:                  │
│ Frontend: Render in timeline      │  │ issue_metadata:changed      │
│                                   │  │ Payload: {                  │
│                                   │  │   issue_id: UUID            │
│                                   │  │   metadata: { ... }         │
│                                   │  │ }                           │
└──────────────────────────────────┘  └────────────────────────────┘
                     │
                     ↓
┌────────────────────────────────────────────────────────────────┐
│ AGENT COMPLETES EXECUTION                                       │
│                                                                  │
│ Daemon calls: POST /api/tasks/:id/complete                    │
│ Payload: { status, result, error? }                            │
│                                                                  │
│ Database update:                                                │
│   status = 'completed' | 'failed'                              │
│   completed_at = NOW()                                         │
│   result = <JSON output>                                        │
│   error = <error message if failed>                            │
└────────────────────┬───────────────────────────────────────────┘
                     │
                     ├───────────────────────────────┐
                     ↓                               ↓
┌──────────────────────────────┐  ┌──────────────────────────────┐
│ IF COMPLETED:                │  │ IF FAILED:                    │
│                               │  │                               │
│ Broadcast:                    │  │ Broadcast:                    │
│ task:completed                │  │ task:failed                   │
│                               │  │ Payload: { task_id, status } │
│ [Optional]                    │  │                               │
│ issue:updated                 │  │ [Optional]                    │
│ (if agent changed issue status│  │ Compute failure_reason:       │
│  or metadata)                 │  │ - agent_error                │
│                               │  │ - timeout                     │
│                               │  │ - runtime_offline            │
│ Agent status:                 │  │ - manual                      │
│ 'working' → 'idle'            │  │                               │
│ (if no more queued tasks)     │  │ Agent status:                 │
│                               │  │ 'working' → 'error'           │
│                               │  │ (if fatal; else 'idle')      │
└──────────────────────────────┘  └──────────────────────────────┘
                     │                              │
                     └──────────────────┬───────────┘
                                        ↓
                    ┌─────────────────────────────────────┐
                    │ OPTIONAL: AUTO-RETRY                │
                    │                                      │
                    │ If failed + retry logic enabled:    │
                    │   parent_task_id = <original task>  │
                    │   attempt = original.attempt + 1    │
                    │   Create new agent_task_queue row   │
                    │   (cycle repeats from "claimed")    │
                    └─────────────────────────────────────┘
```

---

## Squad Dispatch Flow

```
┌──────────────────────────────────────────────────────────────────┐
│ ISSUE ASSIGNED TO SQUAD                                          │
│ CreateIssue(..., assignee_type='squad', assignee_id=<uuid>)     │
│                                                                   │
│ Broadcast: issue:created → Frontend shows squad avatar           │
└────────────────────┬─────────────────────────────────────────────┘
                     │
                     ↓
┌──────────────────────────────────────────────────────────────────┐
│ SQUAD LEADER RECEIVES TASK                                       │
│ (same as agent task: agent_task_queue row, but assignee is      │
│  the leader agent)                                               │
│                                                                   │
│ Leader agent claims task from queue                              │
└────────────────────┬─────────────────────────────────────────────┘
                     │
                     ↓
┌──────────────────────────────────────────────────────────────────┐
│ LEADER AGENT EXECUTION                                           │
│                                                                   │
│ Receives: issue (with squad_id embedded or queried)              │
│ Query: SELECT * FROM squad_member                               │
│        WHERE squad_id = $1                                        │
│                                                                   │
│ Decides which members to activate based on:                      │
│ - Issue requirements                                             │
│ - Squad member availability (agent.status)                       │
│ - Load balancing (max_concurrent_tasks per member)              │
│                                                                   │
│ Actions:                                                         │
│ 1. Create sub-tasks for selected members (new agent_task_queue)│
│ 2. Coordinate via shared issue.metadata keys                     │
│ 3. Aggregate results from member executions                      │
│                                                                   │
│ Example flow:                                                    │
│   "code_review_squad" assigned to "review_pr" issue             │
│   Leader analyzes issue                                          │
│   Creates tasks for: backend_reviewer, frontend_reviewer        │
│   Polls their completion via metadata keys                       │
│   Synthesizes final recommendation in issue.status              │
│                                                                   │
│ Broadcast:                                                       │
│ - task:dispatch (multiple times for each member)                │
│ - issue_metadata:changed (for coordination)                      │
│ - task:progress (as members report)                              │
└────────────────────┬─────────────────────────────────────────────┘
                     │
                     ↓
┌──────────────────────────────────────────────────────────────────┐
│ SQUAD MEMBER EXECUTION (parallel N)                              │
│                                                                   │
│ Each member agent:                                               │
│ 1. Claims its own agent_task_queue row                           │
│ 2. Executes (can read issue.metadata for context)               │
│ 3. Updates metadata with results: PUT /metadata/<key>           │
│ 4. Task completes (individual task:completed event)             │
│                                                                   │
│ All members' metadata updates broadcast via                      │
│ issue_metadata:changed (one event per key change)               │
└────────────────────┬─────────────────────────────────────────────┘
                     │
                     ↓
┌──────────────────────────────────────────────────────────────────┐
│ FRONTEND VIEW                                                    │
│                                                                   │
│ Single issue view shows:                                         │
│ - Squad assignment (assignee_type='squad')                       │
│ - Squad leader task status (task:dispatch, task:completed)      │
│ - Member activities via metadata updates in timeline             │
│ - Final state in issue.status + metadata aggregate               │
│                                                                   │
│ Alternative (detailed view):                                     │
│ - Task list shows leader + members as sub-tasks                 │
│ - Individual metadata updates visible per member                 │
└──────────────────────────────────────────────────────────────────┘
```

---

## WebSocket Event Subscription Model

```
┌─────────────────────────────────────────────────────────────┐
│ Frontend connects: ws://server/api/ws                         │
│ Subscribes: "workspace:<workspace_id>"                       │
└─────────────────────┬───────────────────────────────────────┘
                      │
     ┌────────────────┼────────────────┐
     ↓                ↓                ↓
     
Issue Events      Agent Events    Task Events
─────────         ──────────      ──────────
issue:created     agent:status    task:queued
issue:updated     agent:created   task:dispatch
issue:deleted     agent:archived  task:progress
                                   task:completed
issue_metadata:   Squad Events    task:failed
  changed         ──────────      task:message
issue_labels:     squad:created   task:cancelled
  changed         squad:updated
                  squad:deleted    Inbox Events
Comment Events    
──────────────    Member Events   inbox:new
comment:created   ──────────      inbox:read
comment:updated   member:added    inbox:archived
comment:deleted   member:updated
comment:resolved  member:removed

Reaction Events                   Chat Events
──────────────                    ──────────
reaction:added                    chat:message
reaction:removed                  chat:done
issue_reaction:                   chat:session_*
  added
issue_reaction:
  removed                         Daemon Events
                                  ──────────────
Activity Events                   daemon:heartbeat
───────────────                   daemon:register
activity:created
```

**Dispatch Logic:**
```go
// server/internal/handler/websocket.go (pseudocode)
func broadcastEvent(eventType, payload, actor) {
    for client := range workspace.subscribers {
        if client.canView(payload) {  // Auth check
            client.sendJSON({
                type: eventType,
                payload: payload,
                actor_id: actor.id,
                actor_type: actor.type
            })
        }
    }
}
```

---

## Key Design Patterns

### 1. Metadata as Write-Once-Per-Key
```
Agent task 1: PUT /metadata/pr_number {value: 1234}
Agent task 2: PUT /metadata/pipeline_status {value: "passed"}
              (concurrent, no race condition)

Not allowed:
  PUT /metadata {pr_number: 1234, status: "done"}
  → Would overwrite agent 2's update
```

### 2. Labels: Nil vs. Empty Array
```
Response 1: { id: "...", labels: null }      ← Field absent, preserve cache
Response 2: { id: "...", labels: [] }        ← Empty array, authoritative
Response 3: { id: "...", labels: [{...}] }   ← Refresh labels
```

### 3. Event Payload Consistency
```
All events carry:
- type: string (event constant)
- payload: object (event-specific data)
- actor_id?: string (who triggered it)
- actor_type?: string ("member" | "agent" | "system")

Frontend reducer:
  case "issue:updated": return { ...state, [payload.issue.id]: payload.issue }
  case "task:completed": return { ...state, taskState[payload.task_id]: "completed" }
```

---

## Performance Optimizations

1. **Partial Index on first_executed_at**
   - Only indexes rows where first_executed_at IS NOT NULL
   - Skips large tail of never-executed issues

2. **GIN Index on Metadata**
   - Enables fast `metadata @> {key: value}` containment checks
   - jsonb_path_ops variant (no string search, just keys)

3. **Composite Index on Task Queue**
   - `(runtime_id, priority DESC, created_at ASC)`
   - Daemon's claim query scans in single index pass

4. **Lazy Label Loading**
   - Labels bulk-loaded per issue batch
   - Omitted from batch updates (client cache preserved)

5. **Batch WebSocket Broadcasts**
   - Multiple events grouped when possible
   - Reduces JSON overhead on wire

---

## Summary

- **Issues** are assigned to agents, members, or squads
- **agent_task_queue** tracks work in progress
- **Metadata** stores agent state (atomic per-key updates)
- **WebSocket** broadcasts all state changes in real-time
- **Daemon** polls task queue, executes agents, streams results
- **Squad** enables team workflows with leader + members
- **Task lifecycle** has 6 states (queued → completed/failed/cancelled)
- **Retry logic** creates new task with parent_task_id + incremented attempt

