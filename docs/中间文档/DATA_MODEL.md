# Multica Data Model & Architecture: Issues and Agents

## Executive Summary

This document provides a comprehensive overview of Multica's data model focusing on the issue lifecycle, agent integration, and real-time event handling. The architecture supports issues (work items) that can be assigned to members, agents, or squads, with complex state management, metadata tracking, and agent task queue integration.

---

## 1. DATABASE SCHEMA: Issues

### 1.1 Core Issue Table

**Location:** `server/migrations/001_init.up.sql`

```sql
CREATE TABLE issue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
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
    parent_issue_id UUID REFERENCES issue(id) ON DELETE SET NULL,
    acceptance_criteria JSONB NOT NULL DEFAULT '[]',
    context_refs JSONB NOT NULL DEFAULT '[]',
    position FLOAT NOT NULL DEFAULT 0,
    due_date TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 1.2 Issue Status Flow

**Valid statuses:** `backlog` → `todo` → `in_progress` → `in_review` → `done` or `blocked` or `cancelled`

- **backlog**: Issue not yet scheduled
- **todo**: Issue scheduled for work (default when created)
- **in_progress**: Issue is actively being worked on
- **in_review**: Issue is in review phase (e.g., PR under review)
- **done**: Issue completed successfully
- **blocked**: Issue blocked on dependencies or external events
- **cancelled**: Issue cancelled/will not be completed

### 1.3 Issue Assignment Model

Multica supports **three assignee types**:

1. **member**: Human user (UUID references `member.id`)
2. **agent**: Autonomous AI agent (UUID references `agent.id`)
3. **squad**: Team of agents/members (UUID references `squad.id`)

Representation: `(assignee_type, assignee_id)` pair — nullable for unassigned issues.

### 1.4 Issue Extensions (Migrations)

| Migration | Field | Purpose | Type |
|-----------|-------|---------|------|
| 020 | `number` | Per-workspace sequential ID (e.g., "WS-42") | INT NOT NULL |
| 020 | `workspace_prefix` | Issue prefix for identifiers (e.g., "WS") | TEXT NOT NULL |
| 050 | `first_executed_at` | Atomic timestamp when issue first reached "done" | TIMESTAMPTZ NULL |
| 091 | `start_date` | Planned start date for Gantt chart support | TIMESTAMPTZ NULL |
| 105 | `metadata` | Per-issue KV map for agents to track pipeline state | JSONB NOT NULL DEFAULT '{}' |

### 1.5 Issue Metadata (Migration 105)

**Purpose:** Agents record application-specific state (PR numbers, pipeline status, etc.)

**Constraints:**
- Keys must match regex: `^[a-zA-Z_][a-zA-Z0-9_.-]{0,63}$`
- Maximum 50 keys per issue
- Values are primitives only: `string | number | boolean`
- Maximum 8KB total size (DB CHECK)

**Example:**
```json
{
  "pr_number": 1234,
  "pipeline_status": "passed",
  "waiting_on": "external_review",
  "deployment_env": "staging"
}
```

### 1.6 Related Tables

#### Issue Labels
```sql
CREATE TABLE issue_label (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL,
    name TEXT NOT NULL,
    color TEXT NOT NULL
);

CREATE TABLE issue_to_label (
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES issue_label(id) ON DELETE CASCADE,
    PRIMARY KEY (issue_id, label_id)
);
```

#### Issue Dependencies
```sql
CREATE TABLE issue_dependency (
    id UUID PRIMARY KEY,
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    depends_on_issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('blocks', 'blocked_by', 'related'))
);
```

#### Issue Reactions
```sql
-- Issue-level emoji reactions (separate from comment reactions)
CREATE TABLE issue_reaction (
    id UUID PRIMARY KEY,
    issue_id UUID NOT NULL REFERENCES issue(id),
    actor_type TEXT NOT NULL CHECK (actor_type IN ('member', 'agent')),
    actor_id UUID NOT NULL,
    emoji TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### Issue Subscribers
```sql
CREATE TABLE issue_subscriber (
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    subscriber_type TEXT NOT NULL CHECK (subscriber_type IN ('member', 'agent')),
    subscriber_id UUID NOT NULL,
    PRIMARY KEY (issue_id, subscriber_type, subscriber_id)
);
```

---

## 2. DATABASE SCHEMA: Agents and Assignment

### 2.1 Agent Table

**Location:** `server/migrations/001_init.up.sql`

```sql
CREATE TABLE agent (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    avatar_url TEXT,
    runtime_mode TEXT NOT NULL CHECK (runtime_mode IN ('local', 'cloud')),
    runtime_config JSONB NOT NULL DEFAULT '{}',
    visibility TEXT NOT NULL DEFAULT 'workspace' CHECK (visibility IN ('workspace', 'private')),
    status TEXT NOT NULL DEFAULT 'offline' 
        CHECK (status IN ('idle', 'working', 'blocked', 'error', 'offline')),
    max_concurrent_tasks INT NOT NULL DEFAULT 1,
    owner_id UUID REFERENCES "user"(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 2.2 Agent Status Lifecycle

| Status | Meaning |
|--------|---------|
| **offline** | Runtime daemon not connected |
| **idle** | Runtime connected, no active tasks |
| **working** | One or more tasks running |
| **blocked** | Task blocked (waiting for external event) |
| **error** | Transient runtime error |

**Broadcast:** Changes via WebSocket event `agent:status`

### 2.3 Agent Task Queue

**Location:** `server/migrations/001_init.up.sql`

Core table linking agents to issues:

```sql
CREATE TABLE agent_task_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'dispatched', 'running', 'completed', 'failed', 'cancelled')),
    priority INT NOT NULL DEFAULT 0,
    dispatched_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    result JSONB,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 2.4 Task Status Lifecycle

```
∅ (created)
  ↓
queued ← (agent claim endpoint called)
  ↓
dispatched ← (daemon claimed task)
  ↓
running ← (agent execution started)
  ↓
[completed | failed | cancelled]
```

**Key fields:**
- `priority`: Sort key for claiming (higher priority first)
- `dispatched_at`: When daemon claimed the task
- `started_at`: When agent began execution
- `completed_at`: When agent finished (any terminal state)
- `result`: JSON output from successful execution
- `error`: Error message (if failed)

### 2.5 Squad (Team Assignment)

**Location:** `server/migrations/084_squad.up.sql`

Squads allow assigning issues to collaborative units:

```sql
CREATE TABLE squad (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    leader_id UUID NOT NULL REFERENCES agent(id) ON DELETE RESTRICT,
    creator_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(workspace_id, name)
);

CREATE TABLE squad_member (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    squad_id UUID NOT NULL REFERENCES squad(id) ON DELETE CASCADE,
    member_type TEXT NOT NULL CHECK (member_type IN ('agent', 'member')),
    member_id UUID NOT NULL,
    role TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(squad_id, member_type, member_id)
);
```

**Features:**
- Squad has a leader (always an agent)
- Contains agents and/or members
- Can be assigned to issues like any member/agent
- Enables delegation and team workflows

---

## 3. GO DATA MODELS

### 3.1 Issue Response Model

**Location:** `server/internal/handler/issue.go`

```go
type IssueResponse struct {
    ID            string                  `json:"id"`
    WorkspaceID   string                  `json:"workspace_id"`
    Number        int32                   `json:"number"`
    Identifier    string                  `json:"identifier"`     // e.g., "WS-42"
    Title         string                  `json:"title"`
    Description   *string                 `json:"description"`
    Status        string                  `json:"status"`
    Priority      string                  `json:"priority"`
    AssigneeType  *string                 `json:"assignee_type"`  // member|agent|squad
    AssigneeID    *string                 `json:"assignee_id"`
    CreatorType   string                  `json:"creator_type"`
    CreatorID     string                  `json:"creator_id"`
    ParentIssueID *string                 `json:"parent_issue_id"`
    ProjectID     *string                 `json:"project_id"`
    Position      float64                 `json:"position"`       // Ordering
    StartDate     *string                 `json:"start_date"`
    DueDate       *string                 `json:"due_date"`
    CreatedAt     string                  `json:"created_at"`
    UpdatedAt     string                  `json:"updated_at"`
    Metadata      map[string]any          `json:"metadata"`       // Always present (empty object when unset)
    Reactions     []IssueReactionResponse `json:"reactions,omitempty"`
    Attachments   []AttachmentResponse    `json:"attachments,omitempty"`
    Labels        *[]LabelResponse        `json:"labels,omitempty"` // nil = "don't touch", [] = authoritative
}
```

**Key design notes:**
- `metadata` is ALWAYS present in responses (empty `{}` when unset) for safe frontend access
- `labels` field: `nil` = field absent, preserve client cache; non-nil = authoritative list
- `reactions` and `attachments` only in detail endpoints

### 3.2 Agent Response Model

**Location:** `server/internal/handler/agent.go`

```go
type AgentResponse struct {
    ID                 string              `json:"id"`
    WorkspaceID        string              `json:"workspace_id"`
    RuntimeID          string              `json:"runtime_id"`
    Name               string              `json:"name"`
    Description        string              `json:"description"`
    Instructions       string              `json:"instructions"`
    AvatarURL          *string             `json:"avatar_url"`
    RuntimeMode        string              `json:"runtime_mode"`  // local|cloud
    RuntimeConfig      any                 `json:"runtime_config"`
    CustomEnv          map[string]string   `json:"custom_env"`
    CustomArgs         []string            `json:"custom_args"`
    Visibility         string              `json:"visibility"`    // workspace|private
    Status             string              `json:"status"`        // idle|working|blocked|error|offline
    MaxConcurrentTasks int32               `json:"max_concurrent_tasks"`
    Model              string              `json:"model"`
    ThinkingLevel      string              `json:"thinking_level"` // Runtime reasoning effort
    OwnerID            *string             `json:"owner_id"`
    Skills             []AgentSkillSummary `json:"skills"`
    CreatedAt          string              `json:"created_at"`
    UpdatedAt          string              `json:"updated_at"`
    ArchivedAt         *string             `json:"archived_at"`
    ArchivedBy         *string             `json:"archived_by"`
}
```

### 3.3 Agent Task Response Model

**Location:** `server/internal/handler/agent.go`

```go
type AgentTaskResponse struct {
    ID                      string                `json:"id"`
    AgentID                 string                `json:"agent_id"`
    RuntimeID               string                `json:"runtime_id"`
    IssueID                 string                `json:"issue_id"`
    WorkspaceID             string                `json:"workspace_id"`
    Status                  string                `json:"status"`  // queued|dispatched|running|completed|failed|cancelled
    Priority                int32                 `json:"priority"`
    DispatchedAt            *string               `json:"dispatched_at"`
    StartedAt               *string               `json:"started_at"`
    CompletedAt             *string               `json:"completed_at"`
    Result                  any                   `json:"result"`
    Error                   *string               `json:"error"`
    FailureReason           *string               `json:"failure_reason"`  // agent_error|timeout|runtime_offline|etc
    ChatSessionID           *string               `json:"chat_session_id"`
    AutopilotRunID          *string               `json:"autopilot_run_id"`
    ParentTaskID            *string               `json:"parent_task_id"`
    Attempt                 *int32                `json:"attempt"`        // 1-based, >1 = retry
    TriggerCommentID        *string               `json:"trigger_comment_id"`
    TriggerSummary          *string               `json:"trigger_summary"` // Snapshot of what triggered task
    Kind                    *string               `json:"kind"`           // comment|autopilot|chat|quick_create|direct
    WorkDir                 *string               `json:"work_dir"`
    CreatedAt               string                `json:"created_at"`
}
```

---

## 4. FRONTEND TYPESCRIPT TYPES

### 4.1 Issue Type

**Location:** `packages/core/types/issue.ts`

```typescript
export type IssueStatus = 
  | "backlog" | "todo" | "in_progress" | "in_review" | "done" | "blocked" | "cancelled";

export type IssuePriority = "urgent" | "high" | "medium" | "low" | "none";

export type IssueAssigneeType = "member" | "agent" | "squad";

export type IssueMetadataValue = string | number | boolean;
export type IssueMetadata = Record<string, IssueMetadataValue>;

export interface Issue {
  id: string;
  workspace_id: string;
  number: number;
  identifier: string;          // e.g., "WS-42"
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
  position: number;
  start_date: string | null;
  due_date: string | null;
  metadata: IssueMetadata;      // Always present (empty object when unset)
  reactions?: IssueReaction[];
  labels?: Label[];
  created_at: string;
  updated_at: string;
}
```

### 4.2 Agent Type

**Location:** `packages/core/types/agent.ts`

```typescript
export type AgentStatus = "idle" | "working" | "blocked" | "error" | "offline";
export type AgentRuntimeMode = "local" | "cloud";
export type AgentVisibility = "workspace" | "private";
export type TaskFailureReason = 
  | "agent_error" | "timeout" | "codex_semantic_inactivity" 
  | "runtime_offline" | "runtime_recovery" | "manual";

export interface AgentTask {
  id: string;
  agent_id: string;
  runtime_id: string;
  issue_id: string;                    // Empty when no linked issue
  status: "queued" | "dispatched" | "running" | "completed" | "failed" | "cancelled";
  priority: number;
  dispatched_at: string | null;
  started_at: string | null;
  completed_at: string | null;
  result: unknown;
  error: string | null;
  failure_reason?: TaskFailureReason | "";
  created_at: string;
  chat_session_id?: string;
  autopilot_run_id?: string;
  parent_task_id?: string;
  attempt?: number;                    // 1-based; >1 = retry
  trigger_comment_id?: string;
  trigger_summary?: string;
  kind?: "comment" | "autopilot" | "chat" | "quick_create" | "direct";
  work_dir?: string;
}

export interface Agent {
  id: string;
  workspace_id: string;
  runtime_id: string;
  name: string;
  description: string;
  instructions: string;
  avatar_url: string | null;
  runtime_mode: AgentRuntimeMode;
  runtime_config: Record<string, unknown>;
  custom_env: Record<string, string>;
  custom_args: string[];
  visibility: AgentVisibility;
  status: AgentStatus;
  max_concurrent_tasks: number;
  model: string;
  thinking_level?: string;             // Runtime reasoning effort token
  owner_id: string | null;
  skills: AgentSkillSummary[];
  created_at: string;
  updated_at: string;
  archived_at: string | null;
  archived_by: string | null;
}
```

### 4.3 Squad Type

**Location:** `packages/core/types/squad.ts`

```typescript
export type SquadMemberType = "agent" | "member";
export type SquadActivityOutcome = "action" | "no_action" | "failed";

export interface Squad {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  instructions: string;
  avatar_url: string | null;
  leader_id: string;                   // Always an agent
  creator_id: string;
  created_at: string;
  updated_at: string;
  archived_at: string | null;
  archived_by: string | null;
}

export interface SquadMember {
  id: string;
  squad_id: string;
  member_type: SquadMemberType;
  member_id: string;
  role: string;
  created_at: string;
}

export interface SquadMemberStatus {
  member_type: SquadMemberType;
  member_id: string;
  status: "working" | "idle" | "offline" | "unstable" | null;
  active_issues: SquadActiveIssueBrief[];
  last_active_at: string | null;
}
```

---

## 5. WEBSOCKET EVENT TYPES

### 5.1 Event Type Constants

**Location:** `server/pkg/protocol/events.go`

#### Issue Events
- `issue:created` — New issue created
- `issue:updated` — Issue modified (title, status, priority, assignment, dates, etc.)
- `issue:deleted` — Issue deleted
- `issue_metadata:changed` — Single metadata key changed
- `issue_labels:changed` — Issue labels updated

#### Agent Events
- `agent:status` — Agent status changed (idle → working → offline, etc.)
- `agent:created` — New agent created
- `agent:archived` — Agent archived
- `agent:restored` — Agent restored from archive

#### Task Queue Events
- `task:queued` — Task created/enqueued
- `task:dispatch` — Task dispatched to daemon (queued → dispatched)
- `task:progress` — Execution progress update
- `task:completed` — Task finished successfully (running → completed)
- `task:failed` — Task execution failed (running → failed)
- `task:message` — Individual agent message (text, tool_use, etc.)
- `task:cancelled` — Task cancelled by user

#### Comment/Reaction Events
- `comment:created` — New comment on issue
- `comment:updated` — Comment edited
- `comment:deleted` — Comment deleted
- `comment:resolved` — Comment marked as resolved
- `comment:unresolved` — Comment marked as unresolved
- `reaction:added` — Reaction added to comment
- `reaction:removed` — Reaction removed from comment
- `issue_reaction:added` — Reaction added to issue (not comment)
- `issue_reaction:removed` — Reaction removed from issue

#### Inbox Events
- `inbox:new` — New inbox item
- `inbox:read` — Item marked as read
- `inbox:archived` — Item archived
- `inbox:batch-read` — Batch of items marked as read
- `inbox:batch-archived` — Batch of items archived

#### Squad Events
- `squad:created` — Squad created
- `squad:updated` — Squad modified
- `squad:deleted` — Squad deleted

#### Other System Events
- `workspace:updated` — Workspace settings changed
- `workspace:deleted` — Workspace deleted
- `member:added`, `member:updated`, `member:removed` — Membership changes
- `daemon:heartbeat` — Daemon heartbeat (server → client)
- `daemon:register` — Daemon registration
- `subscriber:added`, `subscriber:removed` — Issue subscriber changes
- `activity:created` — Activity log entry created
- `skill:created`, `skill:updated`, `skill:deleted` — Skill changes
- `chat:message`, `chat:done`, `chat:session_read`, `chat:session_deleted`, `chat:session_updated`
- `project:created`, `project:updated`, `project:deleted`
- `label:created`, `label:updated`, `label:deleted`
- `pin:created`, `pin:deleted`, `pin:reordered`
- `invitation:created`, `invitation:accepted`, `invitation:declined`, `invitation:revoked`
- GitHub integration events

### 5.2 WebSocket Message Envelope

**Location:** `server/pkg/protocol/messages.go`

```go
type Message struct {
    Type    string          `json:"type"`              // Event type constant
    Payload json.RawMessage `json:"payload"`           // Event-specific data
}
```

### 5.3 Key Task Event Payloads

#### TaskProgressPayload
```go
type TaskProgressPayload struct {
    TaskID  string `json:"task_id"`
    Summary string `json:"summary"`
    Step    int    `json:"step,omitempty"`
    Total   int    `json:"total,omitempty"`
}
```

#### TaskCompletedPayload
```go
type TaskCompletedPayload struct {
    TaskID string `json:"task_id"`
    PRURL  string `json:"pr_url,omitempty"`
    Output string `json:"output,omitempty"`
}
```

#### TaskMessagePayload
```go
type TaskMessagePayload struct {
    TaskID  string         `json:"task_id"`
    IssueID string         `json:"issue_id,omitempty"`
    Seq     int            `json:"seq"`
    Type    string         `json:"type"`           // "text", "tool_use", "tool_result", "error"
    Tool    string         `json:"tool,omitempty"`
    Content string         `json:"content,omitempty"`
    Input   map[string]any `json:"input,omitempty"`
    Output  string         `json:"output,omitempty"`
}
```

#### Issue Event Payloads
```typescript
// In packages/core/types/events.ts
interface IssueCreatedPayload { issue: Issue; }
interface IssueUpdatedPayload { issue: Issue; }
interface IssueDeletedPayload { issue_id: string; }
interface IssueMetadataChangedPayload {
  issue_id: string;
  metadata: IssueMetadata;
}
interface IssueLabelsChangedPayload {
  issue_id: string;
  labels: Label[];
}
```

---

## 6. ISSUE LIFECYCLE FLOWCHART

### Complete Issue State Machine

```
CREATE ISSUE
     ↓
[backlog] (default status)
     ↓
[todo] ← User schedules work
     ↓
[in_progress] ← Agent/user starts work
     ↓
┌────┴────┬────────┐
│         │        │
[in_review] [blocked] [cancelled]
│               ↑
└─→ [done] ←───┘

Blocked can transition:
- blocked → in_progress (unblock after dependency resolved)
- blocked → cancelled (give up)
- blocked → done (somehow resolved)
```

### Agent Task Lifecycle (Parallel to Issue)

```
ASSIGN AGENT TO ISSUE
     ↓
CREATE agent_task_queue RECORD
     Status: 'queued'
     Priority: [0-INT_MAX]
     ↓
Agent claims via HTTP /claim endpoint
     Status: 'dispatched'
     dispatched_at = NOW()
     ↓
Daemon receives task, starts agent
     Status: 'running'
     started_at = NOW()
     ↓
Agent emits messages (text, tool_use, tool_result)
     → task:message events broadcast
     ↓
Agent finishes execution
     ↓
┌────────┬───────────┬──────────┐
│        │           │          │
completed failed cancelled   (timeout)
│        │           │
└────────┴─┬─────────┘
        Terminal state
        completed_at = NOW()
        result/error populated
```

### Key Lifecycle Events Emitted

When issue is created with assignment:
1. `issue:created` → Frontend creates row
2. Agent's status may transition to `working` if queued tasks exist
3. `agent:status` → Update agent UI

When agent task progresses:
1. `task:dispatch` → Task claimed by daemon
2. `task:progress` → Progress updates (optional)
3. `task:message` → Individual execution messages streamed
4. `task:completed` or `task:failed` → Terminal state reached

When issue status changes:
1. `issue:updated` → Broadcast new status

When issue metadata changed by agent:
1. `issue_metadata:changed` → Single key change broadcast

---

## 7. AGENT-ISSUE ASSIGNMENT PATTERNS

### Pattern 1: Direct Assignment

```
User creates issue with assignee_type='agent', assignee_id=<agent-uuid>
     ↓
CREATE agent_task_queue { agent_id, issue_id, status='queued' }
     ↓
Agent claims task via HTTP /claim
     ↓
Agent executes issue.title + issue.description
     ↓
Issue updated based on execution results (status, metadata)
```

### Pattern 2: Squad Assignment

```
User creates issue with assignee_type='squad', assignee_id=<squad-uuid>
     ↓
Squad dispatcher (leader_id agent) receives task
     ↓
Leader decides which squad members to activate
     ↓
Multiple agent_task_queue records created (one per active member)
     ↓
Members execute independently or coordinate via shared metadata
     ↓
Issue aggregates results from all members
```

### Pattern 3: Comment-Triggered Assignment

```
User comments on issue with @mention or assigns agent via comment
     ↓
CREATE agent_task_queue with trigger_comment_id populated
     ↓
trigger_summary = <comment text truncated to ~200 chars>
     ↓
kind = 'comment'
     ↓
Agent receives task with comment context
```

### Pattern 4: Autopilot Assignment

```
Autopilot automation rule triggers
     ↓
CREATE agent_task_queue with autopilot_run_id populated
     ↓
kind = 'autopilot'
     ↓
Agent executes per autopilot instructions
```

---

## 8. METADATA KEY PATTERNS

Common metadata keys used by agents:

| Key | Type | Example | Purpose |
|-----|------|---------|---------|
| `pr_number` | number | 1234 | GitHub PR linked to this issue |
| `pr_url` | string | "https://github.com/.../pull/1234" | Full PR URL |
| `pipeline_status` | string | "passed" / "failed" / "pending" | CI/CD pipeline status |
| `waiting_on` | string | "review" / "merge" / "deploy" | Blocking step |
| `deployment_env` | string | "staging" / "production" | Target environment |
| `retry_count` | number | 3 | Number of retries attempted |
| `attempt` | number | 2 | Current attempt number (also in task.attempt) |
| `branch_name` | string | "feature/xyz" | Git branch name |
| `commit_sha` | string | "abc123..." | Associated commit |
| `assignee_at_timestamp` | string | ISO timestamp | When last assigned |

---

## 9. KEY API PATTERNS

### Create Issue

**Endpoint:** `POST /api/issues`

**Request:**
```json
{
  "title": "Fix login bug",
  "description": "Users report login fails...",
  "status": "todo",
  "priority": "high",
  "assignee_type": "agent",
  "assignee_id": "<agent-uuid>",
  "project_id": "<project-uuid>",
  "start_date": "2024-05-25T00:00:00Z",
  "due_date": "2024-05-27T00:00:00Z"
}
```

**Response:** `IssueResponse` with `status=201`

### Update Issue

**Endpoint:** `PATCH /api/issues/<id>`

**Request:** Partial object (all fields optional)
```json
{
  "status": "in_progress",
  "priority": "urgent",
  "assignee_type": "agent",
  "assignee_id": "<different-agent-uuid>"
}
```

**Note:** Does NOT touch metadata (see metadata endpoints below)

### Set Issue Metadata Key

**Endpoint:** `PUT /api/issues/<id>/metadata/<key>`

**Request:**
```json
{ "value": "staging" }  // or number, bool
```

**Validation:**
- Key matches `^[a-zA-Z_][a-zA-Z0-9_.-]{0,63}$`
- Value is primitive JSON (string | number | boolean)
- No more than 50 keys total on issue

### Delete Issue Metadata Key

**Endpoint:** `DELETE /api/issues/<id>/metadata/<key>`

### List Issues (with Filters)

**Endpoint:** `GET /api/issues?status=in_progress&priority=high&assignee_id=<uuid>&metadata={"pr_number":1234}`

**Query Params:**
- `status`: Filter by status
- `priority`: Filter by priority
- `assignee_id` / `assignee_ids[]`: Filter by assignee
- `creator_id`: Filter by creator
- `project_id`: Filter by project
- `scheduled`: Boolean, filter issues with start_date OR due_date
- `metadata`: JSON object for containment filter (@> operator)
- `search`: Full-text search on title/description

---

## 10. DATABASE INDEXES

```sql
CREATE INDEX idx_issue_workspace ON issue(workspace_id);
CREATE INDEX idx_issue_assignee ON issue(assignee_type, assignee_id);
CREATE INDEX idx_issue_status ON issue(workspace_id, status);
CREATE INDEX idx_issue_parent ON issue(parent_issue_id);
CREATE INDEX idx_issue_workspace_number ON issue(workspace_id, number);
CREATE INDEX idx_issue_first_executed_at 
    ON issue (workspace_id, first_executed_at) 
    WHERE first_executed_at IS NOT NULL;
CREATE INDEX idx_issue_metadata_gin ON issue USING GIN (metadata jsonb_path_ops);

CREATE INDEX idx_agent_workspace ON agent(workspace_id);
CREATE INDEX idx_agent_task_queue_agent ON agent_task_queue(agent_id, status);
CREATE INDEX idx_agent_task_queue_runtime_pending 
    ON agent_task_queue(runtime_id, priority DESC, created_at ASC);
```

---

## 11. SQL QUERIES (SQLC Generated)

**Location:** `server/pkg/db/queries/` and `server/pkg/db/generated/`

Key queries for issues:

- `GetIssue(issueID, workspaceID)` → Single issue detail
- `ListIssues(workspaceID, filters...)` → Paginated list with filters
- `ListOpenIssues(workspaceID)` → Issues not in terminal states
- `CountIssues(workspaceID, filters...)` → Filtered count
- `CreateIssue(...)` → Create new issue
- `UpdateIssue(issueID, ...)` → Partial update
- `DeleteIssue(issueID)` → Soft/hard delete
- `ChildIssueProgress(workspaceID)` → Aggregate progress on sub-issues
- `CountCreatedIssueAssignees(workspaceID, creatorID)` → Analytics

Key queries for agents:

- `GetAgent(agentID, workspaceID)` → Single agent
- `ListAgents(workspaceID)` → All agents in workspace
- `UpdateAgentStatus(agentID, status)` → Update agent status
- `CreateAgentTask(agentID, issueID, priority)` → Queue task
- `ClaimAgentTask(runtimeID, limit)` → Daemon claims next task
- `UpdateAgentTaskStatus(taskID, newStatus, ...)` → Status transition

---

## 12. ANALYTICS & FUNNELS

### Issue Execution Funnel

**Metric:** `first_executed_at` on issue table (atomic, set once)

**Flow:**
1. Issue created → `first_executed_at IS NULL`
2. Agent starts working → `first_executed_at IS NULL`
3. Agent completes first time → `first_executed_at = NOW()` (atomic UPDATE WHERE IS NULL)
4. Never changes again (even if issue re-assigned or retried)

**Query:** Count executed issues:
```sql
SELECT COUNT(*) FROM issue 
WHERE workspace_id = $1 AND first_executed_at IS NOT NULL;
```

### Agent Activity Metrics

**Tables:**
- `task_usage_daily`: Per-agent, per-day token counts
- `runtime_usage`: Per-runtime, per-date token+cost tracking

---

## 13. VALIDATION RULES

### Issue Creation
- `title` required, non-empty
- `status` one of: backlog, todo, in_progress, in_review, done, blocked, cancelled
- `priority` one of: urgent, high, medium, low, none
- `assignee_type` must be valid if `assignee_id` provided (and vice versa)
- `assignee_type` in {member, agent, squad}; corresponding ID must exist in workspace
- `parent_issue_id` if provided must exist and belong to same workspace

### Metadata Key
- Key matches `^[a-zA-Z_][a-zA-Z0-9_.-]{0,63}$`
- At most 50 keys per issue
- Value is primitive: `string | number | boolean` (no null, array, or object)

### Agent Assignment
- Agent must belong to same workspace
- Agent status doesn't block assignment (even if offline)
- Multiple agents can be assigned to same issue (via squads or via sequential re-assignment)

---

## 14. SUMMARY TABLE: KEY ENTITIES

| Entity | UUID | Workspace-scoped | Assignable | Has Status |
|--------|------|------------------|-----------|-----------|
| Issue | ✓ | Yes | Yes (to member/agent/squad) | Yes (backlog...done) |
| Agent | ✓ | Yes | No | Yes (idle...offline) |
| AgentTask | ✓ | No (agent-scoped) | N/A | Yes (queued...completed) |
| Squad | ✓ | Yes | Yes (can be assigned) | No (but has leader agent with status) |
| Member | ✓ | Yes (via member table) | Yes (assignable to issues) | No |
| Comment | ✓ | No (issue-scoped) | N/A | No (but has type: comment/status_change/etc) |

---

## 15. DESIGN PRINCIPLES

1. **Agents as First-Class Assignees**: Issues can be assigned to agents just like humans, enabling full automation
2. **Metadata Over Schema**: Complex agent state (PR numbers, pipeline status) lives in JSONB metadata, not schema columns
3. **Atomic Metadata Mutations**: Single-key metadata changes prevent race conditions on concurrent agent writes
4. **Event-Driven Architecture**: All state changes broadcast via WebSocket for real-time frontend sync
5. **Task Queue Decoupling**: Agent assignment creates a task queue entry, allowing async execution and retry logic
6. **Squad Composition**: Squads enable team delegation patterns while maintaining individual agent task tracking
7. **Immutable Snapshots**: `trigger_summary`, `attempt` captured at task creation time for forensics

---

## 16. COMMON QUERIES

### Get all issues assigned to agents in a project
```sql
SELECT i.* FROM issue i
WHERE i.workspace_id = $1
  AND i.project_id = $2
  AND i.assignee_type = 'agent';
```

### Get agent's active tasks
```sql
SELECT atq.* FROM agent_task_queue atq
WHERE atq.agent_id = $1
  AND atq.status IN ('queued', 'dispatched', 'running');
```

### Find issues stuck in "blocked" for >7 days
```sql
SELECT i.* FROM issue i
WHERE i.workspace_id = $1
  AND i.status = 'blocked'
  AND i.updated_at < NOW() - INTERVAL '7 days';
```

### Get agent's metadata usage across all issues
```sql
SELECT DISTINCT jsonb_object_keys(i.metadata) as key_name
FROM issue i
JOIN agent_task_queue atq ON i.id = atq.issue_id
WHERE atq.agent_id = $1
ORDER BY key_name;
```

### Count tasks by agent status over time
```sql
SELECT DATE_TRUNC('day', atq.completed_at) as day,
       atq.status,
       COUNT(*) as count
FROM agent_task_queue atq
WHERE atq.workspace_id = $1
GROUP BY day, atq.status
ORDER BY day DESC;
```

