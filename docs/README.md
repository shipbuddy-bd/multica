# docs/ 目录说明

## 课题相关文档

| 文件 | 说明 |
|------|------|
| **[super-individual-design.md](./super-individual-design.md)** | 核心设计文档（系统架构、六阶段设计、数据模型、开发计划） |
| [要求文档.md](./要求文档.md) | 课题官方要求（评分标准、时间节点、练手题） |
| [Q&A.md](./Q&A.md) | 课题启动会答疑记录 |

## 文档结构

```
docs/
├── README.md                      ← 本文件（导航）
├── super-individual-design.md     ← 核心设计文档（1600+ 行）
│   ├── 一、课题目标对齐
│   ├── 二、核心设计理念 (Git as State Machine)
│   ├── 三、六阶段详细设计 (Clarify→Plan→Locate→Impl→Validate→PR)
│   ├── 四、Skill 蒸馏与自我迭代
│   ├── 五、系统架构
│   ├── 六、数据模型设计 (含现有 Task 体系复用分析)
│   ├── 七、前端设计 (Board + Edge Overlay)
│   ├── 八、Orchestrator 设计
│   ├── 九、Git 分支策略
│   ├── 十、评分系统
│   ├── 十一、用户介入机制
│   ├── 十二、可观测性
│   ├── 十三、Conduit 仓库集成
│   ├── 十四、实施计划 (Phase 0-5, 15 天)
│   ├── 十五、文件结构（新增/改动清单）
│   ├── 十六、与 Multica 现有功能的兼容映射
│   ├── 十七、演示场景（答辩用）
│   ├── 十八、关键技术决策记录
│   ├── 十九、风险与应对
│   └── 二十、交付清单对照
│
├── 要求文档.md                    ← 课题官方要求
├── Q&A.md                         ← 启动会答疑
├── assets/                        ← 图片资源
└── multica-original/              ← Multica 原有文档（归档，不影响课题）
    ├── product-overview.md
    ├── design.md
    ├── analytics.md
    └── ...
```

## 快速入口

- **要理解整体方案** → 读 `super-individual-design.md` 的 §一 ~ §五
- **要开始开发** → 读 §十四（实施计划）+ §十五（文件清单）
- **要理解数据模型** → 读 §六（含现有 Task 机制分析）
- **要做前端** → 读 §七（前端设计）
- **要写 Agent Prompt** → 读 §三（各阶段详细设计）+ §八（Orchestrator）
- **要准备答辩** → 读 §十七（演示场景）+ §二十（交付清单）
