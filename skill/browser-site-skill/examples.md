# 多模态目标描述改写示例

本文档展示如何把脆弱的传统 selector 改写为稳定的多模态目标描述。

## 示例 1: 简单按钮点击

### 传统方式 ❌

```python
# CSS Selector
driver.find_element(By.CSS_SELECTOR, "#submit-btn").click()

# XPath
driver.find_element(By.XPATH, "//button[@id='submit-btn']").click()
```

**问题：**
- ID 改了就挂
- 页面上可能有多个提交按钮
- 点击后不知道是否真正提交成功

### 多模态方式 ✅

```json
{
  "semantic_name": "submit_form_button",
  "visible_text": ["Submit", "提交", "确认"],
  "aria_role": "button",
  "dom_path_summary": "form > actions section",
  "state_before": ["表单已填写"],
  "state_after": ["表单提交成功消息出现", "或页面跳转到确认页"]
}
```

**优势：**
- 即使 ID 改了，仍可通过文本 + 角色 + 位置定位
- 明确是表单区域的按钮，不会误点其他提交按钮
- 验证提交是否真正成功

---

## 示例 2: 搜索框填写

### 传统方式 ❌

```python
# CSS Selector
driver.find_element(By.CSS_SELECTOR, "input.search-input").send_keys("iPhone 15")

# XPath
driver.find_element(By.XPATH, "//input[@placeholder='搜索商品']").send_keys("iPhone 15")
```

**问题：**
- CSS class 名改了就失效
- placeholder 文案改了就失效
- 不验证输入是否真正生效

### 多模态方式 ✅

```json
{
  "semantic_name": "product_search_input",
  "visible_text": ["搜索", "Search"],
  "aria_role": "searchbox",
  "dom_path_summary": "header > search section",
  "visual_cues": ["页面顶部", "搜索图标旁边"],
  "state_before": ["页面已加载", "搜索框可见"],
  "state_after": ["输入框包含 'iPhone 15'", "或搜索建议列表出现"],
  "interaction_type": "fill"
}
```

**优势：**
- 结合语义、文本、角色、位置多维度定位
- 明确是 header 区域的搜索框，不会误填其他输入框
- 验证输入是否生效（可能触发搜索建议）

---

## 示例 3: 列表中的多个相同按钮

### 场景

商品列表，每个商品卡片都有一个"加入购物车"按钮。

### 传统方式 ❌

```python
# 获取所有按钮，选第 3 个
buttons = driver.find_elements(By.CSS_SELECTOR, ".add-to-cart-btn")
buttons[2].click()  # 硬编码索引，非常脆弱
```

**问题：**
- 列表顺序变了就点错
- 商品被删除或新增后索引偏移
- 无法明确要点哪个商品

### 多模态方式 ✅

```json
{
  "semantic_name": "add_to_cart_for_iphone15",
  "visible_text": ["加入购物车", "Add to Cart"],
  "aria_role": "button",
  "dom_path_summary": "product list > product card[title='iPhone 15 Pro'] > actions",
  "visual_cues": ["iPhone 15 Pro 商品卡片", "价格下方", "右侧按钮"],
  "state_before": ["商品卡片可见", "商品有库存"],
  "state_after": ["购物车数量 +1", "或加购成功提示出现"],
  "interaction_type": "click"
}
```

**优势：**
- 通过 DOM 路径明确是哪个商品卡片的按钮
- 视觉线索进一步确认位置
- 验证加购是否真正成功

---

## 示例 4: 下拉菜单选择

### 传统方式 ❌

```python
# XPath
dropdown = driver.find_element(By.XPATH, "//select[@name='category']")
dropdown.find_element(By.XPATH, "//option[@value='electronics']").click()
```

**问题：**
- `name` 属性改了就失效
- `value` 值改了就失效
- 不验证选择是否生效

### 多模态方式 ✅

```json
{
  "semantic_name": "category_selector",
  "visible_text": ["分类", "Category"],
  "aria_role": "combobox",
  "dom_path_summary": "filters section > category filter",
  "state_before": ["筛选区域可见"],
  "state_after": ["分类显示为'电子产品'", "商品列表已更新"],
  "interaction_type": "select",
  "select_option": {
    "visible_text": ["电子产品", "Electronics"],
    "value": "electronics"
  }
}
```

**优势：**
- 同时记录选项的文本和值，双重保险
- 验证选择后商品列表是否更新
- 即使 `value` 改了，仍可通过文本选择

---

## 示例 5: 登录流程（多步骤）

### 传统方式 ❌

```python
driver.find_element(By.ID, "username").send_keys("user@example.com")
driver.find_element(By.ID, "password").send_keys("password123")
driver.find_element(By.CSS_SELECTOR, "button[type='submit']").click()
```

**问题：**
- ID 改了就挂
- 不验证每一步是否成功
- 登录失败时无法感知

### 多模态方式 ✅

**步骤 1: 填写用户名**
```json
{
  "semantic_name": "username_input",
  "visible_text": ["用户名", "邮箱", "Username", "Email"],
  "aria_role": "textbox",
  "dom_path_summary": "login form > username field",
  "state_before": ["登录表单可见"],
  "state_after": ["用户名输入框包含 'user@example.com'"],
  "interaction_type": "fill"
}
```

**步骤 2: 填写密码**
```json
{
  "semantic_name": "password_input",
  "visible_text": ["密码", "Password"],
  "aria_role": "textbox",
  "dom_path_summary": "login form > password field",
  "state_before": ["用户名已填写"],
  "state_after": ["密码输入框包含值（不可见）"],
  "interaction_type": "fill"
}
```

**步骤 3: 点击登录**
```json
{
  "semantic_name": "login_submit_button",
  "visible_text": ["登录", "Login", "Sign In"],
  "aria_role": "button",
  "dom_path_summary": "login form > submit button",
  "state_before": ["用户名和密码已填写", "登录按钮可点击"],
  "state_after": ["URL 变为 /dashboard", "或用户头像出现", "或欢迎消息出现"],
  "interaction_type": "click",
  "wait_conditions": ["登录按钮不再显示加载动画"]
}
```

**优势：**
- 每一步都有状态验证
- 可以在任何一步失败时准确定位问题
- 处理异步登录（等待加载动画消失）

---

## 示例 6: 处理动态内容

### 场景

电商网站的商品详情页，价格会根据促销活动动态变化。

### 传统方式 ❌

```python
# 直接点击"立即购买"，不管价格
driver.find_element(By.CLASS_NAME, "buy-now-btn").click()
```

**问题：**
- 不知道当前价格是否合理
- 促销失效后仍按原价购买
- 无法验证是否按预期价格加购

### 多模态方式 ✅

**步骤 1: 验证价格**
```json
{
  "semantic_name": "product_price_check",
  "visible_text": ["价格", "Price", "¥"],
  "dom_path_summary": "product details > price section",
  "state_before": ["商品详情页已加载"],
  "state_after": ["价格在 5000-8000 元范围内", "或显示促销标签"],
  "interaction_type": "observe"
}
```

**步骤 2: 点击购买**
```json
{
  "semantic_name": "buy_now_button",
  "visible_text": ["立即购买", "Buy Now"],
  "aria_role": "button",
  "dom_path_summary": "product details > actions",
  "visual_cues": ["价格下方", "红色按钮", "最显眼的 CTA"],
  "state_before": ["价格已验证合理", "商品有库存"],
  "state_after": ["进入结算页", "或加入购物车成功"],
  "interaction_type": "click"
}
```

**优势：**
- 先验证价格，再决定是否购买
- 包含业务逻辑（价格范围检查）
- 防止意外按错误价格购买

---

## 示例 7: 处理弹窗和中断

### 场景

点击按钮后可能出现确认弹窗。

### 传统方式 ❌

```python
# 点击删除按钮
driver.find_element(By.ID, "delete-btn").click()

# 硬等待弹窗
time.sleep(2)

# 点击确认（但不知道弹窗是否真的出现了）
driver.find_element(By.XPATH, "//button[text()='确认']").click()
```

**问题：**
- 硬等待不可靠（弹窗可能早出现或根本不出现）
- 弹窗没出现时会报错
- 不验证删除是否真正成功

### 多模态方式 ✅

**步骤 1: 点击删除**
```json
{
  "semantic_name": "delete_item_button",
  "visible_text": ["删除", "Delete"],
  "aria_role": "button",
  "dom_path_summary": "item card > actions",
  "state_before": ["商品卡片可见"],
  "state_after": ["确认弹窗出现", "或商品直接消失（无弹窗）"],
  "interaction_type": "click"
}
```

**步骤 2: 确认删除（条件执行）**
```json
{
  "semantic_name": "confirm_delete_dialog",
  "visible_text": ["确认", "Confirm", "是的，删除"],
  "aria_role": "button",
  "dom_path_summary": "dialog > actions",
  "state_before": ["确认弹窗可见"],
  "state_after": ["弹窗关闭", "商品从列表中消失"],
  "interaction_type": "click",
  "wait_conditions": ["弹窗出现", "或超时 3 秒后跳过"]
}
```

**优势：**
- 验证弹窗是否真的出现
- 处理有弹窗和无弹窗两种情况
- 验证删除是否真正成功

---

## 改写流程总结

把传统 selector 改写为多模态目标描述的通用流程：

### 1. 识别目标的语义

- 这个元素是做什么的？
- 用户如何理解这个元素？
- 给它一个有意义的 `semantic_name`

### 2. 提取多个可见维度

- **文本**：元素显示的文字（包含可能的多语言版本）
- **角色**：button、link、textbox、combobox 等
- **位置**：在页面的哪个区域？相对于什么元素？

### 3. 明确前后状态

- **执行前**：需要满足什么条件？（如表单已填写、页面已加载）
- **执行后**：期望发生什么变化？（如 URL 改变、元素出现/消失、文本更新）

### 4. 处理特殊情况

- **动态内容**：如何处理加载延迟、异步更新？
- **多个相同元素**：如何区分具体要操作哪一个？
- **中断和弹窗**：如何处理不确定出现的元素？

### 5. 逐步补充字段

- 从最小集合（3 个必填）开始
- 如果定位失败或有歧义，补充 DOM 路径、视觉线索
- 如果有异步加载，补充等待条件

---

## 常见误区

### ❌ 误区 1: 把完整 XPath 放进 `dom_path_summary`

```json
{
  "dom_path_summary": "/html/body/div[1]/div[2]/main/div[3]/form/button"
}
```

这仍然是脆弱的，应该用语义化的路径摘要：

```json
{
  "dom_path_summary": "main content > registration form > submit button"
}
```

### ❌ 误区 2: 只填必填字段，不补充其他维度

最小集合够用时确实可以只填必填字段，但如果定位失败，应主动补充而不是放弃多模态方式。

### ❌ 误区 3: 状态验证过于模糊

```json
{
  "state_after": ["页面变化了"]
}
```

应该明确具体的变化：

```json
{
  "state_after": ["URL 包含 /success", "成功消息出现"]
}
```

### ❌ 误区 4: 过度依赖 `fallback_selectors`

`fallback_selectors` 应该是最后手段，不要一开始就依赖它。如果经常需要降级到 fallback，说明多模态描述需要改进。

---

## 实战练习

尝试把以下传统 selector 改写为多模态目标描述：

### 练习 1
```python
driver.find_element(By.ID, "checkout-btn").click()
```

<details>
<summary>参考答案</summary>

```json
{
  "semantic_name": "proceed_to_checkout",
  "visible_text": ["结算", "Checkout", "去结算"],
  "aria_role": "button",
  "dom_path_summary": "shopping cart > actions",
  "visual_cues": ["购物车底部", "主按钮"],
  "state_before": ["购物车有商品"],
  "state_after": ["进入结算页", "URL 包含 /checkout"]
}
```
</details>

### 练习 2
```python
driver.find_element(By.XPATH, "//input[@type='email']").send_keys("test@example.com")
```

<details>
<summary>参考答案</summary>

```json
{
  "semantic_name": "email_input",
  "visible_text": ["邮箱", "Email", "电子邮件"],
  "aria_role": "textbox",
  "dom_path_summary": "registration form > email field",
  "state_before": ["注册表单可见"],
  "state_after": ["邮箱输入框包含 'test@example.com'"],
  "interaction_type": "fill"
}
```
</details>

### 练习 3
```python
items = driver.find_elements(By.CLASS_NAME, "product-card")
items[0].find_element(By.TAG_NAME, "a").click()
```

<details>
<summary>参考答案</summary>

```json
{
  "semantic_name": "view_first_product_details",
  "visible_text": ["查看详情", "View Details", "商品标题"],
  "aria_role": "link",
  "dom_path_summary": "product list > first product card > title link",
  "visual_cues": ["列表第一个商品", "商品图片上方"],
  "state_before": ["商品列表已加载"],
  "state_after": ["进入商品详情页", "URL 包含 /product/"]
}
```
</details>

---

通过这些示例，你应该能够理解如何把脆弱的传统 selector 改写为稳定的多模态目标描述。记住核心原则：**多维度、语义化、状态验证**。
