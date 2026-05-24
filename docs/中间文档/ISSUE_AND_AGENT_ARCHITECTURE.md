# Multica Issue and Agent Data Model & Architecture

## Executive Summary

Multica uses a task-driven architecture where **Issues** represent discrete units of work that can be assigned to **Agents** (autonomous runtimes). When an issue is assigned to an agent, an **AgentTaskQueue** entry is created, triggering real-time WebSocket events that wake the daemon and dispatch work. The system tracks issue lifecycle through status transitions, maintains agent health via heartbeats, and persists task execution state including tool calls, outputs, and token usage.

---

## 1. Database Schema & Models

### 1.1 Core Tables (Initial Migration: `001_init.up.sql`)

#### **issue** table
Key fields:
- `id UUID`: Primary key
- `workspace_id UUID`: Which workspace owns this issue
- `title TEXT`, `description TEXT`: Issue content
- `status TEXT`: One of `backlog|todo|in_progress|in_review|done|blocked|cancelled`
- `priority TEXT`: One of `urgent|high|medium|low|none`
- `assignee_type TEXT`: `member|agent|squad` (nullable)
- `assignee_id UUID`: Reference to user/agent/squad (nullable)
- `creator_type TEXT`: `member|agent` 
- `creator_id UUID`: Who created this issue
- `parent_issue_id UUID`: Hierarchical parent (nullable)
- `project_id UUID`: Project membership (nullable)
- `number INT`: Sequential per-workspace identifier (MUL-42)
- `start_date TIMESTAMPTZ`: When work should begin (nullable)
- `due_date TIMESTAMPTZ`: When work should complete (nullable)
- `metadata JSONB`: Flat KV map for agent pipeline state
- `position FLOAT`: For drag-drop ordering
- `created_at TIMESTAMPTZ`, `updated_at TIMESTAMPTZ`: Timestamps

**Indexes:**
```sql
CREATE INDEX idx_issue_workspace ON issue(workspace_id);
CREATE INDEX idx_issue_status ON issue(workspace_id, status);
CREATE INDEX idx_issue_assignee ON issue(assignee_type, assignee_id);
CREATE UNIQUE INDEX uq_issue_workspace_number ON issue(workspace_id, number);
```

**Key Evolution:**
- **Migration 020**: Added `issue.number` and `workspace.issue_prefix` for human-readable IDs
- **Migration 091**: Added `issue.start_date` for Gantt chart support

---

#### **agent** table
Key fields:
- `id UUID`: Primary key
- `workspace_id UUID`: Which workspace owns this agent
- `name TEXT`: Display name
- `runtime_id UUID`: References AgentRuntime (where it actually runs)
- `status TEXT`: `idle|working|blocked|error|offline`
- `runtime_mode TEXT`: `local|cloud`
- `visibility TEXT`: `workspace|private`
- `owner_id UUID`: Agent creator (nullable)
- `max_concurrent_tasks INT`: How many issues it can work on simultaneously
- `model TEXT`: LLM to use (claude-opus, etc.)
- `thinking_level TEXT`: Reasoning effort token (runtime-specific)
- `instructions TEXT`: System prompt for this agent
- `description TEXT`: What the agent does
- `custom_env JSONB`: Environment variables
- `custom_args JSONB`: CLI arguments
- `mcp_config JSONB`: Model context protocol configuration
- `archived_at TIMESTAMPTZ`, `archived_by UUID`: Soft-delete support

**Three status axes:**
1. **Agent Status** (`status`): `idle|working|blocked|error|offline` 
2. **Runtime Mode** (`runtime_mode`): `local` (desktop app) vs `cloud` (hosted)
3. **Visibility** (`visibility`): `workspace` (anyone can use) vs `private` (owner only)

---

#### **agent_task_queue** table
Key fields:
- `id UUID`: Task ID
- `agent_id UUID`: Which agent should execute this
- `issue_id UUID`: Which issue this is working on
- `status TEXT`: `queued|dispatched|running|completed|failed|cancelled`
- `priority INT`: Higher = more urgent
- `created_at TIMESTAMPTZ`: When task was enqueued
- `dispatched_at TIMESTAMPTZ`: When daemon claimed it
- `started_at TIMESTAMPTZ`: When execution began
- `completed_at TIMESTAMPTZ`: When execution finished
- `runtime_id UUID`: Which specific runtime executed it
- `session_id TEXT`: Daemon-local session identifier
- `work_dir TEXT`: Working directory for this task
- `context JSONB`: Snapshot of issue + environment
- `result JSONB`: Task output
- `error TEXT`: Error message if failed
- `failure_reason TEXT`: Coarse classifier (`agent_error|timeout|runtime_offline|etc`)
- `trigger_comment_id UUID`: If triggered by a comment mention
- `trigger_summary TEXT`: Snapshot of trigger (comment text, autopilot title)
- `chat_session_id UUID`: If spawned from chat
- `autopilot_run_id UUID`: If spawned by autopilot
- `parent_task_id UUID`: For auto-retry linking
- `attempt INT`: 1-based retry counter
- `max_attempts INT`: How many times to retry

**Status Machine:**
```
∅ → queued (created via API or auto-enqueue)
  → dispatched (daemon claims it)
  → running (daemon starts execution)
  → completed (success) or failed (error)
  
* → cancelled (user or system aborted)
```

**Indexes:**
```sql
CREATE INDEX idx_agent_task_queue_agent ON agent_task_queue(agent_id, status);
CREATE INDEX idx_agent_task_queue_pending 
    ON agent_task_queue(agent_id, priority DESC, created_at ASC)
    WHERE status IN ('queued', 'dispatched');
```

---

#### **comment** table
Each comment on an issue:
- `id UUID`: Primary key
- `issue_id UUID`: Which issue
- `content TEXT`: Comment text
- `author_type TEXT`: `member|agent`
- `author_id UUID`: Who wrote it
- `type TEXT`: `comment|status_change|progress_update|system`
- `parent_id UUID`: For threaded replies (nullable)
- `resolved_at TIMESTAMPTZ`: When thread was resolved (nullable)
- `resolved_by_type TEXT`: Who resolved it
- `resolved_by_id UUID`: ID of resolver

---

#### Supporting tables
- **issue_label**: Workspace-scoped labels
- **issue_to_label**: Many-to-many mapping
- **issue_dependency**: Issue relationships (blocks, blocked_by, related)
- **issue_reaction**: Emoji reactions on issues
- **activity_log**: Audit trail of all changes

---

### 1.2 Go Models (`server/pkg/db/generated/models.go`)

```go
type Issue struct {
    ID                 pgtype.UUID
    WorkspaceID        pgtype.UUID
    Title              string
    Description        pgtype.Text        // nullable
    Status             string
    Priority           string
    AssigneeType       pgtype.Text        // nullable
    AssigneeID         pgtype.UUID        // nullable
    CreatorType        string
    CreatorID          pgtype.UUID
    ParentIssueID      pgtype.UUID        // nullable
    Number             int32              // e.g., 42 for "MUL-42"
    ProjectID          pgtype.UUID        // nullable
    StartDate          pgtype.Timestamptz // nullable
    DueDate            pgtype.Timestamptz // nullable
    Metadata           []byte             // JSON KV
    CreatedAt          pgtype.Timestamptz
    UpdatedAt          pgtype.Timestamptz
}

type Agent struct {
    ID                 pgtype.UUID
    WorkspaceID        pgtype.UUID
    Name               string
    RuntimeID          pgtype.UUID
    Status             string             // idle|working|blocked|error|offline
    RuntimeMode        string             // local|cloud
    Visibility         string             // workspace|private
    OwnerID            pgtype.UUID        // nullable
    MaxConcurrentTasks int32
    Model              pgtype.Text
    ThinkingLevel      pgtype.Text
    Instructions       string
    Description        string
    CustomEnv          []byte             // JSON map
    CustomArgs         []byte             // JSON array
    ArchivedAt         pgtype.Timestamptz // nullable
    CreatedAt          pgtype.Timestamptz
    UpdatedAt          pgtype.Timestamptz
}

type AgentTaskQueue struct {
    ID               pgtype.UUID
    AgentID          pgtype.UUID
    IssueID          pgtype.UUID
    Status           string             // queued|dispatched|running|completed|failed|cancelled
    Priority         int32
    RuntimeID        pgtype.UUID
    SessionID        pgtype.Text        // daemon session
    WorkDir          pgtype.Text        // working directory
    Context          []byte             // JSON snapshot
    Result           []byte             // JSON output
    Error            pgtype.Text
    FailureReason    pgtype.Text
    TriggerCommentID pgtype.UUID        // nullable
    TriggerSummary   pgtype.Text        // snapshot of trigger
    ChatSessionID    pgtype.UUID        // nullable
    AutopilotRunID   pgtype.UUID        // nullable
    ParentTaskID     pgtype.UUID        // nullable, for retry chain
    Attempt          int32              // 1-based counter
    MaxAttempts      int32
    DispatchedAt     pgtype.Timestamptz // nullable
    StartedAt        pgtype.Timestamptz // nullable
    CompletedAt      pgtype.Timestamptz // nullable
    CreatedAt        pgtype.Timestamptz
}
```

---

## 2. Issue Lifecycle & Status Flow

### 2.1 Status Definition

```typescript
export type IssueStatus =
  | "backlog"      // Not yet started, parking lot
  | "todo"         // Ready to work
  | "in_progress"  // Being worked on
  | "in_review"    // Waiting for feedback
  | "done"         // Completed successfully
  | "blocked"      // Waiting on external dependency
  | "cancelled"    // Abandoned / not doing
```

### 2.2 Default Initial Status

**New issue creation**: Defaults to `"backlog"`

**Can be overridden at creation time** via request body

### 2.3 Status Transitions

**Manual transitions** (via PATCH `/api/issues/{id}`):
```go
if req.Status != nil {
    params.Status = pgtype.Text{String: *req.Status, Valid: true}
}
```
Any status can transition to any other status. No enforced state machine.

**Implicit transitions** (via task execution):
- **First execution** → `FirstExecutedAt` timestamp is set
- **Trivial output** ("done", "完成", "готово") → Auto-mark as `done`
- **Task failure** → Issue may be auto-marked `blocked`

### 2.4 Open vs Closed Issues

```sql
-- Open issues (work still possible)
WHERE status NOT IN ('done', 'cancelled')

-- Closed/Complete issues
WHERE status IN ('done', 'cancelled')
```

---

## 3. Agent-to-Issue Assignment Model

### 3.1 Assignment Types

An issue's `assignee_type` + `assignee_id` pair identifies who will work on it:

```typescript
export type IssueAssigneeType = "member" | "agent" | "squad"
```

**Three kinds:**
- **member**: Human user (no auto-execution, just notification)
- **agent**: Autonomous LLM agent (auto-executes, enqueued task)
- **squad**: Group of agents led by one leader (squad_leader executes)

### 3.2 Assignment Validation

From `handler/issue.go` (lines 2161-2168):

When `assignee_type` or `assignee_id` is changed, the handler validates:

```go
if touchedType || touchedID {
    if status, msg := h.validateAssigneePair(
        r.Context(), r, workspaceID,
        params.AssigneeType, params.AssigneeID
    ); status != 0 {
        writeError(w, status, msg)
        return
    }
}
```

**Validation checks:**
- Agent exists in workspace
- Agent not archived
- Agent visibility permits assignee (private agents: owner + admins only)
- Squad exists and is valid

### 3.3 Task Enqueuing on Agent Assignment

From `handler/issue.go` (lines 2227-2240):

```go
// When assignee changes:
if assigneeChanged {
    // 1. Cancel any existing running tasks
    h.TaskService.CancelTasksForIssue(r.Context(), issue.ID)
    
    // 2. Enqueue new task if conditions met
    if h.shouldEnqueueAgentTask(r.Context(), issue) {
        h.TaskService.EnqueueTaskForIssue(r.Context(), issue)
    }
    
    // 3. For squads, trigger squad_leader
    if h.shouldEnqueueSquadLeaderOnAssign(r.Context(), issue) {
        h.enqueueSquadLeaderTask(r.Context(), issue, ...)
    }
}
```

**Task Enqueue Preconditions:**
- `assignee_type` is `"agent"`
- Issue status is NOT `"backlog"` (parking lot rule)
- No existing task in flight for this issue

This prevents:
- Duplicate task execution
- Auto-executing backlog items (they stay parked until explicitly moved)
- Orphaned tasks hanging around

---

## 4. Real-Time WebSocket Events

### 4.1 Event Types (`server/pkg/protocol/events.go`)

#### Issue Events
```
issue:created       – New issue added
issue:updated       – Issue fields changed
issue:deleted       – Issue removed
issue_metadata:changed – Metadata KV changed
issue_reaction:added   – Emoji reaction added
issue_reaction:removed – Emoji reaction removed
issue_labels:changed   – Labels attached/detached
```

#### Task Lifecycle Events
```
task:queued       ∅ → queued (enqueue/retry)
task:dispatch     queued → dispatched (daemon claims)
task:progress     In-flight execution update
task:message      Tool call, output, etc.
task:completed    running → completed (success)
task:failed       running → failed (error)
task:cancelled    * → cancelled (abort)
```

#### Agent Events
```
agent:status      Connection state changed
agent:created     New agent added
agent:archived    Agent soft-deleted
agent:restored    Agent un-archived
```

#### Comment Events
```
comment:created       New comment
comment:updated       Comment edited
comment:deleted       Comment removed
comment:resolved      Thread marked resolved
```

#### Other Events
```
activity:created          Audit log entry
daemon:heartbeat          Daemon → Server (keep-alive)
daemon:heartbeat_ack      Server → Daemon (ack + pending actions)
workspace:updated         Workspace fields changed
member:added/removed      Membership changed
```

### 4.2 Event Payload Examples

#### IssueUpdated Payload (handler/issue.go:2206-2225)
```typescript
interface IssueUpdatedPayload {
    issue: IssueResponse;              // Full new state
    assignee_changed: boolean;
    status_changed: boolean;
    priority_changed: boolean;
    start_date_changed: boolean;
    due_date_changed: boolean;
    description_changed: boolean;
    title_changed: boolean;
    
    prev_title: string;
    prev_assignee_type?: string;
    prev_assignee_id?: string;
    prev_status: string;
    prev_priority: string;
    prev_start_date?: string;
    prev_due_date?: string;
    
    creator_type: string;
    creator_id: string;
}
```

#### TaskProgress Payload (protocol/messages.go)
```typescript
interface TaskProgressPayload {
    task_id: string;
    summary: string;          // "Running linter on src/"
    step?: number;
    total?: number;
}
```

#### TaskMessage Payload (protocol/messages.go)
```typescript
interface TaskMessagePayload {
    task_id: string;
    seq: number;
    type: "text" | "tool_use" | "tool_result" | "error";
    tool?: string;            // Tool name for tool_use/tool_result
    content?: string;         // Text output
    input?: Record<string, any>;   // Tool input
    output?: string;          // Tool output
}
```

#### DaemonHeartbeat (protocol/messages.go)
```typescript
// Daemon → Server (periodic, ~30s)
interface DaemonHeartbeatRequestPayload {
    runtime_id: string;
    supports_batch_import?: boolean;
}

// Server → Daemon (response with pending work)
interface DaemonHeartbeatAckPayload {
    runtime_id: string;
    status: string;                           // "ok" or "runtime_gone"
    runtime_gone?: boolean;                   // Runtime was deleted
    pending_update?: { id: string; target_version: string };
    pending_model_list?: { id: string };
    pending_local_skills?: { id: string };
    pending_local_skill_imports?: Array<{
        id: string;
        skill_key: string;
    }>;
}
```

### 4.3 Broadcasting Scope

**Workspace-wide** (all members see):
- Issue events (created/updated/deleted)
- Task lifecycle events
- Agent status changes
- Comments & reactions

**User-private** (specific user only):
- Inbox items
- Personal notifications

**Daemon-private** (specific runtime):
- Task dispatch
- Heartbeat ack

---

## 5. Frontend Types and State Management

### 5.1 Issue Type (`packages/core/types/issue.ts`)

```typescript
export interface Issue {
    id: string;
    workspace_id: string;
    number: number;                // e.g., 42
    identifier: string;            // e.g., "MUL-42"
    title: string;
    description: string | null;
    status: IssueStatus;
    priority: IssuePriority;
    assignee_type: IssueAssigneeType | null;
    assignee_id: string | null;
    creator_type: IssueAssigneeType;
    creator_id: string;
    parent_issue_id: string | null;
    project_id: string | null;
    position: number;              // Ordering
    start_date: string | null;     // ISO timestamp
    due_date: string | null;       // ISO timestamp
    
    metadata: IssueMetadata;       // Flat KV, always present
    reactions?: IssueReaction[];
    labels?: Label[];
    
    created_at: string;
    updated_at: string;
}

export type IssueMetadata = Record<string, IssueMetadataValue>;
export type IssueMetadataValue = string | number | boolean;
```

**Key Design Choices:**
- `metadata` always present (empty `{}` when unset) so code can safely read without nil-guarding
- `identifier` is computed client-side from workspace prefix + number
- Labels are bulk-loaded, `null` pointer means "field absent; don't touch"

### 5.2 Agent Type (`packages/core/types/agent.ts`)

```typescript
export interface Agent {
    id: string;
    workspace_id: string;
    runtime_id: string;
    name: string;
    description: string;
    instructions: string;
    avatar_url: string | null;
    runtime_mode: AgentRuntimeMode;    // local|cloud
    runtime_config: Record<string, unknown>;
    custom_env: Record<string, string>;
    custom_args: string[];
    visibility: AgentVisibility;       // workspace|private
    status: AgentStatus;               // idle|working|blocked|error|offline
    max_concurrent_tasks: number;
    model: string;
    thinking_level?: string;           // empty "" = no override
    skills: AgentSkillSummary[];
    created_at: string;
    updated_at: string;
    archived_at: string | null;
    archived_by: string | null;
}

export interface AgentTask {
    id: string;
    agent_id: string;
    runtime_id: string;
    issue_id: string;                  // empty "" when no linked issue
    status: TaskStatus;                // queued|dispatched|running|completed|failed|cancelled
    priority: number;
    dispatched_at: string | null;
    started_at: string | null;
    completed_at: string | null;
    result: unknown;
    error: string | null;
    failure_reason?: TaskFailureReason;
    created_at: string;
    
    chat_session_id?: string;          // If chat-spawned
    autopilot_run_id?: string;         // If autopilot-spawned
    parent_task_id?: string;           // For retries
    attempt?: number;                  // 1-based
    trigger_comment_id?: string;
    trigger_summary?: string;          // Snapshot at creation
    kind?: "comment" | "autopilot" | "chat" | "quick_create" | "direct";
    work_dir?: string;                 // Daemon-assigned
}

export type TaskFailureReason =
    | "agent_error"
    | "timeout"
    | "codex_semantic_inactivity"
    | "runtime_offline"
    | "runtime_recovery"
    | "manual";
```

### 5.3 Frontend State Management

From `packages/core/issues/stores/`:

- **RecentIssuesStore**: Caches issues by last-viewed order
- **MyIssuesViewStore**: Filters by assignee == current user
- **ActorIssuesViewStore**: Filters by creator == specific actor
- **IssuesScopeStore**: Workspace-level issue list
- **DraftStore**: Local unsaved edits
- **SelectionStore**: Multi-select UI state

**Update Pattern:**
1. WebSocket receives `issue:updated`
2. Handler merges delta into local cache
3. UI re-renders only affected components (no full page refresh)

---

## 6. Task Execution Context & Metadata

### 6.1 Issue Metadata (Pipeline State KV Store)

**Purpose**: Agents record pipeline state without modifying core issue fields

**Example**:
```json
{
    "pr_number": "12345",
    "pr_url": "https://github.com/org/repo/pull/12345",
    "pipeline_status": "passed",
    "waiting_on": "code_review",
    "ci_run_id": "run-789",
    "commit_sha": "abc123"
}
```

**Constraints:**
- Flat structure only (no nested objects)
- Primitive values only (string | number | boolean)
- Always present (empty `{}` when unset)

**Update via API:**
```
PATCH /api/issues/{id}/metadata
{
    "key": "pr_number",
    "value": "12345"  // or number or bool
}
```

**Event on change:**
```typescript
Message {
    type: "issue_metadata:changed",
    payload: {
        issue_id: string,
        key: string,
        value: string | number | boolean | null,  // null = delete key
        previous_value?: any
    }
}
```

### 6.2 Task Context (Execution Snapshot)

From `migrations/003_task_context.up.sql`:
```sql
ALTER TABLE agent_task_queue ADD COLUMN context JSONB;
```

**Captured at task creation**, contains:
- Issue snapshot (title, description, current metadata)
- Agent instructions & model
- Workspace context
- Environment variables
- Caller-provided hints

**Rationale**: Daemon doesn't fetch issue state on-demand; everything it needs is in the task row.

---

## 7. Comment-Triggered Tasks

### 7.1 Flow: User Comments with Mention

```
User: "@agent-codex please fix the validation bug"
    ↓
CreateComment handler
    ├─ Parse mention (@agent-codex)
    ├─ Resolve to agent ID
    ├─ Insert comment into DB
    ├─ Update issue.assignee_type = 'agent', issue.assignee_id = AGENT_ID
    ├─ TaskService.EnqueueTaskForIssue()
    │  ├─ Create agent_task_queue row
    │  ├─ Set trigger_comment_id = comment.id
    │  ├─ Set trigger_summary = truncate(comment.content, 200)
    │  └─ Notify daemon
    ├─ Publish comment:created event
    └─ Publish issue:updated event (assignee changed)
        ↓
All clients (Web + Daemon)
    ├─ See new comment
    ├─ See issue now assigned to @agent-codex
    └─ Daemon woken up, starts task
```

**Task Attributes Set:**
- `trigger_comment_id`: Link back to comment
- `trigger_summary`: Snapshot of comment text (persists if comment is deleted)
- `kind`: "comment" (vs "autopilot", "chat", "quick_create")

---

## 8. API Handler Patterns

### 8.1 Issue CRUD

#### Create
```
POST /api/issues
Request: {title, description, assignee_type?, assignee_id?, status?, ...}
Response: IssueResponse
Side effects:
  - If assigned to agent & not backlog → enqueue task
  - Broadcast issue:created
```

#### Read
```
GET /api/issues/:id
Response: IssueResponse with labels + attachments
```

#### List
```
GET /api/issues?status=...&assignee_id=...&priority=...&metadata=...
Supports filtering by: status, priority, assignee, creator, project, scheduled, metadata
Response: IssueResponse[] (with bulk-loaded labels)
```

#### Update
```
PATCH /api/issues/:id
Request: Partial UpdateIssueRequest

Key logic:
  1. Detect which fields changed (JSON presence detection)
  2. Validate assignee_type + assignee_id pair
  3. If assignee changed:
     - Cancel existing tasks
     - Enqueue new task if appropriate
  4. Broadcast issue:updated with delta info
```

#### Batch Update
```
PATCH /api/issues?ids=...
Request: {updates: UpdateIssueRequest}
Applies same update to multiple issues atomically
```

#### Delete
```
DELETE /api/issues/:id
Broadcast issue:deleted
```

### 8.2 Key Query Patterns

```sql
-- Open issues (work ongoing)
WHERE status NOT IN ('done', 'cancelled')

-- Child issue progress
SELECT parent_issue_id,
       COUNT(*) AS total,
       COUNT(*) FILTER (WHERE status IN ('done', 'cancelled')) AS completed
FROM issue
WHERE workspace_id = $1 AND parent_issue_id IS NOT NULL
GROUP BY parent_issue_id

-- Flexible multi-filter
WHERE workspace_id = $1
  AND ($2::text IS NULL OR status = $2)
  AND ($3::text IS NULL OR priority = $3)
  AND ($4::uuid IS NULL OR assignee_id = $4)
  AND ($5::jsonb IS NULL OR metadata @> $5::jsonb)

-- Task queue for agent
SELECT * FROM agent_task_queue
WHERE agent_id = $1
  AND status IN ('queued', 'dispatched', 'running')
ORDER BY priority DESC, created_at ASC
```

---

## 9. Daemon Interaction

### 9.1 Heartbeat Loop

**Daemon sends every ~30 seconds:**
```
WebSocket Message {
    type: "daemon:heartbeat",
    payload: {
        runtime_id: UUID,
        supports_batch_import: bool
    }
}
```

**Server responds:**
```
WebSocket Message {
    type: "daemon:heartbeat_ack",
    payload: {
        runtime_id: UUID,
        status: "ok" | "runtime_gone",
        pending_update?: {...},
        pending_model_list?: {...},
        pending_local_skills?: {...}
    }
}
```

**Stale Detection**: No heartbeat for 2+ minutes → agent status = `offline`

### 9.2 Task Claim & Dispatch

1. **Daemon polls:** `POST /api/tasks/claim?runtime_id=...`
   - Server: "Do you have any queued tasks for me?"
   - Server returns: `AgentTaskQueue + context`

2. **Server wakes daemon:** `WebSocket Message { type: "task:dispatch", payload: {...} }`
   - Proactive notification that a task is ready

3. **Daemon executes:**
   - Streams progress: `task:progress`, `task:message`
   - Calls tools, gets outputs

4. **Daemon completes:**
   - `POST /api/tasks/{id}/complete` or `HTTP POST /api/tasks/{id}/fail`
   - Server broadcasts: `task:completed` or `task:failed`

---

## 10. Agent Status & Health

### 10.1 Status Levels

```typescript
export type AgentStatus = "idle" | "working" | "blocked" | "error" | "offline";
```

**Transitions:**
- `offline` ← No heartbeat for 2+ min, daemon crash/disconnect
- `idle` ← Daemon connected, no tasks running
- `working` ← Task dispatched and execution started
- `blocked` ← Task failed, awaiting manual intervention
- `error` ← Runtime error (API down, permission issue, etc.)

### 10.2 Max Concurrent Tasks

Agent has `max_concurrent_tasks` limit:
- Defaults to 1 (sequential execution)
- Can be set higher for parallelizable work
- Enforced by daemon: won't claim more than this many queued tasks

---

## 11. Key Design Patterns

### 11.1 Conditional Task Enqueuing

```go
func (h *Handler) shouldEnqueueAgentTask(ctx context.Context, issue *db.Issue) bool {
    // Only enqueue if:
    // 1. Assignee is an agent
    if issue.AssigneeType.String != "agent" {
        return false
    }
    // 2. Not in backlog (parking lot rule)
    if issue.Status == "backlog" {
        return false
    }
    // 3. No existing running task
    return !hasRunningTask(ctx, issue.ID)
}
```

### 11.2 Delta Broadcasting

Instead of full object on every change:
```typescript
interface IssueUpdatedPayload {
    issue: {...},              // Full new state
    assignee_changed: boolean,
    status_changed: boolean,
    prev_assignee_id?: string,
    prev_status: string,
    ...
}
```

**Frontend benefit**: Optimistic updates stay in place until server confirms; merge delta without flickering.

### 11.3 Nullable Field Handling

```go
// Detect which fields were explicitly present in JSON
var rawFields map[string]json.RawMessage
json.Unmarshal(bodyBytes, &rawFields)

// Only override nullable fields if explicitly provided
if _, ok := rawFields["assignee_type"]; ok {
    if req.AssigneeType != nil {
        params.AssigneeType = pgtype.Text{String: *req.AssigneeType, Valid: true}
    } else {
        params.AssigneeType = pgtype.Text{Valid: false}  // Explicit null = unassign
    }
}
```

**Benefit**: Distinguish "not provided" (leave alone) from "provided as null" (clear field).

---

## 12. Issue Metadata as Pipeline State

**Workflow:**

```
1. Create issue: metadata = {}
2. Agent runs, discovers PR from GitHub
   PATCH /api/issues/{id}/metadata
   {key: "pr_number", value: "12345"}
3. All clients see metadata.pr_number = "12345"
4. Later: agent updates CI status
   PATCH /api/issues/{id}/metadata
   {key: "pipeline_status", value: "passed"}
5. Clients now see both fields
```

**Why this design:**
- Agents don't modify issue title/description (those are human-writable)
- Metadata is a structured dump-zone for agent state
- Frontend can display metadata without special handling
- Simple to search/filter issues by pipeline state

---

## 13. Key Files Reference

### Backend
- `server/migrations/001_init.up.sql` – Initial schema
- `server/migrations/020_issue_number.up.sql` – Human-readable IDs
- `server/migrations/091_issue_start_date.up.sql` – Gantt support
- `server/pkg/db/generated/models.go` – Go models
- `server/pkg/db/queries/issue.sql` – SQL queries
- `server/internal/handler/issue.go` – Issue API handlers
- `server/internal/service/task.go` – Task management logic
- `server/pkg/protocol/events.go` – WebSocket event types
- `server/pkg/protocol/messages.go` – Message payloads
- `server/internal/realtime/hub.go` – Broadcast hub

### Frontend
- `packages/core/types/issue.ts` – Issue TypeScript type
- `packages/core/types/agent.ts` – Agent TypeScript type
- `packages/core/issues/stores/` – State management
- `e2e/issues.spec.ts` – End-to-end tests

