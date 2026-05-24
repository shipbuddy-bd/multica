# Multica Daemon and Agent Runtime Architecture

## Executive Summary

Multica's daemon is a local background process that polls for coding tasks from a server, executes them using local AI agent CLIs (Claude Code, Codex, Copilot, etc.), and reports results back. The daemon implements a sophisticated distributed task execution system with workspace isolation, skill injection, session resumption, and advanced task lifecycle management.

---

## 1. Daemon Core Architecture

### 1.1 Main Structure (`server/internal/daemon/daemon.go`)

**Daemon Struct** (118 KB file, ~2700 lines):
```go
type Daemon struct {
    cfg       Config
    client    *Client                    // HTTP/WebSocket client to Multica server
    repoCache repoCacheBackend          // Manages git repo caching
    logger    *slog.Logger
    
    // State management
    mu           sync.Mutex
    workspaces   map[string]*workspaceState  // Per-workspace runtime tracking
    runtimeIndex map[string]Runtime          // Runtime ID -> Runtime lookup
    reloading    sync.Mutex
    runtimeSet   *runtimeSetWatcher         // Pub/sub for runtime changes
    
    // Task execution
    versionsMu    sync.RWMutex          // Guards agentVersions
    agentVersions map[string]string     // provider -> CLI version
    
    // WebSocket heartbeat tracking
    wsHBMu      sync.RWMutex
    wsHBLastAck map[string]time.Time   // runtime_id -> last heartbeat ack
    
    // Runtime recovery when server-side deletion detected
    runtimeGoneMu             sync.Mutex
    runtimeGoneInflight       map[string]struct{}
    reregisterNextAttempt     map[string]time.Time
    reregisterLastCompletedAt map[string]time.Time
    
    // Update/concurrency control
    cancelFunc    context.CancelFunc
    rootCtx       context.Context
    restartBinary string
    updating      atomic.Bool
    activeTasks   atomic.Int64
    
    // Claim pausing during auto-update
    claimMu        sync.Mutex
    pauseClaims    bool
    claimsInFlight int
    
    // Active environment tracking (prevents GC while running)
    activeEnvRootsMu sync.Mutex
    activeEnvRoots   map[string]int
    
    // Background sync tracking
    bgSyncs sync.WaitGroup
    
    // Task execution (injectable for testing)
    runner             taskRunner
    cancelPollInterval time.Duration
    runUpdateFn        func(targetVersion string) (string, error)
}
```

### 1.2 Task Execution Interface

**taskRunner Interface** - Abstraction for task execution:
```go
type taskRunner interface {
    run(ctx context.Context, task Task, provider string, slot int, log *slog.Logger) (TaskResult, error)
}
```

This allows tests to inject fake task runners without spawning real agent processes.

### 1.3 Daemon Lifecycle: `Run()`

Entry point that bootstraps all background loops:

```go
func (d *Daemon) Run(ctx context.Context) error {
    // 1. Bind health check port (prevents duplicate daemons)
    // 2. Load auth token from CLI config
    // 3. Sync all user workspaces from API
    // 4. Deregister on shutdown
    
    // 5. Launch background loops:
    go d.workspaceSyncLoop(ctx)        // Discover new workspaces
    go d.taskWakeupLoop(ctx, taskWakeups)  // WebSocket + poll wakeup
    go d.heartbeatLoop(ctx)             // Periodic heartbeat to all runtimes
    go d.gcLoop(ctx)                    // Garbage collect old task dirs
    go d.autoUpdateLoop(ctx)            // Check for CLI updates
    go d.serveHealth(ctx, healthLn)     // Health check endpoint
    
    // 6. Main polling loop (blocks until context cancelled)
    return d.pollLoop(ctx, taskWakeups)
}
```

---

## 2. Task Lifecycle and Execution Flow

### 2.1 Complete Task Flow

```
1. SERVER: Create task, mark as "dispatched"
   ↓
2. DAEMON: Poll via ClaimTask() on runtime
   ├─ Acquires semaphore slot (max concurrent tasks)
   ├─ Checks pauseClaims (auto-update barrier)
   └─ Issues POST /api/daemon/runtimes/{id}/tasks/claim
   ↓
3. DAEMON: Receives Task with full context (agent data, repos, skills)
   ↓
4. DAEMON: handleTask() goroutine spawned
   ├─ Calls client.StartTask() → POST /api/daemon/tasks/{id}/start
   └─ Sets task status to "running" on server
   ↓
5. DAEMON: Prepare execution environment
   ├─ Create isolated directory: {workspacesRoot}/{workspaceID}/{taskID_short}/
   ├─ Write context files (CLAUDE.md, AGENTS.md, etc.)
   ├─ Inject agent skills (SkillData array from task)
   ├─ Set up CODEX_HOME / OPENCLAW_CONFIG if needed
   └─ Reuse prior work_dir if task.PriorWorkDir set
   ↓
6. DAEMON: Create agent.Backend for provider
   └─ Supports: claude, codex, copilot, opencode, openclaw, hermes, gemini, pi, cursor, kimi, kiro
   ↓
7. DAEMON: Execute agent via Backend.Execute()
   ├─ Spawn agent CLI process with environment variables
   ├─ Stream task messages (text, thinking, tool-use, tool-result, status, error, log)
   ├─ Watch task cancellation (poll GetTaskStatus every 5s)
   └─ Timeout: 2h default, or codex-specific semantic inactivity timeout
   ↓
8. AGENT: Run in isolated environment, emit tool calls
   ├─ Tools: multica CLI commands (issue get/create/update, repo checkout, etc.)
   └─ Skills: Injected as markdown/tool definitions
   ↓
9. DAEMON: Drain tool output, report progress
   ├─ ReportProgress() - optional summary updates
   ├─ ReportTaskMessages() - batch message export
   └─ PinTaskSession() - persist session_id mid-flight for resume on crash
   ↓
10. AGENT: Completes (status: completed/failed/timeout/aborted)
    ↓
11. DAEMON: Process result
    ├─ On success: CompleteTask() + ReportTaskUsage()
    └─ On failure: FailTask() with error reason
    ↓
12. SERVER: Task marked "completed" / "blocked"
    └─ Issue status updated, comment posted, etc.
```

### 2.2 Task Structure (`types.go`)

```go
type Task struct {
    ID                      string
    AgentID                 string
    RuntimeID               string
    IssueID                 string
    WorkspaceID             string
    Agent                   *AgentData     // Agent to dispatch, with skills
    Repos                   []RepoData     // Available repos for checkout
    ProjectID               string
    ProjectTitle            string
    ProjectResources        []ProjectResourceData  // Attached resources (e.g., GitHub repos)
    PriorSessionID          string         // Claude session to resume
    PriorWorkDir            string         // Workdir to reuse
    TriggerCommentID        string         // Comment that triggered task
    TriggerCommentContent   string
    TriggerAuthorType       string         // "agent" or "member"
    TriggerAuthorName       string
    ChatSessionID           string         // For chat tasks
    ChatMessage             string
    ChatMessageAttachments  []ChatAttachmentMeta
    AutopilotRunID          string         // For autopilot tasks
    AutopilotID             string
    AutopilotTitle          string
    AutopilotDescription    string
    AutopilotSource         string         // manual, schedule, webhook, api
    AutopilotTriggerPayload json.RawMessage
    QuickCreatePrompt       string         // For quick-create tasks
    SquadID                 string         // When picker was a squad
    SquadName               string
    RequestingUserName      string         // Profile of user daemon is acting for
    RequestingUserProfileDescription string
}

type TaskResult struct {
    Status        string           // completed, failed, blocked, timeout, aborted
    Comment       string           // Result output for issue comment
    BranchName    string           // Git branch created
    EnvType       string           // Environment type
    SessionID     string           // Claude session for resumption
    WorkDir       string           // Working directory path
    FailureReason string           // Classifier for failures
    Usage         []TaskUsageEntry // Token usage per model
}
```

### 2.3 Runtime Polling: `runRuntimePoller()`

Per-runtime goroutine that polls for tasks:

```go
func (d *Daemon) runRuntimePoller(
    pollerCtx, parentCtx context.Context,
    rid string,                    // runtime ID
    sem chan int,                  // semaphore for slot allocation
    wakeup <-chan struct{},        // wakeup signal from WebSocket
    taskWG *sync.WaitGroup,        // wait group for task completion
) {
    for {
        // 1. Acquire execution slot (max concurrent tasks)
        select {
        case slot = <-sem:
        case <-pollerCtx.Done():
            return
        default:
            // At capacity, sleep
            sleepWithContextOrWakeup(pollerCtx, d.cfg.PollInterval, wakeup)
            continue
        }
        
        // 2. Check auto-update barrier
        if !d.tryEnterClaim() {
            // Auto-update in progress, defer claim
            sem <- slot
            sleepWithContextOrWakeup(pollerCtx, d.cfg.PollInterval, wakeup)
            continue
        }
        
        // 3. Claim task from server
        task, err := d.client.ClaimTask(pollerCtx, rid)
        if err != nil {
            d.exitClaim()
            sem <- slot
            if isRuntimeNotFoundError(err) {
                // Runtime deleted server-side, trigger recovery
                go d.handleRuntimeGone(rid)
                return
            }
            sleepWithContextOrWakeup(pollerCtx, d.cfg.PollInterval, wakeup)
            continue
        }
        
        if task == nil {
            // No task available, sleep
            sleepWithContextOrWakeup(pollerCtx, d.cfg.PollInterval, wakeup)
            continue
        }
        
        // 4. Spawn task handler goroutine
        taskWG.Add(1)
        go func(t Task, slot int) {
            defer taskWG.Done()
            defer d.exitClaim()
            defer func() { sem <- slot }()
            d.handleTask(parentCtx, t, slot)
        }(*task, slot)
    }
}
```

### 2.4 Task Execution: `handleTask()` and `runTask()`

**handleTask()** - High-level task dispatch:
- Logs task details
- Calls StartTask() to mark running on server
- Sets up cancellation watcher
- Calls runTask() (via d.runner)
- Handles result and reports back

**runTask()** - Core execution (2200+ lines):

```go
func (d *Daemon) runTask(ctx context.Context, task Task, provider string, slot int, taskLog *slog.Logger) (TaskResult, error) {
    // 1. Validate task
    if task.WorkspaceID == "" {
        return TaskResult{}, fmt.Errorf("refusing to spawn agent: task has no workspace_id")
    }
    
    // 2. Register task repos (ensure they're in workspace allowlist + cache)
    d.registerTaskRepos(task.WorkspaceID, task.Repos)
    
    // 3. Get agent entry from config
    entry, ok := d.cfg.Agents[provider]
    if !ok {
        return TaskResult{}, fmt.Errorf("no agent configured for provider %q", provider)
    }
    
    // 4. Build task context for environment files
    taskCtx := execenv.TaskContextForEnv{
        IssueID:                 task.IssueID,
        AgentID:                 task.Agent.ID,
        AgentName:               task.Agent.Name,
        AgentInstructions:       task.Agent.Instructions,
        AgentSkills:             convertSkillsForEnv(task.Agent.Skills),
        Repos:                   convertReposForEnv(task.Repos),
        ProjectID:               task.ProjectID,
        ProjectTitle:            task.ProjectTitle,
        ProjectResources:        convertProjectResourcesForEnv(task.ProjectResources),
        // ... (all task metadata injected)
    }
    
    // 5. Mark environment roots as active (prevent GC during execution)
    predictedRoot := execenv.PredictRootDir(d.cfg.WorkspacesRoot, task.WorkspaceID, task.ID)
    d.markActiveEnvRoot(predictedRoot)
    defer d.unmarkActiveEnvRoot(predictedRoot)
    
    // 6. Try to reuse prior workdir, else prepare fresh environment
    var env *execenv.Environment
    if task.PriorWorkDir != "" {
        env = execenv.Reuse(execenv.ReuseParams{
            WorkDir:      task.PriorWorkDir,
            Provider:     provider,
            CodexVersion: d.agentVersion("codex"),
            Task:         taskCtx,
        }, d.logger)
    }
    if env == nil {
        env, err = execenv.Prepare(execenv.PrepareParams{
            WorkspacesRoot: d.cfg.WorkspacesRoot,
            WorkspaceID:    task.WorkspaceID,
            TaskID:         task.ID,
            AgentName:      agentName,
            Provider:       provider,
            CodexVersion:   d.agentVersion("codex"),
            Task:           taskCtx,
        }, d.logger)
        if err != nil {
            return TaskResult{}, fmt.Errorf("prepare execution environment: %w", err)
        }
    }
    
    // 7. Inject runtime config (skills, CLI guidance)
    runtimeBrief, err := execenv.InjectRuntimeConfig(env.WorkDir, provider, taskCtx)
    if err != nil {
        d.logger.Warn("execenv: inject runtime config failed (non-fatal)", "error", err)
    }
    
    // 8. Build prompt from task
    prompt := BuildPrompt(task, provider)
    
    // 9. Prepare agent environment variables
    agentEnv := map[string]string{
        "MULTICA_TOKEN":        d.client.Token(),
        "MULTICA_SERVER_URL":   d.cfg.ServerBaseURL,
        "MULTICA_DAEMON_PORT":  fmt.Sprintf("%d", d.cfg.HealthPort),
        "MULTICA_WORKSPACE_ID": task.WorkspaceID,
        "MULTICA_AGENT_NAME":   agentName,
        "MULTICA_AGENT_ID":     task.AgentID,
        "MULTICA_TASK_ID":      task.ID,
        "MULTICA_TASK_SLOT":    strconv.Itoa(slot),
        // ... add custom env, model overrides, etc.
    }
    
    // 10. Create agent backend (dispatches to provider-specific impl)
    backend, err := agent.New(provider, agent.Config{
        ExecutablePath: entry.Path,
        Env:            agentEnv,
        Logger:         d.logger,
    })
    if err != nil {
        return TaskResult{}, fmt.Errorf("create agent backend: %w", err)
    }
    
    // 11. Execute agent (stream results, drain tool calls)
    result, tools, err := d.executeAndDrain(ctx, backend, prompt, execOpts, taskLog, task.ID)
    if err != nil {
        return TaskResult{}, err
    }
    
    // 12. Handle session resume failures (retry with fresh session)
    if result.Status == "failed" && task.PriorSessionID != "" && result.SessionID == "" {
        // Session resume failed before establishing connection
        // Retry with fresh session...
    }
    
    // 13. Convert token usage to task entries
    var usageEntries []TaskUsageEntry
    for model, u := range result.Usage {
        usageEntries = append(usageEntries, TaskUsageEntry{
            Provider:         provider,
            Model:            model,
            InputTokens:      u.InputTokens,
            OutputTokens:     u.OutputTokens,
            CacheReadTokens:  u.CacheReadTokens,
            CacheWriteTokens: u.CacheWriteTokens,
        })
    }
    
    // 14. Process result status
    switch result.Status {
    case "completed":
        return TaskResult{
            Status:    "completed",
            Comment:   result.Output,
            SessionID: result.SessionID,
            WorkDir:   env.WorkDir,
            EnvRoot:   env.RootDir,
            Usage:     usageEntries,
        }, nil
    case "failed":
        // Check for "poisoned" output (iteration limit, fallback markers)
        // Route through blocked path if detected
        return TaskResult{
            Status:        "blocked",
            Comment:       result.Output,
            SessionID:     result.SessionID,
            WorkDir:       env.WorkDir,
            EnvRoot:       env.RootDir,
            FailureReason: "agent_error",  // or specific reason
            Usage:         usageEntries,
        }, nil
    case "timeout":
        return TaskResult{
            Status:        "blocked",
            Comment:       "Agent timeout",
            FailureReason: "timeout",
            WorkDir:       env.WorkDir,
            EnvRoot:       env.RootDir,
            Usage:         usageEntries,
        }, nil
    case "aborted":
        return TaskResult{
            Status:        "blocked",
            Comment:       "Task cancelled",
            FailureReason: "cancelled",
            WorkDir:       env.WorkDir,
            EnvRoot:       env.RootDir,
            Usage:         usageEntries,
        }, nil
    }
}
```

---

## 3. Daemon Communication with Server

### 3.1 Client Architecture (`client.go`)

HTTP/WebSocket client with auth + identity headers:

```go
type Client struct {
    baseURL string                  // ws://localhost:8080/ws or http://...
    token   string                  // Auth token from CLI config
    client  *http.Client
    
    // Identity headers (X-Client-Platform, X-Client-Version, X-Client-OS)
    platform string                 // "daemon"
    version  string                 // CLI version
    os       string                 // "macos", "windows", "linux"
}
```

### 3.2 Key API Endpoints

**Task Lifecycle:**
- `POST /api/daemon/runtimes/{runtime_id}/tasks/claim` → Claim next task
- `POST /api/daemon/tasks/{task_id}/start` → Mark task running
- `POST /api/daemon/tasks/{task_id}/complete` → Mark task complete with output
- `POST /api/daemon/tasks/{task_id}/fail` → Mark task failed with error
- `POST /api/daemon/tasks/{task_id}/status` → Get current task status (for cancellation watch)

**Task Execution Progress:**
- `POST /api/daemon/tasks/{task_id}/progress` → Report progress (summary, step, total)
- `POST /api/daemon/tasks/{task_id}/messages` → Batch export agent messages (text, tool-use, etc.)
- `POST /api/daemon/tasks/{task_id}/usage` → Report token usage per model
- `POST /api/daemon/tasks/{task_id}/session` → Pin session_id/work_dir mid-flight

**Session Recovery:**
- `POST /api/daemon/runtimes/{runtime_id}/recover-orphans` → Fail tasks from crashed daemon

**Heartbeat (HTTP or WebSocket):**
- `POST /api/daemon/heartbeat` → Heartbeat pulse, get pending actions

**Pending Actions Response:**
```go
type HeartbeatResponse struct {
    PendingUpdate           []PendingUpdate           // CLI auto-update
    PendingModelList        []PendingModelList        // Model discovery
    PendingLocalSkills      []PendingLocalSkills      // Local skills list
    PendingLocalSkillImport []PendingLocalSkillImport // Local skill import
    // ...
}
```

### 3.3 WebSocket Communication (`wakeup.go`)

**Real-time task wakeup (opt-in to polling):**
- Connection: `GET /ws?runtime_ids=id1,id2,...` (upgrade to WebSocket)
- Receives: `{ "task_ready": true }` when task is available (faster than poll)
- Heartbeat acks: HTTP fallback when WebSocket unavailable
- Runtime gone recovery: WebSocket ack with `RuntimeGone=true`

```go
func (d *Daemon) readTaskWakeupMessages(conn *websocket.Conn, taskWakeups chan<- struct{}) error {
    // Receive messages:
    // { "task_ready": true }  → trigger immediate poll
    // { "runtime_gone": <id> } → runtime deleted server-side
    // Heartbeat ack with RuntimeGone=true → trigger re-register
}
```

---

## 4. Agent Runtime and Execution Environment

### 4.1 Agent Backend Interface (`server/pkg/agent/agent.go`)

Unified interface for all agent types:

```go
type Backend interface {
    Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error)
}

type ExecOptions struct {
    Cwd                       string
    Model                     string
    SystemPrompt              string              // for system-prompt-capable agents
    MaxTurns                  int
    Timeout                   time.Duration
    SemanticInactivityTimeout time.Duration      // Codex-specific
    ResumeSessionID           string              // resume session
    ExtraArgs                 []string            // daemon-wide defaults
    CustomArgs                []string            // per-agent overrides
    McpConfig                 json.RawMessage     // MCP server config
    ThinkingLevel             string              // "low"|"high"|"xhigh" etc.
}

type Session struct {
    Messages <-chan Message                       // Stream of events
    Result   <-chan Result                        // Final outcome
}

type Message struct {
    Type      MessageType  // "text", "thinking", "tool-use", "tool-result", "status", "error", "log"
    Content   string       // text content
    Tool      string       // tool name
    CallID    string       // tool call ID
    Input     map[string]any  // tool input
    Output    string       // tool output
    Status    string       // agent status string
    Level     string       // log level
    SessionID string       // backend session id (for early pinning)
}

type Result struct {
    Status     string                  // "completed", "failed", "aborted", "timeout", "cancelled"
    Output     string                  // accumulated text output
    Error      string                  // error message if failed
    DurationMs int64
    SessionID  string
    Usage      map[string]TokenUsage   // keyed by model name
}
```

**Supported Providers:**
- `claude` - Claude Code CLI
- `codex` - Codex app-server
- `copilot` - GitHub Copilot
- `opencode` - OpenCode
- `openclaw` - OpenClaw
- `hermes` - Hermes ACP
- `gemini` - Google Gemini
- `pi` - Pi coding agent
- `cursor` - Cursor agent
- `kimi` - Kimi Code
- `kiro` - Kiro CLI

### 4.2 Execution Environment Setup (`server/internal/daemon/execenv/`)

Each task gets an isolated directory structure:

```
{workspacesRoot}/{workspaceID}/{taskID_short}/
├── workdir/                    # Agent's working directory (CWD)
│   ├── CLAUDE.md               # Claude-specific context (or AGENTS.md for others)
│   ├── AGENTS.md               # Copilot/OpenCode/OpenClaw/Hermes/Pi/Cursor context
│   ├── GEMINI.md               # Gemini-specific context
│   ├── .multica/               # Multica runtime files
│   │   ├── project/
│   │   │   └── resources.json  # Attached project resources (structured)
│   │   └── skills/             # Injected skills (Multica-provided + project-level)
│   │       ├── skill1.md
│   │       ├── skill2/
│   │       │   └── SKILL.md
│   │       └── ...
│   ├── .claude/                # Claude-specific skill discovery
│   │   └── skills/
│   ├── .github/                # Copilot skill discovery
│   │   └── skills/
│   ├── .config/opencode/       # OpenCode skill discovery
│   │   └── skills/
│   ├── .openclaw/              # OpenClaw skill discovery
│   │   └── skills/
│   ├── .pi/                    # Pi skill discovery
│   │   └── agent/skills/
│   ├── .cursor/                # Cursor skill discovery
│   │   └── skills/
│   ├── .kiro/                  # Kiro skill discovery
│   │   └── skills/
│   └── [checked-out repos]     # Via `multica repo checkout <url>`
├── codex-home/                 # CODEX_HOME (per-task Codex config + skills)
│   └── skills/
├── output/                     # For output files
├── logs/                       # Task execution logs
└── openclaw-config.json        # Synthesized OpenClaw config (pins workspace to workdir)
```

**Context Files Generated:**

1. **CLAUDE.md** (Claude) / **AGENTS.md** (most others)
   - Agent identity + instructions
   - Requesting user profile context
   - Available commands (multica CLI reference)
   - Repositories available for checkout
   - Project context + attached resources
   - Provider-specific guidance (Codex Windows note, etc.)

2. **Project Resources** (`.multica/project/resources.json`)
   - Structured JSON of attached resources
   - Resource type-specific payloads
   - Human-readable labels

### 4.3 Configuration Loading (`config.go`)

**Config Struct:**
```go
type Config struct {
    ServerBaseURL                  string
    DaemonID                       string
    LegacyDaemonIDs                []string  // Historical IDs for runtime merge
    DeviceName                     string
    RuntimeName                    string    // Display name (default: "Local Agent")
    CLIVersion                     string
    LaunchedBy                     string    // "desktop" if from Electron
    Profile                        string    // Profile name
    Agents                         map[string]AgentEntry  // provider -> path + model override
    WorkspacesRoot                 string    // Task directory base
    KeepEnvAfterTask               bool
    HealthPort                     int       // Health check port (default: 19514)
    MaxConcurrentTasks             int       // Default: 20
    GCEnabled                      bool
    GCInterval                     time.Duration
    GCTTL                          time.Duration         // 24h default
    GCOrphanTTL                    time.Duration         // 72h default
    GCArtifactTTL                  time.Duration         // 12h default
    GCArtifactPatterns             []string  // node_modules, .next, .turbo (regenerable)
    AutoUpdateEnabled              bool
    AutoUpdateCheckInterval        time.Duration         // 6h default
    PollInterval                   time.Duration         // 30s default
    HeartbeatInterval              time.Duration         // 15s default
    AgentTimeout                   time.Duration         // 2h default
    CodexSemanticInactivityTimeout time.Duration         // 10m default
    AgentIdleWatchdog              time.Duration         // 30m default (0 = disabled)
    ClaudeArgs                     []string
    CodexArgs                      []string
}
```

**Agent Detection:**
- Probes PATH for agent CLIs: `claude`, `codex`, `copilot`, `opencode`, `openclaw`, `hermes`, `gemini`, `pi`, `cursor-agent`, `kimi`, `kiro-cli`
- Per-agent model overrides: `MULTICA_CLAUDE_MODEL`, `MULTICA_CODEX_MODEL`, etc.
- Explicit paths: `MULTICA_CLAUDE_PATH`, `MULTICA_CODEX_PATH`, etc.
- Shell fallback: When bare command misses PATH, spawns user's login shell to resolve

---

## 5. Skills System

### 5.1 Skill Data Structure

**From Task (Multica-provided):**
```go
type SkillData struct {
    Name        string          // "GitHub PR Review"
    Description string          // "Review pull requests on GitHub"
    Content     string          // Markdown content
    Files       []SkillFileData // Supporting files
}

type SkillFileData struct {
    Path    string  // "tools/check_pr.ts"
    Content string  // File content
}
```

**Local Skills (User-installed):**
```go
type runtimeLocalSkillSummary struct {
    Key         string  // "nested/skill/path"
    Name        string
    Description string
    SourcePath  string  // "~/.claude/skills/myskill"
    Provider    string  // "claude", "codex", etc.
    FileCount   int
}
```

### 5.2 Skill Discovery Paths

Per-provider skill roots:
- **Claude**: `~/.claude/skills/`
- **Codex**: `$CODEX_HOME/skills/` (default `~/.codex/skills/`)
- **Copilot**: `~/.copilot/skills/`
- **OpenCode**: `~/.config/opencode/skills/`
- **OpenClaw**: `~/.openclaw/skills/`
- **Pi**: `~/.pi/agent/skills/`
- **Cursor**: `~/.cursor/skills/`
- **Kiro**: `~/.kiro/skills/`

**Discovery Mechanism:**
- Each skill is a directory with `SKILL.md` (main file)
- Frontmatter in `SKILL.md` contains name/description:
  ```markdown
  ---
  name: "My Skill"
  description: "Does something useful"
  ---
  # Skill content...
  ```
- Supporting files walk up to 4 levels deep
- Symlinks followed (supports symlinked skill installations like lark-cli)
- Limits: 128 files max per skill, 8MB total per skill, 1MB per file

### 5.3 Local Skills API

**Endpoints:**
- `GET /api/daemon/runtimes/{id}/local-skills` → List available local skills for provider
- `POST /api/daemon/runtimes/{id}/local-skills/{skill_key}/import` → Load skill bundle
- `POST /api/daemon/runtimes/{id}/local-skills/{request_id}/result` → Report results

**Heartbeat Pending Actions:**
```go
type PendingLocalSkills struct {
    RequestID string  // UUID for result reporting
}

type PendingLocalSkillImport struct {
    RequestID string  // UUID for result reporting
    SkillKey  string  // "category/skill" path
}
```

---

## 6. Message Streaming and Tool Execution

### 6.1 Agent Message Types

During execution, agents emit a stream of typed messages:

```go
const (
    MessageText       MessageType = "text"       // Human-readable text
    MessageThinking   MessageType = "thinking"   // Extended thinking/reasoning
    MessageToolUse    MessageType = "tool-use"   // Agent calling a tool
    MessageToolResult MessageType = "tool-result" // Result of tool execution
    MessageStatus     MessageType = "status"     // Agent status update + early session_id
    MessageError      MessageType = "error"      // Error occurred
    MessageLog        MessageType = "log"        // Debug log
)
```

### 6.2 Message Batching

**ReportTaskMessages()** - Batch export of all messages for archival:
```go
type TaskMessageData struct {
    Seq     int            // Sequence number
    Type    string         // "text", "tool-use", "tool-result", etc.
    Tool    string         // Tool name (for tool-use/tool-result)
    Content string         // Text content
    Input   map[string]any // Tool input
    Output  string         // Tool output
}

POST /api/daemon/tasks/{task_id}/messages {
    "messages": [...]
}
```

---

## 7. Concurrency and Synchronization

### 7.1 Semaphore-Based Rate Limiting

```go
// Allocate N slots (default 20)
sem := make(chan int, MaxConcurrentTasks)
for i := 0; i < MaxConcurrentTasks; i++ {
    sem <- i
}

// In poller: acquire slot before claiming
slot := <-sem  // Blocks if at capacity
defer func() { sem <- slot }()  // Release after task completes
```

**MULTICA_TASK_SLOT** environment variable identifies which slot agent is running on.

### 7.2 Auto-Update Barrier

Prevents new task claims during CLI restart:

```go
// Poller: Check barrier before claiming
if !d.tryEnterClaim() {
    // Auto-update in progress, sleep
    continue
}
defer d.exitClaim()

// Auto-update: Wait for current claims to complete
d.claimMu.Lock()
d.pauseClaims = true
d.claimMu.Unlock()
// ... perform update ...
// Clear when done
```

### 7.3 Cancellation Watching

Per-task goroutine polls server status:

```go
func (d *Daemon) watchTaskCancellation(ctx context.Context, taskID string, pollInterval time.Duration) <-chan struct{} {
    cancelled := make(chan struct{})
    go func() {
        ticker := time.NewTicker(pollInterval)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                status, err := d.client.GetTaskStatus(ctx, taskID)
                if status == "cancelled" || isTaskNotFoundError(err) {
                    close(cancelled)  // Signal agent to stop
                    return
                }
            }
        }
    }()
    return cancelled
}
```

---

## 8. Runtime Registration and Lifecycle

### 8.1 Registration Flow

When daemon starts:

```go
1. Fetch all user workspaces via API
2. For each workspace:
   - For each configured agent provider:
     - Detect CLI version via `{agent} --version`
     - Validate minimum version
     - POST /api/daemon/register with metadata
   → Server creates runtime row per provider
3. Maintain runtimeIndex: runtime_id -> provider + name

Register request:
{
    "workspace_id": "ws-123",
    "daemon_id": "daemon-uuid",
    "legacy_daemon_ids": [...],
    "device_name": "my-laptop",
    "runtime_name": "Local Agent",
    "runtimes": [
        {
            "provider": "claude",
            "name": "Claude",
            "version": "1.2.3"
        },
        ...
    ]
}
```

### 8.2 Runtime Gone Recovery

When server deletes a runtime row (UI delete, 7-day offline GC):

**Detection:**
- HTTP heartbeat returns 404 "runtime not found"
- OR WebSocket receives `{ "runtime_gone": <id> }`

**Recovery:**
```go
func (d *Daemon) handleRuntimeGone(runtimeID string) {
    // 1. Remove from local state
    workspaceID, removed := d.removeStaleRuntime(runtimeID)
    
    // 2. Coalesce recovery attempts (30s window)
    if !d.tryClaimRegisterSlot(workspaceID, entryAt, now) {
        // Another recovery already in flight
        return
    }
    
    // 3. Re-register workspace (creates new runtime rows)
    err := d.reregisterWorkspaceAfterRuntimeGone(d.recoveryContext(), workspaceID)
    
    // 4. Record result (clear slot on success, extend backoff on failure)
    d.recordRegisterCompletion(workspaceID, now, err)
}
```

---

## 9. Heartbeat and Health Monitoring

### 9.1 Heartbeat Loop

```go
func (d *Daemon) heartbeatLoop(ctx context.Context) {
    ticker := time.NewTicker(d.cfg.HeartbeatInterval)  // 15s default
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            for _, runtimeID := range d.allRuntimeIDs() {
                go d.sendHeartbeat(ctx, runtimeID)
            }
        }
    }
}

func (d *Daemon) sendHeartbeat(ctx context.Context, runtimeID string) {
    // Via HTTP:
    resp, err := d.client.SendHeartbeat(ctx, runtimeID)
    // OR via WebSocket (taskWakeupLoop establishes and maintains connection)
    
    // Response contains pending actions:
    // - CLI updates to download
    // - Model list discovery requests
    // - Local skills inventory requests
    // - Local skill import requests
    
    // Ack with results:
    d.client.ReportUpdateResult(ctx, runtimeID, updateID, result)
    d.client.ReportModelListResult(ctx, runtimeID, requestID, result)
    d.client.ReportLocalSkillListResult(ctx, runtimeID, requestID, result)
    d.client.ReportLocalSkillImportResult(ctx, runtimeID, requestID, result)
}
```

### 9.2 Health Check Endpoint

HTTP health endpoint (`:19514` default):

- `GET /health` → JSON with daemon state
  - Active tasks
  - Runtime statuses
  - Memory usage
  - Workspace sync status
  - Last heartbeat timestamps

---

## 10. Garbage Collection

### 10.1 GC Loop

```go
func (d *Daemon) gcLoop(ctx context.Context) {
    ticker := time.NewTicker(d.cfg.GCInterval)  // 1h default
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            d.performGC(ctx)
        }
    }
}

func (d *Daemon) performGC(ctx context.Context) {
    // Clean task dirs based on:
    
    // 1. Issue status + TTL
    // - Issue done/cancelled and updated_at < now()-GCTTL (24h) → delete
    // - Issue missing (404 from gc-check) → check age
    //   - If orphan or > GCOrphanTTL (72h) → delete
    
    // 2. Artifact cleanup
    // - Task completed > GCArtifactTTL (12h) ago + issue still open
    // - Delete regenerable artifacts: node_modules, .next, .turbo (customizable)
    
    // 3. Active env protection
    // - Never clean directories in activeEnvRoots map
    // - Prevents deletion of in-flight task dirs
}
```

---

## 11. Auto-Update Mechanism

### 11.1 Auto-Update Flow

```go
func (d *Daemon) autoUpdateLoop(ctx context.Context) {
    ticker := time.NewTicker(d.cfg.AutoUpdateCheckInterval)  // 6h default
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if d.updating.Load() {
                continue  // Skip if already updating
            }
            d.tryAutoUpdate(ctx)
        }
    }
}

func (d *Daemon) tryAutoUpdate(ctx context.Context, targetVersion string) error {
    // 1. Pause new task claims
    d.claimMu.Lock()
    if d.claimsInFlight > 0 {
        // Tasks in flight, defer update
        d.claimMu.Unlock()
        return nil
    }
    d.pauseClaims = true
    d.claimMu.Unlock()
    
    // 2. Wait for running tasks to complete (taskWG.Wait)
    d.taskWG.Wait()
    
    // 3. Download/install new binary
    newPath, err := d.runUpdateFn(targetVersion)  // Via brew or direct download
    
    // 4. Record path + signal restart via context cancel
    d.restartBinary = newPath
    d.cancelFunc()
}
```

---

## 12. Session Resumption

### 12.1 Session-Aware Task Flow

**First Run:**
1. Agent executes with empty `ResumeSessionID`
2. Backend returns `session_id` from first message (MessageStatus)
3. Daemon calls `PinTaskSession()` immediately (mid-flight, before task completion)
4. Result includes `SessionID` and `WorkDir`

**Resume Run (same agent on same issue):**
1. Server includes `PriorSessionID` and `PriorWorkDir` in claimed task
2. Daemon attempts to reuse `PriorWorkDir` via `execenv.Reuse()`
3. Agent runs with `ExecOptions.ResumeSessionID` set
4. Backend loads session state and resumes

**Resume Failure Handling:**
```go
if result.Status == "failed" && task.PriorSessionID != "" && result.SessionID == "" {
    // Session resume failed before establishing connection
    // Retry entire task with fresh session
    execOpts.ResumeSessionID = ""
    retryResult, _, _ := d.executeAndDrain(ctx, backend, prompt, execOpts, taskLog, task.ID)
    // Merge usage from both attempts
    result = retryResult
    result.Usage = mergeUsage(firstUsage, result.Usage)
}
```

---

## 13. CLI Commands Related to Daemon

### 13.1 Main Daemon Commands

Located in `server/cmd/multica/cmd_daemon.go`:

```bash
multica daemon start         # Start daemon in background
multica daemon stop          # Stop running daemon
multica daemon restart       # Stop + start
multica daemon status        # Show daemon status (agents, runtimes, tasks)
multica daemon logs          # Show daemon logs (with -f to follow)
multica daemon disk-usage    # Disk usage by task or workspace
```

**Start Flags:**
```bash
--foreground                 # Run in foreground (don't daemonize)
--daemon-id <id>            # Unique daemon ID
--device-name <name>        # Human-readable device name
--runtime-name <name>       # Runtime display name
--poll-interval <duration>  # Task poll interval (default 30s)
--heartbeat-interval <duration>  # Heartbeat interval (default 15s)
--agent-timeout <duration>  # Per-task timeout (default 2h)
--codex-semantic-inactivity-timeout <duration>  # Codex idle timeout
--max-concurrent-tasks <n>  # Max parallel tasks (default 20)
--no-auto-update            # Disable CLI auto-update
--auto-update-interval <duration>  # Update check interval
```

### 13.2 Configuration via Environment

```bash
MULTICA_SERVER_URL=...      # Server URL
MULTICA_DAEMON_ID=...       # Daemon ID
MULTICA_DAEMON_DEVICE_NAME=...
MULTICA_AGENT_RUNTIME_NAME=...
MULTICA_DAEMON_POLL_INTERVAL=...
MULTICA_DAEMON_HEARTBEAT_INTERVAL=...
MULTICA_AGENT_TIMEOUT=...
MULTICA_AGENT_IDLE_WATCHDOG=...  # Force-stop if silent > duration
MULTICA_DAEMON_MAX_CONCURRENT_TASKS=...
MULTICA_DAEMON_AUTO_UPDATE=false   # Disable
MULTICA_DAEMON_AUTO_UPDATE_INTERVAL=...
MULTICA_WORKSPACES_ROOT=~/multica_workspaces

# Per-provider
MULTICA_CLAUDE_PATH=claude
MULTICA_CLAUDE_MODEL=claude-opus-4-1
MULTICA_CODEX_PATH=codex
MULTICA_CODEX_MODEL=...
# ... similar for other agents

# Garbage collection
MULTICA_GC_ENABLED=true
MULTICA_GC_INTERVAL=1h
MULTICA_GC_TTL=24h
MULTICA_GC_ORPHAN_TTL=72h
MULTICA_GC_ARTIFACT_TTL=12h
MULTICA_GC_ARTIFACT_PATTERNS=node_modules,.next,.turbo
```

---

## 14. Execution Model Summary

### Complete Task Execution Pipeline

```
┌─────────────────────────────────────────────────────────────┐
│ SERVER: Create issue → Dispatch to agent                    │
└────────────────────┬────────────────────────────────────────┘
                     │
                     v
┌─────────────────────────────────────────────────────────────┐
│ DAEMON: ClaimTask (HTTP POST or WebSocket wakeup)           │
│ ├─ Acquire semaphore slot (max concurrent)                  │
│ ├─ Check auto-update pause flag                             │
│ └─ POST /api/daemon/runtimes/{id}/tasks/claim               │
└────────────────────┬────────────────────────────────────────┘
                     │
                     v
┌─────────────────────────────────────────────────────────────┐
│ DAEMON: Receive Task + AgentData (skills, instructions)     │
└────────────────────┬────────────────────────────────────────┘
                     │
                     v
┌─────────────────────────────────────────────────────────────┐
│ DAEMON: handleTask() goroutine spawned                       │
│ ├─ StartTask() → POST /api/daemon/tasks/{id}/start          │
│ └─ Launch cancellation watcher (poll GetTaskStatus)         │
└────────────────────┬────────────────────────────────────────┘
                     │
                     v
┌─────────────────────────────────────────────────────────────┐
│ DAEMON: Prepare Execution Environment                       │
│ ├─ Create isolated directory                                │
│ ├─ Write context files (CLAUDE.md / AGENTS.md)              │
│ ├─ Inject task skills (SkillData array)                     │
│ ├─ Try reuse prior workdir, else fresh                      │
│ └─ Mark env root as active (prevent GC)                     │
└────────────────────┬────────────────────────────────────────┘
                     │
                     v
┌─────────────────────────────────────────────────────────────┐
│ DAEMON: Create Agent Backend (provider-specific)            │
│ ├─ Detect provider (claude, codex, copilot, etc.)           │
│ ├─ Set environment vars (MULTICA_* + custom_env)            │
│ └─ Instantiate agent.Backend for provider                   │
└────────────────────┬────────────────────────────────────────┘
                     │
                     v
┌─────────────────────────────────────────────────────────────┐
│ DAEMON: Execute Agent via Backend.Execute()                 │
│ ├─ Spawn agent CLI process                                  │
│ ├─ Stream messages (text, thinking, tool-use, etc.)         │
│ ├─ Timeout: 2h or codex semantic inactivity timeout         │
│ └─ Interrupt on cancellation signal or context cancel       │
└────────────────────┬────────────────────────────────────────┘
                     │
                     v
┌─────────────────────────────────────────────────────────────┐
│ AGENT: Run in Isolated Environment                          │
│ ├─ Load context files (CLAUDE.md, skills, repos)            │
│ ├─ Emit tool calls (multica CLI commands)                   │
│ ├─ Manage issue state via multica CLI                       │
│ └─ Stream output + decisions                                │
└────────────────────┬────────────────────────────────────────┘
                     │
                     v
┌─────────────────────────────────────────────────────────────┐
│ DAEMON: Drain and Report Results                            │
│ ├─ ReportProgress() - optional status updates               │
│ ├─ ReportTaskMessages() - batch message export              │
│ ├─ PinTaskSession() - persist session_id mid-flight         │
│ └─ Poll cancellation status continuously                    │
└────────────────────┬────────────────────────────────────────┘
                     │
                     v
┌─────────────────────────────────────────────────────────────┐
│ AGENT: Completes (status: completed/failed/timeout)         │
└────────────────────┬────────────────────────────────────────┘
                     │
                     v
┌─────────────────────────────────────────────────────────────┐
│ DAEMON: Process Result                                      │
│ ├─ On success:                                              │
│ │  ├─ CompleteTask() + comment output                       │
│ │  └─ ReportTaskUsage() - token consumption                 │
│ ├─ On failure/timeout:                                      │
│ │  └─ FailTask() with error reason + usage                 │
│ ├─ Unmark active env root (allow GC)                        │
│ └─ Release semaphore slot                                   │
└────────────────────┬────────────────────────────────────────┘
                     │
                     v
┌─────────────────────────────────────────────────────────────┐
│ SERVER: Task marked completed/blocked                       │
│ ├─ Issue status updated                                     │
│ ├─ Comment posted with agent output                         │
│ └─ Session + workdir cached for resume                      │
└─────────────────────────────────────────────────────────────┘
```

---

## 15. Key Design Patterns

### 15.1 Concurrency Patterns

1. **Semaphore slots** - Rate limit concurrent agents
2. **Goroutine pools** - Per-runtime pollers + per-task handlers
3. **Context cancellation** - Clean shutdown via context.CancelFunc
4. **Mutex-protected maps** - Runtime index, workspace state, versions
5. **WaitGroup synchronization** - Track task completion for updates

### 15.2 Error Handling

1. **Runtime gone recovery** - Coalesced re-register with backoff
2. **Session resume fallback** - Retry with fresh session on 404
3. **Claim pause during updates** - Prevent new tasks during restart
4. **Task cancellation watching** - Poll server for task status changes
5. **Poison detection** - Mark iteration limits as blocked, not completed

### 15.3 Resilience Features

1. **Session resumption** - Continue agent work across daemon restarts
2. **Workspace root protection** - Never GC active env directories
3. **Orphan recovery** - Clean up tasks left running after daemon crash
4. **Auto-update safety** - Defer updates until running tasks complete
5. **Heartbeat feedback loop** - Get pending actions (updates, model lists, skills)

---

## 16. Data Flow Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                         MULTICA SERVER                           │
│ ┌────────────────────────────────────────────────────────────┐   │
│ │  Issue / Chat Session / Autopilot Run                       │   │
│ │  ├─ Dispatch to Agent + Runtime                            │   │
│ │  └─ Create Task row (status: dispatched)                   │   │
│ └────────────────────────────────────────────────────────────┘   │
└──────────────────┬───────────────────────┬──────────────────────┘
                   │                       │
         ClaimTask │                       │ Heartbeat (15s)
         (WebSocket wakeup or HTTP poll)   │ (detect pending actions)
                   │                       │
                   v                       v
┌──────────────────────────────────────────────────────────────────┐
│                          LOCAL DAEMON                            │
│ ┌────────────────────────────────────────────────────────────┐   │
│ │ pollLoop                                                    │   │
│ │ ├─ For each runtime: runRuntimePoller (goroutine)          │   │
│ │ │  ├─ ClaimTask (HTTP POST)                               │   │
│ │ │  ├─ handleTask (goroutine per task)                     │   │
│ │ │  │  ├─ StartTask()                                      │   │
│ │ │  │  ├─ runTask()                                        │   │
│ │ │  │  │  ├─ Prepare execenv                              │   │
│ │ │  │  │  ├─ InjectRuntimeConfig (skills, context)        │   │
│ │ │  │  │  ├─ Backend.Execute (spawn agent)                │   │
│ │ │  │  │  ├─ Drain messages + report progress            │   │
│ │ │  │  │  └─ CompleteTask() / FailTask()                  │   │
│ │ │  │  └─ Report usage, session, workdir                  │   │
│ │ │  └─ Sleep or wakeup (poll interval or WebSocket)       │   │
│ │ │                                                         │   │
│ │ └─ Background loops:                                      │   │
│ │    ├─ taskWakeupLoop (WebSocket to server)               │   │
│ │    ├─ heartbeatLoop (HTTP POST to server)                │   │
│ │    ├─ gcLoop (clean old task dirs)                       │   │
│ │    ├─ autoUpdateLoop (check for new CLI)                 │   │
│ │    └─ workspaceSyncLoop (discover new workspaces)        │   │
│ │                                                         │   │
│ └────────────────────────────────────────────────────────────┘   │
│                                                                  │
│ ┌────────────────────────────────────────────────────────────┐   │
│ │ Task Execution Environment                                 │   │
│ │ {workspacesRoot}/{workspaceID}/{taskID}/                  │   │
│ │ ├─ workdir/                                               │   │
│ │ │  ├─ CLAUDE.md / AGENTS.md (context brief)              │   │
│ │ │  ├─ .multica/ (runtime files + skills)                 │   │
│ │ │  ├─ .claude/.github/.openclaw/... (provider skills)    │   │
│ │ │  └─ [checked-out repos]                                │   │
│ │ ├─ codex-home/ (Codex per-task config)                   │   │
│ │ ├─ openclaw-config.json (synthesized config)             │   │
│ │ └─ output/, logs/                                        │   │
│ │                                                         │   │
│ └────────────────────────────────────────────────────────────┘   │
│                                                                  │
│ ┌────────────────────────────────────────────────────────────┐   │
│ │ Agent CLIs (on PATH)                                       │   │
│ │ ├─ claude (Claude Code)                                   │   │
│ │ ├─ codex (Codex app-server)                               │   │
│ │ ├─ copilot (GitHub Copilot)                               │   │
│ │ ├─ opencode (OpenCode)                                   │   │
│ │ ├─ openclaw (OpenClaw)                                   │   │
│ │ ├─ hermes (Hermes ACP)                                   │   │
│ │ ├─ gemini (Google Gemini)                                │   │
│ │ ├─ pi (Pi)                                                │   │
│ │ ├─ cursor-agent (Cursor)                                 │   │
│ │ ├─ kimi (Kimi)                                            │   │
│ │ └─ kiro-cli (Kiro)                                        │   │
│ │                                                         │   │
│ └────────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────┘
                        ↓
            CompleteTask() / FailTask()
            ReportTaskUsage()
            ReportProgress()
                        ↓
            Issue comment + metadata
            updated, task marked complete
```

---

## Key Takeaways

1. **Daemon as Orchestrator** - Multica daemon polls for tasks and delegates to local AI agent CLIs
2. **Skill Injection System** - Both Multica-provided and local user skills injected into execution environment
3. **Isolated Environments** - Each task gets its own workdir with context files + skills
4. **Session Resumption** - Tasks can resume from prior session_id + work_dir for multi-turn work
5. **Concurrency Control** - Semaphore-based slot allocation + auto-update barriers
6. **Robust Recovery** - Runtime gone handling, orphan task recovery, session resume fallback
7. **Message Streaming** - Real-time tool-use tracking, batch message export
8. **Real-time Wakeup** - WebSocket for instant task notifications (falls back to polling)
9. **Provider Abstraction** - Unified Backend interface supporting 11+ agent types
10. **GC and Cleanup** - Periodic workspace cleanup with TTLs and artifact patterns
