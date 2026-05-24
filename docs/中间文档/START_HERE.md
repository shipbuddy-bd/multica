# 🎯 Multica Architecture Documentation - START HERE

**Generated:** May 24, 2026  
**Scope:** Complete Issue & Agent Architecture with Daemon Integration  
**Status:** ✅ Production-Ready Documentation Suite

---

## 📚 Documentation Files at a Glance

### 1. **COMPREHENSIVE_ARCHITECTURE.md** ⭐ START HERE
**👉 Your main reference (29KB)**
- Executive summary of core concepts
- Issue data model with status lifecycle
- Agent architecture (3 status axes)
- Task execution model
- Daemon & runtime subsystem
- Real-time WebSocket events (70+ types)
- 3 key communication flows
- 6 design decisions with rationale

**Best for:** System design, architecture reviews, understanding "how it all works"

---

### 2. **QUICK_REFERENCE.md**
**👉 Field lookup & common patterns (12KB)**
- Issue field table (24 fields)
- Agent field table (23 fields)  
- AgentTaskQueue field table (18 fields)
- Common SQL queries
- Filter patterns
- Validation rules

**Best for:** Quick lookup while coding, debugging field issues

---

### 3. **DATA_MODEL.md**
**👉 Deep dive on database schema (33KB)**
- All 4 core tables with migrations
- Status machines (detailed)
- Assignee model breakdown
- Metadata design rationale
- Performance indexes
- Common aggregations

**Best for:** Database schema study, optimization, SQL query writing

---

### 4. **DAEMON_ARCHITECTURE.md**
**👉 Daemon subsystem details (56KB)**
- Daemon lifecycle and goroutines
- Heartbeat protocol (HTTP + WebSocket)
- Task claiming and execution
- Repository caching strategy
- Runtime registration flow
- Auto-update mechanism
- Error handling and recovery
- Concurrency patterns

**Best for:** Understanding daemon operation, debugging daemon issues

---

### 5. **DAEMON_QUICK_REFERENCE.md**
**👉 Daemon field reference (11KB)**
- Daemon struct fields explained
- WorkspaceState fields
- RuntimeManager fields
- Concurrency primitives
- Common configurations

**Best for:** Understanding daemon code, reviewing PRs

---

### 6. **DEVELOPER_QUICK_START.md**
**👉 Getting up to speed (8.2KB)**
- Recommended reading order
- Key concepts explained simply
- Common workflows
- Where to find what in the code
- Integration points

**Best for:** New team members, quick onboarding

---

### 7. **DATA_MODEL_INDEX.md**
**👉 Navigation guide (9.9KB)**
- Use-case-based quick starts
- Learning paths by role
- Cross-references
- FAQ with answers

**Best for:** Finding specific information, role-based learning

---

### 8. **DOCUMENTATION_INDEX.md**
**👉 File directory (11KB)**
- All docs listed by topic
- Quick links to sections
- Topic map
- Search index

**Best for:** Browsing available documentation

---

### 9. **ISSUE_AND_AGENT_ARCHITECTURE.md**
**👉 Previous comprehensive doc (28KB)**
- Alternative perspective on architecture
- Issue lifecycle deep dive
- Agent status transitions
- Task queue design

**Best for:** Different angle on same material, reinforcement

---

### 10. **DOCUMENTATION_SUMMARY.md** ← You are here
**👉 Meta-documentation (5.1KB)**
- Overview of all files
- Key highlights
- Cross-references
- Quick navigation

---

## 🗺️ Navigation Guide by Role

### 👨‍💼 **Engineering Manager / Architect**
1. Read: COMPREHENSIVE_ARCHITECTURE.md (Executive Summary + Design Decisions)
2. Skim: DAEMON_ARCHITECTURE.md (overview only)
3. Reference: DOCUMENTATION_INDEX.md (for team)

### 👨‍💻 **Backend Engineer (New)**
1. Start: DEVELOPER_QUICK_START.md
2. Study: COMPREHENSIVE_ARCHITECTURE.md (full read)
3. Deep dive: DATA_MODEL.md for schema
4. Reference: QUICK_REFERENCE.md while coding

### 🔧 **Backend Engineer (Daemon Focus)**
1. Skim: COMPREHENSIVE_ARCHITECTURE.md (Daemon section)
2. Study: DAEMON_ARCHITECTURE.md (full)
3. Reference: DAEMON_QUICK_REFERENCE.md while coding
4. Quick check: QUICK_REFERENCE.md (field lookups)

### 🎨 **Frontend Engineer**
1. Start: COMPREHENSIVE_ARCHITECTURE.md (Real-time Events section)
2. Study: QUICK_REFERENCE.md (Issue/Agent fields)
3. Reference: DATA_MODEL_INDEX.md (FAQ section)

### 📊 **DevOps / Infrastructure**
1. Skim: COMPREHENSIVE_ARCHITECTURE.md (Daemon & Runtime section)
2. Study: DAEMON_ARCHITECTURE.md (sections on registration, heartbeats)
3. Reference: DAEMON_QUICK_REFERENCE.md (runtime configs)

### 🔍 **Code Reviewer / QA**
1. Reference: QUICK_REFERENCE.md (field validation)
2. Study: COMPREHENSIVE_ARCHITECTURE.md (relevant sections)
3. Check: DATA_MODEL.md (constraints, indexes)

---

## 🎯 Common Questions → Find Answer In

| Question | Document | Section |
|----------|----------|---------|
| How does an issue get executed? | COMPREHENSIVE_ARCHITECTURE | Communication Flows → Flow 1 |
| What are all the issue statuses? | QUICK_REFERENCE | Issue Status table |
| How does the daemon claim tasks? | DAEMON_ARCHITECTURE | Task Claiming |
| What's in issue.metadata? | DATA_MODEL | Metadata Design |
| How many concurrent tasks can an agent run? | QUICK_REFERENCE | Agent Fields |
| What WebSocket events exist? | COMPREHENSIVE_ARCHITECTURE | Real-time Events |
| How does daemon heartbeat work? | DAEMON_ARCHITECTURE | Heartbeat Protocol |
| How are runtimes tracked? | COMPREHENSIVE_ARCHITECTURE | Daemon & Runtime → Runtime Tracking |
| What happens if daemon dies? | DAEMON_ARCHITECTURE | Recovery & Reconnection |
| How to query issues by status? | QUICK_REFERENCE | Common SQL Queries |

---

## 📊 Key Stats

| Metric | Count |
|--------|-------|
| Database tables covered | 4 (issue, agent, agent_task_queue, agent_runtime) |
| Issue status values | 7 (backlog, todo, in_progress, in_review, done, blocked, cancelled) |
| Agent status values | 5 (idle, working, blocked, error, offline) |
| Task queue status values | 6 (queued, dispatched, running, completed, failed, cancelled) |
| WebSocket event types | 70+ |
| Issue fields documented | 24 |
| Agent fields documented | 23 |
| Task queue fields documented | 18+ |
| Design decisions explained | 6 |
| Communication flows detailed | 3 |

---

## ⚡ 30-Second TL;DR

**Multica's architecture:**
- Issues are work units assigned to Agents (AI) or Members (humans)
- When assigned to Agent, an AgentTaskQueue entry is created
- Server broadcasts WebSocket event to wake daemon (~real-time)
- Daemon polls server & claims tasks on behalf of agents
- Agents execute with full execution context (no live fetches)
- Daemon streams progress back via WebSocket
- Heartbeat protocol (~30s) for liveness + pending actions
- All state persisted: PostgreSQL tables + JSONB metadata
- Status machines: Issues (7 states) → Tasks (6 states) → Agents (5 states)

---

## 🔗 Cross-Document Links

Use Ctrl+F (or Cmd+F on Mac) to search for these terms across all docs:
- `agent_task_queue` - Core task execution table
- `metadata` - JSONB pipeline state storage
- `heartbeat` - Liveness detection mechanism
- `WebSocket` - Real-time event broadcast
- `context snapshot` - Execution environment capture
- `daemon` - Local execution process
- `runtime_gone` - Recovery from deleted runtime

---

## ✅ How to Use These Docs

1. **Start with your role above** ↑
2. **Read the recommended documents in order**
3. **Use QUICK_REFERENCE.md as a field lookup reference**
4. **Use DOCUMENTATION_INDEX.md for topic search**
5. **Share DOCUMENTATION_SUMMARY.md with team members** for overview
6. **Link to COMPREHENSIVE_ARCHITECTURE.md in design docs**

---

## 🚀 Next Steps

- [ ] Read the "START HERE" section above for your role
- [ ] Open COMPREHENSIVE_ARCHITECTURE.md
- [ ] Bookmark QUICK_REFERENCE.md (you'll use it often)
- [ ] Share this file with your team
- [ ] Add these docs to your wiki/knowledge base

---

**Last Updated:** May 24, 2026  
**Confidence Level:** ✅ High (sourced from actual codebase exploration)  
**Maintenance:** Refer to source code when docs need updates
