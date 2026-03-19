# 浏览器站点技能 (Browser Site Skill)

一个通用的浏览器自动化 skill，用**多模态目标描述**替代脆弱的 CSS/XPath selector，并支持将一次探索成功沉淀为可复用的**站点技能文档**。

## 核心理念

### 1. 多模态目标描述

传统浏览器自动化依赖单一维度的定位器（如 CSS selector 或 XPath），一旦页面改版就容易失效。多模态目标描述结合**语义、文本、角色、位置、状态**等多个维度，像人类一样理解页面。

**示例对比：**

```python
# ❌ 传统方式：脆弱
driver.find_element(By.ID, "submit-btn").click()

# ✅ 多模态方式：稳定
{
  "semantic_name": "submit_order",
  "visible_text": ["Submit", "提交订单"],
  "aria_role": "button",
  "dom_path_summary": "checkout > footer > actions",
  "state_before": ["购物车有商品"],
  "state_after": ["订单确认页出现"]
}
```

### 2. 站点技能沉淀

不让 Agent 每次都重新理解一个网站，而是把首次探索成功后的经验沉淀为**站点技能文档**，包含：
- 关键目标的多模态描述
- 工作流步骤
- 决策点与异常分支
- 成功判定标准
- 漂移信号（用于检测网站改版）

## 文件结构

```
browser-site-skill/
├── SKILL.md                    # 主 skill 入口，定义触发条件和核心规则
├── reference.md                # 多模态目标描述的详细规范
├── examples.md                 # selector 到多模态描述的改写示例
├── site-skill-template.md      # 站点技能文档模板
└── README.md                   # 本文件
```

## 快速开始

### 场景 1：自动化某个网站任务

当你说：「帮我在某购物网站搜索 iPhone，筛选价格 5000-8000 的商品」

Agent 会：
1. 进入**探索模式**
2. 用多模态描述识别关键元素（搜索框、价格筛选器）
3. 执行操作并验证每步的状态变化
4. 记录探索轨迹（可选）

### 场景 2：沉淀站点技能

当你说：「把刚才的探索沉淀成可复用的站点技能」

Agent 会：
1. 进入**沉淀模式**
2. 分析轨迹，识别稳定部分与变化项
3. 参数化变化项（如搜索词、价格范围）
4. 基于 `site-skill-template.md` 生成站点技能文档
5. 展示给你确认后写入文件

## 使用指南

### 阅读顺序

1. **[SKILL.md](SKILL.md)** - 了解何时使用、核心规则、两种工作模式
2. **[examples.md](examples.md)** - 看几个具体例子，理解如何改写 selector
3. **[reference.md](reference.md)** - 深入了解 schema 定义和匹配流程（按需阅读）
4. **[site-skill-template.md](site-skill-template.md)** - 准备沉淀站点技能时使用

### 何时使用本 skill

- ✅ 需要稳定的网页自动化（点击、填写、导航）
- ✅ 网站结构相对稳定，但 CSS class/ID 可能改变
- ✅ 需要验证每步操作是否真正生效
- ✅ 需要把探索经验沉淀为可复用的站点技能

### 何时不使用本 skill

- ❌ 只是抓取网页内容（用 `scrapling` skill）
- ❌ 一次性简单任务（直接用浏览器工具即可）
- ❌ 网站结构极不稳定，每周都改版

## 与其他 skill 的关系

- **vs scrapling**：scrapling 专注网页抓取（获取内容），本 skill 专注网页操作（交互、填表、点击）
- **vs open-prose**：可配合使用，open-prose 负责工作流编排，本 skill 负责单站点自动化的稳定执行

## 关键特性

### ✅ 多维度定位
结合语义、文本、角色、位置等多个维度，不依赖单一脆弱的 selector

### ✅ 状态验证
每次关键动作后立刻验证 `state_after`，确保操作真正生效

### ✅ 失败回退
当锚点冲突、页面漂移时，重新观察而不是盲目重试

### ✅ 知识沉淀
把一次探索编译成可复用的站点技能，避免重复劳动

### ✅ 人类可读
所有描述都是自然语言，便于理解、调试和维护

## 示例：完整流程

### 步骤 1: 探索模式

**用户**：「帮我在 example.com 搜索 iPhone 15，价格 5000-8000 元，加入购物车」

**Agent**：
```json
// 步骤 1: 识别搜索框
{
  "semantic_name": "product_search_input",
  "visible_text": ["搜索", "商品名称"],
  "aria_role": "searchbox",
  "state_after": ["输入框包含 'iPhone 15'"]
}

// 步骤 2: 识别价格筛选
{
  "semantic_name": "price_filter",
  "visible_text": ["价格", "筛选"],
  "dom_path_summary": "filters section > price range",
  "state_after": ["显示 5000-8000 元筛选结果"]
}

// 步骤 3: 加入购物车
{
  "semantic_name": "add_to_cart_first_item",
  "visible_text": ["加入购物车"],
  "aria_role": "button",
  "state_after": ["购物车数量 +1"]
}
```

**结果**：✅ 任务完成，商品已加入购物车

### 步骤 2: 沉淀模式

**用户**：「把这个流程沉淀成站点技能」

**Agent**：
1. 分析轨迹，识别：
   - 稳定部分：搜索框位置、筛选器结构、加购按钮
   - 变化项：搜索词（`search_keyword`）、价格范围（`price_range`）

2. 生成站点技能文档：
   ```markdown
   # 站点技能：Example.com 商品搜索与加购
   
   ## 输入参数
   - search_keyword: 搜索关键词
   - price_range: 价格范围
   
   ## 关键目标
   - product_search_input: {...}
   - price_filter: {...}
   - add_to_cart_button: {...}
   
   ## 工作流
   1. 搜索商品
   2. 筛选价格
   3. 加入购物车
   
   ## 成功判定
   - 购物车中包含指定商品
   ```

3. 展示给用户确认

4. 写入 `site-skills/example-com-search.md`

**结果**：✅ 站点技能已沉淀，下次可直接复用

## 进阶话题

### 如何处理动态内容

在多模态描述中加入 `wait_conditions`：

```json
{
  "semantic_name": "dynamic_list_item",
  "wait_conditions": ["列表加载完成", "骨架屏消失"],
  "state_after": ["至少有 1 个商品卡片"]
}
```

### 如何处理多个相同元素

使用 `dom_path_summary` 和 `visual_cues` 区分：

```json
{
  "semantic_name": "delete_button_for_item_3",
  "dom_path_summary": "item list > item[3] > actions",
  "visual_cues": ["第 3 个商品卡片", "右上角删除按钮"]
}
```

### 如何与 OpenProse 集成

```prose
# example-com-search.prose

input search_keyword
input price_range

use "site-skills/example-com-search.md" as site_skill

session search:
  prompt: "使用站点技能在 example.com 搜索：{search_keyword}"
  context: { site_skill, search_keyword, price_range }

output result
```

## 常见问题

### Q: 多模态描述会不会太复杂？

A: 从最小集合（3 个必填字段）开始，只在需要时补充其他字段。大多数场景下，语义 + 文本 + 状态验证就足够了。

### Q: 如果网站改版了怎么办？

A: 多模态描述比传统 selector 更稳定，但如果仍失效，会触发**漂移信号**，提示你重新探索并更新站点技能。

### Q: 能不能直接用传统 selector？

A: 可以在 `fallback_selectors` 字段中提供降级方案，但不建议依赖它。多模态描述才是长期稳定的选择。

### Q: 站点技能文档必须手写吗？

A: 不需要。Agent 会基于探索轨迹自动生成，你只需确认和微调。

## 贡献与反馈

如果你在使用中发现问题或有改进建议，欢迎：
- 提交 issue
- 分享你的站点技能文档
- 补充更多示例

---

**快速链接：**
- [SKILL.md](SKILL.md) - 主 skill 入口
- [reference.md](reference.md) - 详细规范
- [examples.md](examples.md) - 改写示例
- [site-skill-template.md](site-skill-template.md) - 站点技能模板
