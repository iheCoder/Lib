# 多模态目标描述参考文档

本文档详细说明多模态目标描述的 schema、匹配流程、验证机制，以及如何在浏览器自动化中稳定使用。

## 为什么需要多模态目标描述

传统浏览器自动化依赖单一维度的定位器：

```python
# 脆弱的传统方式
driver.find_element(By.CSS_SELECTOR, "#submit-btn").click()
driver.find_element(By.XPATH, "//button[@class='checkout']").click()
```

**问题：**
- CSS class 名改了就挂
- ID 属性重构就断
- 页面结构调整后 XPath 失效
- 找到多个匹配元素时不知道点哪个
- 点击后不知道是否真正生效

**多模态目标描述的优势：**
- 结合语义、文本、角色、位置、状态等多个维度
- 单一维度失效时，其他维度仍可定位
- 包含前后状态约束，确保动作真正完成
- 像人类一样理解页面："点那个语义是提交订单、文本是 Submit、位于结算区块右下角、且点击后会出现订单确认页的按钮"

## 多模态目标描述 Schema

### 完整字段定义

```json
{
  "semantic_name": "string",
  "visible_text": ["string", "..."],
  "aria_role": "string",
  "dom_path_summary": "string",
  "visual_cues": ["string", "..."],
  "state_before": ["string", "..."],
  "state_after": ["string", "..."],
  "fallback_selectors": ["string", "..."],
  "interaction_type": "string",
  "wait_conditions": ["string", "..."]
}
```

### 字段说明

#### 必填字段

| 字段 | 类型 | 说明 | 示例 |
|------|------|------|------|
| `semantic_name` | string | 目标的语义名称，描述用途 | `"submit_order"`, `"product_search"` |
| `visible_text` | string[] | 元素可见文本，支持模糊匹配 | `["Submit", "Place order", "提交订单"]` |
| `state_after` | string[] | 动作执行后必须满足的状态 | `["订单确认页出现", "URL 包含 /order/success"]` |

#### 高优先级字段

| 字段 | 类型 | 说明 | 示例 |
|------|------|------|------|
| `aria_role` | string | ARIA 角色，提升可访问性 | `"button"`, `"searchbox"`, `"navigation"` |
| `dom_path_summary` | string | DOM 路径摘要，非完整路径 | `"checkout > footer > actions"` |
| `state_before` | string[] | 动作执行前必须满足的前置条件 | `["购物车有商品", "结算区块可见"]` |

#### 可选字段

| 字段 | 类型 | 说明 | 示例 |
|------|------|------|------|
| `visual_cues` | string[] | 视觉线索，帮助人类理解位置 | `["右下角", "蓝色主按钮", "最大的 CTA"]` |
| `fallback_selectors` | string[] | 降级选择器，仅在多模态匹配失败时使用 | `["#submit-btn", ".checkout-button"]` |
| `interaction_type` | string | 交互类型 | `"click"`, `"fill"`, `"select"`, `"hover"` |
| `wait_conditions` | string[] | 等待条件，处理异步加载 | `["按钮变为可点击", "加载动画消失"]` |

### 最小有效描述

虽然上述 schema 定义了多个字段，但实际使用中可以从最小集合开始，逐步补充：

**最小集合（3 个必填）：**
```json
{
  "semantic_name": "login_button",
  "visible_text": ["登录", "Login"],
  "state_after": ["进入用户主页", "URL 变为 /dashboard"]
}
```

**推荐集合（+3 个高优先级）：**
```json
{
  "semantic_name": "login_button",
  "visible_text": ["登录", "Login"],
  "aria_role": "button",
  "dom_path_summary": "header > auth section",
  "state_before": ["登录表单已填写"],
  "state_after": ["进入用户主页", "URL 变为 /dashboard"]
}
```

**完整集合（需要时补充可选字段）：**
```json
{
  "semantic_name": "login_button",
  "visible_text": ["登录", "Login"],
  "aria_role": "button",
  "dom_path_summary": "header > auth section",
  "visual_cues": ["表单底部", "蓝色按钮"],
  "state_before": ["登录表单已填写", "按钮为可点击状态"],
  "state_after": ["进入用户主页", "URL 变为 /dashboard"],
  "fallback_selectors": ["button[type='submit']"],
  "interaction_type": "click",
  "wait_conditions": ["按钮不再显示加载动画"]
}
```

## 匹配流程

多模态目标描述的匹配遵循从宽到窄的漏斗策略：

```
所有页面元素
    ↓ 步骤 1: 语义与状态预检
候选元素集合 A
    ↓ 步骤 2: 文本与角色过滤
候选元素集合 B
    ↓ 步骤 3: DOM 路径与视觉线索缩小
候选元素集合 C
    ↓ 步骤 4: 选择最佳匹配
唯一目标元素
    ↓ 步骤 5: 验证前置状态
执行动作
    ↓ 步骤 6: 验证后置状态
动作确认完成
```

### 步骤 1: 语义与状态预检

检查 `state_before` 是否满足。如果不满足，暂停并报告当前页面状态。

```python
# 伪代码
if not all(check_state(s) for s in target["state_before"]):
    raise StateNotReadyError(f"前置条件未满足: {target['state_before']}")
```

### 步骤 2: 文本与角色过滤

获取所有包含 `visible_text` 且角色为 `aria_role` 的元素：

```python
# 伪代码
candidates = []
for element in page.all_elements:
    text_match = any(text in element.visible_text for text in target["visible_text"])
    role_match = element.aria_role == target["aria_role"]
    if text_match and role_match:
        candidates.append(element)
```

如果 `candidates` 为空，尝试放宽匹配（如模糊文本匹配、忽略大小写）。

### 步骤 3: DOM 路径与视觉线索缩小

如果 `candidates` 包含多个元素，使用 `dom_path_summary` 和 `visual_cues` 进一步过滤：

```python
# 伪代码
if len(candidates) > 1 and "dom_path_summary" in target:
    candidates = [e for e in candidates if matches_path_summary(e, target["dom_path_summary"])]

if len(candidates) > 1 and "visual_cues" in target:
    # 视觉线索匹配（位置、样式、大小等）
    candidates = filter_by_visual_cues(candidates, target["visual_cues"])
```

### 步骤 4: 选择最佳匹配

- 如果只剩 1 个候选，使用它
- 如果仍有多个候选，选择最符合语义的（如文本完全匹配优先于部分匹配）
- 如果无法决定，报告冲突并请求用户补充描述

```python
# 伪代码
if len(candidates) == 0:
    if "fallback_selectors" in target:
        return try_fallback_selectors(target["fallback_selectors"])
    raise ElementNotFoundError(f"无法找到目标: {target['semantic_name']}")

if len(candidates) == 1:
    return candidates[0]

if len(candidates) > 1:
    raise AmbiguousTargetError(f"找到 {len(candidates)} 个匹配元素，需要更多线索")
```

### 步骤 5: 验证前置状态

再次确认 `state_before`（可能在匹配过程中页面发生变化）：

```python
# 伪代码
if not all(check_state(s) for s in target["state_before"]):
    raise StateChangedError("前置状态在匹配过程中改变")
```

### 步骤 6: 执行动作并验证后置状态

执行交互，然后立刻验证 `state_after`：

```python
# 伪代码
element.perform_interaction(target.get("interaction_type", "click"))

# 等待状态变化
wait_for_stable_state(timeout=5)

# 验证后置状态
if not all(check_state(s) for s in target["state_after"]):
    raise StateVerificationError(f"动作执行后状态未达预期: {target['state_after']}")
```

## 状态验证机制

`state_before` 和 `state_after` 是多模态目标描述的核心，确保动作真正完成。

### 状态表达方式

状态可以用自然语言描述，Agent 负责将其转换为可验证的条件：

| 状态描述 | 验证方式 |
|---------|---------|
| `"购物车有商品"` | 检查购物车元素内是否有商品列表，数量 > 0 |
| `"登录表单已填写"` | 检查用户名和密码输入框是否非空 |
| `"按钮为可点击状态"` | 检查按钮是否不是 `disabled` 状态 |
| `"URL 包含 /order/success"` | 检查 `window.location.href` |
| `"订单确认页出现"` | 检查页面是否包含"订单确认"相关文本或特定元素 |
| `"加载动画消失"` | 检查 loading spinner 元素是否不再可见 |
| `"弹窗已关闭"` | 检查 modal/dialog 元素是否不在 DOM 或不可见 |

### 状态检查优先级

1. **URL 变化**：最可靠，优先检查
2. **特定元素出现/消失**：明确的 DOM 变化
3. **文本内容变化**：页面关键信息更新
4. **样式/属性变化**：按钮状态、表单验证等

### 失败时的处理

状态验证失败时，不应直接放弃，而应：

1. **等待重试**：某些状态变化有延迟（如网络请求、动画）
2. **检查异常**：是否有错误提示、警告弹窗
3. **截图记录**：保存当前页面状态，便于调试
4. **降级方案**：如果有 `fallback_selectors`，尝试使用
5. **报告失败**：向用户说明失败原因和当前页面状态

## DOM 路径摘要规范

`dom_path_summary` 不是完整的 XPath 或 CSS selector，而是人类可读的路径描述。

### 好的 DOM 路径摘要

```
✅ "header > search section"
✅ "checkout > footer > actions"
✅ "product list > item card > add to cart button"
✅ "sidebar > filters > price range"
```

### 不好的 DOM 路径摘要

```
❌ "/html/body/div[1]/div[2]/header/div[3]/form/input"  (太具体，易碎)
❌ "div > div > div > button"  (太模糊，无语义)
❌ "#root > .container-fluid > .row"  (依赖实现细节)
```

### 构建原则

1. 使用语义化的区块名称（header, footer, sidebar, main content）
2. 只包含关键层级，跳过无意义的 div 嵌套
3. 从大到小（从页面区域到具体元素）
4. 避免数字索引，除非是列表项

## 视觉线索规范

`visual_cues` 帮助 Agent 像人类一样理解页面布局。

### 好的视觉线索

```json
✅ ["页面右下角", "蓝色主按钮", "最大的 CTA"]
✅ ["表单底部", "与取消按钮并排", "右侧按钮"]
✅ ["页面顶部", "搜索框右侧", "放大镜图标"]
✅ ["商品卡片右上角", "价格下方", "红色按钮"]
```

### 不好的视觉线索

```json
❌ ["按钮"]  (太模糊)
❌ ["坐标 (850, 320)"]  (依赖屏幕分辨率)
❌ ["第 3 个 div 里面"]  (无语义)
```

### 构建原则

1. 使用相对位置（相对于页面区域或其他元素）
2. 包含颜色、大小等显著特征
3. 描述在视觉层次中的重要性（"主按钮"、"次要链接"）

## 实战建议

### 何时需要补充更多字段

- **只有 3 个必填字段足够**：页面结构简单，目标唯一明确
- **需要补充 DOM 路径**：页面有多个文本相同的按钮（如列表中的多个"删除"按钮）
- **需要补充视觉线索**：页面布局复杂，需要空间关系辅助定位
- **需要补充等待条件**：页面有异步加载、动画、延迟渲染

### 从失败中学习

第一次匹配失败时，不要立刻放弃多模态描述，而是：

1. 检查是哪个维度匹配失败
2. 补充失败维度的信息
3. 更新多模态描述，重新尝试
4. 如果多次失败，考虑页面结构是否已变化

### 与传统 selector 的共存

`fallback_selectors` 字段允许在多模态匹配失败时降级到传统 selector，但应该：

1. 仅作为最后手段
2. 记录降级事件，提示可能需要更新多模态描述
3. 不要依赖 fallback，而应持续改进多模态描述

## 调试与审计

### 记录匹配过程

每次匹配应记录：
- 初始候选数量
- 每个过滤步骤后剩余候选数量
- 最终选择的元素及其属性
- 状态验证结果

### 可视化调试

建议在调试模式下：
- 高亮所有候选元素
- 显示每个候选元素的匹配得分
- 截图保存关键步骤
- 生成匹配过程的可视化报告

### 性能考虑

多模态匹配比单一 selector 更复杂，但应保持高效：
- 缓存元素查询结果
- 优先使用高效的匹配维度（如 aria_role）
- 避免在大列表上进行视觉线索匹配
- 合理设置超时时间

## 总结

多模态目标描述的核心思想是：**像人类一样理解页面，用多个维度共同确定目标，并验证动作真正完成**。

记住三个关键点：
1. **多维度**：不依赖单一脆弱的 selector
2. **语义化**：用人类可理解的方式描述目标
3. **状态验证**：确保动作真正生效

这种方式虽然比传统 selector 更复杂，但能显著提升自动化脚本的稳定性和可维护性。
