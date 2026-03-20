---
name: project-cognition
description: 项目认知系统，管理项目规则、领域知识和操作经验。当用户提到项目架构、系统链路、领域知识、操作经验、修 bug 手册、知识沉淀、项目记忆更新、禁区原则、或希望了解某模块/系统的背景时激活。负责读取已有认知并按规范写入新的认知。
---

# Project Cognition

项目认知分三层，每层职责互不重叠：

| 层级 | 目录 | 存什么 |
|------|------|--------|
| Core Manifest | `project_cognition/core-manifest/` | 项目原则、禁区、修改策略、验证要求 |
| Domain Knowledge | `project_cognition/domain-knowledge/` | 业务链路、系统关系、关键字段、数据流 |
| Runbooks | `project_cognition/runbooks/` | 操作命令、脚本用法、故障修复、巡检步骤 |

## 读取顺序

1. 先读 `project_cognition/INDEX.md` 了解全局结构
2. 任何任务都先读 `core-manifest/` 中相关的规则文件
3. 按任务类型再选：
   - 理解系统/背景 → `domain-knowledge/`
   - 执行操作/修复 → `runbooks/`

## 写入决策树

```
新知识应该写在哪里？
├── 这是规则、原则、禁区或修改策略？ → core-manifest/
├── 这是系统关系、数据链路、字段口径、业务背景？ → domain-knowledge/
└── 这是命令、脚本步骤、故障修复或巡检流程？ → runbooks/
```

写入前：先搜索是否已有匹配文件，优先增量追加，不要重建已有文档。

## 写入约束

- **不要**把一次性聊天结论直接写成 `verified` 状态
- **不要**把操作命令写进 `domain-knowledge/`
- **不要**把业务原则写进 `runbooks/`
- **不要**复制大段代码或 README，优先 `@引用` 或链接到 canonical 文档
- 未经验证的内容标记为 `draft` 或注释 `[待确认]`
- 写完后同步更新 `INDEX.md`

## 参考文档

- 写入流程与决策规则：[references/update-protocol.md](references/update-protocol.md)
- 三层写法示例：[references/examples.md](references/examples.md)
