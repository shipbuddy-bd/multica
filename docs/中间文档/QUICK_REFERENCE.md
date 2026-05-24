# Multica Data Model: Quick Reference

## Issue Fields Quick Lookup

| Field | Type | Nullable | Key | Notes |
|-------|------|----------|-----|-------|
| `id` | UUID | No | PK | Auto-generated |
| `workspace_id` | UUID | No | FK | Scoped to workspace |
| `number` | INT | No | UQ (w/ workspace) | Sequential per workspace (WS-42) |
| `title` | TEXT | No | - | Required at creation |
| `description` | TEXT | Yes | - | Markdown support |
| `status` | TEXT | No | Index | backlog\|todo\|in_progress\|in_review\|done\|blocked\|cancelled |
| `priority` | TEXT | No | - | urgent\|high\|medium\|low\|none |
| `assignee_type` | TEXT | Yes | Index | member\|agent\|squad |
| `assignee_id` | UUID | Yes | Index | References member/agent/squad.id |
| `creator_type` | TEXT | No | - | member\|agent |
| `creator_id` | UUID | No | - | Issue creator |
| `parent_issue_id` | UUID | Yes | FK,Index | Subtask relationship |
| `project_id` | UUID | Yes | - | Project grouping |
| `position` | FLOAT | No | - | Ordering in lists |
| `start_date` | TIMESTAMPTZ | Yes | - | Gantt chart start |
| `due_date` | TIMESTAMPTZ | Yes | - | Deadline |
| `metadata` | JSONB | No | GIN Index | Agent pipeline state (max 8KB, ≤50 keys) |
| `created_at` | TIMESTAMPTZ | No | - | Audit trail |
| `updated_at` | TIMESTAMPTZ | No | - | Audit trail |
| `first_executed_at` | TIMESTAMPTZ | Yes | Partial Index | Set once when issue reaches "done" |

## Agent Fields Quick Lookup

| Field | Type | Nullable | Key | Notes |
|-------|------|----------|-----|-------|
| `id` | UUID | No | PK | Auto-generated |
| `workspace_id` | UUID | No | FK,Index | Scoped to workspace |
| `runtime_id` | UUID | No | FK | References runtime_device.id |
| `name` | TEXT | No | - | Agent name |
| `description` | TEXT | No | - | Max 255 unicode chars (CHECK constraint) |
| `instructions` | TEXT | No | - | Agent system prompt |
| `avatar_url` | TEXT | Yes | - | Agent avatar image |
| `runtime_mode` | TEXT | No | - | local\|cloud |
| `runtime_config` | JSONB | No | - | Provider-specific config |
| `custom_env` | JSONB | No | - | Custom environment variables |
| `custom_args` | JSONB | No | - | Custom CLI arguments (array) |
| `mcp_config` | JSONB | Yes | - | MCP server configuration |
| `visibility` | TEXT | No | - | workspace\|private |
| `status` | TEXT | No | Index | idle\|working\|blocked\|error\|offline |
| `max_concurrent_tasks` | INT | No | - | Default 1 |
| `model` | TEXT | No | - | e.g., "claude-opus-4", "gpt-4o" |
| `thinking_level` | TEXT | No | - | Runtime reasoning effort (empty = default) |
| `owner_id` | UUID | Yes | FK | Agent owner |
| `created_at` | TIMESTAMPTZ | No | - | Audit trail |
| `updated_at` | TIMESTAMPTZ | No | - | Audit trail |
| `archived_at` | TIMESTAMPTZ | Yes | - | Soft delete marker |
| `archived_by` | UUID | Yes | - | Who archived |

## Agent Task Queue Fields

| Field | Type | Nullable | Key | Notes |
|-------|------|----------|-----|-------|
| `id` | UUID | No | PK | Auto-generated |
| `agent_id` | UUID | No | FK,Index | Task assigned to agent |
| `issue_id` | UUID | No | FK | Issue being worked on |
| `runtime_id` | UUID | No | FK,Index | Daemon runtime executing |
| `status` | TEXT | No | CHECK | queued\|dispatched\|running\|completed\|failed\|cancelled |
| `priority` | INT | No | Index | Sort key: higher first |
| `dispatched_at` | TIMESTAMPTZ | Yes | - | When daemon claimed it |
| `started_at` | TIMESTAMPTZ | Yes | - | When execution began |
| `completed_at` | TIMESTAMPTZ | Yes | - | When execution ended |
| `result` | JSONB | Yes | - | Successful output |
| `error` | TEXT | Yes | - | Error message if failed |
| `failure_reason` | TEXT | Yes | - | Discriminator: agent_error\|timeout\|runtime_offline\|etc |
| `work_dir` | TEXT | Yes | - | Daemon-pinned working directory |
| `chat_session_id` | UUID | Yes | - | If spawned from chat |
| `autopilot_run_id` | UUID | Yes | - | If spawned from autopilot |
| `parent_task_id` | UUID | Yes | FK | For retry chains |
| `attempt` | INT | Yes | - | 1-based attempt number (>1 = retry) |
| `trigger_comment_id` | UUID | Yes | FK | If triggered by comment |
| `trigger_summary` | TEXT | Yes | - | Snapshot of what triggered task |
| `kind` | TEXT | Yes | - | comment\|autopilot\|chat\|quick_create\|direct |
| `created_at` | TIMESTAMPTZ | No | - | Audit trail |

## WebSocket Event Types

### Payload by Event Type

```
issue:created           → { issue: Issue }
issue:updated           → { issue: Issue }
issue:deleted           → { issue_id: string }
issue_metadata:changed  → { issue_id: string, metadata: IssueMetadata }
issue_labels:changed    → { issue_id: string, labels: Label[] }

agent:status            → { agent: Agent }
agent:created           → { agent: Agent }
agent:archived          → { agent: Agent }
agent:restored          → { agent: Agent }

task:queued             → { task_id, agent_id, issue_id, status }
task:dispatch           → { task_id, agent_id, issue_id, runtime_id }
task:progress           → { task_id, summary, step?, total? }
task:completed          → { task_id, agent_id, issue_id, status }
task:failed             → { task_id, agent_id, issue_id, status }
task:message            → { task_id, issue_id?, seq, type, tool?, content?, input?, output? }
task:cancelled          → { task_id, agent_id, issue_id, status }

squad:created           → { squad: Squad }
squad:updated           → { squad: Squad }
squad:deleted           → { squad: Squad }

comment:created         → { comment: Comment }
comment:updated         → { comment: Comment }
comment:deleted         → { comment_id, issue_id }
comment:resolved        → { comment: Comment }
comment:unresolved      → { comment: Comment }

reaction:added          → { reaction: Reaction, issue_id }
reaction:removed        → { comment_id, issue_id, emoji, actor_type, actor_id }
issue_reaction:added    → { reaction: IssueReaction, issue_id }
issue_reaction:removed  → { issue_id, emoji, actor_type, actor_id }

inbox:new               → { item: InboxItem }
inbox:read              → { item_id, recipient_id }
inbox:archived          → { item_id, recipient_id }

workspace:updated       → { workspace: Workspace }
member:added            → { member: Member, workspace_id }
member:updated          → { member: Member }
member:removed          → { member_id, user_id, workspace_id }

daemon:heartbeat        → { runtime_id, ... }
daemon:register         → { daemon_id, agent_id, runtimes[] }
```

## HTTP Endpoints: Issues

```
POST   /api/issues
       CreateIssueRequest → IssueResponse (201)

GET    /api/issues
       Query: status, priority, assignee_id, creator_id, project_id,
              scheduled, metadata, search
       → IssueResponse[] (200)

GET    /api/issues/:id
       → IssueResponse with full details (200)

PATCH  /api/issues/:id
       UpdateIssueRequest (partial) → IssueResponse (200)
       ⚠️ Does NOT update metadata

DELETE /api/issues/:id
       → 204 No Content

PUT    /api/issues/:id/metadata/:key
       { value: <primitive JSON> } → 204
       Atomic single-key update

GET    /api/issues/:id/metadata/:key
       → { value: <primitive JSON> } (200)

DELETE /api/issues/:id/metadata/:key
       → 204 No Content

GET    /api/issues/grouped/by-assignee
       → { groups: [{ assignee_type, assignee_id, issues[], total }] }

GET    /api/issues/open
       → IssueResponse[] (only non-terminal statuses)
```

## HTTP Endpoints: Agents & Tasks

```
POST   /api/agents
       CreateAgentRequest → AgentResponse (201)

GET    /api/agents
       → AgentResponse[] (200)

GET    /api/agents/:id
       → AgentResponse with skills (200)

PATCH  /api/agents/:id
       UpdateAgentRequest (partial) → AgentResponse (200)

DELETE /api/agents/:id
       → 204 No Content

POST   /api/tasks/claim
       { runtime_id, limit } → AgentTaskResponse[] (200)
       Daemon calls this to claim work

PATCH  /api/tasks/:id
       UpdateTaskStatusRequest → AgentTaskResponse (200)
       Daemon calls to report progress

GET    /api/tasks/:id
       → AgentTaskResponse (200)

GET    /api/tasks?agent_id=...&status=...
       → AgentTaskResponse[] (200)
```

## Common Filter Patterns

### List Issues with Filters
```
GET /api/issues?
    status=in_progress&
    priority=high&
    assignee_id=<uuid>&
    project_id=<uuid>&
    metadata={"pr_number":1234}&
    search=login

→ Filtered IssueResponse[]
```

### List Tasks for Agent
```
GET /api/tasks?
    agent_id=<uuid>&
    status=running

→ AgentTaskResponse[] (all running tasks for this agent)
```

### Get Agent Activity
```
GET /api/agents/<id>/tasks?
    completed_at_gte=2024-05-01&
    completed_at_lte=2024-05-31

→ AgentTaskResponse[] (completed in date range)
```

## Validation Rules

### Issue Metadata
- **Key regex:** `^[a-zA-Z_][a-zA-Z0-9_.-]{0,63}$`
- **Max keys:** 50 per issue
- **Value types:** string | number | boolean (no null, array, object)
- **Size limit:** ≤8KB total JSONB

### Agent Description
- **Max length:** 255 unicode characters (CHECK constraint)

### Assignment Validation
- assignee_type must match assignee_id:
  - type='member' → id references member.id
  - type='agent' → id references agent.id
  - type='squad' → id references squad.id
- All references must exist in same workspace

## Key Patterns

### Task Lifecycle State Machine
```
∅ → queued ⇄ dispatched → running → [completed | failed | cancelled]
    ↑_____↓ (retry)
```

### Metadata Write Pattern (Atomic)
```
PUT /api/issues/123/metadata/pr_number { value: 1234 }
PUT /api/issues/123/metadata/status { value: "passed" }
// No race; independent keys
```

### Squad Dispatch
```
Issue → Squad → Leader Agent → Multiple Member Agents
         (1 leader, N members can execute in parallel)
```

### First Executed Tracking
```
Issue created → first_executed_at IS NULL
Agent completes → UPDATE issue SET first_executed_at = NOW()
                  WHERE first_executed_at IS NULL
                  (atomic, fires exactly once)
```

## Field Aliasing (Frontend)

- `issue.creator_type` can be "member" or "agent" (not "squad")
- `issue.assignee_type` can be "member", "agent", or "squad"
- `agent.status` is display-only (set by server)
- `agent_task_queue.status` has 6 distinct values, flows one-way to terminal

## Performance Tips

- **Metadata queries:** Use GIN index: `metadata @> '{"key": "value"}'`
- **First executed:** Partial index skips unexecuted issues: `WHERE first_executed_at IS NOT NULL`
- **Task claiming:** Single index pass via `(runtime_id, priority DESC, created_at ASC)`
- **Labels:** Omitted from batch updates; client caches; nil = don't touch

## Key Constraints

| Constraint | Location | Enforces |
|-----------|----------|----------|
| `issue_status_check` | DB | status ∈ {backlog, todo, ...} |
| `agent_task_queue_status_check` | DB | status ∈ {queued, dispatched, ...} |
| `assignee_type_check` | DB | assignee_type ∈ {member, agent, squad} |
| `agent_description_length` | DB | ≤255 unicode chars |
| `issue_metadata_is_object` | DB | JSONB must be object `{}` |
| `issue_metadata_size_limit` | DB | ≤8KB |
| `uq_issue_workspace_number` | DB | (workspace_id, number) UNIQUE |
| `uq_squad_workspace_name` | DB | (workspace_id, name) UNIQUE for squads |

## Common Aggregations

### Issue Progress
```sql
SELECT parent_issue_id, COUNT(*) as total, 
       COUNT(*) FILTER (WHERE status IN ('done', 'cancelled')) as done
FROM issue
WHERE workspace_id = $1 AND parent_issue_id IS NOT NULL
GROUP BY parent_issue_id
```

### Agent Workload
```sql
SELECT agent_id, status, COUNT(*) as count
FROM agent_task_queue
WHERE created_at >= NOW() - INTERVAL '24 hours'
GROUP BY agent_id, status
```

### User's Assigned Issues
```sql
SELECT * FROM issue i
WHERE i.workspace_id = $1
  AND i.assignee_type = 'agent'
  AND i.assignee_id IN (
    SELECT a.id FROM agent a
    WHERE a.owner_id = $2
  )
```

