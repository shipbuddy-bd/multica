# Multica Data Model Documentation Index

Complete reference documentation for Multica's issue and agent architecture.

## 📚 Documentation Files

### 1. **DATA_MODEL.md** (33KB)
The **definitive** comprehensive guide to Multica's data model.

**Contents:**
- **1. Database Schema: Issues** - Complete schema definition with all fields and migrations
- **2. Database Schema: Agents and Assignment** - Agent table, task queue, squads
- **3. Go Data Models** - Backend types (IssueResponse, AgentResponse, AgentTaskResponse)
- **4. Frontend TypeScript Types** - Frontend types (Issue, Agent, Squad, AgentTask)
- **5. WebSocket Event Types** - All 70+ event types with payloads
- **6. Issue Lifecycle Flowchart** - Complete state machine diagram
- **7. Agent-Issue Assignment Patterns** - Direct, squad, comment-triggered, autopilot
- **8. Metadata Key Patterns** - Common metadata keys (pr_number, pipeline_status, etc.)
- **9. Key API Patterns** - HTTP endpoint examples
- **10. Database Indexes** - Query optimization indexes
- **11. SQL Queries** - SQLC generated query overview
- **12. Analytics & Funnels** - Metrics and aggregations
- **13. Validation Rules** - Issue/metadata/agent constraints
- **14. Summary Table** - Quick entity reference
- **15. Design Principles** - Architecture philosophy
- **16. Common Queries** - Ready-to-run SQL examples

**Use when:** You need complete, authoritative information on any aspect of the model.

---

### 2. **ARCHITECTURE_DIAGRAM.md** (33KB)
Visual ASCII diagrams and flow charts showing system architecture.

**Contents:**
- **Data Flow Diagram** - Frontend ↔ Server ↔ Database ↔ Daemon flow
- **Issue State Machine** - Visual FSM for issue statuses
- **Agent Task Queue Lifecycle** - Complete task journey with events
- **Squad Dispatch Flow** - Team assignment and coordination patterns
- **WebSocket Event Subscription Model** - Event type tree
- **Key Design Patterns** - Metadata write patterns, label handling, event payloads
- **Performance Optimizations** - Index strategies, batch operations
- **Summary** - Key takeaways

**Use when:** You need to understand system flows, see visual relationships, or explain architecture to others.

---

### 3. **QUICK_REFERENCE.md** (12KB)
Fast lookup tables and common queries.

**Contents:**
- **Issue Fields Quick Lookup** - All 24 issue fields in one table
- **Agent Fields Quick Lookup** - All 23 agent fields in one table
- **Agent Task Queue Fields** - All 18 task queue fields in one table
- **WebSocket Event Types** - Event type → payload mapping
- **HTTP Endpoints: Issues** - All issue endpoints
- **HTTP Endpoints: Agents & Tasks** - All agent/task endpoints
- **Common Filter Patterns** - Real-world query examples
- **Validation Rules** - Field constraints
- **Key Patterns** - State machines, write patterns, squad dispatch
- **Field Aliasing** - Type considerations
- **Performance Tips** - Index usage
- **Key Constraints** - Database constraints table
- **Common Aggregations** - SQL aggregate queries

**Use when:** You need quick answers, field definitions, or endpoint reference.

---

## 🎯 Quick Navigation by Use Case

### "I need to understand the issue model"
1. Start: **QUICK_REFERENCE.md** → Issue Fields Quick Lookup
2. Then: **DATA_MODEL.md** → Section 1 (Database Schema: Issues)
3. Deep dive: **DATA_MODEL.md** → Sections 6-7 (Lifecycle and Patterns)

### "I need to understand how agents work"
1. Start: **QUICK_REFERENCE.md** → Agent Fields Quick Lookup
2. Then: **ARCHITECTURE_DIAGRAM.md** → Agent Task Queue Lifecycle
3. Deep dive: **DATA_MODEL.md** → Sections 2-3 (Database + Go Models)

### "I need to understand the real-time event system"
1. Start: **QUICK_REFERENCE.md** → WebSocket Event Types
2. Then: **ARCHITECTURE_DIAGRAM.md** → WebSocket Event Subscription Model
3. Deep dive: **DATA_MODEL.md** → Section 5 (WebSocket Event Types)

### "I need to implement a feature"
1. Understand the flow: **ARCHITECTURE_DIAGRAM.md** → relevant flowchart
2. Check constraints: **QUICK_REFERENCE.md** → Validation Rules
3. Find endpoints: **QUICK_REFERENCE.md** → HTTP Endpoints
4. Implement: Use **DATA_MODEL.md** as authoritative reference

### "I need to write a query"
1. Check fields: **QUICK_REFERENCE.md** → Field tables
2. Review indexes: **DATA_MODEL.md** → Section 10 (Database Indexes)
3. Run example: **QUICK_REFERENCE.md** or **DATA_MODEL.md** → Common Queries

### "I need to debug a data issue"
1. Understand the flow: **ARCHITECTURE_DIAGRAM.md** → relevant flowchart
2. Check constraints: **QUICK_REFERENCE.md** → Key Constraints
3. Trace updates: **DATA_MODEL.md** → Section 5 (WebSocket Events) + Section 12 (Analytics)

---

## 📋 Key Concepts at a Glance

### Issue Assignment (3 types)
- **member** - Human user
- **agent** - Autonomous AI (can have status: idle/working/offline/error/blocked)
- **squad** - Team (has leader agent + N members)

### Issue Status Flow
```
backlog → todo → in_progress → (in_review | blocked | done | cancelled)
          ↑____↓ (can loop)
```

### Agent Task Status Flow
```
queued → dispatched → running → (completed | failed | cancelled)
  ↑_______________↓ (retry)
```

### Metadata Pattern
- Per-issue KV map (JSONB)
- Agents use for pipeline state (PR numbers, statuses, etc.)
- Atomic per-key updates (no race conditions)
- 50 key limit, 8KB max size
- Keys match: `^[a-zA-Z_][a-zA-Z0-9_.-]{0,63}$`

### WebSocket Events
- 70+ event types
- Real-time sync between server and clients
- Broadcast scope: workspace-level
- Payload varies per event type

### Daemon (Local Runtime)
- Connects via WebSocket
- Claims tasks via POST /api/tasks/claim
- Executes agent with Claude API or local LLM
- Reports progress via WebSocket events + HTTP updates
- Reads/updates issue metadata

### Squads (Team Coordination)
- Leader: Always an agent
- Members: Agents or humans
- Workflow: Issue → Squad → Leader creates subtasks → Members execute
- Coordination via shared metadata keys

---

## 🔗 Cross-References

### From Issue to Related Data
```
issue.id
  ├→ issue.parent_issue_id (hierarchical)
  ├→ issue.project_id (grouping)
  ├→ issue.assignee_id + issue.assignee_type
  │   ├→ member.id (human)
  │   ├→ agent.id (with agent.status + agent.owner_id)
  │   └→ squad.id (with squad.leader_id + squad_member[])
  ├→ issue_to_label.label_id
  ├→ issue_dependency (blocking relationships)
  ├→ agent_task_queue (work in progress)
  ├→ comment (discussions)
  └→ activity_log (audit trail)
```

### From Agent to Related Data
```
agent.id
  ├→ agent.workspace_id (scoped)
  ├→ agent.runtime_id (daemon binding)
  ├→ agent.owner_id (agent owner, possibly null)
  ├→ agent_task_queue (assigned work)
  └→ squad_member (squad membership)
```

### From Task to Related Data
```
agent_task_queue.id
  ├→ agent_id (executor)
  ├→ issue_id (work item)
  ├→ runtime_id (daemon)
  ├→ parent_task_id (for retries)
  ├→ chat_session_id (if from chat)
  └→ autopilot_run_id (if from automation)
```

---

## 🎓 Learning Path

**For new team members:**
1. Read **QUICK_REFERENCE.md** (15 min) - Get field names and endpoints
2. Read **ARCHITECTURE_DIAGRAM.md** (30 min) - Understand flows
3. Read **DATA_MODEL.md** sections 1-5 (45 min) - Database + types
4. Bookmark **QUICK_REFERENCE.md** for daily reference
5. Deep dive into specific sections as needed

**For backend developers:**
1. Focus on: DATA_MODEL.md sections 1-3, 10-11 (schemas, Go models, queries)
2. Reference: QUICK_REFERENCE.md for field definitions
3. Implement: Use QUICK_REFERENCE.md HTTP endpoints section

**For frontend developers:**
1. Focus on: DATA_MODEL.md section 4, QUICK_REFERENCE.md WebSocket events
2. Reference: ARCHITECTURE_DIAGRAM.md for event flows
3. Implement: Use event payload tables from QUICK_REFERENCE.md

**For DevOps/Analytics:**
1. Focus on: DATA_MODEL.md sections 10, 12 (indexes, analytics)
2. Reference: QUICK_REFERENCE.md for common aggregations
3. Monitor: Metadata table growth, task queue depth, first_executed funnel

---

## 📊 Document Statistics

| Document | Size | Sections | Tables | Diagrams |
|----------|------|----------|--------|----------|
| DATA_MODEL.md | 33KB | 16 | 10+ | 3+ |
| ARCHITECTURE_DIAGRAM.md | 33KB | 6 | 1 | 8+ ASCII |
| QUICK_REFERENCE.md | 12KB | 12 | 15+ | 2+ |
| **Total** | **78KB** | **34** | **26+** | **13+** |

---

## 🔍 Last Updated

Generated: **May 24, 2026**

Covers:
- Backend: Go models (server/internal/handler)
- Frontend: TypeScript types (packages/core/types)
- Database: 105+ migrations (server/migrations/)
- Protocols: WebSocket + HTTP (server/pkg/protocol)
- Generated code: SQLC queries (server/pkg/db/generated)

**Valid for:** Multica v1.x (as of 2026-05-24)

---

## 💡 Tips for Using This Documentation

1. **Bookmarks:** Save QUICK_REFERENCE.md to your browser favorites
2. **Search:** Most information is in DATA_MODEL.md (use browser search or Cmd+F)
3. **Diagrams:** ARCHITECTURE_DIAGRAM.md is best read in a full-width editor
4. **SQL:** Copy-paste queries from QUICK_REFERENCE.md or DATA_MODEL.md section 16
5. **Validation:** Check QUICK_REFERENCE.md constraints before implementing
6. **Endpoints:** Use QUICK_REFERENCE.md HTTP Endpoints section as API reference

---

## 🙋 Questions?

- **"What fields does issue have?"** → QUICK_REFERENCE.md Issue Fields
- **"How do agents claim work?"** → ARCHITECTURE_DIAGRAM.md Task Queue Lifecycle
- **"What WebSocket events exist?"** → QUICK_REFERENCE.md WebSocket Event Types
- **"How do squads work?"** → ARCHITECTURE_DIAGRAM.md Squad Dispatch Flow
- **"What's the metadata format?"** → DATA_MODEL.md Section 1.5
- **"How do I query issues?"** → QUICK_REFERENCE.md Common Filter Patterns
- **"What's the agent status flow?"** → DATA_MODEL.md Section 2.2
- **"How are tasks retried?"** → ARCHITECTURE_DIAGRAM.md Task Queue Lifecycle

---

**Document Set:** Multica Data Model Complete Reference  
**Primary Author:** System Documentation Generator  
**Status:** Current and Complete ✓

