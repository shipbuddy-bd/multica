# Multica Architecture Documentation - Summary

## Overview

This folder now contains comprehensive, production-ready documentation on Multica's issue and agent architecture. The documentation was created through systematic exploration of the codebase, database migrations, and protocol definitions.

## Files Generated

### 1. **COMPREHENSIVE_ARCHITECTURE.md** (29KB)
**The main reference document covering:**

- Executive Summary: Core concepts (Issue, Agent, AgentTaskQueue, Daemon, Runtime)
- Issue Data Model: Schema, status lifecycle, assignee types, metadata
- Agent Architecture: Status axes (operational/runtime/visibility), runtime tracking
- Task Execution Model: AgentTaskQueue schema, state machine, context snapshots
- Daemon & Runtime: Task loop, heartbeat protocol, runtime gone recovery
- Real-time Events: 70+ WebSocket event types, delta broadcasting
- Communication Flows: 3 key flows (task creation, heartbeat, recovery)
- Key Design Decisions: 6 major architectural decisions with rationale

**Use Case:** Architecture review, system design discussions, onboarding new engineers

---

## Key Architectural Concepts

### Core Tables
1. **issue** - Work units with 7 status values, 3 assignee types, JSONB metadata
2. **agent** - Autonomous runtimes with 3 orthogonal status axes
3. **agent_task_queue** - Execution queue with 6 status values and retry support
4. **agent_runtime** - Physical execution environments with heartbeat tracking

### Status Machines
- **Issue:** backlog → todo → in_progress → in_review → done (blocked/cancelled from any)
- **Agent:** idle/working/blocked/error/offline
- **Task:** queued → dispatched → running → completed/failed/cancelled

### Key Design Patterns
1. **Context Snapshots:** Full execution state captured at task creation (no race conditions)
2. **Metadata as JSONB:** Agent pipeline state in flexible KV store (no schema migrations)
3. **Heartbeat-based Liveness:** HTTP ~30s + WebSocket push + polling fallback
4. **Delta Broadcasting:** Only changed fields in WebSocket events (frontend optimistic updates)
5. **Coalesced Recovery:** Multiple detection paths converge on single recovery handler

---

## Quick Navigation

### For **First-time Contributors**
Start with: Executive Summary → Issue Data Model → Agent Architecture

### For **System Designers**
Focus on: Key Design Decisions → Communication Flows → Daemon & Runtime

### For **Integration Work**
Reference: Real-time Events (70+ WebSocket types) → Communication Flows

### For **Database Tuning**
Check: All table schemas with indexes, metadata JSONB constraints

---

## Technical Highlights

### Issue Lifecycle
```
Backlog → Todo → In Progress → In Review → Done
                 (auto on agent execution start)
```

### Assignee Model
- **member** (human): Manual work, no automation
- **agent** (AI): Automatic execution via daemon
- **squad** (team): Coordinated multi-agent with leader

### Daemon Architecture
- Polls server for tasks (~30s per runtime)
- Subscribes to WebSocket for real-time wake-up notifications
- Sends heartbeats to report liveness and receive pending actions
- Streams task execution progress back to server
- Auto-updates when new version available

### Event System
- **Server → Clients:** 70+ WebSocket event types for real-time sync
- **Daemon → Server:** Task progress, tool calls, heartbeats
- **Client subscriptions:** By prefix (task:*, agent:*, etc.)

---

## Cross-References

| Need | Section |
|------|---------|
| How does an issue get executed? | Communication Flows → Flow 1 |
| Why heartbeats instead of persistent connections? | Key Design Decisions → Decision 3 |
| How to query issues by pipeline status? | Key Design Decisions → Decision 2 |
| What if a daemon dies? | Daemon & Runtime → Runtime Gone Recovery |
| How many concurrent tasks per agent? | Agent Architecture → Agent Schema |
| What's the timeout for marking agent offline? | Daemon & Runtime → Runtime Tracking |

---

## Data Model Snapshot

**Issue Fields (24 total):**
id, workspace_id, title, description, status, priority, assignee_type, assignee_id,
creator_type, creator_id, parent_issue_id, project_id, number, start_date, due_date,
metadata, position, created_at, updated_at, labels, ...

**Agent Fields (23 total):**
id, workspace_id, name, description, instructions, runtime_id, status, runtime_mode,
visibility, owner_id, max_concurrent_tasks, model, thinking_level, custom_env,
custom_args, mcp_config, archived_at, archived_by, created_at, updated_at, ...

**AgentTaskQueue Fields (18 total):**
id, agent_id, issue_id, status, priority, runtime_id, session_id, work_dir, context,
result, error, failure_reason, trigger_comment_id, trigger_summary, chat_session_id,
autopilot_run_id, parent_task_id, attempt, max_attempts, timestamps, ...

---

## Generated During
May 24, 2026 - Complete system exploration and documentation generation

## Architecture Type
**Task-driven agent execution with server-pulled polling + WebSocket push notifications**

---

For questions or updates needed, refer to the COMPREHENSIVE_ARCHITECTURE.md file for detailed explanations.
