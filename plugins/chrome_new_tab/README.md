# ChatGPT New Tab

一个极简、零权限的 Chrome 扩展：每次打开新标签页时，直接进入官方
[ChatGPT](https://chatgpt.com/) 首页。

## 安装

1. 在 Chrome 地址栏打开 `chrome://extensions/`。
2. 打开右上角的「开发者模式」。
3. 点击「加载已解压的扩展程序」。
4. 选择本目录 `plugins/chrome_new_tab/`。
5. 如果 Chrome 提示多个扩展都想控制新标签页，请保留启用本扩展。

安装后新建一个标签页即可验证。ChatGPT 会沿用浏览器中已有的登录状态。

## 设计说明

ChatGPT 的安全策略不允许第三方页面通过 iframe 嵌入，因此扩展会从本地新
标签页立即跳转到 `https://chatgpt.com/`。这可以完整保留官网功能，也避免扩展
接触账号、对话或浏览记录。

扩展不申请任何 Chrome 权限，不包含后台服务，不收集或传输数据。

## 自检

```bash
npm test
```

该命令只使用 Node.js 内置模块，不需要安装依赖。
