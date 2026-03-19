---
name: browser-site-skill
description: 浏览器站点自动化与站点技能沉淀框架。用户提到浏览器操作、网页自动化、站点工作流、页面交互、沉淀站点技能、复用某网站流程时激活。用于先探索网站，再将稳定入口、参数、关键目标、成功判定和 fallback 数据源沉淀为可复用的站点 skill。
---

# 浏览器站点技能 v2

这个 skill 的目标不是讲概念，而是帮助你稳定完成两件事：

1. 在浏览器里完成一个网站任务
2. 把这次成功流程沉淀成一个可复用的站点 skill

核心原则：

- 用自然语言可解释的锚点，不依赖单一脆弱 selector
- 每个关键动作都要有成功判定
- 对动态网站，优先识别更稳定的 truth source，例如 SSR payload、JSON hydration、接口响应
- 沉淀 skill 时只保留高价值、可复用、可验证的信息

## 何时使用

- 用户要你在某个网站执行一段流程
- 用户要你把一次网站探索沉淀成 site skill
- 任务需要反复复用同一站点的浏览器流程
- 任务是排行榜、搜索页、表格页、管理台之类有稳定页面结构的网站流程

不适用：

- 纯抓取静态网页内容
- 一次性非常简单的点击任务
- 网站极不稳定，且没有更稳定的数据源

## 两种模式

### 模式 1：探索并执行

适用于第一次做某个站点任务。

输出至少包括：

- 任务结果
- 成功判定
- 一份探索轨迹

### 模式 2：沉淀站点技能

适用于已经完成一次探索，要把经验编译成可复用 skill。

输出至少包括：

- 轻量站点 skill 文档
- 稳定入口
- 参数
- 关键目标
- 成功判定
- fallback 数据源
- 漂移信号

## 站点分型

先判断站点属于哪一类，再决定沉淀策略。

### A 类：纯 UI 交互站

特征：

- 主要靠点击、输入、跳转完成流程
- 页面本身就是主要 truth source

沉淀重点：

- 入口 URL
- 关键按钮/输入框锚点
- 每步 state_after

### B 类：UI + 数据载荷站

特征：

- 页面可操作
- 但关键信息也出现在 SSR payload、hydration 数据、内联 JSON 或接口响应里

沉淀重点：

- UI 流程
- fallback 数据源
- UI 条件与数据条件的映射关系

排行榜、搜索结果、表格筛选页通常属于这一类。

### C 类：API 驱动站

特征：

- UI 只是壳
- 真正稳定结果来自接口

沉淀重点：

- UI 入口
- 关键请求或稳定响应结构
- 如何用页面语义映射到数据字段

## 最小可用锚点规范

不要默认把所有字段都写满。按优先级记录。

### 必填

```json
{
  "semantic_name": "license_open_source",
  "visible_text": ["Open Source"],
  "state_after": ["Open Source 已选中", "结果列表已更新"]
}
```

### 推荐补充

```json
{
  "aria_role": "radio",
  "visual_cues": ["位于 License Type 分组内", "与 Proprietary 并列"]
}
```

### 按需补充

```json
{
  "dom_path_summary": "left filter panel > License Type",
  "state_before": ["筛选面板可见"],
  "interaction_type": "select"
}
```

原则：

- 能靠 `visible_text + state_after` 说清楚，就不要硬写 DOM
- 只有在候选太多时，再加视觉线索或路径摘要
- 如果页面是数据型页面，锚点之外必须再写 fallback source

## 探索流程

1. 明确任务目标与最终成功状态
2. 识别稳定入口 URL
3. 判断站点分型
4. 识别关键目标，只记录最小可用锚点
5. 执行动作
6. 每一步都验证 `state_after`
7. 如果结果来自列表、搜索、排行榜，检查是否存在稳定的数据 fallback
8. 产出探索轨迹

## 探索轨迹格式

沉淀前，先把本次成功流程记录成下面这种结构。可以是 JSON，也可以是等价的结构化笔记。

```json
{
  "site_name": "Arena",
  "site_type": "ui-plus-data",
  "task_goal": "找到 Code / React / Open Source 下排名最高模型",
  "start_url": "https://arena.ai/leaderboard/code/react",
  "parameters": {
    "arena": "Code",
    "category": "React",
    "license_type": "Open Source"
  },
  "actions": [
    {
      "target": "license_open_source",
      "action": "select",
      "state_after": ["Open Source 已选中"]
    }
  ],
  "stable_targets": [
    {
      "semantic_name": "license_open_source",
      "visible_text": ["Open Source"],
      "state_after": ["筛选结果更新"]
    }
  ],
  "fallback_sources": [
    {
      "kind": "server-rendered-payload",
      "location": "HTML payload",
      "usage": "在 UI 难以可靠读取表格时，用于验证 entries"
    }
  ],
  "success_checks": [
    "页面位于正确 arena/category",
    "license filter 为 Open Source",
    "返回第一条匹配结果"
  ]
}
```

如果没有这类轨迹，说明还没准备好沉淀站点 skill。

## 沉淀规则

沉淀 skill 时，必须做这几件事：

1. 区分稳定部分与变化项
2. 把变化项抽成参数
3. 写清楚 stable entry point
4. 写清楚 success checks
5. 写清楚 fallback source of truth
6. 写清楚 drift signals

不要把探索过程的噪音原样拷进 skill。

### 稳定部分

- 固定入口 URL
- 长期存在的过滤器、搜索框、结果表格
- 可稳定复现的成功判定

### 变化项

- 搜索词
- category
- 日期范围
- license type
- 排名条件

## 推荐的 skill 结构

沉淀后的站点 skill 默认只保留下面几块：

1. Skill 目标
2. Stable entry points
3. Parameters
4. Workflow
5. Key anchors
6. Validation rules
7. Fallback strategy
8. Drift signals

除非站点确实复杂，否则不要默认写冗长状态机、异常百科、禁止事项大全。

## 动态数据网站的特别规则

当任务涉及排行榜、搜索结果、表格、筛选列表时，额外检查：

- 页面结果是否来自 SSR payload
- 是否有 hydration 数据
- 是否能从页面中提取结构化 entries
- UI 里的词，如 `Open Source`，与数据字段，如 `license`，之间的映射关系是什么

如果你发现 UI 条件与底层数据不是一一等价，必须把这种解释规则写进 fallback strategy。

## 失败处理

### 找不到目标

- 重新观察页面
- 补充视觉线索
- 不要盲目沿用旧锚点

### 动作后状态不成立

- 检查是否有弹窗、加载延迟、权限问题
- 把动作视为失败，不要假设成功

### 页面结构变了

- 优先用 fallback 数据源确认任务还能否完成
- 如果 UI 与 fallback 都失效，标记 skill 待更新

## 写 skill 时最常见的坏味道

- 把“理念”写很多，入口和成功判定写很少
- 锚点写得过重，但没有 fallback
- 只写 UI，不写数据 truth source
- 没有参数化，导致 skill 只能服务一次任务
- 没有 drift signals，导致技能老化后仍被盲目复用

## 何时加载附加文档

按需加载：

- [reference.md](reference.md)：当你需要更细的多模态字段定义时
- [examples.md](examples.md)：当你要把脆弱 selector 改写成语义锚点时
- [site-skill-template.md](site-skill-template.md)：当你要输出最终沉淀 skill 时

默认不要把这些文档全量搬进最终 skill。
