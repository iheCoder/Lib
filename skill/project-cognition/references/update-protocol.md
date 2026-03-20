# 更新协议

Agent 在维护项目认知时必须遵循本协议，防止内容发散、覆盖已有知识或写入不准确结论。

## 写入前的检查流程

```
1. 判断这是「新增」还是「修订」？
   ├── 修订：先找到对应文件，在原地追加，不新建
   └── 新增：确认没有更合适的已有文件后再新建

2. 判断应落在哪一层？
   ├── 规则/原则/禁区/修改策略 → core-manifest/
   ├── 系统关系/数据链路/字段口径/业务背景 → domain-knowledge/
   └── 命令/脚本/修复步骤/巡检 → runbooks/

3. 内容是否经过验证？
   ├── 已验证（有代码/线上/文档佐证） → status: verified
   └── 未验证（来自对话推断/记忆） → status: draft，内容中注明 [待确认]

4. 写完后：
   └── 同步更新 project_cognition/INDEX.md 对应表格
```

## 增量写入规则

### 修改已有文档时

- 在对应小节末尾追加内容，不删改原有事实
- 如果原有内容有误：不要直接修改，在其下方追加 `> [修正 YYYY-MM-DD]` 块说明正确内容
- 如果原有内容已废弃：追加 `> [已废弃 YYYY-MM-DD]` 块，并在 frontmatter 里改 status 为 `deprecated`

### 新建文档时

1. 从对应层的 `_template.md` 复制
2. 文件名使用 kebab-case（小写+连字符）
3. 填写全部 frontmatter 必填字段，至少：`status`、`scope`、`last_reviewed`、`when_to_update`
4. 主内容优先写「已知确认」的部分；不确定的部分用 `[待确认]` 标注

### 写入禁止行为

| 禁止 | 原因 |
|------|------|
| 把聊天推断直接标记为 `verified` | 未经验证的结论会误导后续 Agent |
| 删除原有已验证内容 | 破坏知识的可溯源性 |
| 把操作步骤写进 domain-knowledge | 破坏层级语义，导致搜索混乱 |
| 把系统原则写进 runbooks | 同上 |
| 复制大段代码或 README 正文 | 造成维护负担，优先链接 canonical 文档 |
| 不更新 INDEX.md | 索引失效，新认知无法被发现 |

## 更新 INDEX.md

每次写完认知文档后，在 `project_cognition/INDEX.md` 对应表格中追加或更新一行：

**core-manifest 表格**：

```markdown
| core-manifest/<file>.md | <主题一句话> | verified/draft | YYYY-MM-DD |
```

**domain-knowledge 表格**：

```markdown
| domain-knowledge/<file>.md | <主题一句话> | <全局/子系统> | verified/draft | YYYY-MM-DD |
```

**runbooks 表格**：

```markdown
| runbooks/<file>.md | <主题一句话> | <适用场景> | verified/draft | YYYY-MM-DD |
```

## 触发更新的典型场景

| 场景 | 应写入哪层 | 操作 |
|------|-----------|------|
| 发现禁止操作被绕过 | core-manifest | 在「已知例外」小节追加记录 |
| 了解到新的数据链路 | domain-knowledge | 新建或追加到已有链路文档 |
| 修复了一个数据 Bug | runbooks | 新建或追加修复步骤 |
| 某个脚本参数有变化 | runbooks | 更新对应 runbook 步骤 |
| 架构调整导致旧链路废弃 | domain-knowledge | 追加废弃标记，建新文档描述新链路 |
| 某字段口径被重新定义 | domain-knowledge | 追加历史修正块 |

## Status 状态转换

```
draft → verified：有明确代码/文档/线上验证佐证时
verified → deprecated：相关系统/功能下线，或被新文档替代时
draft → deprecated：内容长期无法验证，且无人使用时
```
