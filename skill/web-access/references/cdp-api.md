# CDP Proxy API 参考

## 基础信息

- 地址：`http://localhost:3456`
- 启动：`node ../scripts/cdp-proxy.mjs &`
- 启动后持续运行，不建议主动停止（重启需 Chrome 重新授权）
- 强制停止：`pkill -f cdp-proxy.mjs`

## Chrome 调试连接方式

有两种方式让 Chrome 开启远程调试：

### 方式一：命令行参数启动（推荐）

用 `--remote-debugging-port` 参数启动 Chrome：

```bash
# macOS
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome --remote-debugging-port=9222

# 或使用 chrome-profile-cdp-skill 的克隆模式（推荐，不影响当前浏览器）
bash /path/to/chrome-profile-cdp-skill/scripts/start_profiled_debug_chrome.sh https://example.com
```

**优点**：
- 不会弹出授权确认框
- CDP 连接直接可用
- 可以使用克隆 profile 保留登录态

**缺点**：
- 需要单独启动一个 Chrome 实例
- clone 模式可能丢失部分 session 状态

### 方式二：chrome://inspect 开启

1. 打开 `chrome://inspect/#remote-debugging`
2. 勾选 "Allow remote debugging for this browser instance"

**优点**：
- 可以直接在当前浏览器上操作
- 登录态完全保留

**缺点**：
- **每次 CDP 连接都会弹出授权确认框**
- 需要用户手动点击"允许"

### 避免重复授权的策略

如果使用 `chrome://inspect` 方式：
- **保持 CDP Proxy 长期运行**，不要频繁重启
- Proxy 只要不断开 WebSocket 连接，就不会再弹授权框
- 如果 Proxy 意外断开，重新连接时会再次弹窗

## API 端点

### GET /health
健康检查，返回连接状态。
```bash
curl -s http://localhost:3456/health
```

### GET /targets
列出所有已打开的页面 tab。返回数组，每项含 `targetId`、`title`、`url`。
```bash
curl -s http://localhost:3456/targets
```

### GET /new?url=URL
创建新后台 tab，自动等待页面加载完成。返回 `{ targetId }`.
```bash
curl -s "http://localhost:3456/new?url=https://example.com"
```

### GET /close?target=ID
关闭指定 tab。
```bash
curl -s "http://localhost:3456/close?target=TARGET_ID"
```

### GET /navigate?target=ID&url=URL
在已有 tab 中导航到新 URL，自动等待加载。
```bash
curl -s "http://localhost:3456/navigate?target=ID&url=https://example.com"
```

### GET /back?target=ID
后退一页。
```bash
curl -s "http://localhost:3456/back?target=ID"
```

### GET /info?target=ID
获取页面基础信息（title、url、readyState）。
```bash
curl -s "http://localhost:3456/info?target=ID"
```

### POST /eval?target=ID
执行 JavaScript 表达式，POST body 为 JS 代码。
```bash
curl -s -X POST "http://localhost:3456/eval?target=ID" -d 'document.title'
```

### POST /click?target=ID
JS 层面点击（`el.click()`），POST body 为 CSS 选择器。自动 scrollIntoView 后点击。简单快速，覆盖大多数场景。
```bash
curl -s -X POST "http://localhost:3456/click?target=ID" -d 'button.submit'
```

### POST /clickAt?target=ID
CDP 浏览器级真实鼠标点击（`Input.dispatchMouseEvent`），POST body 为 CSS 选择器。先获取元素坐标，再模拟鼠标按下/释放。算真实用户手势，能触发文件对话框、绕过部分反自动化检测。
```bash
curl -s -X POST "http://localhost:3456/clickAt?target=ID" -d 'button.upload'
```

### POST /setFiles?target=ID
给 file input 设置本地文件路径（`DOM.setFileInputFiles`），完全绕过文件对话框。POST body 为 JSON。
```bash
curl -s -X POST "http://localhost:3456/setFiles?target=ID" -d '{"selector":"input[type=file]","files":["/path/to/file1.png","/path/to/file2.png"]}'
```

### GET /scroll?target=ID&y=3000&direction=down
滚动页面。`direction` 可选 `down`（默认）、`up`、`top`、`bottom`。滚动后自动等待 800ms 供懒加载触发。
```bash
curl -s "http://localhost:3456/scroll?target=ID&y=3000"
curl -s "http://localhost:3456/scroll?target=ID&direction=bottom"
```

### GET /screenshot?target=ID&file=/tmp/shot.png
截图。指定 `file` 参数保存到本地文件；不指定则返回图片二进制。可选 `format=jpeg`。
```bash
curl -s "http://localhost:3456/screenshot?target=ID&file=/tmp/shot.png"
```

## /eval 使用提示

- POST body 为任意 JS 表达式，返回 `{ value }` 或 `{ error }`
- 支持 `awaitPromise`：可以写 async 表达式
- 返回值必须是可序列化的（字符串、数字、对象），DOM 节点不能直接返回，需要提取属性
- 提取大量数据时用 `JSON.stringify()` 包裹，确保返回字符串
- 根据页面实际 DOM 结构编写选择器，不要套用固定模板

## 错误处理

| 错误 | 原因 | 解决 |
|------|------|------|
| `Chrome 未开启远程调试端口` | Chrome 未开启远程调试 | 提示用户打开 `chrome://inspect/#remote-debugging` 并勾选 Allow，或运行 `check-deps.sh --launch-clone` |
| `attach 失败` | targetId 无效或 tab 已关闭 | 用 `/targets` 获取最新列表 |
| `CDP 命令超时` | 页面长时间未响应 | 重试或检查 tab 状态 |
| `端口已被占用` | 另一个 proxy 已在运行 | 已有实例可直接复用 |

## 操作 xterm.js 终端

部分平台内嵌了 xterm.js 终端。通过 `/eval` 发送键盘事件来输入命令：

### 发送命令到终端

```javascript
// 通过 /eval 执行以下 JS
(() => {
  const textarea = document.querySelector(".xterm-helper-textarea");
  if (!textarea) return { error: "未找到终端" };
  
  textarea.focus();
  
  // 发送字符（用 keypress）
  const text = "ls -la";
  for (const char of text) {
    const pressEvent = new KeyboardEvent("keypress", {
      key: char,
      charCode: char.charCodeAt(0),
      bubbles: true
    });
    textarea.dispatchEvent(pressEvent);
  }
  
  // 发送 Enter 执行（用 keydown）
  const enterDown = new KeyboardEvent("keydown", {
    key: "Enter",
    code: "Enter",
    keyCode: 13,
    which: 13,
    bubbles: true
  });
  textarea.dispatchEvent(enterDown);
  
  return { sent: text };
})()
```

### 注意事项

- **不要同时发 keydown + keypress**：会导致字符重复
- **字符用 keypress，Enter 用 keydown**
- xterm 可能在 iframe 或 shadow DOM 中，需要先定位正确的 document
- 如果终端未聚焦，先点击终端区域：`/clickAt` + `.terminal.xterm`
