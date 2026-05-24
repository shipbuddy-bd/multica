# Multica Documentation Index

Comprehensive reference for understanding Multica's architecture, data model, and systems.

---

## 🚀 Start Here

### For New Developers
1. **[DEVELOPER_QUICK_START.md](DEVELOPER_QUICK_START.md)** (30 min read)
   - Issue creation → agent execution flow
   - Key files by use case
   - Common pitfalls
   - Debugging checklist

### For System Architects
1. **[COMPREHENSIVE_ARCHITECTURE.md](COMPREHENSIVE_ARCHITECTURE.md)** (60 min read)
   - Full system design with ASCII diagrams
   - All core concepts explained
   - Communication flows
   - Key design decisions with rationale

### For Code Reviews
1. **[ARCHITECTURE_SUMMARY.txt](ARCHITECTURE_SUMMARY.txt)** (10 min read)
   - Quick reference tables
   - Database schema overview
   - Event types summary
   - API endpoints

---

## 📚 Detailed Guides

### Issue & Agent Data Model
- **[ISSUE_AND_AGENT_ARCHITECTURE.md](ISSUE_AND_AGENT_ARCHITECTURE.md)**
  - Issue database schema (v1-v6 migrations)
  - Agent configuration model
  - Task queue design
  - Assignee type system (member/agent/squad)
  - Frontend type definitions
  - Real-time event system

- **[DATA_MODEL.md](DATA_MODEL.md)**
  - Database schema with detailed field descriptions
  - Issue status lifecycle
  - Agent status axes
  - Metadata JSONB design
  - Type definitions

### Daemon & Runtime
- **[CLI_AND_DAEMON.md](CLI_AND_DAEMON.md)**
  - Daemon architecture overview
  - Task claiming and execution
  - Heartbeat protocol
  - Local vs cloud agents
  - Auto-update mechanism

### Integration & Setup
- **[CLI_INSTALL.md](CLI_INSTALL.md)**
  - Installing the daemon CLI
  - Configuration
  - First-time setup

- **[SELF_HOSTING.md](SELF_HOSTING.md)**
  - Self-hosted deployment
  - Docker setup
  - Configuration options

- **[SELF_HOSTING_ADVANCED.md](SELF_HOSTING_ADVANCED.md)**
  - Advanced deployment scenarios
  - Performance tuning
  - Multi-tenant setups

### Contributing
- **[CONTRIBUTING.md](CONTRIBUTING.md)**
  - Development environment setup
  - Git workflow
  - Testing requirements
  - Code style guidelines

---

## 🎯 By Topic

### Understanding Issues
| Document | Section | Purpose |
|----------|---------|---------|
| COMPREHENSIVE_ARCHITECTURE.md | Issue Data Model | Full schema and lifecycle |
| DEVELOPER_QUICK_START.md | Quick Facts | Issue → task flow |
| DATA_MODEL.md | Issue Status Flow | Lifecycle transitions |
| ARCHITECTURE_SUMMARY.txt | Issue Lifecycle | ASCII state machine |

**Key Files in Codebase:**
- `server/internal/handler/issue.go` — Issue API handlers
- `server/migrations/001_init.up.sql` — Schema definition
- `packages/core/types/issue.ts` — Frontend types

### Understanding Agents
| Document | Section | Purpose |
|----------|---------|---------|
| COMPREHENSIVE_ARCHITECTURE.md | Agent Architecture | Agent design, status axes |
| DATA_MODEL.md | Agent Model | Schema and configuration |
| CLI_AND_DAEMON.md | Agent Execution | How agents execute tasks |
| DEVELOPER_QUICK_START.md | Agent Status | Status field meanings |

**Key Files in Codebase:**
- `server/internal/daemon/daemon.go` — Daemon execution logic (118KB)
- `server/internal/daemon/config.go` — Agent configuration (31KB)
- `packages/core/types/agent.ts` — Frontend agent types

### Understanding Task Execution
| Document | Section | Purpose |
|----------|---------|---------|
| COMPREHENSIVE_ARCHITECTURE.md | Task Execution Model | Queue, state machine, context |
| DEVELOPER_QUICK_START.md | Issue Creation → Execution | End-to-end flow |
| CLI_AND_DAEMON.md | Task Claiming & Execution | Daemon workflow |
| ARCHITECTURE_SUMMARY.txt | Task Status Machine | State transitions |

**Key Tables:**
- `agent_task_queue` — Task persistence
- `agent_runtime` — Runtime tracking
- `agent` — Agent definitions

### Understanding Real-time Events
| Document | Section | Purpose |
|----------|---------|---------|
| COMPREHENSIVE_ARCHITECTURE.md | Real-time Events (WebSocket) | Event types and flow |
| ISSUE_AND_AGENT_ARCHITECTURE.md | WebSocket Event System | Event broadcasting |
| ARCHITECTURE_SUMMARY.txt | WebSocket Events | Event type table |

**Key Files in Codebase:**
- `server/pkg/protocol/events.go` — Event constants (125 lines)
- `server/pkg/protocol/messages.go` — Event payloads (177 lines)

### Understanding Daemon & Runtime
| Document | Section | Purpose |
|----------|---------|---------|
| COMPREHENSIVE_ARCHITECTURE.md | Daemon & Runtime | Full daemon architecture |
| CLI_AND_DAEMON.md | Daemon Design | Process architecture |
| DEVELOPER_QUICK_START.md | Daemon not claiming task | Debugging checks |

**Key Concepts:**
- Daemon = local process that polls for and executes tasks
- Runtime = physical execution environment (local machine or cloud)
- Heartbeat = ~30s keep-alive that pulls pending actions

### Understanding Database Schema
| Document | Section | Purpose |
|----------|---------|---------|
| COMPREHENSIVE_ARCHITECTURE.md | Issue Data Model, Agent Architecture | Table schemas |
| DATA_MODEL.md | All sections | Comprehensive schema docs |
| ARCHITECTURE_SUMMARY.txt | Database Core Tables | Quick reference |

**Key Tables:**
- `issue` — Work items
- `agent` — Agent definitions
- `agent_task_queue` — Task queue
- `agent_runtime` — Runtime tracking
- See migrations in `server/migrations/` for full history

---

## 🔍 Common Scenarios

### "I need to add a feature to issues"
1. Read: DEVELOPER_QUICK_START.md → "I want to add a new issue field"
2. Example: Add migration, update Go models, update frontend types
3. Reference: COMPREHENSIVE_ARCHITECTURE.md → "Issue Data Model"

### "I need to debug a task that's stuck"
1. Checklist: DEVELOPER_QUICK_START.md → "Debugging Checklist"
2. Query: Common SQL queries in same section
3. Deep dive: COMPREHENSIVE_ARCHITECTURE.md → "Task Execution Model"

### "I need to understand the daemon-server interaction"
1. Quick view: COMPREHENSIVE_ARCHITECTURE.md → "Communication Flows"
2. Flow diagrams: Same section shows 3 key flows
3. Deep dive: CLI_AND_DAEMON.md → Full daemon architecture

### "I need to add a new WebSocket event"
1. Steps: DEVELOPER_QUICK_START.md → "I want to add a new WebSocket event"
2. Reference: COMPREHENSIVE_ARCHITECTURE.md → "Real-time Events"
3. Code locations: events.go, messages.go, issue.go

### "I need to optimize database queries"
1. Guide: DEVELOPER_QUICK_START.md → "Performance Notes"
2. Schema understanding: COMPREHENSIVE_ARCHITECTURE.md → "Indexes"
3. Specific queries: DATA_MODEL.md → "Key Evolution" sections

---

## 📖 Reading Paths

### For Code Review (15 min)
- ARCHITECTURE_SUMMARY.txt (quick tables)
- DEVELOPER_QUICK_START.md → Common Pitfalls section
- Check against COMPREHENSIVE_ARCHITECTURE.md for major changes

### For New Team Member (2 hours)
1. DEVELOPER_QUICK_START.md (30 min)
2. COMPREHENSIVE_ARCHITECTURE.md → Executive Summary + Quick Reference (30 min)
3. COMPREHENSIVE_ARCHITECTURE.md → Full read (60 min)
4. Explore source files:
   - `server/internal/handler/issue.go` (key handler)
   - `server/internal/daemon/daemon.go` (daemon logic)

### For Onboarding New Database Migration (30 min)
1. COMPREHENSIVE_ARCHITECTURE.md → "Issue Data Model" or "Agent Architecture"
2. Study existing migration: `server/migrations/001_init.up.sql`
3. Study related Go models: `server/pkg/db/generated/models.go`
4. Study related handler: `server/internal/handler/issue.go`

### For Debugging Production Issue (30 min)
1. DEVELOPER_QUICK_START.md → "Debugging Checklist"
2. Query relevant tables (SQL queries provided)
3. COMPREHENSIVE_ARCHITECTURE.md → related section for deeper understanding
4. Check daemon logs, WebSocket events, etc.

---

## 🗂️ All Documentation Files

| File | Purpose | Time | Audience |
|------|---------|------|----------|
| **DEVELOPER_QUICK_START.md** | Hands-on guide for common tasks | 30 min | Developers |
| **COMPREHENSIVE_ARCHITECTURE.md** | Complete system design and flows | 60 min | Architects, senior devs |
| **ARCHITECTURE_SUMMARY.txt** | Quick reference tables | 10 min | Anyone |
| **ISSUE_AND_AGENT_ARCHITECTURE.md** | Deep dive on data model | 45 min | Backend engineers |
| **DATA_MODEL.md** | Database schema documentation | 40 min | Database engineers |
| **CLI_AND_DAEMON.md** | Daemon and runtime details | 40 min | System engineers |
| **CLI_INSTALL.md** | Installation guide | 15 min | Users, operators |
| **SELF_HOSTING.md** | Basic self-hosting | 20 min | DevOps, self-hosted users |
| **SELF_HOSTING_ADVANCED.md** | Advanced deployment | 45 min | Advanced DevOps |
| **CONTRIBUTING.md** | Development guidelines | 20 min | Contributors |
| **CLAUDE.md** | AI-focused project info | Varied | Claude Code users |
| **AGENTS.md** | Agent system overview | 15 min | General audience |
| **README.md** | Project overview | 15 min | General audience |

---

## 🔗 Key Codebase References

### Core Application Files (Sorted by Importance)

**Issue Handling:**
- `server/internal/handler/issue.go` (90KB+) — All issue operations
- `server/pkg/db/queries/issue.sql` — SQL queries (auto-generated)
- `packages/core/types/issue.ts` (59 lines) — Frontend types

**Agent & Task Execution:**
- `server/internal/daemon/daemon.go` (118KB) — Core daemon logic
- `server/internal/daemon/config.go` (31KB) — Configuration
- `server/internal/daemon/execenv/` — Execution environments

**Real-time Communication:**
- `server/pkg/protocol/events.go` (125 lines) — Event constants
- `server/pkg/protocol/messages.go` (177 lines) — Message payloads
- WebSocket hub in `server/internal/handler/` (implementation)

**Database & Models:**
- `server/migrations/001_init.up.sql` — Core schema
- `server/pkg/db/generated/models.go` (638 lines) — Generated models
- Other migrations in `server/migrations/` (numbered by date)

**Frontend Types:**
- `packages/core/types/issue.ts` — Issue interface
- `packages/core/types/agent.ts` (566 lines) — Agent & task interfaces

---

## ❓ FAQ

**Q: What's the difference between `agent` and `agent_runtime`?**
A: See COMPREHENSIVE_ARCHITECTURE.md § Agent Architecture § "Agent Runtime Tracking"

**Q: Why doesn't my task get enqueued?**
A: Check DEVELOPER_QUICK_START.md § "Pitfall 1: Creating issue in backlog with agent assignee"

**Q: How do I trace a task's execution?**
A: Follow DEVELOPER_QUICK_START.md § "I want to trace a task through the system"

**Q: What happens when an issue is updated?**
A: See COMPREHENSIVE_ARCHITECTURE.md § "Communication Flows" § "Flow 1"

**Q: How often does the daemon heartbeat?**
A: ~30 seconds. See COMPREHENSIVE_ARCHITECTURE.md § "Heartbeat Protocol"

---

## 📞 Getting Help

- **Architecture questions:** Start with COMPREHENSIVE_ARCHITECTURE.md
- **Implementation questions:** Check DEVELOPER_QUICK_START.md use cases
- **Debugging:** Use the debugging checklist in DEVELOPER_QUICK_START.md
- **Code review:** Quick check with ARCHITECTURE_SUMMARY.txt

---

**Last Updated:** May 24, 2026  
**Total Documentation:** ~1500 lines across 14 files
