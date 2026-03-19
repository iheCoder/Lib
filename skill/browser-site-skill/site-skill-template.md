# 站点技能模板 v2

这个模板是给 Codex 用的轻量沉淀模板。

目标不是写一份“网站说明书”，而是写出一份之后能稳定复用的 skill。

只保留真正影响执行稳定性的内容：

- 目标
- 入口
- 参数
- 关键锚点
- 成功判定
- fallback
- 漂移信号

---

# [站点名] Site Skill

> 创建日期：YYYY-MM-DD
> 最后验证：YYYY-MM-DD
> 站点类型：`ui-only` | `ui-plus-data` | `api-driven`
> 状态：`active` | `needs-review`

## Goal

一句话说明这个 skill 用来完成什么。

示例：

- 在 Arena leaderboard 中找到 Code / React / Open Source 下排名最高模型
- 在某后台中按条件筛选订单并导出 CSV

## Stable Entry Points

- 主入口：`https://...`
- 可选快捷入口：`https://...`

优先写能直达任务上下文的 URL。

## Parameters

只列变化项。

```yaml
parameters:
  arena:
    required: true
    example: "Code"
  category:
    required: true
    example: "React"
  license_type:
    required: false
    default: "All"
```

如果没有参数，就写：

```yaml
parameters: {}
```

## Workflow

只写完成任务真正必要的步骤。

1. 打开稳定入口 URL
2. 确认页面位于正确任务上下文
3. 设置必要筛选或输入参数
4. 读取结果
5. 用成功判定确认结果有效

如果某一步只在特定参数存在时执行，直接写条件。

## Key Anchors

每个关键目标只写最小可用锚点。

### [semantic_name]

```json
{
  "semantic_name": "[semantic_name]",
  "visible_text": ["..."],
  "state_after": ["..."]
}
```

按需补充：

```json
{
  "aria_role": "...",
  "visual_cues": ["..."],
  "dom_path_summary": "..."
}
```

推荐只记录 2 到 5 个关键目标。

## Validation Rules

完成任务前，至少验证这些条件：

- 页面在正确站点
- 参数对应的上下文正确
- 关键筛选已生效
- 读取到的结果确实属于目标集合

如果是列表型页面，明确说明“为什么第一条就是答案”。

## Output Shape

说明最终要返回什么字段。

```text
- Model: ...
- Organization: ...
- License: ...
- Rank: ...
- Rating: ...
- URL: ...
```

## Fallback Strategy

这是模板里最重要的一段之一。

如果 UI 难以稳定读取，写明更稳定的数据来源：

- SSR payload
- hydration JSON
- 页面接口响应
- 导出文件

同时写明 UI 条件如何映射到底层数据。

示例：

```text
If License Type is "Open Source" in the UI, treat matching rows as entries whose
license field is not "Proprietary" in the current page payload.
```

如果没有 fallback，就明确写：

```text
No reliable non-UI fallback identified. Re-check through page state only.
```

## Drift Signals

只写最关键的 3 到 6 条。

- 某关键筛选器消失
- 入口 URL 不再直达对应页面
- 列表结构变了，第一行不再对应最高排名
- fallback 数据结构改变
- UI 文案与底层字段映射关系失效

## Known Good Snapshot

可选，但对排行榜、后台配置页这类动态页面很有帮助。

```yaml
last_verified: YYYY-MM-DD
example_result:
  model: "..."
  rank: "..."
```

强调它只是参考，不是最终 truth。

---

## 编写检查清单

提交前确认：

- 是否写清楚了稳定入口
- 是否把变化项参数化了
- 是否只保留了关键锚点
- 是否写清楚了成功判定
- 是否写清楚了 fallback source of truth
- 是否写清楚了 drift signals

如果答案有任何一个是否定的，先别沉淀为正式 site skill。
