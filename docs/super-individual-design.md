# "超级个体" 系统设计文档

> 基于 Multica 平台改造，实现端到端交付全栈项目的 AI 需求交付系统

## 一、课题目标对齐

### 1.1 核心链路

```
需求澄清 → 方案拆解 → 模块定位 → 代码生成 → 自动化测试 → 提测(PR)
```

### 1.2 目标仓库

[Conduit](https://github.com/TonyMckes/conduit-realworld-example-app) — React 18 + Vite + Express 4 + Sequelize + PostgreSQL 单仓全栈博客。

### 1.3 AI 模型

- 主模型：doubao-seed-2.0 lite（火山方舟）
- EP：`ep-20260514110933-mzh58`
- Endpoint：`https://ark.cn-beijing.volces.com/api/v3`

### 1.4 评分维度覆盖

| 评分维度 | 权重 | 本方案覆盖 |
|---------|------|-----------|
| 技术深度与创新性 | 30% | Git-as-state-machine + 多方案竞争评分 + Skill 蒸馏闭环 |
| 技术实现与工程完整度 | 25% | 六阶段端到端链路 + lint/单测绿 + token/耗时监控 |
| 业务价值与场景契合度 | 20% | PM 对话自然、歧义追问、新模式低成本接入 |
| 代码质量与文档 | 10% | 本文档 + 架构图 + README |
| 数据与合规性 | 5% | 仅使用官方 doubao EP |
| 项目材料完整度 | 10% | Demo + 视频 + 仓库 + 文档全量交付 |

### 1.5 加分项覆盖

| 加分锚点 | 覆盖方案 |
|---------|---------|
| Skill 抽象可扩展 | Skill 从实际运行结果蒸馏，新增模式 = 新增 1 个 Skill 文件 |
| 断点重放 | 每阶段 = git branch + commit，任意 checkpoint 可回退/修改/重放 |
| 跨栈一致性 | 模块定位阶段自动识别前后端关联文件，代码生成统一处理 |
| 可观测性 | 每次 AI 调用记录 token/耗时/成本，Edge 详情面板展示 |
| 业务上下文反哺 | 历史高分方案沉淀为 Skill，相似需求自动召回 |
| 澄清深度 | Clarify Agent 识别模糊点主动追问，结构化输出需求 DSL |

---

## 二、核心设计理念

### 2.1 Git 即状态机

**核心洞察：用 git 本身作为持久化层和流程引擎。**

| 关注点 | 传统做法 | 本方案 |
|--------|---------|--------|
| 代码版本 | git | git |
| 流程状态 | 数据库/Redis | git branch + commit |
| 需求文档 | Confluence | git 中的 spec.md |
| 方案对比 | 人工 review | git diff between branches |
| 回滚/断点 | 自建 undo | git checkout |
| 审计追踪 | 日志系统 | git log |
| 评分数据 | 额外数据库 | score.json committed alongside |

一条 `git log --graph --all` 就能看到整个需求的生命周期。

### 2.2 每个 Checkpoint 就是一个 Issue

复用 Multica 现有 Issue 系统：

- 主需求 = Parent Issue（如 MUL-42 "文章加封面图"）
- 每个阶段的产物 = Sub-Issue（通过 `parent_issue_id` 关联）
- 阶段间的流转 = Edge（新增 `pipeline_edge` 表）
- Board 看板 = Pipeline 可视化（列 = 阶段，箭头 = 流转）

### 2.3 多方案竞争 + 自动评分 + Skill 蒸馏

```
PM 输入需求
    → Agent 产出 N 个方案（N 个分支）
    → 各自实现 + 测试
    → 自动打分
    → 最优方案提 PR
    → 好/差方案对比 → 蒸馏 Skill
    → 下次相似需求自动召回 Skill
```

这不是简单的"AI 生成代码"，而是 **"AI 生成 → 自动评判 → 沉淀方法论 → 指导下一次生成"** 的 meta-learning 循环。

---

## 三、六阶段详细设计

### Stage 1: 需求澄清 (Clarify)

**目标**：将 PM 的模糊自然语言输入转化为结构化需求 DSL。

**触发方式**：
- PM 在 Chat 中输入需求描述
- 系统自动创建 Parent Issue + 第一个 Clarify Sub-Issue

**Agent 行为**：
1. 分析 PM 输入，识别模糊点和歧义
2. 主动追问（生成澄清问题列表）
3. 结合 PM 回答，输出结构化 spec.md

**Git 操作**：
```bash
git checkout -b feat/MUL-42/clarify/v1  # 从 main 切出
# Agent 写入 .multica/spec.md
git commit -m "clarify: structured requirements for MUL-42"
```

**spec.md 格式（OpenSpec 风格）**：
```markdown
---
id: MUL-42
stage: clarification
version: 1
---

# 需求：文章加封面图字段

## 功能描述
Article 模型新增 coverImage 字段，支持在新建/编辑文章时输入图片 URL，
列表卡片和详情页展示封面图。

## 验收标准
- [ ] Article 模型新增 coverImage (string, nullable)
- [ ] 新建/编辑表单增加 URL 输入框
- [ ] 文章列表卡片展示封面图
- [ ] 文章详情页顶部展示封面图
- [ ] 无封面时不显示占位图

## 技术约束
- 前端：React 组件
- 后端：Express route + Sequelize model
- 数据库：PostgreSQL migration

## 模糊点（需澄清）
- 封面图尺寸是否需要校验？
- 是否支持图片上传还是仅 URL 输入？
```

**用户介入点**：
- PM 可以在 Chat 中追加说明
- PM 可以直接编辑 sub-issue 的描述来修正
- 系统检测到用户输入后创建 clarify/v2 分支

**输出**：结构化 spec.md committed to git，Sub-Issue 状态变为 done

**评分记录**：
```json
{
  "stage": "clarify",
  "metrics": {
    "questions_asked": 3,
    "ambiguities_identified": 2,
    "rounds": 2,
    "token_consumed": 1200,
    "duration_ms": 5000
  }
}
```

---

### Stage 2: 方案拆解 (Plan)

**目标**：基于结构化需求，产出 1-3 个可行技术方案。

**触发方式**：Clarify Sub-Issue 状态变为 done 时自动触发。

**Agent 行为**：
1. 读取 spec.md
2. 召回历史相似需求的 Skill（如果存在）
3. 分析 Conduit 仓库结构，确定影响范围
4. 输出 1-3 个方案的 plan.md

**Git 操作**：
```bash
# 方案 A
git checkout -b feat/MUL-42/plan-a main
# Agent 写入 .multica/plan.md
git commit -m "plan: approach A for MUL-42"

# 方案 B（如果有）
git checkout -b feat/MUL-42/plan-b main
git commit -m "plan: approach B for MUL-42"
```

**plan.md 格式**：
```markdown
---
id: MUL-42
stage: plan
variant: A
estimated_complexity: medium
affected_files: 7
---

# 方案 A：标准全栈新增字段

## 实现步骤
1. 数据库：添加 migration 新增 coverImage 列
2. 后端 Model：Sequelize model 加字段
3. 后端 Route：article CRUD 支持 coverImage
4. 前端 API：更新 API 调用类型
5. 前端组件：表单 + 列表卡片 + 详情页

## 影响文件列表
- backend/src/models/Article.js
- backend/src/routes/api/articles.js
- frontend/src/components/ArticleForm.jsx
- frontend/src/components/ArticlePreview.jsx
- frontend/src/pages/Article.jsx

## 风险评估
- 低风险：纯新增字段，无破坏性变更
- 注意：需要处理旧数据 null 值
```

**用户介入点**：
- 查看各方案 plan.md，标注偏好
- 在 Edge 上标注 "方案 A 更好" 或 "方案 B 思路有问题"
- 可以选择只执行某个方案（跳过其他）
- 可以修改 plan.md 后让 Agent 基于修改重新执行

**输出**：1-3 个 Plan Sub-Issue，各自 branch 上有 plan.md

---

### Stage 3: 模块定位 (Locate)

**目标**：精确定位代码改动涉及的文件和函数，为代码生成提供上下文。

**说明**：模块定位在实现上不作为独立阶段，而是 Plan 阶段的产出之一（plan.md 中的"影响文件列表"），以及 Code Gen 阶段的输入预处理。

**Agent 行为**：
1. 基于 plan.md 中的影响文件列表
2. 读取这些文件的实际内容
3. 分析函数/组件边界、import 依赖
4. 确定精确的修改点（行号级别）
5. 输出 context 信息供 Code Gen 使用

**上下文工程**：
```
精确切出的上下文（避免 token 浪费）：
- 只给 Agent 看相关文件，不是整个仓库
- 文件级别的摘要 + 重点函数的完整代码
- import 关系图（知道改 A 会影响 B）
```

**模块定位结果存入 plan.md 的扩展字段**：
```markdown
## 精确修改点

### backend/src/models/Article.js
- Line 15-30: Model definition，需新增 coverImage field
- 依赖方：routes/api/articles.js (line 45, 78, 102)

### frontend/src/components/ArticleForm.jsx
- Line 22-45: Form fields，需新增 input
- Line 60: handleSubmit，需包含 coverImage
```

---

### Stage 4: 代码生成 (Implement)

**目标**：基于方案和精确定位，生成可工作的代码变更。

**触发方式**：Plan Sub-Issue 状态变为 done（或用户确认方案）后触发。

**Agent 行为**：
1. 读取 spec.md + plan.md + 精确修改点
2. 注入相关 Skill（如果有 "新增字段" 类型的历史 Skill）
3. 在 Conduit worktree 中执行代码修改
4. 生成所有必要的文件变更
5. 确保前后端一致性（类型定义、API 契约）

**Git 操作**：
```bash
git checkout -b feat/MUL-42/plan-a/impl feat/MUL-42/plan-a
# Agent 修改实际代码文件
git add .
git commit -m "feat(article): add coverImage field - plan A implementation"
```

**Skill 注入**：
```
如果存在相似 Skill "add-field-to-model"：
- 将 Skill 的 code_template 作为 few-shot example 注入 prompt
- 包含 Skill 的 best-practice notes（从历史高分方案蒸馏）
```

**跨栈一致性保证**：
```
Code Gen Agent 的 System Prompt 要求：
1. 后端加字段 → 必须同步前端类型
2. Route 加参数 → 必须更新前端 API 调用
3. DB migration → 必须更新 Model
4. 生成的代码必须满足 ESLint 规则
```

**用户介入点**：
- 查看代码 diff，直接修改（追加 commit）
- 标注 "这里应该加空值检查"
- 要求 Agent 重新生成某个文件

**输出**：实际代码变更 committed to branch

---

### Stage 5: 自动化测试 (Validate)

**目标**：验证生成的代码能通过 lint 和单元测试。

**触发方式**：Implement Sub-Issue 完成后自动触发。

**执行内容**：
```bash
# 在对应的 worktree 中执行
cd /path/to/worktree

# 1. Lint 检查
npx eslint . --ext .js,.jsx 2>&1 | tee .multica/lint-result.txt

# 2. 单元测试
npm test 2>&1 | tee .multica/test-result.txt

# 3. 可选：启动应用做 smoke test
npm run dev &
# 检查关键页面是否能正常加载
curl http://localhost:3000/api/articles | jq .
```

**评分计算**：
```json
{
  "stage": "validate",
  "metrics": {
    "lint_pass": true,
    "lint_errors": 0,
    "lint_warnings": 2,
    "test_total": 24,
    "test_passed": 22,
    "test_failed": 2,
    "test_pass_rate": 0.917,
    "test_coverage": null
  }
}
```

**如果测试失败**：
- 自动将失败信息反馈给 Code Gen Agent
- Agent 修复后追加 commit
- 重新跑测试（最多 3 轮）
- 3 轮后仍失败则标记 checkpoint 为 failed

**Git 操作**：
```bash
# 测试结果也 commit（供后续 Skill 蒸馏分析）
git add .multica/lint-result.txt .multica/test-result.txt .multica/score.json
git commit -m "test: validation results for plan A"
```

---

### Stage 6: 提测/提 PR (Handoff)

**目标**：选择最优方案，合并到主分支，创建 Pull Request。

**触发方式**：所有方案的 Validate 完成后触发。

**流程**：
1. 比较各方案的 score
2. 自动选择最高分（或等待用户指定）
3. 创建 PR（包含完整信息）

**PR 内容**：
```markdown
## Summary
- 需求：文章加封面图字段 (MUL-42)
- 方案：Plan A (score: 85/100)
- 变更：Article model + CRUD + 前端组件

## Changes
- [x] DB migration: add coverImage column
- [x] Backend model: update Article.js
- [x] Backend routes: support coverImage in CRUD
- [x] Frontend form: add image URL input
- [x] Frontend display: show cover in list and detail

## Test Results
- Lint: PASS (0 errors, 2 warnings)
- Tests: 22/24 passed (91.7%)

## AI Metrics
- Token consumed: 4,523
- Generation time: 18.2s
- Attempts: 1 (first pass)
```

**Git 操作**：
```bash
git checkout -b feat/MUL-42/final main
git merge feat/MUL-42/plan-a/impl
# 创建 PR
gh pr create --title "feat(article): add coverImage field" --body "..."
```

**完成后**：
- Parent Issue MUL-42 状态变为 Done
- 触发 Skill 蒸馏流程

---

## 四、Skill 蒸馏与自我迭代

### 4.1 蒸馏触发

Pipeline 完成后，系统自动分析本次流程：

```
输入：
- 所有方案的 spec.md + plan.md + code diff + score.json
- 用户标注（annotations）
- 最终选择了哪个方案

输出：
- 新 Skill 或更新已有 Skill
```

### 4.2 Skill 数据结构

复用 Multica 现有 `skill` 表：

```
skill:
  name: "add-field-to-model"
  description: "在 Conduit 项目中为现有模型新增字段的标准流程"
  content: (SKILL.md 内容，见下)
  config: {
    "trigger_pattern": {
      "keywords": ["新增字段", "add field", "加字段"],
      "affected_layers": ["model", "route", "frontend"]
    },
    "source_type": "distilled",
    "scoring_stats": {
      "avg_score": 85,
      "usage_count": 3,
      "success_rate": 1.0
    },
    "source_checkpoints": ["uuid-1", "uuid-2"]
  }
```

**SKILL.md 内容示例**：
```markdown
# Skill: 新增字段到模型

## 触发条件
当需求涉及为现有数据模型添加新字段时触发。

## 标准步骤
1. 创建 Sequelize migration
2. 更新 Model 定义
3. 更新 Route 的 CRUD 操作（create/update 接收新字段，get 返回新字段）
4. 更新前端 API 类型
5. 更新相关 UI 组件

## Best Practices（从历史高分方案蒸馏）
- 新字段应默认 nullable，避免破坏现有数据
- 前端使用可选链 (?.) 访问新字段
- 列表页只展示非空值（不显示占位）
- 表单使用 controlled input with empty string default

## Anti-Patterns（从历史低分方案学习）
- 不要 NOT NULL without default（会导致 migration 失败）
- 不要在列表组件中硬编码图片尺寸
- 不要忘记更新 API validation schema

## Code Template (参考)
### Migration
```js
module.exports = {
  up: async (queryInterface, Sequelize) => {
    await queryInterface.addColumn('Articles', '{{fieldName}}', {
      type: Sequelize.{{fieldType}},
      allowNull: true,
      defaultValue: null
    });
  },
  down: ...
};
```
```

### 4.3 Skill 召回机制

当新需求进入 Plan 阶段时：

```
1. 提取新需求的关键词 + 结构化标签
2. 在 Skill Registry 中匹配 trigger_pattern
3. 按相关度排序，取 top-3 Skill
4. 将 Skill 内容注入 Agent 的 System Prompt
```

### 4.4 迭代闭环

```
需求完成 → 对比好/差方案 → 蒸馏 Skill
     ↑                              │
     └──── 新需求召回 Skill ←────────┘
```

**差方案也有价值**：其 Anti-Patterns 部分来自低分方案的错误，告诉 Agent "不要这么做"。

---

## 五、系统架构

### 5.1 整体架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                          Frontend (Next.js)                       │
│                                                                   │
│  ┌───────────┐  ┌──────────────────┐  ┌──────────────────────┐  │
│  │  Chat UI  │  │  Board + Edges   │  │  Edge Detail Panel   │  │
│  │(PM 对话入口)│  │(Pipeline 可视化) │  │(Diff/Score/标注)     │  │
│  └─────┬─────┘  └────────┬─────────┘  └──────────┬───────────┘  │
│        │                  │                        │              │
└────────┼──────────────────┼────────────────────────┼──────────────┘
         │                  │                        │
         ▼                  ▼                        ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Go Backend (server)                           │
│                                                                   │
│  ┌──────────┐  ┌─────────────┐  ┌───────────┐  ┌────────────┐  │
│  │Issue/Chat│  │Pipeline Edge│  │Orchestrator│  │   Skill    │  │
│  │ Handler  │  │  Handler    │  │  (核心)    │  │  Handler   │  │
│  └────┬─────┘  └──────┬──────┘  └─────┬─────┘  └─────┬──────┘  │
│       │                │               │               │          │
│       └────────────────┴───────┬───────┴───────────────┘          │
│                                │                                  │
│  ┌─────────────────────────────┼──────────────────────────────┐  │
│  │           agent_task_queue   │                              │  │
│  └─────────────────────────────┼──────────────────────────────┘  │
│                                │                                  │
└────────────────────────────────┼──────────────────────────────────┘
                                 │
                                 ▼ (WebSocket push task)
┌─────────────────────────────────────────────────────────────────┐
│                        Daemon (Agent Runtime)                     │
│                                                                   │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────────┐    │
│  │  Task Poller │  │  RepoCache   │  │  Agent Executor    │    │
│  │  (认领任务)  │  │  (Git 管理)   │  │ (doubao-seed调用) │    │
│  └──────┬───────┘  └──────┬───────┘  └────────┬───────────┘    │
│         │                  │                    │                 │
│         │    ┌─────────────┴─────────────┐     │                 │
│         └───→│   Conduit Fork (Worktree) │←────┘                 │
│              │   feat/MUL-42/plan-a/impl │                       │
│              └───────────────────────────┘                       │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

### 5.2 组件职责

| 组件 | 职责 | 改动程度 |
|------|------|---------|
| **Chat UI** | PM 输入需求、查看 Agent 回复、确认/追问 | 不改，直接复用 |
| **Board + Edges** | Pipeline 可视化，箭头连接各 checkpoint | 新增 SVG overlay |
| **Edge Detail Panel** | 点击箭头查看 diff/score/标注 | 新增组件 |
| **Issue/Chat Handler** | Issue CRUD + Chat 消息 | 不改 |
| **Pipeline Edge Handler** | Edge CRUD API | 新增 |
| **Orchestrator** | 六阶段编排核心，监听完成事件 → 创建下一步 | 新增 |
| **Skill Handler** | Skill CRUD + 蒸馏 + 召回 | 小改（加蒸馏逻辑） |
| **Daemon** | 认领任务、创建 worktree、执行 Agent | 小改（prompt 注入） |
| **RepoCache** | Git 仓库 clone/pull/worktree 管理 | 不改，直接复用 |
| **Agent Executor** | 调用 doubao-seed 模型，执行代码操作 | 改（对接火山方舟 API） |

### 5.3 数据流

```
1. PM 在 Chat 输入 "文章加封面图"
2. Chat Handler 创建 chat_message
3. Orchestrator 监听 → 创建 parent issue + clarify sub-issue
4. Orchestrator 创建 pipeline_edge (chat → clarify)
5. Orchestrator 入队 agent_task_queue (stage=clarify, branch=feat/MUL-42/clarify/v1)
6. Daemon 认领 task → 创建 worktree → 执行 Clarify Agent
7. Agent 输出 spec.md → commit to branch
8. Daemon 回调 task completed
9. Orchestrator 监听 → 解析结果 → 创建 plan sub-issues + edges
10. ... 循环直到 PR 提交
11. Pipeline 完成 → 触发 Skill 蒸馏
```

### 5.4 复用现有 WebSocket 事件驱动 Pipeline

Multica 已有完善的 WebSocket 事件系统，Pipeline 的实时推送完全复用：

```
现有事件:                     Pipeline 中的用途:
─────────                    ─────────────────
task:queued                → 前端展示 "checkpoint 排队中"
task:dispatch              → 前端展示 "checkpoint 执行中"
task:progress / task:message → 实时展示 Agent 执行过程 (thinking/tool_use)
task:completed             → ★ Orchestrator 监听此事件触发下一步
task:failed                → Orchestrator 决定重试或标记失败
issue:created              → Board 上新卡片出现 (sub-issue)
issue:updated              → 卡片状态/score 更新
```

**Orchestrator 的触发机制**：在 Go 后端现有的 `task:completed` 处理逻辑中加一个 hook——如果 task 的 `context` 里有 `pipeline_issue_id`，就调用 `orchestrator.OnCheckpointCompleted()`。

```go
// server/internal/handler/ 中已有的 task completed 回调中追加:
if pipelineID := task.Context.Get("pipeline_issue_id"); pipelineID != "" {
    go h.Orchestrator.OnCheckpointCompleted(ctx, task)
}
```

这是整个 Pipeline 与现有代码的**唯一耦合点**——一个 if 判断 + 一行函数调用。

---

## 六、数据模型设计

### 6.0 现有 Task 体系复用分析

Multica 已有一套完整的 Task 执行体系，我们的 Pipeline **不重新发明轮子，而是把每个 checkpoint 当作一个独立 task 来复用**。

#### agent_task_queue 现有字段（与 Pipeline 的映射）

```
agent_task_queue 现有字段:              Pipeline 中的用法:
─────────────────────────              ────────────────────
id                                →    checkpoint 的执行实例 ID
agent_id                          →    执行此 stage 的 Agent
issue_id                          →    关联的 sub-issue (checkpoint issue)
status (queued→dispatched→running→completed/failed)  →  单个 checkpoint 的执行状态
context JSONB                     →    ★ 传递 pipeline 上下文:
                                       {
                                         "pipeline_issue_id": "parent issue uuid",
                                         "stage": "exec",
                                         "variant": "plan-a",
                                         "branch": "feat/MUL-42/plan-a/impl",
                                         "skill_ids": ["uuid-1"],
                                         "spec_hash": "sha256:..."
                                       }
session_id + work_dir             →    ★ Agent CLI 会话恢复 (已有断点恢复!)
                                       Daemon 崩溃后 --resume session_id 恢复
attempt + max_attempts            →    ★ 同一 stage 的自动重试 (test 失败→重跑)
parent_task_id                    →    重试链: 失败 task → 新 task 的指向关系
result JSONB                      →    ★ 存放 stage 产出:
                                       {
                                         "score": {...},
                                         "diff_summary": {...},
                                         "spec_output": "...",
                                         "branch_commit": "abc123"
                                       }
trigger_summary                   →    "Pipeline stage: exec (plan-a)"
chat_session_id                   →    关联的 PM 对话 (澄清阶段)
runtime_id                        →    执行在哪个 Daemon runtime
dispatched_at/started_at/completed_at → 耗时计算的数据源
```

#### task_message 现有机制（可观测性数据源）

```sql
task_message:
  task_id   → 属于哪个 checkpoint task
  seq       → 执行步骤序号
  type      → "text" | "thinking" | "tool_use" | "tool_result" | "error"
  tool      → 调用了什么工具 (Read, Write, Bash...)
  content   → 输出内容
  input     → 工具输入参数 JSONB
  output    → 工具执行结果
```

**这就是我们的可观测性数据！** 每次 Agent 执行，所有 thinking/tool_use/tool_result 都被逐条记录。我们只需要在 Edge Detail Panel 中聚合展示：
- 统计 token（从 content 长度 + API 响应估算）
- 统计工具调用次数
- 提取 diff 相关的 tool_result

#### Autopilot 机制（参考但不直接复用）

```
autopilot 现有流程:
  trigger → create_issue → enqueue_task → running → completed

我们的 Pipeline 与 Autopilot 的区别:
  ┌────────────────┬──────────────────────┬──────────────────────┐
  │                │ Autopilot            │ 我们的 Pipeline       │
  ├────────────────┼──────────────────────┼──────────────────────┤
  │ 步数           │ 单步                 │ 多步 (6 stages)      │
  │ 分支           │ 无                   │ 有 (多方案并行)      │
  │ 评分           │ 无                   │ 有                   │
  │ 触发方式       │ cron/webhook/api     │ Chat 输入 / 手动     │
  │ 编排逻辑       │ 固定: issue→task     │ DAG: 动态决定下一步  │
  │ 人工介入       │ 无                   │ 每步可暂停/标注      │
  └────────────────┴──────────────────────┴──────────────────────┘
```

结论：Autopilot 是"定时单步任务"，Pipeline 是"交互式多步 DAG"。
不复用 autopilot_run 表，但**参考其 pattern**（trigger → run → status tracking）。

#### 核心设计决策：Orchestrator 是 Task 之间的"胶水"

```
┌─────────────────────────────────────────────────────────────────┐
│  Pipeline 执行流程                                                │
│                                                                   │
│  Orchestrator                                                     │
│       │                                                          │
│       ├── 创建 Task A (stage=clarify)                            │
│       │        │                                                 │
│       │        ├── Daemon 认领 → Agent 执行 → task_message 记录  │
│       │        ├── session_id 保存 (断点恢复)                     │
│       │        └── task completed → result JSONB 写入            │
│       │                                                          │
│       ├── 监听 task:completed 事件                                │
│       ├── 解析 Task A 的 result → 决定下一步                      │
│       ├── 创建 pipeline_edge (A → B)                             │
│       │                                                          │
│       ├── 创建 Task B (stage=plan, context 里带上 spec)           │
│       │        │                                                 │
│       │        └── ... 同样的执行流程 ...                         │
│       │                                                          │
│       └── ... 循环直到 pipeline 完成 ...                          │
│                                                                   │
│  关键点: Daemon/Task Runner 完全不知道自己在一个 Pipeline 里！      │
│  它只看到一个普通的 task，照常执行。                                │
│  Pipeline 的多步逻辑全部在 Orchestrator 层。                       │
└─────────────────────────────────────────────────────────────────┘
```

**这意味着**：
- `agent_task_queue` 表 **零改动**
- `task_message` 表 **零改动**
- Daemon 的 task runner **几乎零改动**（只需在 prompt 构建时读 context JSONB 里的 pipeline 信息）
- 所有 Pipeline 逻辑都是 **增量新增**，不影响现有功能

### 6.1 新增表

```sql
-- Pipeline Edge：checkpoint 之间的流转关系
CREATE TABLE pipeline_edge (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    source_issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    target_issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    edge_type TEXT NOT NULL DEFAULT 'flow'
        CHECK (edge_type IN ('flow', 'branch', 'merge', 'retry')),
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 索引
CREATE INDEX idx_pipeline_edge_source ON pipeline_edge(source_issue_id);
CREATE INDEX idx_pipeline_edge_target ON pipeline_edge(target_issue_id);
CREATE INDEX idx_pipeline_edge_workspace ON pipeline_edge(workspace_id);

-- Pipeline Annotation：用户在 edge 上的标注
CREATE TABLE pipeline_annotation (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    edge_id UUID NOT NULL REFERENCES pipeline_edge(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES "user"(id),
    content TEXT NOT NULL,
    annotation_type TEXT NOT NULL DEFAULT 'note'
        CHECK (annotation_type IN ('note', 'approval', 'rejection', 'suggestion')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_pipeline_annotation_edge ON pipeline_annotation(edge_id);
```

### 6.2 Issue.metadata 扩展（不改表结构）

```json
{
  "pipeline": {
    "role": "checkpoint",
    "stage": "exec",
    "variant": "plan-a",
    "branch_name": "feat/MUL-42/plan-a/impl",
    "git_commit": "abc123def456",
    "score": {
      "total": 85,
      "lint_pass": true,
      "lint_errors": 0,
      "test_total": 24,
      "test_passed": 22,
      "test_pass_rate": 0.917,
      "token_consumed": 4523,
      "generation_time_ms": 18200,
      "diff_lines_added": 47,
      "diff_lines_removed": 3,
      "diff_files_changed": 5
    },
    "spec_hash": "sha256:...",
    "parent_pipeline_issue_id": "uuid-of-MUL-42"
  }
}
```

对于 Parent Issue（主需求），metadata 格式：
```json
{
  "pipeline": {
    "role": "root",
    "repo_url": "https://github.com/our-fork/conduit-realworld-example-app",
    "base_branch": "main",
    "status": "active",
    "stages_completed": ["clarify", "plan"],
    "current_stage": "exec",
    "selected_variant": null,
    "final_pr_url": null
  }
}
```

### 6.3 Skill 表扩展（ALTER）

```sql
-- 在现有 skill 表上添加蒸馏相关字段
ALTER TABLE skill ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT 'manual'
    CHECK (source_type IN ('manual', 'distilled', 'imported'));
ALTER TABLE skill ADD COLUMN IF NOT EXISTS trigger_pattern JSONB;
ALTER TABLE skill ADD COLUMN IF NOT EXISTS scoring_stats JSONB DEFAULT '{}';
```

### 6.4 Edge Metadata 详解

```json
{
  "transition": {
    "from_stage": "plan",
    "to_stage": "exec",
    "trigger": "auto",
    "triggered_by": "orchestrator"
  },
  "diff_summary": {
    "files_changed": 5,
    "lines_added": 47,
    "lines_removed": 3,
    "key_changes": ["Article.js model", "ArticleForm.jsx"]
  },
  "ai_metrics": {
    "model": "doubao-seed-2.0-lite",
    "token_input": 3200,
    "token_output": 1323,
    "token_total": 4523,
    "duration_ms": 18200,
    "api_calls": 3
  },
  "score": {
    "total": 85,
    "breakdown": {
      "test_pass_rate": 30,
      "lint_clean": 20,
      "diff_precision": 17,
      "token_efficiency": 10,
      "time_efficiency": 8
    }
  }
}
```

---

## 七、前端设计

### 7.1 Board + Edge Overlay

在现有 Board 视图上叠加 SVG 箭头层：

```
┌─────────────────────────────────────────────────────────────────┐
│  Board: Pipeline View (filter: parent=MUL-42)                    │
│                                                                   │
│  Clarify      Plan         Exec         Test        Done         │
│  ┌──────┐    ┌──────┐    ┌──────┐    ┌──────┐    ┌──────┐     │
│  │cl/v1 │───→│plan-a│───→│impl-a│───→│test-a│───→│final │     │
│  │ done │    │ done │    │ done │    │ 85分 │    │ PR   │     │
│  └──────┘    └──────┘    └──────┘    └──────┘    └──────┘     │
│       │      ┌──────┐    ┌──────┐    ┌──────┐                  │
│       └─────→│plan-b│───→│impl-b│───→│test-b│                  │
│              │ done │    │ done │    │ 72分 │                  │
│              └──────┘    └──────┘    └──────┘                  │
│                                                                   │
│  ──→ = flow edge (点击查看详情)                                  │
│  ─┬→ = branch edge                                              │
└─────────────────────────────────────────────────────────────────┘
```

**实现方式**：
- Board 容器设为 `position: relative`
- 叠加一层 `<svg>` 绝对定位覆盖
- 使用 `ResizeObserver` 追踪卡片位置
- 箭头用贝塞尔曲线连接（`<path d="M... C...">`）
- 箭头颜色编码：绿色=flow，蓝色=branch，紫色=merge

### 7.2 Edge Detail Panel（侧边面板）

点击箭头时弹出，展示该 transition 的完整信息：

```
┌──────────────────────────────────────────┐
│  Edge: plan-a → impl-a                   │
│  Type: flow | Stage: plan → exec         │
├──────────────────────────────────────────┤
│                                          │
│  📊 Score: 85/100                         │
│  ├── Test pass rate: 30/35              │
│  ├── Lint clean: 20/20                  │
│  ├── Diff precision: 17/20             │
│  ├── Token efficiency: 10/15           │
│  └── Time efficiency: 8/10             │
│                                          │
│  ⏱️ Duration: 18.2s                      │
│  🪙 Tokens: 4,523 (in:3200 + out:1323)  │
│  🔀 Branch: feat/MUL-42/plan-a/impl     │
│                                          │
├──── Diff Preview ────────────────────────┤
│  M backend/src/models/Article.js    +12  │
│  M backend/src/routes/api/articles.js +8 │
│  M frontend/src/components/Form.jsx +15  │
│  A frontend/src/components/Cover.jsx +22 │
│  [展开完整 Diff]                          │
│                                          │
├──── 标注 ────────────────────────────────┤
│  💬 "方案 A 的字段命名更规范" - PM       │
│  [+ 添加标注]                            │
│                                          │
├──── 操作 ────────────────────────────────┤
│  [🔄 重新执行] [⏸️ 暂停后续] [✅ 选择此方案]│
└──────────────────────────────────────────┘
```

### 7.3 Issue Card 增强

Sub-Issue 卡片上显示额外信息：

```
┌──────────────────────────┐
│  MUL-42-4                │
│  implement plan A        │
│                          │
│  🔀 feat/.../plan-a/impl │  ← branch 标识
│  📊 85分                  │  ← score badge
│  🪙 4.5k tokens          │  ← token 消耗
│                          │
│  👤 Agent · ⏱️ 18s       │
└──────────────────────────┘
```

### 7.4 Pipeline Filter

Board 顶部新增 filter，进入 "Pipeline 模式"：

```
[Board] [Filter] [Display] ──── [🔗 Pipeline: MUL-42 ✕]
```

选择某个 parent issue 后：
- Board 只显示该 issue 的所有 sub-issues
- 列按 stage 排列（而非通用 status）
- 显示 Edge 箭头

---

## 八、Orchestrator 设计

### 8.1 核心接口

```go
// server/internal/orchestrator/orchestrator.go

type Orchestrator struct {
    queries    *db.Queries
    taskSvc    *service.TaskService
    skillSvc   *service.SkillService
    wsHub      *websocket.Hub
    repoConfig RepoConfig
}

// 主入口：某个 checkpoint issue 完成时调用
func (o *Orchestrator) OnCheckpointCompleted(ctx context.Context, issue db.Issue) error

// 创建下一阶段的 sub-issue + edge + enqueue task
func (o *Orchestrator) advanceToNextStage(ctx context.Context, parentID, currentID uuid.UUID, currentStage string) error

// 创建 Pipeline（从 chat message 触发）
func (o *Orchestrator) CreatePipeline(ctx context.Context, wsID uuid.UUID, requirement string) (*db.Issue, error)

// 为某个 stage 构建 Agent prompt
func (o *Orchestrator) buildPrompt(ctx context.Context, stage string, issue db.Issue) (string, error)

// Skill 召回
func (o *Orchestrator) recallSkills(ctx context.Context, wsID uuid.UUID, requirement string) ([]db.Skill, error)

// 打分
func (o *Orchestrator) scoreCheckpoint(ctx context.Context, issue db.Issue, testResults TestResults) (Score, error)

// Skill 蒸馏
func (o *Orchestrator) distillSkill(ctx context.Context, parentIssue db.Issue) error
```

### 8.2 阶段流转逻辑

```go
func (o *Orchestrator) advanceToNextStage(ctx context.Context, ...) error {
    meta := parsePipelineMeta(currentIssue.Metadata)
    
    switch meta.Stage {
    case "clarify":
        // 读取 spec.md 内容 → 判断是否需要继续澄清
        // 如果 spec 完整 → 创建 plan 阶段 (1-3 个分支)
        specs := o.parseClarifyResult(currentIssue)
        if specs.HasAmbiguity {
            return o.createClarifyFollowup(ctx, ...)
        }
        return o.createPlanStage(ctx, parentID, currentID, specs, numVariants)
        
    case "plan":
        // plan 完成 → 创建 impl 阶段
        return o.createImplStage(ctx, parentID, currentID)
        
    case "exec":
        // 代码生成完成 → 自动进入 validate
        return o.createValidateStage(ctx, parentID, currentID)
        
    case "validate":
        // 测试完成 → 打分 → 检查是否所有方案都完成
        score := o.scoreCheckpoint(ctx, currentIssue, results)
        o.updateScore(ctx, currentID, score)
        
        // 如果该 parent 下所有 impl 都完成了 validate
        if o.allVariantsScored(ctx, parentID) {
            return o.createHandoffStage(ctx, parentID)
        }
        
    case "handoff":
        // 选择最高分方案，创建 PR
        best := o.selectBestVariant(ctx, parentID)
        o.createPR(ctx, best)
        o.markPipelineComplete(ctx, parentID)
        // 触发 Skill 蒸馏
        go o.distillSkill(ctx, parentID)
    }
    return nil
}
```

### 8.3 Prompt 构建策略

每个阶段注入不同的 System Prompt：

```go
func (o *Orchestrator) buildPrompt(ctx context.Context, stage string, issue db.Issue) string {
    var parts []string
    
    // 1. 基础角色设定
    parts = append(parts, stageRolePrompt(stage))
    
    // 2. 仓库结构上下文
    parts = append(parts, o.getRepoContext(ctx, issue))
    
    // 3. 本阶段任务说明
    parts = append(parts, stageTaskPrompt(stage, issue))
    
    // 4. 相关 Skill 注入
    skills := o.recallSkills(ctx, issue.WorkspaceID, issue.Title)
    for _, s := range skills {
        parts = append(parts, fmt.Sprintf("## Relevant Skill: %s\n%s", s.Name, s.Content))
    }
    
    // 5. 输出格式要求
    parts = append(parts, stageOutputFormat(stage))
    
    return strings.Join(parts, "\n\n---\n\n")
}
```

---

## 九、Git 分支策略

### 9.1 分支命名规范

```
feat/{issue-number}/{stage}/{variant}

示例：
feat/MUL-42/clarify/v1          ← 第一轮澄清
feat/MUL-42/clarify/v2          ← 用户追问后第二轮
feat/MUL-42/plan-a              ← 方案 A 设计
feat/MUL-42/plan-b              ← 方案 B 设计
feat/MUL-42/plan-a/impl         ← 方案 A 代码实现
feat/MUL-42/plan-a/impl+test    ← 含测试结果
feat/MUL-42/final               ← 最终合并，用于提 PR
```

### 9.2 .multica/ 目录结构

每个分支上的元信息目录：

```
.multica/
├── spec.md              ← 结构化需求（clarify 阶段产出）
├── plan.md              ← 技术方案（plan 阶段产出）
├── score.json           ← 评分结果（validate 阶段产出）
├── context.json         ← AI 调用元数据
├── lint-result.txt      ← Lint 输出
└── test-result.txt      ← 测试输出
```

### 9.3 Git 操作时序

```
main ─────────────────────────────────────────────────── main
  │
  ├── feat/MUL-42/clarify/v1 (commit: spec.md v1)
  │     │
  │     └── feat/MUL-42/clarify/v2 (commit: spec.md v2, 用户补充)
  │           │
  │           ├── feat/MUL-42/plan-a (commit: plan.md)
  │           │     │
  │           │     └── feat/MUL-42/plan-a/impl (commits: code changes)
  │           │           │
  │           │           └── (validate → score.json committed)
  │           │
  │           └── feat/MUL-42/plan-b (commit: plan.md)
  │                 │
  │                 └── feat/MUL-42/plan-b/impl (commits: code changes)
  │                       │
  │                       └── (validate → score.json committed)
  │
  └── feat/MUL-42/final (merge from best plan) ──→ PR to main
```

---

## 十、评分系统

### 10.1 评分维度与权重

| 维度 | 权重 | 数据来源 | 计算方式 |
|------|------|---------|---------|
| 测试通过率 | 35% | `npm test` 输出 | passed / total * 35 |
| Lint 通过 | 20% | `eslint` exit code | pass=20, fail=0 |
| Diff 精简度 | 20% | `git diff --stat` | 基于 lines/files 的效率评估 |
| Token 效率 | 15% | API 调用记录 | 越少 token 完成同样功能得分越高 |
| 生成耗时 | 10% | wall clock | 越快得分越高 |

### 10.2 评分公式

```
total_score = 
    test_pass_rate * 35 +
    (lint_pass ? 20 : 0) +
    diff_precision_score(lines_changed, files_changed) * 20 +
    token_efficiency_score(tokens_used) * 15 +
    time_efficiency_score(duration_ms) * 10
```

其中：
```
diff_precision_score = max(0, 1 - (total_lines / 200)) 
  // 200 行以内满分，越多扣分越多

token_efficiency_score = max(0, 1 - (tokens / 10000))
  // 10000 token 以内满分

time_efficiency_score = max(0, 1 - (duration_ms / 60000))
  // 60s 以内满分
```

### 10.3 多方案比较

当同一需求有多个方案时，展示对比：

```
┌─────────────┬────────────┬────────────┐
│             │  Plan A    │  Plan B    │
├─────────────┼────────────┼────────────┤
│ Total Score │  85/100    │  72/100    │
│ Test Rate   │  91.7%     │  83.3%     │
│ Lint        │  PASS      │  PASS      │
│ Lines       │  +47/-3    │  +89/-12   │
│ Tokens      │  4,523     │  6,841     │
│ Time        │  18.2s     │  24.7s     │
├─────────────┼────────────┼────────────┤
│ Decision    │  ✅ Selected │  ❌ Lower  │
└─────────────┴────────────┴────────────┘
```

---

## 十一、用户介入机制

### 11.1 介入点

| 阶段 | 介入方式 | 效果 |
|------|---------|------|
| Clarify | Chat 追问 / 修改 sub-issue 描述 | 创建 clarify/v(n+1) 分支 |
| Plan | 在 Edge 上标注偏好 / 修改 plan.md | 影响后续执行顺序或跳过某方案 |
| Exec | 查看 diff → 标注问题 / 直接改代码 | Agent 基于人工修改继续 |
| Validate | 查看失败测试 → 决定是否重试 | 触发重试或手动修复 |
| Handoff | 选择最终方案 / 修改 PR 描述 | 覆盖自动选择 |

### 11.2 标注数据结构

```json
{
  "annotation_type": "suggestion",
  "content": "方案 A 的字段命名更规范，建议采用",
  "target": {
    "edge_id": "uuid",
    "file_path": "backend/src/models/Article.js",
    "line_range": [15, 20]
  }
}
```

### 11.3 标注用于 Skill 蒸馏

用户标注是高价值信号：
- "这里好" → 强化相关 pattern 到 Best Practices
- "这里不好" → 记入 Anti-Patterns
- "选择方案 A" → 方案 A 的 pattern 权重更高

---

## 十二、可观测性

### 12.1 数据采集

每次 AI 调用自动记录：

```json
{
  "call_id": "uuid",
  "model": "doubao-seed-2.0-lite",
  "endpoint": "ep-20260514110933-mzh58",
  "timestamp": "2026-05-24T10:30:00Z",
  "stage": "exec",
  "issue_id": "uuid",
  "token_input": 3200,
  "token_output": 1323,
  "duration_ms": 18200,
  "success": true,
  "cost_estimate_rmb": 0.012
}
```

### 12.2 监控面板（Edge Detail 集成）

不单独做监控页面，将指标集成在：
1. **Edge Detail Panel** — 每个转换的 AI 调用详情
2. **Parent Issue Summary** — 整条 Pipeline 的总计消耗
3. **Skill Detail Page** — 使用该 Skill 的历史效果统计

### 12.3 Pipeline 总览指标

```
Pipeline MUL-42 Summary:
├── Total duration: 2m 34s
├── Total tokens: 12,847
├── Total API calls: 8
├── Estimated cost: ¥0.05
├── Variants explored: 2
├── Final score: 85/100
└── Human interventions: 1
```

---

## 十三、Conduit 仓库集成

### 13.1 Fork 配置

```bash
# Fork 并配置
git clone https://github.com/TonyMckes/conduit-realworld-example-app our-conduit-fork
cd our-conduit-fork

# 添加 .multica/ 到版本控制（不在 .gitignore 中忽略）
mkdir .multica
echo "# Pipeline metadata directory" > .multica/README.md
git add .multica && git commit -m "chore: add .multica metadata directory"
```

### 13.2 Agent 上下文配置

为 Agent 预配置 Conduit 仓库结构理解：

```markdown
# Conduit 仓库结构 (注入 Agent System Prompt)

## 目录结构
- `backend/` — Express.js 后端
  - `src/models/` — Sequelize 模型
  - `src/routes/api/` — API 路由
  - `src/middleware/` — 中间件
- `frontend/` — React + Vite 前端
  - `src/components/` — 组件
  - `src/pages/` — 页面
  - `src/services/` — API 调用
- `package.json` — 根 monorepo 配置

## 技术栈
- 后端：Express 4, Sequelize, PostgreSQL
- 前端：React 18, Vite, React Router
- 语言：纯 JavaScript (非 TypeScript)
- 测试：Jest (后端), Vitest (前端)
- Lint：ESLint

## 开发命令
- `npm install` — 安装依赖
- `npm run dev` — 启动开发
- `npm test` — 运行测试
- `npx eslint .` — 运行 lint
```

### 13.3 Daemon RepoCache 配置

```json
// workspace settings 中配置
{
  "repos": [
    {
      "url": "https://github.com/our-org/conduit-fork",
      "default_branch": "main",
      "auto_sync": true
    }
  ]
}
```

---

## 十四、实施计划

### 开发原则

```
1. 先验证风险最高环节 (Agent + Conduit 能否产出可用代码)
2. 纵向切片优先 (先跑通一条完整链路，再横向扩展)
3. 每个 Phase 结束有明确验收标准
4. 降级策略预设 (时间不够时从后往前砍)
```

### Phase 0: 验证地基（Day 1 上午，4h）

**目标**：确认 Agent 能在 Conduit 上执行并产出代码。

| 任务 | 验收标准 |
|------|---------|
| Fork Conduit → 推到我们的 repo | repo 可 clone |
| 在服务器上确认 `npm install` / `npm test` 能跑 | 测试绿 |
| 手动测试 doubao-seed API 对 Conduit 代码的理解 | 返回合理代码 |
| 确认 Daemon 能创建 worktree 并执行命令 | worktree 创建成功 |

**⚠️ 卡点决策**：如果 doubao-seed 对 Conduit 的理解能力不够，需要在此调整 prompt 策略或考虑补充上下文方案。

### Phase 1: 单链路端到端（Day 1 下午 - Day 3）

**目标**：一条需求从头走到尾，不管 UI，只要能跑通。

```
Day 1 下午:
  □ Orchestrator 骨架 (Go)
    - 最简版: hardcoded 6 个 stage 按顺序推进
    - 监听 task:completed → 创建下一个 task
    - 文件: server/internal/orchestrator/orchestrator.go
  □ 每个 stage 的 Agent Prompt 初版
    - server/internal/orchestrator/prompts/clarify.md
    - server/internal/orchestrator/prompts/plan.md
    - server/internal/orchestrator/prompts/implement.md
    - server/internal/orchestrator/prompts/validate.md

Day 2:
  □ Daemon prompt 注入
    - 读 task.context JSONB 里的 stage/branch/skill
    - 根据 stage 选择对应 system prompt
    - 改动文件: server/internal/daemon/prompt.go (约 50 行)
  □ Git 分支操作
    - Orchestrator 创建 task 时指定 branch 名到 context
    - Daemon 在 worktree 里 checkout 到对应分支
    - Agent 执行完毕后自动 commit

Day 3:
  □ Validate 阶段
    - 在 worktree 里执行 eslint + npm test
    - 解析输出，计算 score，写入 task.result
  □ 端到端联调
    - 通过 API/Chat 输入 "文章列表加阅读量"
    - 验证: clarify → plan → impl → validate 各步完成
    - 验证: 最终 branch 上有可工作的代码变更
```

**Day 3 验收标准**：
```bash
# 能在 Conduit fork 上看到 Agent 产出的分支
git log --oneline feat/MUL-1/plan-a/impl
# 分支上的代码改动合理
git diff main..feat/MUL-1/plan-a/impl
# lint 通过（或至少有结果）
cat .multica/score.json
```

### Phase 2: 数据持久化 + API（Day 4-5）

**目标**：Pipeline 流转数据能存 DB、能查询、有 API。

```
Day 4:
  □ DB migration: pipeline_edge + pipeline_annotation 表
  □ sqlc 代码生成 (make sqlc)
  □ Pipeline Edge Handler (Go CRUD API)
    - POST   /api/workspaces/:wsId/pipeline-edges
    - GET    /api/workspaces/:wsId/pipeline-edges?parent_issue_id=xxx
    - POST   /api/workspaces/:wsId/pipeline-edges/:id/annotations
    - GET    /api/workspaces/:wsId/issues/:id/pipeline (完整 DAG)
  □ Orchestrator 改造: 每步推进时创建 edge 记录

Day 5:
  □ Issue.metadata 写入
    - 创建 sub-issue 时写入 pipeline metadata (stage/branch/variant)
    - task completed 时更新 score 到 metadata
  □ Pipeline 查询 API 联调
    - 跑一遍流程，检查 DB 中数据完整性
    - 验证 edges 正确反映 checkpoint 之间的关系
  □ 前端 API client + TanStack Query hooks
    - packages/core/api/pipeline-edge.ts
    - packages/core/queries/pipeline-edge.ts
```

**Day 5 验收标准**：
```bash
# API 返回完整 pipeline 数据
curl /api/workspaces/:wsId/issues/MUL-1/pipeline
# 返回: { issues: [...], edges: [...], scores: {...} }
```

### Phase 3: 前端可视化（Day 6-8）

**目标**：Board 上能看到 Pipeline 流转 + 箭头 + 点击详情。

```
Day 6:
  □ Pipeline Filter 组件
    - Board 顶部加 "Pipeline: MUL-XX" 选择器
    - 选中后 Board 只显示该 pipeline 的 sub-issues
    - 列标题对应 stage 名称 (Clarify/Plan/Exec/Test/Done)
  □ Board Edge Overlay (SVG 箭头层)
    - 查询 pipeline_edges 数据
    - 计算卡片 DOM 位置 → 绘制贝塞尔曲线箭头
    - 颜色区分: 绿=flow, 蓝=branch, 紫=merge

Day 7:
  □ Edge Detail Panel (侧边面板)
    - 点击箭头弹出
    - 展示: score radar + token/duration + diff summary
    - 展示: branch name + git commit hash
  □ Score Badge
    - Issue 卡片上显示分数 (绿/黄/红色系)

Day 8:
  □ Annotation 功能
    - Edge Detail Panel 中 "添加标注" 按钮
    - 标注列表展示 (author + content + time)
  □ Pipeline 状态指示
    - 当前执行阶段高亮 + 脉动动画
    - 已完成阶段显示 ✓
  □ 整体联调: 跑一条完整 pipeline，观察 UI 实时更新
```

**Day 8 验收标准**：可以给人 demo 了——Chat 输入需求，Board 上看到卡片逐步出现 + 箭头连接 + 点击查看详情。

### Phase 4: 多方案竞争 + 评分（Day 9-11）

**目标**：Plan 阶段产出多方案，各自跑完并打分对比。

```
Day 9:
  □ Plan Agent prompt 改造
    - 要求输出 2-3 个方案 (结构化 JSON 输出)
    - Orchestrator 解析 → 创建多个 plan sub-issues + branch edges
  □ Orchestrator 分支逻辑
    - clarify done → N 个 plan (branch edge)
    - 每个 plan done → 各自 impl (flow edge)
    - 所有 impl 的 validate done → 触发方案选择

Day 10:
  □ 评分系统完善
    - 权重可配置 (score_config.json)
    - 分数标准化 0-100
    - 多方案对比数据结构
  □ 最优方案选择 + PR
    - 自动选最高分 或 等待用户标注选择
    - Final merge + gh pr create
  □ 用户介入: 暂停/继续/跳过 按钮

Day 11:
  □ 前端多方案对比视图
    - Board 上并行显示分支
    - Score 对比表 (维度 × 方案)
    - "选择此方案" 按钮
  □ 修复多方案下的 edge 渲染 (分叉 + 合流)
```

### Phase 5: Skill 蒸馏 + 收尾（Day 12-15）

**目标**：闭环验证 + 交付材料完成。

```
Day 12:
  □ Skill 蒸馏逻辑
    - Pipeline 完成后自动触发
    - 高分方案 → 提取 pattern → 生成 SKILL.md
    - 低分方案 → 提取错误 → 写入 Anti-Patterns
  □ Skill 召回机制
    - 新需求进入 plan 阶段时匹配 trigger_pattern
    - 注入到 Agent system prompt

Day 13:
  □ 闭环验证 (关键!)
    - 跑第一个需求 "文章加阅读量" → 蒸馏出 Skill
    - 跑第二个需求 "评论加点赞数" (类似模式) → 验证 Skill 被召回
    - 对比: 有 Skill vs 无 Skill 的 score 差异
  □ Clarify Agent 追问能力增强
    - 模糊需求检测逻辑
    - 多轮对话 → 结构化 spec

Day 14:
  □ 练手题批量验证
    - L1: 文章列表加阅读量 (纯前端)
    - L1: Popular Tags 前 5 打标
    - L2: 文章加封面图字段 (跨栈)
  □ Bug 修复 + 稳定性调优

Day 15:
  □ 部署到服务器 (49.232.233.9)
  □ 录制演示视频 (3-5min)
    - 场景 1: L1 简单需求端到端
    - 场景 2: L2 跨栈需求 + 多方案对比
    - 场景 3: Skill 召回效果展示
  □ README 完善 + 运行说明
  □ 补充架构图实际截图
```

### 关键里程碑检查点

| 时间 | 检查点 | 不通过的应对 |
|------|--------|-------------|
| **Day 1 中午** | doubao-seed 能对 Conduit 代码产出合理修改 | 调整 prompt 策略 / 增加上下文 |
| **Day 3 结束** | 单链路完整走通 (Chat→代码→lint pass) | 简化流程，砍多方案 |
| **Day 5 结束** | 数据持久化完成，API 就绪 | 前端先用 mock 数据 |
| **Day 8 结束** | 前端 Pipeline 可视化 Demo ready | 精简 UI 范围 |
| **Day 11 结束** | 多方案 + 评分工作 | 退回单方案但 Skill 做好 |
| **Day 14 结束** | 至少 2 道练手题稳定通过 | 集中修复最常见的失败模式 |

### 降级策略（从下往上砍）

```
优先级从高到低:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
1. 端到端单链路 + Board 箭头     ← MVP 底线，不能砍
2. 数据持久化 + Edge API         ← 展示需要
3. 多方案 + 评分                 ← 核心创新点
4. Skill 蒸馏闭环               ← 加分大项
5. 用户标注                     ← 可后置
6. 前端雷达图/对比表             ← 可简化为纯文字
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

如果 Day 11 时间不够:
- 多方案退化为单方案 (砍 Phase 4 的并行部分)
- Skill 蒸馏做最简版 (手动 import 而非自动蒸馏)
- 评分只做 lint pass/fail + test count (不做加权)
```

### 并行开发建议（如有多人）

```
人员 A (后端):  Phase 0 → Phase 1 → Phase 2 → Phase 4 后端
人员 B (前端):  Phase 2 API 确认后 → Phase 3 → Phase 4 前端
人员 C (AI/Prompt): Phase 0 → Phase 1 Prompt → Phase 5 Skill
```

---

## 十五、文件结构（新增/改动清单）

```
server/
├── migrations/
│   └── 09x_pipeline_edge.up.sql              🆕 新增
├── pkg/db/queries/
│   └── pipeline_edge.sql                     🆕 新增
├── internal/
│   ├── handler/
│   │   ├── pipeline_edge.go                  🆕 新增 (Edge CRUD API)
│   │   └── pipeline_edge_test.go             🆕 新增
│   ├── orchestrator/                         🆕 新增目录
│   │   ├── orchestrator.go                   🆕 核心编排
│   │   ├── stages.go                         🆕 各阶段逻辑
│   │   ├── scoring.go                        🆕 评分计算
│   │   ├── skill_recall.go                   🆕 Skill 召回
│   │   ├── skill_distill.go                  🆕 Skill 蒸馏
│   │   ├── prompts.go                        🆕 各阶段 Prompt
│   │   └── git_branch.go                     🆕 分支命名策略
│   └── daemon/
│       └── prompt.go                         ✏️ 小改 (注入 pipeline prompt)

packages/views/
├── issues/components/
│   ├── board-view.tsx                        ✏️ 小改 (加 EdgeOverlay wrapper)
│   ├── board-edge-overlay.tsx                🆕 SVG 箭头层
│   ├── edge-detail-panel.tsx                 🆕 点击箭头弹出面板
│   ├── score-badge.tsx                       🆕 分数 badge
│   └── pipeline-filter.tsx                   🆕 Pipeline 模式过滤器

packages/core/
├── api/
│   └── pipeline-edge.ts                      🆕 Edge API 客户端
├── queries/
│   └── pipeline-edge.ts                      🆕 TanStack Query hooks
└── types/
    └── pipeline.ts                           🆕 Pipeline 类型定义

docs/
└── super-individual-design.md                🆕 本文档
```

---

## 十六、与 Multica 现有功能的兼容映射

### 16.1 完全复用（不改动）

| 功能 | 复用方式 |
|------|---------|
| 用户认证 | PM 登录使用 |
| Workspace | 项目隔离 |
| Issue CRUD | 需求 + checkpoint 载体 |
| Sub-Issue | 各阶段的 checkpoint |
| Issue Status | 映射 Pipeline 阶段 |
| Issue Metadata JSONB | 存 pipeline 配置 + score |
| Chat Session | PM 交互入口 |
| Chat Message | 对话记录 |
| Agent | 执行主体 |
| agent_task_queue | 任务调度 |
| Task Message | 执行日志 |
| Daemon | Agent 运行时 |
| RepoCache | Git 仓库管理 |
| Worktree | 并行执行隔离 |
| Skill 表 | Skill Registry |
| Skill File | Skill 内容文件 |
| Agent-Skill 关联 | Skill 注入 |
| Activity Log | 审计追踪 |
| WebSocket | 实时推送 |
| Comment | 用户介入 |

### 16.2 小幅扩展

| 功能 | 改动内容 |
|------|---------|
| Board 视图 | 加 SVG overlay 层（~100 行） |
| Issue Card | 加 score badge 展示（~20 行） |
| Skill 模型 | 加 3 个字段 (source_type, trigger_pattern, scoring_stats) |
| Daemon prompt | 根据 task metadata 中的 stage 切换 prompt（~50 行） |

### 16.3 新增模块

| 模块 | 代码量 | 说明 |
|------|--------|------|
| pipeline_edge 表 + API | ~200 行 Go | 1 表 + CRUD |
| Orchestrator | ~500 行 Go | 核心编排 |
| 前端 Pipeline 组件 | ~400 行 TSX | Overlay + Panel |
| 评分模块 | ~150 行 Go | 计算 + 存储 |
| Skill 蒸馏 | ~200 行 Go | 分析 + 生成 |
| **总计** | **~1500 行** | |

---

## 十七、演示场景（答辩用）

### 场景 1：L1 入门题 — 文章列表加阅读量字段

```
PM: "在首页文章卡片上增加阅读量 icon + 数字展示"

1. [Clarify] Agent 追问："阅读量数据从哪来？前端假数据还是后端？"
   PM: "前端假数据即可"
   → spec.md 明确：纯前端、随机数、icon+数字

2. [Plan] 方案 A: 在 ArticlePreview 组件内加 span
          方案 B: 新建 ReadCount 组件复用

3. [Exec] 两个方案各自生成代码

4. [Validate] Lint pass, 无需后端测试

5. [Score] A: 92 (改动少), B: 85 (过度设计)

6. [Handoff] 选 A, 提 PR
```

### 场景 2：L2 进阶题 — 文章加封面图字段

```
PM: "文章加封面图"

1. [Clarify] Agent 追问尺寸、输入方式、展示位置、默认值
2. [Plan] 标准全栈新增字段方案
3. [Exec] Migration + Model + Route + 前端组件
4. [Validate] 后端 + 前端测试
5. [Score] 综合打分
6. [Handoff] PR 包含完整前后端变更
```

### 场景 3：现场新题验证 Skill 扩展性

```
评委: "给评论加一个 editedAt 字段"

系统自动召回 Skill "add-field-to-model"：
- 已知 pattern: migration + model + route + frontend
- Best practice: nullable, optional chain
- Template: 直接套用

→ 只用了 Skill，没有改主干代码
→ 证明扩展性
```

---

## 十八、关键技术决策记录

| # | 决策 | 选择 | 原因 |
|---|------|------|------|
| 1 | Pipeline 持久化 | Git branches + Issue metadata | 统一管理，天然支持断点重放 |
| 2 | Checkpoint 载体 | Sub-Issue (parent_issue_id) | 完全复用现有 UI |
| 3 | 多方案分支 | Git branch per variant | 与代码管理逻辑统一 |
| 4 | 阶段编排位置 | Go backend (Orchestrator) | 与现有 handler 同进程，延迟低 |
| 5 | Agent 数量 | 1 个 Agent + 不同 prompt | "超级个体"概念，Skill 决定行为 |
| 6 | 前端可视化 | Board + SVG Edge Overlay | 改动最小，复用看板 |
| 7 | 评分存储 | Issue.metadata + Edge.metadata | 不新增额外表 |
| 8 | Skill 蒸馏触发 | Pipeline 完成后自动 | 无需人工干预 |
| 9 | 模型对接 | doubao-seed-2.0 lite via OpenAI 兼容 API | 官方要求 |
| 10 | Conduit 操作 | Daemon RepoCache + Worktree | 已有基建，零成本复用 |

---

## 十九、风险与应对

| 风险 | 影响 | 应对 |
|------|------|------|
| Token 配额用尽 | 无法演示 | 错峰调用 + 缓存中间结果 + prompt 精简 |
| doubao-seed 生成质量不稳定 | 代码错误率高 | 多轮重试 + 人工 fallback |
| 3 周时间紧张 | MVP 不完整 | 严格按 Phase 1→2→3 优先级 |
| Conduit 仓库测试不完善 | validate 阶段无法打分 | 自写基础 lint 检查作为 baseline |
| 前端 SVG 箭头性能 | 卡片多时卡顿 | 限制 Pipeline 视图只显示单个 pipeline |
| Git 分支爆炸 | 仓库管理困难 | Pipeline 完成后归档非 final 分支 |

---

## 二十、交付清单对照

| 提交材料 | 本方案对应产出 |
|---------|--------------|
| 在线 Demo | 部署在 49.232.233.9，PM 可通过 Chat 输入需求 |
| 演示视频 | 3-5min 视频展示 L1+L2 题目端到端流程 |
| 源代码仓库 | Multica 改造仓 + Conduit fork |
| README | 启动步骤 + 目录结构 + 配置说明 |
| 系统架构图 | 本文档 §5.1 |
| 工程难点 | Git-as-state-machine / Skill 蒸馏 / 多方案竞争评分 |
| 核心技术栈 | Go + Next.js + PostgreSQL + doubao-seed |
| 项目亮点 | Skill 蒸馏闭环 / 断点重放 / 可观测性 / 最小改动复用 Multica |
