---
name: web-access
license: MIT
github: https://github.com/eze-is/web-access
description:
  处理联网任务的统一策略 skill。适用于搜索、网页阅读、页面核实、登录后访问、动态站点交互、社交平台内容获取，以及任何需要在 Codex 原生 web 能力与本机真实浏览器之间做正确切换的场景。
metadata:
  author: 一泽Eze
  adapted_for: Codex
  version: "2.4.0-codex.1"
---

# web-access Skill

这是一个为 Codex 适配过的版本。它保留原仓库最有价值的三件事：

1. 联网时先想清楚目标，再选工具，而不是固定走某一条链路
2. 需要登录态或真实交互时，用 CDP 接管本机 Chrome
3. 对特定站点积累经验，减少重复踩坑

同时，它不照搬 Claude Code 的工具假设。在 Codex 中：

- 搜索、打开网页、找文本、获取最新信息，优先使用内建 `web` 工具
- 只有在 `web` 不足以完成任务时，才升级到 `curl`、Jina 或本地 CDP
- 只有当用户明确要求委派、并行子 agent 或多代理协作时，才拆分给子 agent

## 浏览哲学

像人一样围绕目标行动，而不是像脚本一样固守步骤。

1. 先定义成功标准
什么算完成？是找到最新来源、核实某条说法、读完一个页面，还是在登录后台完成交互？

2. 选最可能成功的第一步
能用 Codex `web` 直接完成，就先别上浏览器。只有遇到动态渲染、登录墙、反爬、复杂交互时再升级。

3. 用结果校正路径
搜索没命中，不代表要机械重复搜索；页面报“内容不存在”，也不一定真不存在，可能是访问方式不对。

4. 完成即止
达到用户目标就停，不为了“更完整”而继续增加成本或侵入用户环境。

## 工具分层

| 场景 | 首选 |
|------|------|
| 查最新信息、搜索候选来源、找官方链接 | Codex `web.search_query` |
| 已知 URL，需要阅读页面、找特定段落、点站内链接 | Codex `web.open` / `web.find` / `web.click` |
| 需要天气、价格、比赛、时间等结构化实时数据 | 对应 Codex `web` 子工具 |
| 需要原始 HTML、meta、JSON-LD、接口返回 | `curl` |
| 文章、博客、文档、PDF，希望低 token 提取正文 | Jina (`https://r.jina.ai/http://...`) |
| 需要登录态、动态渲染、文件上传、视频采帧、真实点击 | 本地 CDP Proxy |

原则：

- 能用 Codex 原生 `web` 完成的，不要先启动浏览器
- 用户问“最新”“今天”“刚刚”“现在”，必须真的联网核实
- 核实真伪时，一手来源优先于聚合结果
- 页面内已有完整链接时，优先使用页面真实生成的 URL，不手搓地址

## CDP 模式

CDP 模式通过 HTTP Proxy 连接本机 Chrome，让 Codex 在不接管用户当前 tab 的前提下，使用后台 tab 完成交互。

默认策略：

- 优先附着已经开启远程调试的 Chrome
- 如未开启，可提示用户打开 `chrome://inspect/#remote-debugging` 并允许当前浏览器实例远程调试
- 若用户接受启动一个隔离的调试 Chrome，可使用现有 `chrome-profile-cdp-skill` 的 clone 模式兜底

前置检查：

```bash
bash ./scripts/check-deps.sh
```

如需在当前机器自动拉起一个隔离调试 Chrome：

```bash
bash ./scripts/check-deps.sh --launch-clone
```

说明：

- Node.js 22+ 已足够运行 Proxy
- `--launch-clone` 会尝试调用同工作区下的 `chrome-profile-cdp-skill`
- 除非用户明确接受，不要主动替用户拉起新的 Chrome 实例

## Proxy API

```bash
curl -s http://127.0.0.1:3456/targets
curl -s "http://127.0.0.1:3456/new?url=https://example.com"
curl -s "http://127.0.0.1:3456/info?target=ID"
curl -s -X POST "http://127.0.0.1:3456/eval?target=ID" -d 'document.title'
curl -s -X POST "http://127.0.0.1:3456/click?target=ID" -d 'button.submit'
curl -s -X POST "http://127.0.0.1:3456/clickAt?target=ID" -d 'button.upload'
curl -s -X POST "http://127.0.0.1:3456/setFiles?target=ID" -d '{"selector":"input[type=file]","files":["/path/to/file.png"]}'
curl -s "http://127.0.0.1:3456/scroll?target=ID&direction=bottom"
curl -s "http://127.0.0.1:3456/screenshot?target=ID&file=/tmp/shot.png"
curl -s "http://127.0.0.1:3456/close?target=ID"
```

工作方式：

- 用 `/new` 创建后台 tab
- 先 `/eval` 观察页面结构，再决定点击、滚动、填表还是提取媒体
- 结束后关闭自己创建的 tab，不碰用户原有 tab

## Codex 下的使用约束

1. 如果任务只是“查资料”，先用 `web`
2. 如果任务涉及当前时间、价格、新闻、法规、产品变更，必须联网核实
3. 如果任务引用了具体网页或官方文档，尽量给出来源链接
4. 如果用户没有明确要求多 agent，不主动做并行子 agent 拆分
5. 如果浏览器层失败，先判断是登录问题、URL 构造问题还是反爬问题，不盲目重试

## 登录判断

核心只看一件事：目标内容有没有拿到。

只有在确认目标内容拿不到，而且登录大概率能解决时，才提示用户：

当前页面在未登录状态下无法获取目标内容，请先在你的 Chrome 中登录对应网站，然后告诉我继续。

如果已经拿到内容，就不要额外要求用户登录。

## 技术事实

- DOM 中常有已加载但未展示的内容，可直接通过结构化提取触达
- 先滚动再提取图片 URL，通常更完整
- 平台返回“内容不存在”可能是访问方式问题，不一定是内容真的不存在
- 页面生成的完整链接通常比手动拼接的链接更可靠
- 短时间批量开很多 tab 可能触发风控

## 站点经验

站点经验存放在 `references/site-patterns/`。

确定目标域名后，如果存在对应文件，先读取它；如果没有，再按通用模式探索。

成功完成后，如发现了可复用且已验证的站点规律，可以补充经验文件，但只记录事实，不记录猜测。

已有经验可通过下面命令查看：

```bash
ls ./references/site-patterns 2>/dev/null | sed 's/\.md$//' || echo "暂无"
```

## 何时读取额外参考

| 文件 | 何时加载 |
|------|---------|
| `references/cdp-api.md` | 需要更细的 CDP API、错误处理、JS 提取示例时 |
| `references/site-patterns/{domain}.md` | 已经确定目标站点时 |
