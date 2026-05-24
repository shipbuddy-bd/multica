# Multica Daemon - Quick Reference Guide

## Key Files to Understand

| File | Size | Purpose |
|------|------|---------|
| `server/internal/daemon/daemon.go` | 118 KB | Core daemon orchestration (2700+ lines) |
| `server/internal/daemon/types.go` | - | Task, TaskResult, SkillData definitions |
| `server/internal/daemon/client.go` | - | HTTP/WebSocket client to server |
| `server/internal/daemon/config.go` | - | Configuration loading from env vars |
| `server/internal/daemon/local_skills.go` | - | Local skill discovery (user-installed skills) |
| `server/pkg/agent/agent.go` | - | Unified Backend interface for all agent types |
| `server/pkg/agent/claude.go` | - | Claude Code backend implementation |
| `server/pkg/agent/codex.go` | - | Codex backend implementation |
| `server/internal/daemon/execenv/execenv.go` | - | Task environment preparation |
| `server/internal/daemon/execenv/runtime_config.go` | - | Context file generation (CLAUDE.md, AGENTS.md) |
| `server/cmd/multica/cmd_daemon.go` | - | CLI commands (daemon start/stop/logs/status) |

## Core Concepts

### 1. Daemon Startup (`Daemon.Run()`)

```
┌─ Bind health check port (19514 default)
├─ Load auth token from ~/.multica/config.json
├─ Fetch all user workspaces from server API
├─ Register runtimes (one per provider per workspace)
│
└─ Launch background loops:
   ├─ workspaceSyncLoop (detect new workspaces)
   ├─ taskWakeupLoop (WebSocket connection for instant notification)
   ├─ heartbeatLoop (15s periodic heartbeat to all runtimes)
   ├─ gcLoop (1h periodic garbage collection)
   ├─ autoUpdateLoop (6h periodic CLI self-update check)
   └─ pollLoop (main task polling loop)
```

### 2. Task Polling and Execution

**Per-runtime poller** (`runRuntimePoller`):
```
While daemon running:
  1. Acquire semaphore slot (max 20 concurrent by default)
  2. Check auto-update pause flag
  3. POST /api/daemon/runtimes/{id}/tasks/claim
  4. If task received:
     - Spawn handleTask() goroutine
     - Release slot (for next poll)
  5. Else: Sleep or wait for WebSocket wakeup
```

**Task execution** (`runTask`):
```
1. Validate task has workspace_id
2. Register task repos (ensure in workspace + cache)
3. Get agent entry from config (path to CLI binary)
4. Build TaskContextForEnv (agent name, skills, repos, etc.)
5. Mark env root as active (prevent GC)
6. Try reuse prior workdir, else prepare fresh environment
7. Inject runtime config (CLAUDE.md / AGENTS.md + skills)
8. Create agent.Backend (provider-specific)
9. Execute: Backend.Execute(prompt, execOpts)
   └─ Spawn agent CLI process
   └─ Stream messages (text, thinking, tool-use, tool-result, status, error, log)
   └─ Timeout: 2h default (or codex semantic timeout)
10. Drain and report:
    ├─ ReportProgress() - optional status updates
    ├─ ReportTaskMessages() - batch export of messages
    ├─ PinTaskSession() - persist session_id mid-flight
    └─ Watch for task cancellation (poll every 5s)
11. On completion:
    ├─ CompleteTask() + ReportTaskUsage() (on success)
    └─ FailTask() + FailureReason + Usage (on failure)
12. Unmark env root (allow GC)
13. Return TaskResult to caller
```

### 3. Skill System

**Skills Flow:**
```
Task claimed from server
  ↓
  ├─ Contains: AgentData { Skills: []SkillData }
  │  Where: SkillData = { Name, Description, Content, Files[] }
  │
  └─→ Injected into environment by provider:
      ├─ CLAUDE.md → .claude/skills/
      ├─ AGENTS.md → .github/skills/ (Copilot)
      ├─ AGENTS.md → .config/opencode/skills/ (OpenCode)
      ├─ AGENTS.md → .openclaw/skills/ (OpenClaw via per-task config)
      ├─ AGENTS.md → .pi/agent/skills/ (Pi)
      ├─ AGENTS.md → .cursor/skills/ (Cursor)
      └─ Codex/Hermes/Kimi/Kiro: All via AGENTS.md

Local skills (user-installed):
  ~/.claude/skills/
  ~/.codex/skills/
  ~/.copilot/skills/
  ~/.config/opencode/skills/
  ~/.openclaw/skills/
  ~/.pi/agent/skills/
  ~/.cursor/skills/
  ~/.kiro/skills/
  
  Discovered via heartbeat pending actions:
  ├─ Server requests local-skills inventory
  ├─ Daemon walks skill directories
  ├─ Returns: Key, Name, Description, FileCount
  └─ Server can request full bundle import
```

### 4. Environment Structure

```
{workspacesRoot}/{workspaceID}/{taskID_short}/
├── workdir/                    # Agent's CWD
│   ├── CLAUDE.md               # Claude context brief
│   ├── AGENTS.md               # Copilot/OpenCode/OpenClaw/Hermes/etc. brief
│   ├── GEMINI.md               # Gemini context brief
│   ├── .multica/
│   │   ├── project/resources.json  # Attached resources
│   │   └── skills/                 # Multica-provided skills
│   ├── .claude/skills/         # Claude-specific skills
│   ├── .github/skills/         # Copilot skills
│   ├── .config/opencode/skills/  # OpenCode skills
│   └── [checked-out repos]     # Via `multica repo checkout`
├── codex-home/                 # Per-task CODEX_HOME (if provider=codex)
├── openclaw-config.json        # Synthesized config (if provider=openclaw)
└── output/, logs/              # Output/log directories
```

### 5. Agent Execution Flow

```
Backend.Execute(prompt, ExecOptions)
  ↓
provider-specific spawn:
  ├─ claude:   claude ... --output stream-json
  ├─ codex:    codex app-server
  ├─ copilot:  copilot run (json mode)
  ├─ openclaw: openclaw agent run
  ├─ hermes:   hermes acp
  └─ etc.
  
Agent runs, emits:
  1. MessageStatus (early session_id pinning)
  2. MessageText, MessageThinking (reasoning/output)
  3. MessageToolUse (tool call name + input)
  4. MessageToolResult (tool result output)
  5. MessageStatus (periodic updates)
  6. (Optional) MessageError, MessageLog
  
Stream closes → Agent finished
  ↓
Result { Status, Output, Error, SessionID, Usage, DurationMs }
```

### 6. Server Communication

**HTTP API Endpoints:**
```
POST /api/daemon/runtimes/{id}/tasks/claim       ← Claim next task
POST /api/daemon/tasks/{id}/start                ← Mark running
POST /api/daemon/tasks/{id}/complete             ← Complete + comment
POST /api/daemon/tasks/{id}/fail                 ← Fail + error
POST /api/daemon/tasks/{id}/status               ← Get current status
POST /api/daemon/tasks/{id}/progress             ← Report progress
POST /api/daemon/tasks/{id}/messages             ← Batch export messages
POST /api/daemon/tasks/{id}/usage                ← Report token usage
POST /api/daemon/tasks/{id}/session              ← Pin session_id
POST /api/daemon/heartbeat                       ← Heartbeat + get pending actions
GET  /api/daemon/runtimes/{id}/local-skills      ← List local skills
POST /api/daemon/runtimes/{id}/local-skills/...  ← Import skill bundle
```

**WebSocket Connection:**
```
GET /ws?runtime_ids=id1,id2,...
  ↓
Receive:
  { "task_ready": true }          ← Wakeup on new task
  { "runtime_gone": <id> }        ← Runtime deleted server-side
  
Send (automatic heartbeat acks):
  { "runtime_id": "...", "acks": [...] }
```

### 7. Key Configuration

**Environment Variables:**
```bash
MULTICA_SERVER_URL               # ws://localhost:8080/ws
MULTICA_WORKSPACES_ROOT          # ~/multica_workspaces

# Per-provider agent discovery
MULTICA_CLAUDE_PATH              # claude (or full path)
MULTICA_CLAUDE_MODEL             # claude-opus-4-1
MULTICA_CODEX_PATH               # codex
MULTICA_CODEX_MODEL              # ...
# ... (similar for other agents)

# Tuning
MULTICA_DAEMON_POLL_INTERVAL     # 30s
MULTICA_DAEMON_HEARTBEAT_INTERVAL # 15s
MULTICA_AGENT_TIMEOUT            # 2h
MULTICA_DAEMON_MAX_CONCURRENT_TASKS # 20
MULTICA_AGENT_IDLE_WATCHDOG      # 30m (force-stop if silent this long)

# GC
MULTICA_GC_ENABLED               # true
MULTICA_GC_INTERVAL              # 1h
MULTICA_GC_TTL                   # 24h (clean done issues older than this)
MULTICA_GC_ARTIFACT_TTL          # 12h (clean regenerable artifacts)
MULTICA_GC_ARTIFACT_PATTERNS     # node_modules,.next,.turbo
```

### 8. CLI Commands

```bash
multica daemon start            # Start daemon (background)
multica daemon start --foreground  # Start daemon (foreground)
multica daemon stop             # Stop daemon
multica daemon restart          # Restart
multica daemon status           # Show runtimes + status
multica daemon logs             # Show daemon logs
multica daemon logs -f          # Follow logs
multica daemon disk-usage       # Show workspace disk usage
multica daemon disk-usage --by-workspace  # Aggregate by workspace
```

### 9. Runtime Lifecycle

```
Daemon.Run()
  ↓
1. Register runtimes:
   FOR EACH workspace:
     FOR EACH configured agent provider:
       Detect CLI version
       POST /api/daemon/register { workspace_id, daemon_id, runtimes: [...] }
       → Server creates runtime row per provider
   
2. Maintain runtime index: { runtime_id -> { provider, name, status } }

3. Poll per-runtime: ClaimTask() on every configured runtime

4. On server-side deletion (UI delete or 7-day offline GC):
   HTTP 404 "runtime not found" during heartbeat
   OR WebSocket { "runtime_gone": runtime_id }
   
   Recovery:
   ├─ Remove from local state
   ├─ Coalesce re-register (30s window to batch multiple deletions)
   ├─ POST /api/daemon/register (re-register workspace)
   └─ Get new runtime IDs, update index

5. Shutdown:
   Deregister all runtimes
   → Server marks runtimes offline
```

### 10. Session Resumption

```
First Run:
  Task executes with ResumeSessionID=""
  ↓
  Backend returns session_id from first message
  ↓
  Daemon calls PinTaskSession() immediately (mid-flight)
  ↓
  Result includes SessionID + WorkDir
  ↓
  Server stores in task row

Resume Run:
  Server includes PriorSessionID + PriorWorkDir in claimed task
  ↓
  Daemon tries execenv.Reuse(prior_workdir)
  ↓
  Agent runs with ResumeSessionID set
  ↓
  Backend loads session state + resumes
  
Resume Failure:
  If session resume fails before establishing connection:
  - result.Status="failed" AND result.SessionID=""
  ↓
  Daemon retries entire task with fresh session (no ResumeSessionID)
  ↓
  Merges token usage from both attempts
```

## Quick Debug Checklist

- [ ] Agent CLI on PATH? `which claude`, `which codex`, etc.
- [ ] Daemon running? `multica daemon status`
- [ ] Auth token set? `multica profile show` (check for token)
- [ ] Workspaces visible? `multica workspace list`
- [ ] Runtimes registered? `multica daemon status` → runtime IDs
- [ ] Check logs? `multica daemon logs -f`
- [ ] Disk space? `multica daemon disk-usage`
- [ ] WebSocket connected? Look for "task wakeup websocket connected" in logs
- [ ] Task stuck? Check if it's in active env roots (protected from GC)
- [ ] Session ID persisted? `multica daemon logs` → "session resume"

## Architecture Patterns

| Pattern | Implementation |
|---------|-----------------|
| **Slot Semaphore** | Buffered channel [0..N) allocates concurrency slots |
| **Per-Runtime Poller** | Goroutine per runtime, independent claim loops |
| **Task Goroutine** | New goroutine per claimed task, waits for completion |
| **Context Cancellation** | Interrupt agent on server cancel or timeout |
| **Runtime Gone Recovery** | Coalesced 30s window + backoff retry on failure |
| **Active Env Tracking** | In-flight tasks marked active, GC skips them |
| **Session Mid-Flight Pin** | Persist session_id before task completion (crash safety) |
| **Poison Detection** | Check output for known failure markers (iteration limit) |

