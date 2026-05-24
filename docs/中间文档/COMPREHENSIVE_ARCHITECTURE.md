# Multica: Comprehensive System Architecture Guide

**Last Updated:** May 24, 2026  
**Scope:** Issue lifecycle, Agent execution model, WebSocket real-time events, Daemon architecture  
**Audience:** Backend engineers, system architects, new team members

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Issue Data Model](#issue-data-model)
3. [Agent Architecture](#agent-architecture)
4. [Task Execution Model](#task-execution-model)
5. [Daemon & Runtime](#daemon--runtime)
6. [Real-time Events (WebSocket)](#real-time-events-websocket)
7. [Communication Flows](#communication-flows)
8. [Key Design Decisions](#key-design-decisions)

---

## Executive Summary

Multica is a **task-driven agent execution platform** with the following core concepts:

| Concept | Definition | Purpose |
|---------|-----------|---------|
| **Issue** | Discrete unit of work with status, priority, assignee | Core work item users interact with |
| **Agent** | Autonomous runtime (local or cloud) running AI models | Executes tasks on behalf of users |
| **AgentTaskQueue** | Queue entry linking issue to agent execution | Persistence + status tracking for work |
| **Daemon** | Local process on user's machine | Polls server, claims tasks, executes agents |
| **Runtime** | Physical/virtual execution environment | Hosts one or more agents; heartbeat-monitored |

**Architecture Type:** Server-driven polling with push notifications  
**Communication:** REST API (task ops) + WebSocket (real-time events) + HTTP heartbeats (liveness)  
**Execution Model:** Async task queue with per-agent concurrency limits  
**State Storage:** PostgreSQL (normalized schema) + JSONB (flexible metadata)

---

## Issue Data Model

### 1. Issue Table Schema

**Primary Location:** `server/migrations/001_init.up.sql`

```sql
CREATE TABLE issue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'backlog'
        CHECK (status IN ('backlog', 'todo', 'in_progress', 'in_review', 'done', 'blocked', 'cancelled')),
    priority TEXT NOT NULL DEFAULT 'none'
        CHECK (priority IN ('urgent', 'high', 'medium', 'low', 'none')),
    assignee_type TEXT CHECK (assignee_type IN ('member', 'agent', 'squad')),
    assignee_id UUID,
    creator_type TEXT NOT NULL CHECK (creator_type IN ('member', 'agent')),
    creator_id UUID NOT NULL,
    parent_issue_id UUID REFERENCES issue(id),
    project_id UUID,
    number INT UNIQUE PER workspace,
    start_date TIMESTAMPTZ,
    due_date TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}',
    position FLOAT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- Critical indexes for query performance
CREATE INDEX idx_issue_workspace ON issue(workspace_id);
CREATE INDEX idx_issue_status ON issue(workspace_id, status);
CREATE INDEX idx_issue_assignee ON issue(assignee_type, assignee_id);
CREATE UNIQUE INDEX uq_issue_workspace_number ON issue(workspace_id, number);
```

### 2. Issue Identifier System

**Human-Readable ID Format:** `{workspace.issue_prefix}-{issue.number}`
- Example: `MUL-42` (workspace prefix "MUL", issue number 42)
- **Migration 020** (`020_issue_number.up.sql`):
  - Added `issue.number` INT column (per-workspace sequential)
  - Added `workspace.issue_prefix` TEXT column
  - Ensures unique constraint: `UNIQUE(workspace_id, number)`

**API Response Example:**
```json
{
  "id": "uuid-123",
  "number": 42,
  "identifier": "MUL-42",
  "title": "Implement feature X",
  "status": "in_progress"
}
```

### 3. Status Lifecycle

```
                    ┌─────────────┐
                    │   backlog   │  (default on creation)
                    └──────┬──────┘
                           │
                           v
                    ┌─────────────┐
                    │    todo     │
                    └──────┬──────┘
                           │
                           v
                    ┌─────────────────┐
                    │   in_progress   │  (implicit on agent execution start)
                    └──────┬──────────┘
                           │
              ┌────────────┴────────────┐
              v                         v
        ┌──────────┐          ┌──────────────┐
        │   done   │          │   in_review  │
        └──────────┘          └──────┬───────┘
                                     v
                              ┌─────────────┐
                              │    done     │
                              └─────────────┘

Blocked/Cancelled state can be reached from any state
```

**Transitions:**
- **backlog → todo:** Manual user action or agent auto-transition on first execution
- **todo → in_progress:** Agent starts execution
- **in_progress → in_review:** Custom workflow (e.g., PR review)
- **in_review/in_progress → done:** Task completion
- **any → blocked:** Dependency failure
- **any → cancelled:** User abort or auto-retry exhaustion

### 4. Assignee Model: Member, Agent, Squad

| Type | Stored In | Execution | Access |
|------|-----------|-----------|--------|
| **member** | `user` table | Manual work by human | Workspace users |
| **agent** | `agent` table | Automatic execution via daemon | Owner or workspace-wide |
| **squad** | `squad` table (led by 1 agent) | Coordinated multi-agent | Workspace admins |

```go
// From packages/core/types/agent.ts
type IssueAssigneeType = 'member' | 'agent' | 'squad';

// Frontend validation
if (issue.assignee_type === 'agent') {
    // Enqueue task, wake daemon
} else if (issue.assignee_type === 'member') {
    // Notify human, wait for manual action
} else if (issue.assignee_type === 'squad') {
    // Coordinate squad lead + members
}
```

### 5. Metadata: Flexible Pipeline State

**Design:** Flat JSONB KV map for agent pipeline tracking without schema changes

```json
{
  "pr_number": "1234",
  "pipeline_status": "tests_running",
  "ci_url": "https://github.com/...",
  "error_count": 2,
  "last_attempt_time": "2026-05-24T10:30:00Z"
}
```

**Key Properties:**
- **Always present:** `issue.metadata` is never null (defaults to `{}`)
- **Schema-less:** No migration needed to add new KV pairs
- **Agent-writable:** Agents can record pipeline state during execution
- **Full update:** Metadata updates replace the entire object (not JSON merge)

---

## Agent Architecture

### 1. Agent Table Schema

```sql
CREATE TABLE agent (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL,
    name TEXT NOT NULL UNIQUE PER workspace,
    description TEXT,
    instructions TEXT,  -- System prompt
    avatar_url TEXT,
    runtime_id UUID REFERENCES agent_runtime(id),
    status TEXT DEFAULT 'idle'
        CHECK (status IN ('idle', 'working', 'blocked', 'error', 'offline')),
    runtime_mode TEXT DEFAULT 'local'
        CHECK (runtime_mode IN ('local', 'cloud')),
    visibility TEXT DEFAULT 'private'
        CHECK (visibility IN ('workspace', 'private')),
    owner_id UUID,
    max_concurrent_tasks INT DEFAULT 1,
    model TEXT NOT NULL,  -- e.g., 'claude-opus'
    thinking_level TEXT DEFAULT '',
    custom_env JSONB,  -- Environment variables
    custom_args JSONB,  -- CLI arguments
    mcp_config JSONB,  -- Model context protocol config
    archived_at TIMESTAMPTZ,
    archived_by UUID,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
```

### 2. Agent Status: Three Orthogonal Axes

```
┌─────────────────────────────────────────────────────────────┐
│ Agent Status has THREE independent dimensions              │
├─────────────────────────────────────────────────────────────┤
│ 1. OPERATIONAL STATUS (status field)                        │
│    ├─ idle         : Waiting for work                       │
│    ├─ working      : Actively executing task                │
│    ├─ blocked      : Waiting for external event             │
│    ├─ error        : Permanent failure state                │
│    └─ offline      : Runtime unreachable (30s heartbeat)    │
│                                                              │
│ 2. RUNTIME MODE (runtime_mode field)                        │
│    ├─ local        : Desktop app on user's machine          │
│    └─ cloud        : Hosted service (managed by Multica)    │
│                                                              │
│ 3. VISIBILITY (visibility field)                            │
│    ├─ workspace    : Anyone in workspace can use            │
│    └─ private      : Only owner can use                     │
└─────────────────────────────────────────────────────────────┘
```

**Example Combinations:**
- `status=working, runtime_mode=local, visibility=workspace` → Local agent actively executing a workspace task
- `status=offline, runtime_mode=local, visibility=private` → User's private desktop agent unreachable

### 3. Agent Runtime Tracking

Separate **agent_runtime** table tracks physical execution environments:

```sql
CREATE TABLE agent_runtime (
    id UUID PRIMARY KEY,
    daemon_id TEXT NOT NULL,  -- Unique daemon process ID
    workspace_id UUID NOT NULL,
    type TEXT,  -- 'claude-agent', 'claude-mcp', etc.
    version TEXT,  -- CLI version
    status TEXT,  -- 'online', 'offline'
    last_seen_at TIMESTAMPTZ,  -- Heartbeat tracking
    timezone TEXT,  -- For local scheduling
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now()
);
```

**Key Insight:** Runtime ≠ Agent
- One **daemon process** can have multiple **runtimes** (for different Python versions, etc.)
- One **runtime** can host multiple **agents** (different configs/models)
- **Heartbeat mechanism:** Runtime sends `POST /api/daemon/heartbeat` every ~30s; server responds with pending actions

---

## Task Execution Model

### 1. AgentTaskQueue Table

```sql
CREATE TABLE agent_task_queue (
    id UUID PRIMARY KEY,
    agent_id UUID NOT NULL REFERENCES agent(id),
    issue_id UUID NOT NULL REFERENCES issue(id),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'dispatched', 'running', 'completed', 'failed', 'cancelled')),
    priority INT NOT NULL DEFAULT 0,  -- Higher = more urgent
    runtime_id UUID REFERENCES agent_runtime(id),
    session_id TEXT,  -- Daemon-local session identifier
    work_dir TEXT,  -- Working directory for execution
    context JSONB NOT NULL,  -- Snapshot of execution environment
    result JSONB,  -- Task output
    error TEXT,  -- Error message if failed
    failure_reason TEXT,  -- Classifier: agent_error|timeout|runtime_offline
    
    -- Trigger metadata
    trigger_comment_id UUID,  -- If @mentioned in comment
    trigger_summary TEXT,  -- Snapshot of trigger
    
    -- Relations
    chat_session_id UUID,  -- If spawned from chat
    autopilot_run_id UUID,  -- If spawned by autopilot
    parent_task_id UUID,  -- For retry chains
    
    -- Retry tracking
    attempt INT DEFAULT 1,  -- 1-based attempt number
    max_attempts INT DEFAULT 1,
    
    -- Timestamps
    dispatched_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_agent_task_queue_agent ON agent_task_queue(agent_id);
CREATE INDEX idx_agent_task_queue_issue ON agent_task_queue(issue_id);
CREATE INDEX idx_agent_task_queue_status ON agent_task_queue(status);
CREATE INDEX idx_agent_task_queue_queued ON agent_task_queue(status) 
    WHERE status = 'queued';  -- Fast "any work available?" scan
```

### 2. Task Status State Machine

```
CREATION (server-side)
    │
    v
┌──────────┐
│ queued   │  Task added to queue, waiting for daemon claim
└────┬─────┘
     │
     │ [Daemon polls: GET /api/tasks/claim]
     v
┌──────────┐
│dispatched│  Daemon claimed task, about to start execution
└────┬─────┘
     │
     │ [Daemon starts agent process]
     v
┌──────────┐
│ running  │  Agent executing (may emit task:progress events)
└────┬─────┘
     │
     ├─────────────────────┬──────────────────────┐
     │                     │                      │
     v                     v                      v
┌───────────┐      ┌──────────┐           ┌───────────┐
│ completed │      │ failed   │           │ cancelled │
└───────────┘      └──────────┘           └───────────┘
     │                   │                      │
     └───────────┬───────┴──────────────────────┘
                 │
                 v
         [FINAL STATE]
```

**Transitions:**
- **queued → dispatched:** Daemon `ClaimTask()` succeeds
- **dispatched → running:** Agent process started
- **running → completed:** `POST /api/tasks/complete` with result
- **running → failed:** `POST /api/tasks/fail` with error
- **any → cancelled:** User abort or manual intervention

### 3. Context Snapshot: Fixed Execution Environment

At task creation, the entire execution context is captured:

```json
{
  "issue": {
    "id": "issue-uuid",
    "title": "Fix authentication bug",
    "description": "OAuth token refresh broken",
    "metadata": { ... }
  },
  "workspace": {
    "id": "workspace-uuid",
    "name": "Acme Corp",
    "settings": { ... }
  },
  "github_token": "ghp_...",
  "repos": [
    {
      "url": "https://github.com/acme/api",
      "local_path": "/tmp/worktree/repo-123",
      "branch": "main"
    }
  ],
  "environment": {
    "PATH": "...",
    "PYTHONPATH": "..."
  }
}
```

**Design Rationale:** Daemon doesn't fetch state on-demand → No race conditions between issue updates and task execution

---

## Daemon & Runtime

### 1. Daemon Architecture

**Location:** `server/internal/daemon/daemon.go` (118KB, core logic)

```go
type Daemon struct {
    cfg           Config
    client        *Client          // HTTP/WS to server
    repoCache     repoCacheBackend // Manages worktrees
    logger        *slog.Logger
    
    // Workspace state tracking
    workspaces    map[string]*workspaceState
    runtimeIndex  map[string]Runtime
    
    // Runtime lifecycle
    wsHBLastAck   map[string]time.Time  // Last successful heartbeat
    runtimeGone   map[string]struct{}   // Recovery tracking
    
    // Concurrency
    activeTasks   atomic.Int64
    pauseClaims   bool  // Auto-update barrier
    claimsInFlight int
}
```

### 2. Daemon Task Loop

```
┌─────────────────────────────────────────┐
│ Daemon.Run() (main event loop)          │
├─────────────────────────────────────────┤
│                                         │
│ 1. Connect to WebSocket                 │
│    → Subscribe: task_available,         │
│      runtime_gone, etc.                 │
│                                         │
│ 2. Register runtimes with server        │
│    → POST /api/daemons/register         │
│    → Send runtime capabilities          │
│                                         │
│ 3. Poll for tasks (per runtime)         │
│    → Goroutine per runtime              │
│    → GET /api/tasks/claim               │
│    → Pass to handleTask()               │
│                                         │
│ 4. Run task (blocking)                  │
│    → Spawn agent process                │
│    → Stream output to WebSocket         │
│    → POST /api/tasks/complete on done   │
│                                         │
│ 5. Send heartbeats (~15-30s)            │
│    → HTTP: POST /api/daemon/heartbeat   │
│    → WS: Send heartbeat_request         │
│    → Receive pending_update, etc.       │
│                                         │
│ 6. Handle auto-updates                  │
│    → Pause claims during upgrade        │
│    → Run brew/download upgrade          │
│    → Spawn new binary, exit             │
│                                         │
└─────────────────────────────────────────┘
```

### 3. Heartbeat Protocol

**HTTP Heartbeat** (`POST /api/daemon/heartbeat`):
```go
type DaemonHeartbeatRequestPayload struct {
    RuntimeID           string
    SupportsBatchImport bool
}

type DaemonHeartbeatAckPayload struct {
    RuntimeID           string
    Status              string  // "ok" or "runtime_gone"
    RuntimeGone         bool    // true = runtime deleted server-side
    PendingUpdate       *Update
    PendingModelList    *ModelListRequest
    PendingLocalSkills  *LocalSkillsRequest
    PendingLocalSkillImports []LocalSkillImport
}
```

**Frequency:** ~30 seconds (configurable)  
**Purpose:** 
- Liveness detection (server knows daemon is alive)
- Pull pending actions (updates, model lists, skill imports)
- Communicate task completion status

**WebSocket Heartbeat** (mirrors HTTP but bi-directional):
- Server → Daemon: `daemon:heartbeat_ack` with pending actions
- Daemon → Server: `daemon:heartbeat_request` (optional, for real-time pull)

### 4. Runtime Gone Recovery

When a runtime is deleted server-side (UI delete, 7-day offline GC):

```
1. Heartbeat/poller detects 404 "runtime not found"
   │
2. handleRuntimeGone() coalesces concurrent recovery attempts
   ├─ Per-runtimeID stampede control
   ├─ Per-workspaceID coalesce window (30s)
   │
3. Remove stale runtime from daemon's local state
   │
4. Re-register remaining runtimes
   └─ Single registerRuntimesForWorkspace() call
```

**Design:** Prevents cascade of duplicate registration calls when one delete clears all workspace runtimes

---

## Real-time Events (WebSocket)

### 1. Event Types

**Server → Clients** (WebSocket broadcasts):

| Category | Events | Purpose |
|----------|--------|---------|
| **Issue** | `issue:created`, `issue:updated`, `issue:deleted`, `issue_metadata:changed` | Issue lifecycle |
| **Comment** | `comment:created`, `comment:updated`, `comment:deleted`, `comment:resolved` | Discussion threads |
| **Reactions** | `reaction:added`, `reaction:removed`, `issue_reaction:added` | Emoji/sentiment |
| **Task** | `task:queued`, `task:dispatch`, `task:progress`, `task:completed`, `task:failed`, `task:message`, `task:cancelled` | Execution tracking |
| **Agent** | `agent:status`, `agent:created`, `agent:archived`, `agent:restored` | Agent lifecycle |
| **Daemon** | `daemon:heartbeat`, `daemon:heartbeat_ack`, `daemon:register`, `daemon:task_available` | Runtime management |
| **Chat** | `chat:message`, `chat:done`, `chat:session_read`, `chat:session_deleted`, `chat:session_updated` | Conversation |
| **Workspace** | `workspace:updated`, `workspace:deleted` | Org-level changes |
| **Integration** | `github_installation:created`, `pull_request:linked`, `pull_request:updated` | External integrations |

**Daemon → Server** (WebSocket):

| Event | Payload | Purpose |
|-------|---------|---------|
| `daemon:heartbeat_request` | `{runtime_id, supports_batch_import}` | Pull pending actions |
| `daemon:register` | `{daemon_id, agent_id, runtimes[]}` | Register new runtimes |
| `task:progress` | `{task_id, summary, step, total}` | Stream execution status |
| `task:message` | `{task_id, type, tool, content, input, output}` | Stream tool calls & results |

### 2. Delta Broadcasting (Issue Updates)

When issue is updated, server broadcasts delta (not full object):

```go
// From server/internal/handler/issue.go UpdateIssue handler
type IssueUpdatedDelta struct {
    IssueID         string
    AssigneeChanged bool
    StatusChanged   bool
    PriorityChanged bool
    
    PrevTitle      string
    PrevStatus     string
    PrevPriority   string
    PrevAssigneeID string
    PrevAssigneeType string
    
    CreatorType string
    CreatorID   string
}
```

**Why:** Frontend can apply optimistic updates without full refetch; reduces flickering

### 3. WebSocket Message Format

```json
{
  "type": "issue:updated",
  "payload": {
    "issue_id": "uuid-123",
    "status_changed": true,
    "prev_status": "todo",
    "new_status": "in_progress",
    "assignee_changed": true,
    "prev_assignee_id": "agent-1",
    "prev_assignee_type": "agent"
  }
}
```

---

## Communication Flows

### Flow 1: Create Issue and Assign to Agent

```
User (Web UI)
    │
    └─→ POST /api/issues
        {
          "title": "Fix bug",
          "assignee_type": "agent",
          "assignee_id": "agent-123",
          "status": "todo"
        }
        │
        └─→ Server creates issue.issue
            └─→ Checks: assignee_type == "agent" && status != "backlog"
                └─→ INSERT agent_task_queue (status: queued)
                    └─→ WS broadcast: task:queued, issue:created
                        └─→ WS: daemon:task_available {runtime_id, task_id}
                            │
                            └─→ Daemon receives task_available event
                                └─→ Wake runtime poller (no need to wait for next cycle)
                                    └─→ GET /api/tasks/claim {runtime_id}
                                        └─→ Server transitions task: queued → dispatched
                                            └─→ WS broadcast: task:dispatch
                                                │
                                                └─→ Daemon processes:
                                                    ├─ Clone/fetch repo
                                                    ├─ Spawn agent process
                                                    ├─ Stream task:progress, task:message
                                                    └─ POST /api/tasks/complete or fail
                                                        └─→ Server transitions task: running → completed/failed
                                                            └─→ WS broadcast: task:completed or task:failed
                                                                └─→ Frontend updates UI, shows results
```

### Flow 2: Daemon Heartbeat & Pending Actions

```
Daemon (every 30s)
    │
    ├─→ POST /api/daemon/heartbeat {runtime_id, supports_batch_import}
    │
    ├─→ Server checks pending actions:
    │   ├─ PendingUpdate: new daemon version available
    │   ├─ PendingModelList: enumerate supported models
    │   ├─ PendingLocalSkills: fetch local skill inventory
    │   └─ PendingLocalSkillImport: import specific skill
    │
    └─→ Server responds with DaemonHeartbeatAckPayload
        │
        └─→ Daemon processes pending actions:
            ├─ If PendingUpdate: run auto-update, spawn new binary
            ├─ If PendingModelList: gather models, POST /api/runtimes/{id}/models
            ├─ If PendingLocalSkills: scan filesystem, POST /api/runtimes/{id}/local-skills
            └─ If PendingLocalSkillImport: download skill, register
```

### Flow 3: Runtime Gone Detection & Recovery

```
Scenario: Server deletes runtime (UI or 7-day GC)
│
├─ Heartbeat path detects:
│  └─ POST /api/daemon/heartbeat → 404 "runtime not found"
│
├─ Poller path detects:
│  └─ GET /api/tasks/claim → 404 "runtime not found"
│
└─ WS handler path receives:
   └─ daemon:heartbeat_ack {runtime_gone: true}

All three paths converge on:
    │
    └─→ Daemon.handleRuntimeGone(runtimeID)
        ├─ Lock: per-runtimeID stampede control
        ├─ Remove stale runtime from local state
        ├─ Notify subscribers (runtimeSet pub/sub)
        ├─ Try claim re-register slot:
        │   ├─ Per-workspaceID coalesce window (30s)
        │   └─ Skip if sibling already re-registered
        │
        └─→ Call registerRuntimesForWorkspace()
            └─→ POST /api/daemons/register {workspace_id, runtimes[]}
                └─→ Server replaces all workspace runtimes
```

---

## Key Design Decisions

### 1. Context Snapshots (No On-Demand Fetches)

**Decision:** Capture entire execution environment at task creation, store in `agent_task_queue.context`

**Rationale:**
- Daemon can execute offline (if context was cached)
- No race conditions between issue updates and task execution
- Audit trail of what state was used for each execution
- Reduces server-daemon API surface

**Trade-off:** Metadata updated during execution won't see live issue changes (by design)

---

### 2. Metadata as Flat JSONB KV Store

**Decision:** Store agent pipeline state in `issue.metadata` as untyped map, not normalized columns

**Rationale:**
- No schema migration for new pipeline metrics
- Agents can record arbitrary state without coordination
- Keeps issue table lean (only human-writable fields normalized)
- Easy for frontend to show "pipeline status" sidebar

**Trade-off:** No SQL indexes on metadata fields; filtering requires `@>` operator

```sql
-- Find issues with failed pipeline
SELECT * FROM issue 
WHERE metadata @> '{"pipeline_status": "failed"}';
```

---

### 3. Heartbeat-based Liveness vs Persistent Connections

**Decision:** HTTP heartbeats (~30s) + WebSocket push events (real-time) + Polling (fallback)

**Rationale:**
- Resilient to network flakes (heartbeat can recover from transient outages)
- Offline-aware (daemon can work locally if context was cached)
- Real-time feel via WebSocket without relying on always-on connection
- Automatic cleanup when heartbeats stop (7-day GC for offline runtimes)

**Trade-off:** Slight delay between server action and daemon discovery (up to 30s)

---

### 4. Per-Issue vs Per-Agent Task Enqueueing

**Decision:** When issue status changes, check if agent is assigned and enqueue in `agent_task_queue`

**Preconditions:**
```
- assignee_type == "agent"
- status != "backlog"
```

**Rationale:**
- Prevents spurious task creation when issue created in backlog
- Only commits to execution when ready for work
- Can reassign issue without triggering new task

**Trade-off:** Manual retry needed if issues reassigned frequently

---

### 5. Delta Broadcasting for Issue Updates

**Decision:** WebSocket broadcasts only what changed (prev_* fields), not full issue

**Rationale:**
- Smaller payload (network efficient)
- Frontend can apply optimistic updates
- Reduces UI flicker when multiple tabs update same issue

**Trade-off:** Frontend must maintain full issue state locally

---

### 6. Coalesced Runtime Gone Recovery

**Decision:** Multiple paths (heartbeat, poller, WS) converge on single `handleRuntimeGone()` with per-runtimeID and per-workspaceID coalescing

**Rationale:**
- One delete can kill all workspace runtimes → Don't stampede with 10 register calls
- Coalesce window (30s) absorbs concurrent detections
- Per-workspace last-completed timestamp prevents same-wave stragglers
- Failure backoff (60s) prevents log floods

**Trade-off:** Up to 30s delay before re-register if coalescing with slower peer

---

## References

| Component | Location | Size |
|-----------|----------|------|
| Daemon core | `server/internal/daemon/daemon.go` | 118KB |
| Daemon config | `server/internal/daemon/config.go` | 31KB |
| Daemon task execution | `server/internal/daemon/daemon.go` (runTask method) | Part of 118KB |
| Issue handler | `server/internal/handler/issue.go` | 90KB+ |
| Issue queries | `server/pkg/db/queries/issue.sql` | Generated |
| Protocol definitions | `server/pkg/protocol/events.go`, `messages.go` | 302 lines |
| Frontend issue types | `packages/core/types/issue.ts` | 59 lines |
| Frontend agent types | `packages/core/types/agent.ts` | 566 lines |
| Database models | `server/pkg/db/generated/models.go` | 638 lines |

