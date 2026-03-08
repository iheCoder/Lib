# OpenProse (plugin)

Adds the OpenProse skill pack and `/prose` slash command.

## 安装到本地 Open Claw

若希望在本机 Open Claw 中使用此插件（含飞书 stage 结果卡片投递和 QQ 图片投递），可安装到 `~/.openclaw/extensions`：

```bash
# 在仓库根目录执行（Lib 或包含 skill/open-prose 的仓库）
./skill/open-prose/install-openclaw.sh
```

或手动操作：

1. 创建扩展目录并链接（或复制）插件：
   ```bash
   mkdir -p ~/.openclaw/extensions
   ln -sfn "$(pwd)/skill/open-prose" ~/.openclaw/extensions/open-prose
   ```
2. 在 `~/.openclaw/openclaw.json` 的 `plugins.entries` 中启用：
   ```json
   "open-prose": { "enabled": true }
   ```
3. 重启 Open Claw / Gateway 使插件生效。

## Enable

Bundled plugins are disabled by default. Enable this one:

```json
{
  "plugins": {
    "entries": {
      "open-prose": { "enabled": true }
    }
  }
}
```

Restart the Gateway after enabling.

## What you get

- `/prose` slash command (user-invocable skill)
- OpenProse VM semantics (`.prose` programs + multi-agent orchestration)
- Telemetry support (best-effort, per OpenProse spec)
- **Stage 结果投递**：在飞书或 QQ 渠道运行 investment 类 `.prose` 时，各 stage 的结果会自动发送到当前会话
  - **飞书**：以 Interactive Card 形式发送，失败时回退为普通文本消息（需在 `openclaw.json` 中配置 `channels.feishu` 且当前会话的 `ctx.to` 为 chat_id）
  - **QQ**：将 stage markdown 渲染成图片并发送

## 投资研究怎么触发

OpenProse 是**斜杠命令**，只有发带 `/prose` 的消息才会走插件。发一句「深度研究下牧原股份现在是否值得投资」不会自动进 OpenProse。

请直接发：

```text
/prose invest 深度研究下牧原股份现在是否值得投资
```

或带文件名：

```text
/prose run investment_research_pipeline.prose 深度研究下牧原股份现在是否值得投资
```

发完后会跑多阶段投资研究流程，并在飞书里以 Interactive Card 形式发送各 stage 的结果（若已配置飞书）。

## 故障排查：❌ Subagent main failed

当 OpenProse 用 VM 模式跑多代理（如 `invest_muyuan.prose`）时，每个 `session` 会通过 OpenClaw 的 Task 起一个 **subagent**。若出现 **❌ Subagent main failed**，多半是**子代理在拉模型时失败**，而不是你写的 .prose 逻辑错。

### 从日志确认原因

看 `~/.openclaw/logs/gateway.err.log` 里**紧挨着**失败时间点的报错，例如：

- **OAuth token refresh failed for openai-codex**  
  子代理要用 `openai-codex`，刷新 OAuth 时失败。常见原因：
  - **连不上 auth.openai.com**：日志里会有 `ConnectTimeoutError (attempted address: auth.openai.com:443, timeout: 10000ms)`。  
    - 解决：检查本机网络/代理，确保能访问 `auth.openai.com:443`（如用 VPN 或换网络后再试）。
  - **Token 过期或无效**：  
    - 解决：在终端执行 `openclaw auth`（或 Open Claw 里重新登录 openai-codex），再重跑工作流。

- **Model Not Exist / 模型不存在**  
  若用的是非 DeepSeek 的 provider，在 2026.3.1+ 曾出现过子代理解析模型失败。  
  - 解决：确认 `openclaw.json` 里 `agents.defaults.model.primary` 等配置的模型名与当前 Open Claw 版本兼容，或暂时改用 DeepSeek 等已知可用的模型。

### 建议操作顺序

1. 打开 `~/.openclaw/logs/gateway.err.log`，找到报错行（如 `lane task error: lane=subagent` 或 `OAuth token refresh failed`）。
2. 若是 **Connect Timeout**：先保证网络能访问 OpenAI（或换网络/代理），再重试。
3. 若是 **OAuth / 需要重新登录**：执行 `openclaw auth` 重新认证 openai-codex，然后再次运行同一 .prose。
4. 若仍失败：把 `gateway.err.log` 里相关时间段的几行错误贴出来，便于进一步排查。

---

## 为什么只返回一个 Subagent 结果、没有多阶段也没有图片？

### 日志结论（你这次的情况）

- 飞书里你发的是 **`/prose invest 深度研究下牧原股份现在是否值得投资`**。
- `gateway.log` 显示这条消息被 **dispatch 到了 agent**（`dispatching to agent (session=agent:main:main)`），没有先走插件的 **registerCommand("prose")**。
- `subagents/runs.json` 里对应那次运行的 task 是 **`Program: invest_muyuan.prose`**、**`let raw_data = session: researcher`**，说明是**主 agent 在当 VM**，跑的是 workspace 里的 **invest_muyuan.prose**（多 session：researcher → analyst → parallel → output），不是插件里的 **11 阶段 investment 管线**（planner → financial → industry → … → report）。
- 运行目录在 **workspace**：`~/.openclaw/workspace/.prose/runs/20260307-220700-invest/`，里面只有 `bindings/` 和 `state.md`，**没有** `stages/`（planner.md、financial.md 等）。插件若跑过，会在 **extension 下的 .prose/runs/** 生成带 `stages/` 的目录。

所以可以确定：

1. **这次从飞书发的 `/prose invest` 没有进插件**，而是被主 agent 当普通消息处理，agent 自己选了 invest_muyuan.prose 当 VM 程序。
2. **多阶段 + 投递飞书** 只在**插件路径**里实现：  
   - 插件里是 `executeInvestmentWorkflow`（11 个 stage：planner / financial / industry / news / macro / sentiment / thesis / risk / critic / report），每个 stage 会写 `stages/*.md`，最后通过飞书 API 以 Interactive Card 形式发送。  
   - VM 路径（invest_muyuan.prose）是 agent 用 Task 起 subagent，没有走插件的「stage → 卡片 → 飞书」逻辑，所以**不会有卡片投递**。
3. **只看到一个 “Subagent main finished”**：VM 只跑完了**第一个** session（researcher），就结束回合、把这一段结果总结给你了，没有继续 spawn analyst、parallel、output 等后续 session（可能是回合/上下文限制或 VM 没继续往下执行）。

### 想要「完整多阶段 + 卡片发飞书」该怎么做

- **目标**：让 **插件的 `/prose` 命令** 处理 `/prose invest ...`，这样才会跑 11 阶段管线并以卡片形式投递。
- **当前**：在飞书里发 `/prose invest ...` 时，OpenClaw 把整句当普通消息给了 agent，所以走的是 VM + invest_muyuan.prose，不是插件。
- **建议**：
  1. **先用 CLI 验证插件路径**（能跑满 11 阶段且会写 stages、可本地看是否发送卡片）：  
     ```bash
     openclaw agent --message "/prose invest 深度研究下牧原股份现在是否值得投资"
     ```  
     若 CLI 下能跑出多阶段并在 extension 的 `.prose/runs/` 下看到 `stages/` 和 `feishu-delivery.json`，说明插件逻辑正常，问题在飞书侧没有把 `/prose` 路由到插件。
  2. **飞书侧**：若 OpenClaw 支持「斜杠命令先由插件处理」，需在配置或 channel 里把 **`/prose`** 配成由 **prose 插件** 处理，而不是直接进 agent。具体要看 OpenClaw 的 Feishu 集成文档里「命令路由」或「slash command」的说明。
  3. **临时绕开**：在飞书里不发 `/prose invest`，而是发一句明确「用插件跑投资研究」的指令（若 agent 会调用插件或能触发插件命令的话）；或者继续用 CLI 跑 `/prose invest` 拿完整报告和卡片。

总结：**不是 OpenProse 多 agent 或卡片投递逻辑坏了，而是飞书这条消息没进插件，只进了 VM 路径，且 VM 只跑了一个 session 就结束了。** 要多阶段 + 卡片，需要让 `/prose invest` 走插件命令路径（例如通过 CLI 或调整 Feishu 命令路由）。

### 飞书侧：为什么 /prose 没进插件？（可直接按下面排查）

OpenClaw 的斜杠命令逻辑是：**先**尝试插件命令（`handlePluginCommand`，其中 `matchPluginCommand` 匹配 `/prose`），匹配则执行插件并直接返回；**否则**再走内置命令和 agent。所以理论上飞书发 `/prose invest ...` 应该先被插件接到。若实际没进插件，可能是下面几种情况之一：

1. **命令未作为「整条消息」或格式被改**
   - 要求：消息必须是**一条独立消息**，且以 `/` 开头（不要前面有空格或引用）。
   - 在飞书里发：`/prose help`（不要加前缀或空格），看是否收到插件返回的帮助文案。若收到，说明 `/prose` 能进插件；若收到的是 agent 的回复，说明这条没进插件。

2. **`commands.text` 被关掉**
   - 在 `~/.openclaw/openclaw.json` 里确认没有把斜杠命令关掉：
   - 若有 `"commands": { "text": false }`，改成 `true` 或删掉该项（默认 true）。改完重启 Gateway。

3. **open-prose 插件未加载**
   - 若 Feishu 用的 Gateway 进程里 open-prose 没加载，插件命令就不会注册，`matchPluginCommand` 永远匹配不到 `/prose`。
   - 看 `~/.openclaw/logs/gateway.err.log` 里是否有 open-prose 加载错误；确认 `plugins.entries["open-prose"]` 为 `{ "enabled": true }`，且 `~/.openclaw/extensions/open-prose` 存在且为当前插件目录。重启 Gateway 后再试一次 `/prose help`。

4. **飞书通道未传对 CommandBody**
   - 理论上 Feishu 会把用户消息原样放进 `CommandBody`，再变成 `commandBodyNormalized`。若飞书端或中间层对「以 / 开头的消息」做了特殊处理（例如当成应用命令走另一套接口），可能导致进 reply 管线的 body 已经不是 `/prose ...`，从而匹配不到插件。
   - 若你那边有飞书或 OpenClaw 的定制代码，检查是否有对 `/` 开头的消息做重写或分流。

**建议操作顺序**：先在飞书发一条 **`/prose help`**，看回复是插件帮助还是 agent 在说话；再确认 `commands.text` 和 open-prose 已启用并重启；若仍不对，再查 gateway 日志和飞书传参。
