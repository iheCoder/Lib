import { randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import type { OpenClawPluginApi, PluginCommandContext } from "openclaw/plugin-sdk";
import { runPluginCommandWithTimeout } from "openclaw/plugin-sdk";

type ProseAction =
  | { kind: "help" }
  | { kind: "examples" }
  | { kind: "compile"; target: string; request?: string }
  | { kind: "run"; target: string; request?: string }
  | { kind: "invest"; request?: string }
  | { kind: "invalid"; reason: string };

type DeterministicWritePlan = {
  targetPath: string;
  content: string;
};

type RunMeta = {
  id: string;
  action: string;
  target: string;
  source: string;
  createdAt: string;
  channel: string;
  senderId?: string;
  mode: "deterministic-file-write" | "agent-bridge" | "staged-agent-bridge" | "compile";
};

type StageResult = {
  stage: string;
  text: string;
  stdout: string;
  stderr: string;
  stagePath: string;
};

type InvestmentInputs = {
  company: string;
  ticker: string;
  market: string;
  investmentQuestion: string;
};

type QQTarget = {
  type: "c2c" | "group" | "channel";
  id: string;
};

type QQDeliveryResult = {
  stageImages: Array<{
    stage: string;
    markdownPath: string;
    imagePath?: string;
    delivered: boolean;
    error?: string;
  }>;
  fileProbe?: {
    supported: boolean;
    attempted: boolean;
    filePath?: string;
    error?: string;
  };
};

type FeishuDeliveryResult = {
  stageImages: Array<{
    stage: string;
    markdownPath: string;
    imagePath?: string;
    delivered: boolean;
    error?: string;
  }>;
};

const COMMAND_HELP = [
  "OpenProse commands:",
  "",
  "/prose help",
  "/prose examples",
  "/prose run <file.prose>",
  "/prose compile <file.prose>",
  "/prose invest <你的研究请求>  例：/prose invest 深度研究下牧原股份现在是否值得投资",
].join("\n");

function parseAction(rawArgs: string): ProseAction {
  const args = rawArgs.trim();
  if (!args) {
    return { kind: "help" };
  }

  const [command, ...rest] = args.split(/\s+/);
  const action = command.toLowerCase();
  const parsedTarget = parseTargetAndRequest(rest.join(" "));

  if (action === "help") {
    return { kind: "help" };
  }
  if (action === "examples") {
    return { kind: "examples" };
  }
  if (action === "run") {
    if (!parsedTarget) {
      return { kind: "invalid", reason: "Usage: /prose run <file.prose>" };
    }
    return { kind: "run", target: parsedTarget.target, request: parsedTarget.request };
  }
  if (action === "compile") {
    if (!parsedTarget) {
      return { kind: "invalid", reason: "Usage: /prose compile <file.prose>" };
    }
    return { kind: "compile", target: parsedTarget.target, request: parsedTarget.request };
  }
  if (action === "invest") {
    const request = rest.join(" ").trim();
    return { kind: "invest", request: request || undefined };
  }

  return { kind: "invalid", reason: `Unknown subcommand: ${action}` };
}

function parseTargetAndRequest(raw: string): { target: string; request?: string } | null {
  const trimmed = raw.trim();
  if (!trimmed) {
    return null;
  }

  // 约定：第一个以 .prose 结尾的 token 视为程序路径，后续内容作为运行期请求补充。
  // 这样 QQ 路由层可以把自然语言原始请求拼在命令体后面，而不会再被当成文件路径。
  const proseIndex = trimmed.toLowerCase().indexOf(".prose");
  if (proseIndex === -1) {
    return { target: trimmed };
  }

  const targetEnd = proseIndex + ".prose".length;
  const target = trimmed.slice(0, targetEnd).trim();
  const request = trimmed.slice(targetEnd).trim();
  return {
    target,
    request: request || undefined,
  };
}

async function resolveProgramPath(target: string): Promise<string> {
  const expanded = target.startsWith("~/") ? path.join(process.env.HOME ?? "", target.slice(2)) : target;
  const resolved = path.isAbsolute(expanded) ? expanded : path.resolve(process.cwd(), expanded);
  const stat = await fs.stat(resolved);
  if (!stat.isFile()) {
    throw new Error(`Program is not a file: ${resolved}`);
  }
  if (!resolved.endsWith(".prose")) {
    throw new Error(`Program must end with .prose: ${resolved}`);
  }
  return resolved;
}

function buildRunId(): string {
  const now = new Date();
  const stamp = now.toISOString().replace(/[-:.TZ]/g, "").slice(0, 14);
  return `${stamp}-${randomUUID().slice(0, 8)}`;
}

async function ensureRunDir(programPath: string, runId: string): Promise<string> {
  const runDir = path.join(path.dirname(programPath), ".prose", "runs", runId);
  await fs.mkdir(runDir, { recursive: true });
  return runDir;
}

async function writeRunMeta(runDir: string, meta: RunMeta): Promise<void> {
  await fs.writeFile(path.join(runDir, "meta.json"), `${JSON.stringify(meta, null, 2)}\n`, "utf8");
}

async function ensureStagesDir(runDir: string): Promise<string> {
  const stagesDir = path.join(runDir, "stages");
  await fs.mkdir(stagesDir, { recursive: true });
  return stagesDir;
}

async function fileExists(targetPath: string): Promise<boolean> {
  try {
    await fs.access(targetPath);
    return true;
  } catch {
    return false;
  }
}

function stageSlug(stage: string): string {
  return stage.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

function stageLabel(stage: string): string {
  return stage
    .split("-")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function stageSyntheticRecipient(runId: string, stage: string): string {
  const seed = `${runId}${stage}`.replace(/\D/g, "");
  const padded = `${seed}123456789012345`.slice(0, 12);
  return `+86${padded}`;
}

function parseQQTarget(to?: string): QQTarget | null {
  if (!to?.trim()) {
    return null;
  }

  const normalized = to.replace(/^qqbot:/i, "");
  if (normalized.startsWith("c2c:")) {
    return { type: "c2c", id: normalized.slice(4) };
  }
  if (normalized.startsWith("group:")) {
    return { type: "group", id: normalized.slice(6) };
  }
  if (normalized.startsWith("channel:")) {
    return { type: "channel", id: normalized.slice(8) };
  }
  return { type: "c2c", id: normalized };
}

function canDeliverToQQ(ctx: PluginCommandContext): boolean {
  return ctx.channel === "qqbot" && Boolean(parseQQTarget(ctx.to));
}

function canDeliverToFeishu(ctx: PluginCommandContext): boolean {
  return ctx.channel === "feishu" && Boolean(ctx.to?.trim());
}

function parseFeishuReceiveTarget(rawTarget?: string): {
  receiveId: string;
  receiveIdType: "chat_id" | "open_id" | "user_id" | "union_id";
} | null {
  const target = rawTarget?.trim();
  if (!target) {
    return null;
  }

  const [prefix, ...restParts] = target.split(":");
  const rest = restParts.join(":").trim();
  const normalized = rest || prefix;

  if (!normalized) {
    return null;
  }

  if (prefix === "user") {
    return {
      receiveId: normalized,
      receiveIdType: normalized.startsWith("ou_") ? "open_id" : "user_id",
    };
  }
  if (prefix === "open_id") {
    return { receiveId: normalized, receiveIdType: "open_id" };
  }
  if (prefix === "user_id") {
    return { receiveId: normalized, receiveIdType: "user_id" };
  }
  if (prefix === "union_id") {
    return { receiveId: normalized, receiveIdType: "union_id" };
  }
  if (prefix === "chat" || prefix === "chat_id" || prefix === "group") {
    return { receiveId: normalized, receiveIdType: "chat_id" };
  }
  if (normalized.startsWith("ou_")) {
    return { receiveId: normalized, receiveIdType: "open_id" };
  }
  if (normalized.startsWith("oc_")) {
    return { receiveId: normalized, receiveIdType: "chat_id" };
  }
  return { receiveId: normalized, receiveIdType: "chat_id" };
}

async function getFeishuTenantToken(appId: string, appSecret: string): Promise<string> {
  const res = await fetch("https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ app_id: appId, app_secret: appSecret }),
  });
  if (!res.ok) {
    throw new Error(`Feishu auth failed: ${res.status} ${await res.text()}`);
  }
  const data = (await res.json()) as { tenant_access_token?: string; code?: number };
  if (data.code && data.code !== 0) {
    throw new Error(`Feishu auth error: code=${data.code}`);
  }
  if (!data.tenant_access_token) {
    throw new Error("Feishu auth: no tenant_access_token");
  }
  return data.tenant_access_token;
}

async function feishuUploadImage(accessToken: string, imagePath: string): Promise<string> {
  const form = new FormData();
  form.append("image_type", "message");
  form.append("image", new Blob([await fs.readFile(imagePath)]), path.basename(imagePath));

  const res = await fetch("https://open.feishu.cn/open-apis/im/v1/images", {
    method: "POST",
    headers: { Authorization: `Bearer ${accessToken}` },
    body: form as unknown as BodyInit,
  });
  if (!res.ok) {
    throw new Error(`Feishu upload image failed: ${res.status} ${await res.text()}`);
  }
  const data = (await res.json()) as { data?: { image_key?: string }; code?: number };
  if (data.code && data.code !== 0) {
    throw new Error(`Feishu upload error: code=${data.code}`);
  }
  if (!data.data?.image_key) {
    throw new Error("Feishu upload: no image_key");
  }
  return data.data.image_key;
}

async function feishuSendImage(
  accessToken: string,
  receiveTarget: { receiveId: string; receiveIdType: "chat_id" | "open_id" | "user_id" | "union_id" },
  imageKey: string,
): Promise<void> {
  const res = await fetch(
    `https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=${receiveTarget.receiveIdType}`,
    {
      method: "POST",
      headers: {
        Authorization: `Bearer ${accessToken}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        receive_id: receiveTarget.receiveId,
        msg_type: "image",
        content: JSON.stringify({ image_key: imageKey }),
      }),
    },
  );
  if (!res.ok) {
    throw new Error(`Feishu send message failed: ${res.status} ${await res.text()}`);
  }
  const data = (await res.json()) as { code?: number };
  if (data.code && data.code !== 0) {
    throw new Error(`Feishu send error: code=${data.code}`);
  }
}

function getFeishuConfigFromCtx(ctx: PluginCommandContext): { appId: string; appSecret: string } | null {
  const config = ctx.config as Record<string, unknown> | undefined;
  const channels = config?.channels as Record<string, unknown> | undefined;
  const feishu = channels?.feishu as Record<string, unknown> | undefined;
  const accounts = feishu?.accounts as Record<string, { appId?: string; appSecret?: string }> | undefined;
  const defaultAccount = (feishu?.defaultAccount as string) || "main";
  const account = accounts?.[defaultAccount];
  if (!account?.appId || !account?.appSecret) {
    return null;
  }
  return { appId: account.appId, appSecret: account.appSecret };
}

async function deliverStageImagesToFeishu(params: {
  ctx: PluginCommandContext;
  stageResults: StageResult[];
}): Promise<FeishuDeliveryResult["stageImages"]> {
  const receiveTarget = parseFeishuReceiveTarget(params.ctx.to);
  if (!receiveTarget) {
    return params.stageResults.map((s) => ({
      stage: s.stage,
      markdownPath: s.stagePath,
      delivered: false,
      error: "missing or invalid feishu receive target",
    }));
  }
  const feishuConfig = getFeishuConfigFromCtx(params.ctx);
  if (!feishuConfig) {
    return params.stageResults.map((s) => ({
      stage: s.stage,
      markdownPath: s.stagePath,
      delivered: false,
      error: "feishu account not configured in openclaw.json channels.feishu",
    }));
  }
  // #region agent log
  fetch("http://127.0.0.1:7383/ingest/5e67e177-188a-465c-80d1-54ac3658e0c5", {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Debug-Session-Id": "e182aa" },
    body: JSON.stringify({
      sessionId: "e182aa",
      runId: "feishu-delivery-target",
      hypothesisId: "H9",
      location: "skill/open-prose/index.ts:356",
      message: "resolved feishu receive target",
      data: {
        rawTo: params.ctx.to ?? null,
        receiveIdType: receiveTarget.receiveIdType,
        receiveIdPreview: receiveTarget.receiveId.slice(0, 8),
      },
      timestamp: Date.now(),
    }),
  }).catch(() => {});
  // #endregion
  let accessToken: string;
  try {
    accessToken = await getFeishuTenantToken(feishuConfig.appId, feishuConfig.appSecret);
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    return params.stageResults.map((s) => ({
      stage: s.stage,
      markdownPath: s.stagePath,
      delivered: false,
      error: `feishu auth failed: ${msg}`,
    }));
  }
  const deliveries: FeishuDeliveryResult["stageImages"] = [];
  for (const stage of params.stageResults) {
    try {
      const imagePath = await renderMarkdownToImage(stage.stagePath);
      const imageKey = await feishuUploadImage(accessToken, imagePath);
      await feishuSendImage(accessToken, receiveTarget, imageKey);
      deliveries.push({
        stage: stage.stage,
        markdownPath: stage.stagePath,
        imagePath,
        delivered: true,
      });
    } catch (err) {
      deliveries.push({
        stage: stage.stage,
        markdownPath: stage.stagePath,
        delivered: false,
        error: err instanceof Error ? err.message : String(err),
      });
    }
  }
  return deliveries;
}

async function renderMarkdownToImage(markdownPath: string): Promise<string> {
  const outputDir = path.dirname(markdownPath);
  const renderedPath = `${markdownPath}.png`;

  const render = await runPluginCommandWithTimeout({
    argv: ["qlmanage", "-t", "-s", "2200", "-o", outputDir, markdownPath],
    timeoutMs: 30_000,
    cwd: outputDir,
    env: {
      ...process.env,
      OPENCLAW_DISABLE_INTERACTIVE: "1",
    },
  });

  if (render.code !== 0) {
    throw new Error(render.stderr || render.stdout || "qlmanage render failed");
  }

  if (!(await fileExists(renderedPath))) {
    throw new Error(`Rendered image not found: ${renderedPath}`);
  }

  return renderedPath;
}

async function deliverStageImagesToQQ(params: {
  ctx: PluginCommandContext;
  stageResults: StageResult[];
}): Promise<QQDeliveryResult["stageImages"]> {
  const target = parseQQTarget(params.ctx.to);
  if (!target) {
    return params.stageResults.map((stage) => ({
      stage: stage.stage,
      markdownPath: stage.stagePath,
      delivered: false,
      error: "missing qq target",
    }));
  }

  let resolveQQBotAccount: (config: unknown, accountId?: string) => { appId?: string; clientSecret?: string };
  let sendQQMedia: (opts: {
    to: string;
    mediaUrl: string;
    accountId?: string;
    account: { appId?: string; clientSecret?: string };
  }) => Promise<{ error?: string }>;
  try {
    const qqConfig = await import("../qqbot/src/config.js");
    const qqApi = await import("../qqbot/src/api.js");
    const qqOut = await import("../qqbot/src/outbound.js");
    resolveQQBotAccount = qqConfig.resolveQQBotAccount;
    sendQQMedia = qqOut.sendMedia;
  } catch {
    return params.stageResults.map((stage) => ({
      stage: stage.stage,
      markdownPath: stage.stagePath,
      delivered: false,
      error: "qqbot module not available (optional dependency)",
    }));
  }

  const account = resolveQQBotAccount(params.ctx.config, params.ctx.accountId);
  const deliveries: QQDeliveryResult["stageImages"] = [];

  for (const stage of params.stageResults) {
    try {
      const imagePath = await renderMarkdownToImage(stage.stagePath);
      const result = await sendQQMedia({
        to: params.ctx.to ?? "",
        text: "",
        mediaUrl: imagePath,
        accountId: params.ctx.accountId,
        replyToId: undefined,
        account,
      });

      deliveries.push({
        stage: stage.stage,
        markdownPath: stage.stagePath,
        imagePath,
        delivered: !result.error,
        error: result.error,
      });
    } catch (error) {
      deliveries.push({
        stage: stage.stage,
        markdownPath: stage.stagePath,
        delivered: false,
        error: error instanceof Error ? error.message : String(error),
      });
    }
  }

  return deliveries;
}

async function probeQQFileSupport(params: {
  ctx: PluginCommandContext;
  runDir: string;
}): Promise<QQDeliveryResult["fileProbe"]> {
  const target = parseQQTarget(params.ctx.to);
  if (!target) {
    return {
      attempted: false,
      supported: false,
      error: "missing qq target",
    };
  }
  if (target.type === "channel") {
    return {
      attempted: false,
      supported: false,
      error: "channel target does not support qq file probe",
    };
  }

  let resolveQQBotAccount: (config: unknown, accountId?: string) => { appId?: string; clientSecret?: string };
  let getAccessToken: (appId: string, secret: string) => Promise<string>;
  let uploadC2CMedia: (token: string, id: string, type: unknown, a: unknown, data: string, b: boolean) => Promise<{ file_info: unknown }>;
  let uploadGroupMedia: (token: string, id: string, type: unknown, a: unknown, data: string, b: boolean) => Promise<{ file_info: unknown }>;
  let sendC2CMediaMessage: (token: string, id: string, fileInfo: unknown, a?: unknown, b?: unknown) => Promise<unknown>;
  let sendGroupMediaMessage: (token: string, id: string, fileInfo: unknown, a?: unknown, b?: unknown) => Promise<unknown>;
  let MediaFileType: { FILE: unknown };
  try {
    const qqConfig = await import("../qqbot/src/config.js");
    const qqApi = await import("../qqbot/src/api.js");
    resolveQQBotAccount = qqConfig.resolveQQBotAccount;
    getAccessToken = qqApi.getAccessToken;
    uploadC2CMedia = qqApi.uploadC2CMedia;
    uploadGroupMedia = qqApi.uploadGroupMedia;
    sendC2CMediaMessage = qqApi.sendC2CMediaMessage;
    sendGroupMediaMessage = qqApi.sendGroupMediaMessage;
    MediaFileType = qqApi.MediaFileType;
  } catch {
    return {
      attempted: false,
      supported: false,
      error: "qqbot module not available (optional dependency)",
    };
  }

  const account = resolveQQBotAccount(params.ctx.config, params.ctx.accountId);
  if (!account.appId || !account.clientSecret) {
    return {
      attempted: false,
      supported: false,
      error: "qq account credentials are incomplete",
    };
  }

  const probeFilePath = path.join(params.runDir, "qq-file-probe.txt");
  const probeContent = [
    "QQ_FILE_ATTACHMENT_PROBE",
    `run_dir=${params.runDir}`,
    `created_at=${new Date().toISOString()}`,
  ].join("\n");

  await fs.writeFile(probeFilePath, `${probeContent}\n`, "utf8");

  try {
    const accessToken = await getAccessToken(account.appId, account.clientSecret);
    const fileData = (await fs.readFile(probeFilePath)).toString("base64");

    if (target.type === "c2c") {
      const upload = await uploadC2CMedia(accessToken, target.id, MediaFileType.FILE, undefined, fileData, false);
      await sendC2CMediaMessage(accessToken, target.id, upload.file_info, undefined, undefined);
    } else {
      const upload = await uploadGroupMedia(accessToken, target.id, MediaFileType.FILE, undefined, fileData, false);
      await sendGroupMediaMessage(accessToken, target.id, upload.file_info, undefined, undefined);
    }

    return {
      attempted: true,
      supported: true,
      filePath: probeFilePath,
    };
  } catch (error) {
    return {
      attempted: true,
      supported: false,
      filePath: probeFilePath,
      error: error instanceof Error ? error.message : String(error),
    };
  }
}

async function deliverQQArtifacts(params: {
  ctx: PluginCommandContext;
  runDir: string;
  stageResults: StageResult[];
}): Promise<QQDeliveryResult | null> {
  if (!canDeliverToQQ(params.ctx)) {
    return null;
  }

  const stageImages = await deliverStageImagesToQQ({
    ctx: params.ctx,
    stageResults: params.stageResults,
  });
  const fileProbe = await probeQQFileSupport({
    ctx: params.ctx,
    runDir: params.runDir,
  });

  const result: QQDeliveryResult = {
    stageImages,
    fileProbe,
  };

  await fs.writeFile(path.join(params.runDir, "qq-delivery.json"), `${JSON.stringify(result, null, 2)}\n`, "utf8");
  return result;
}

async function deliverFeishuArtifacts(params: {
  ctx: PluginCommandContext;
  runDir: string;
  stageResults: StageResult[];
}): Promise<FeishuDeliveryResult | null> {
  if (!canDeliverToFeishu(params.ctx)) {
    return null;
  }
  const stageImages = await deliverStageImagesToFeishu({
    ctx: params.ctx,
    stageResults: params.stageResults,
  });
  const result: FeishuDeliveryResult = { stageImages };
  await fs.writeFile(
    path.join(params.runDir, "feishu-delivery.json"),
    `${JSON.stringify(result, null, 2)}\n`,
    "utf8",
  );
  return result;
}

function extractPlannerField(text: string, key: string): string {
  const escaped = key.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const match = text.match(new RegExp(`^${escaped}:\\s*(.+)$`, "im"));
  return match?.[1]?.trim() ?? "";
}

function inferCompanyFromRequest(request: string): string {
  const compact = request.replace(/\s+/g, "");
  const companyMatch = compact.match(/调研下?(.+?)(公司)?是否/);
  if (companyMatch?.[1]) {
    return companyMatch[1].trim();
  }
  const buyMatch = compact.match(/帮我看看(.+?)现在值不值得买/);
  if (buyMatch?.[1]) {
    return buyMatch[1].trim();
  }
  return "";
}

function buildFallbackInputs(request: string, plannerText: string): InvestmentInputs {
  const company = extractPlannerField(plannerText, "company") || inferCompanyFromRequest(request) || "Unknown company";
  const ticker = extractPlannerField(plannerText, "ticker");
  const market = extractPlannerField(plannerText, "market");
  const investmentQuestion =
    extractPlannerField(plannerText, "investment_question") || request.trim() || "Is this company investable now?";

  return {
    company,
    ticker,
    market,
    investmentQuestion,
  };
}

function buildStagePrompt(params: {
  roleTitle: string;
  stageObjective: string;
  request: string;
  inputs: InvestmentInputs;
  priorContext: Array<{ label: string; text: string }>;
  extraRequirements?: string[];
}): string {
  const sections: string[] = [];
  sections.push(`You are the ${params.roleTitle} for a staged OpenProse investment workflow.`);
  sections.push("");
  sections.push("Original user request:");
  sections.push(params.request);
  sections.push("");
  sections.push("Resolved inputs:");
  sections.push(`company: ${params.inputs.company}`);
  sections.push(`ticker: ${params.inputs.ticker || "(unknown)"}`);
  sections.push(`market: ${params.inputs.market || "(unknown)"}`);
  sections.push(`investment_question: ${params.inputs.investmentQuestion}`);
  sections.push("");
  sections.push("Stage objective:");
  sections.push(params.stageObjective.trim());
  if (params.priorContext.length > 0) {
    sections.push("");
    sections.push("Prior stage context:");
    for (const item of params.priorContext) {
      sections.push(`## ${item.label}`);
      sections.push(item.text.trim());
      sections.push("");
    }
  }
  sections.push("Requirements:");
  sections.push("- Use tools when needed instead of relying only on memory.");
  sections.push("- Clearly separate facts, inferences, and remaining unknowns.");
  sections.push("- If evidence is insufficient, say so explicitly.");
  if (params.extraRequirements) {
    for (const requirement of params.extraRequirements) {
      sections.push(`- ${requirement}`);
    }
  }
  sections.push("- Keep the output structured and stage-specific.");
  return sections.join("\n");
}

async function runAgentStage(params: {
  runId: string;
  runDir: string;
  stage: string;
  prompt: string;
}): Promise<StageResult> {
  const stagesDir = await ensureStagesDir(params.runDir);
  const slug = stageSlug(params.stage);
  const markdownPath = path.join(stagesDir, `${slug}.md`);

  const result = await runPluginCommandWithTimeout({
    argv: [
      "openclaw",
      "agent",
      "--agent",
      "main",
      "--to",
      stageSyntheticRecipient(params.runId, params.stage),
      "--message",
      params.prompt,
      "--json",
      "--timeout",
      "300",
    ],
    timeoutMs: 310_000,
    cwd: params.runDir,
    env: {
      ...process.env,
      OPENCLAW_DISABLE_INTERACTIVE: "1",
    },
  });

  await fs.writeFile(path.join(stagesDir, `${slug}.stdout.json`), result.stdout || "", "utf8");
  await fs.writeFile(path.join(stagesDir, `${slug}.stderr.log`), result.stderr || "", "utf8");

  if (result.code !== 0) {
    throw new Error(`${params.stage} failed: ${result.stderr || `exit ${result.code}`}`);
  }

  let stageText = result.stdout.trim();
  try {
    const parsed = JSON.parse(result.stdout) as {
      result?: { payloads?: Array<{ text?: string }> };
    };
    stageText = parsed.result?.payloads?.map((item) => item.text ?? "").filter(Boolean).join("\n\n") || stageText;
  } catch {
    // Keep raw stdout when the agent output is not JSON.
  }

  await fs.writeFile(markdownPath, `${stageText}\n`, "utf8");
  return {
    stage: params.stage,
    text: stageText,
    stdout: result.stdout,
    stderr: result.stderr,
    stagePath: markdownPath,
  };
}

function isInvestmentResearchProgram(programPath: string, programText: string): boolean {
  if (path.basename(programPath) === "investment_research_pipeline.prose") {
    return true;
  }
  return programText.includes("agent financial-analyst:") && programText.includes("agent report-writer:");
}

async function executeInvestmentWorkflow(params: {
  ctx: PluginCommandContext;
  runId: string;
  runDir: string;
  programPath: string;
  programText: string;
  request?: string;
}): Promise<string> {
  const request = params.request?.trim() || "Please run this investment workflow.";
  await ensureStagesDir(params.runDir);
  // #region agent log
  fetch('http://127.0.0.1:7383/ingest/5e67e177-188a-465c-80d1-54ac3658e0c5',{method:'POST',headers:{'Content-Type':'application/json','X-Debug-Session-Id':'e182aa'},body:JSON.stringify({sessionId:'e182aa',runId:params.runId,hypothesisId:'H4',location:'skill/open-prose/index.ts:756',message:'executeInvestmentWorkflow entered',data:{channel:params.ctx.channel,programPath:params.programPath,runDir:params.runDir,request},timestamp:Date.now()})}).catch(()=>{});
  // #endregion

  const plannerPrompt = [
    "You are the planner stage of an OpenProse investment workflow.",
    "",
    "Infer the workflow inputs from the original request and build a research plan.",
    "",
    "Return exactly this structure:",
    "company: ...",
    "ticker: ...",
    "market: ...",
    "investment_question: ...",
    "",
    "## Research Plan",
    "1. Company snapshot",
    "2. Key questions",
    "3. Evidence checklist",
    "4. Important unknowns",
    "",
    "Original request:",
    request,
  ].join("\n");

  const planner = await runAgentStage({
    runId: params.runId,
    runDir: params.runDir,
    stage: "planner",
    prompt: plannerPrompt,
  });

  const inputs = buildFallbackInputs(request, planner.text);

  const sharedContext = [
    {
      label: "Planner",
      text: planner.text,
    },
  ];

  const researchStages = await Promise.all([
    runAgentStage({
      runId: params.runId,
      runDir: params.runDir,
      stage: "financial",
      prompt: buildStagePrompt({
        roleTitle: "financial analyst",
        stageObjective: `Analyze ${inputs.company}'s financial profile: revenue growth, margins, cash flow, balance sheet, and valuation metrics.`,
        request,
        inputs,
        priorContext: sharedContext,
        extraRequirements: [
          "List 3-5 concrete facts before giving conclusions.",
          "Call out bullish signals, bearish signals, and open questions.",
        ],
      }),
    }),
    runAgentStage({
      runId: params.runId,
      runDir: params.runDir,
      stage: "industry",
      prompt: buildStagePrompt({
        roleTitle: "industry analyst",
        stageObjective: `Analyze the industry for ${inputs.company}: market size, competition, customer demand, regulation, and technology shifts.`,
        request,
        inputs,
        priorContext: sharedContext,
        extraRequirements: [
          "Explain tailwinds and headwinds separately.",
          "Distinguish structural trends from temporary noise.",
        ],
      }),
    }),
    runAgentStage({
      runId: params.runId,
      runDir: params.runDir,
      stage: "news",
      prompt: buildStagePrompt({
        roleTitle: "news analyst",
        stageObjective: `Research recent news and catalysts for ${inputs.company} over the last 6-12 months.`,
        request,
        inputs,
        priorContext: sharedContext,
        extraRequirements: [
          "Return a dated event timeline.",
          "Highlight positive catalysts, negative catalysts, and pending confirmations.",
        ],
      }),
    }),
    runAgentStage({
      runId: params.runId,
      runDir: params.runDir,
      stage: "macro",
      prompt: buildStagePrompt({
        roleTitle: "macro analyst",
        stageObjective: `Assess only the macro drivers that materially affect ${inputs.company}.`,
        request,
        inputs,
        priorContext: sharedContext,
        extraRequirements: [
          "Focus on rates, policy, FX, commodity sensitivity, and geopolitics only when material.",
        ],
      }),
    }),
    runAgentStage({
      runId: params.runId,
      runDir: params.runDir,
      stage: "sentiment",
      prompt: buildStagePrompt({
        roleTitle: "market sentiment analyst",
        stageObjective: `Assess current market narrative, expectations, positioning, and sentiment risks for ${inputs.company}.`,
        request,
        inputs,
        priorContext: sharedContext,
        extraRequirements: [
          "Do not substitute sentiment for fundamentals.",
          "Highlight expectation mismatch if any.",
        ],
      }),
    }),
  ]);

  const researchContext = [
    ...sharedContext,
    ...researchStages.map((stage) => ({
      label: stageLabel(stage.stage),
      text: stage.text,
    })),
  ];

  const thesis = await runAgentStage({
    runId: params.runId,
    runDir: params.runDir,
    stage: "thesis",
    prompt: buildStagePrompt({
      roleTitle: "senior investment analyst",
      stageObjective: `Synthesize the evidence into an investment thesis for ${inputs.company}.`,
      request,
      inputs,
      priorContext: researchContext,
      extraRequirements: [
        "Answer whether the business is fundamentally attractive.",
        "Answer whether the current valuation is attractive.",
        "End with one provisional conclusion: investable now / interesting but needs better price or evidence / not investable now.",
      ],
    }),
  });

  const risk = await runAgentStage({
    runId: params.runId,
    runDir: params.runDir,
    stage: "risk",
    prompt: buildStagePrompt({
      roleTitle: "risk analyst",
      stageObjective: `Stress test ${inputs.company} as an investment target.`,
      request,
      inputs,
      priorContext: [...researchContext, { label: "Thesis", text: thesis.text }],
      extraRequirements: [
        "Provide a ranked risk table by severity and probability.",
        "Include early warning indicators and what future data would change conviction most.",
      ],
    }),
  });

  const critic = await runAgentStage({
    runId: params.runId,
    runDir: params.runDir,
    stage: "critic",
    prompt: buildStagePrompt({
      roleTitle: "investment committee skeptic",
      stageObjective: `Attack the current investment thesis for ${inputs.company}.`,
      request,
      inputs,
      priorContext: [...researchContext, { label: "Thesis", text: thesis.text }, { label: "Risk", text: risk.text }],
      extraRequirements: [
        "Find weak assumptions, evidence gaps, and signs of overconfidence.",
        "Return the strongest bear case you can make.",
      ],
    }),
  });

  const report = await runAgentStage({
    runId: params.runId,
    runDir: params.runDir,
    stage: "report",
    prompt: buildStagePrompt({
      roleTitle: "report writer",
      stageObjective: `Write the final investment memo for ${inputs.company}.`,
      request,
      inputs,
      priorContext: [
        ...researchContext,
        { label: "Thesis", text: thesis.text },
        { label: "Risk", text: risk.text },
        { label: "Critic", text: critic.text },
      ],
      extraRequirements: [
        "Structure the memo as: executive summary, business snapshot, financials, industry, news, macro, sentiment, thesis, valuation, risks, bull/base/bear, final recommendation.",
        "Recommendation must be one of Strong yes / Watchlist / No for now.",
        "Make the reasoning traceable to evidence from prior stages.",
      ],
    }),
  });

  const summary = {
    status: "ok",
    mode: "staged-agent-bridge",
    programPath: params.programPath,
    inputs,
    stages: {
      planner: planner.stagePath,
      financial: researchStages.find((item) => item.stage === "financial")?.stagePath,
      industry: researchStages.find((item) => item.stage === "industry")?.stagePath,
      news: researchStages.find((item) => item.stage === "news")?.stagePath,
      macro: researchStages.find((item) => item.stage === "macro")?.stagePath,
      sentiment: researchStages.find((item) => item.stage === "sentiment")?.stagePath,
      thesis: thesis.stagePath,
      risk: risk.stagePath,
      critic: critic.stagePath,
      report: report.stagePath,
    },
  };

  const orderedStages = [
    planner,
    ...researchStages,
    thesis,
    risk,
    critic,
    report,
  ];
  // #region agent log
  fetch('http://127.0.0.1:7383/ingest/5e67e177-188a-465c-80d1-54ac3658e0c5',{method:'POST',headers:{'Content-Type':'application/json','X-Debug-Session-Id':'e182aa'},body:JSON.stringify({sessionId:'e182aa',runId:params.runId,hypothesisId:'H4',location:'skill/open-prose/index.ts:983',message:'executeInvestmentWorkflow completed all stages before delivery',data:{stageCount:orderedStages.length,stageNames:orderedStages.map((stage)=>stage.stage),runDir:params.runDir},timestamp:Date.now()})}).catch(()=>{});
  // #endregion
  const [qqDelivery, feishuDelivery] = await Promise.all([
    deliverQQArtifacts({
      ctx: params.ctx,
      runDir: params.runDir,
      stageResults: orderedStages,
    }),
    deliverFeishuArtifacts({
      ctx: params.ctx,
      runDir: params.runDir,
      stageResults: orderedStages,
    }),
  ]);
  const summaryWithDelivery = {
    ...summary,
    qqDelivery,
    feishuDelivery,
  };

  await fs.writeFile(path.join(params.runDir, "result.json"), `${JSON.stringify(summaryWithDelivery, null, 2)}\n`, "utf8");
  await fs.writeFile(path.join(params.runDir, "report.md"), `${report.text}\n`, "utf8");

  const deliveryNotes: string[] = [];
  if (qqDelivery) {
    const deliveredCount = qqDelivery.stageImages.filter((item) => item.delivered).length;
    deliveryNotes.push(`STAGE_IMAGE_DELIVERY_QQ=${deliveredCount}/${qqDelivery.stageImages.length}`);
    if (qqDelivery.fileProbe?.attempted) {
      deliveryNotes.push(
        qqDelivery.fileProbe.supported
          ? "QQ_FILE_ATTACHMENT_PROBE=supported"
          : `QQ_FILE_ATTACHMENT_PROBE=unsupported (${qqDelivery.fileProbe.error ?? "unknown error"})`,
      );
    }
  }
  if (feishuDelivery) {
    const deliveredCount = feishuDelivery.stageImages.filter((item) => item.delivered).length;
    deliveryNotes.push(`STAGE_IMAGE_DELIVERY_FEISHU=${deliveredCount}/${feishuDelivery.stageImages.length}`);
  }

  return [report.text, "", ...deliveryNotes, `RUN_ARTIFACT_DIR=${params.runDir}`].join("\n");
}

function parseDeterministicWrite(programText: string): DeterministicWritePlan | null {
  const targetMatch = programText.match(/Create a file at\s+([^\n]+)\n/i);
  const contentMatch = programText.match(/Write exactly this content:\s*\n([\s\S]*?)\n\nIf the file already exists, overwrite it\./i);
  if (!targetMatch || !contentMatch) {
    return null;
  }

  const targetPath = targetMatch[1]?.trim();
  const content = contentMatch[1]?.replace(/\s+$/g, "") ?? "";
  if (!targetPath || !content) {
    return null;
  }

  return { targetPath, content: `${content}\n` };
}

async function executeDeterministicWrite(params: {
  plan: DeterministicWritePlan;
  runDir: string;
  programPath: string;
}): Promise<string> {
  const { plan, runDir, programPath } = params;
  await fs.mkdir(path.dirname(plan.targetPath), { recursive: true });
  await fs.writeFile(plan.targetPath, plan.content, "utf8");

  const result = {
    status: "ok",
    mode: "deterministic-file-write",
    targetPath: plan.targetPath,
    bytesWritten: Buffer.byteLength(plan.content),
    programPath,
  };
  await fs.writeFile(path.join(runDir, "result.json"), `${JSON.stringify(result, null, 2)}\n`, "utf8");

  return [
    "OpenProse run completed.",
    `- mode: deterministic-file-write`,
    `- target: ${plan.targetPath}`,
    `- run: ${runDir}`,
  ].join("\n");
}

async function executeViaAgentBridge(params: {
  api: OpenClawPluginApi;
  runDir: string;
  programPath: string;
  programText: string;
  request?: string;
}): Promise<string> {
  const { api, runDir, programPath, programText, request } = params;
  const bridgePrompt = [
    "Use the \"prose\" skill for this request.",
    "",
    `Program path: ${programPath}`,
    "",
    "Program source:",
    "```prose",
    programText,
    "```",
    request
      ? [
          "",
          "Original execution request:",
          request,
        ].join("\n")
      : "",
    "",
    "Requirements:",
    "1. Treat this as an OpenProse workflow run.",
    "2. If you perform research or tool calls, be evidence-driven.",
    "3. Do not claim filesystem side effects unless they actually happened.",
    `4. Final output must end with: RUN_ARTIFACT_DIR=${runDir}`,
  ].join("\n");

  const env = {
    ...process.env,
    OPENCLAW_DISABLE_INTERACTIVE: "1",
  };

  const result = await runPluginCommandWithTimeout({
    argv: [
      "openclaw",
      "agent",
      "--agent",
      "main",
      "--message",
      bridgePrompt,
      "--json",
      "--timeout",
      "300",
    ],
    timeoutMs: 310_000,
    cwd: path.dirname(programPath),
    env,
  });

  await fs.writeFile(path.join(runDir, "agent-bridge.stdout.json"), result.stdout || "", "utf8");
  await fs.writeFile(path.join(runDir, "agent-bridge.stderr.log"), result.stderr || "", "utf8");

  if (result.code !== 0) {
    throw new Error(result.stderr || `Agent bridge failed with code ${result.code}`);
  }

  let replyText = result.stdout.trim();
  try {
    const parsed = JSON.parse(result.stdout) as {
      result?: { payloads?: Array<{ text?: string }> };
    };
    replyText = parsed.result?.payloads?.map((item) => item.text ?? "").filter(Boolean).join("\n\n") || replyText;
  } catch {
    // Keep raw stdout if the bridge did not return JSON.
  }

  const summary = {
    status: "ok",
    mode: "agent-bridge",
    programPath,
    stdoutBytes: Buffer.byteLength(result.stdout),
    stderrBytes: Buffer.byteLength(result.stderr),
  };
  await fs.writeFile(path.join(runDir, "result.json"), `${JSON.stringify(summary, null, 2)}\n`, "utf8");
  await fs.writeFile(path.join(runDir, "report.md"), `${replyText}\n`, "utf8");

  return [replyText, "", `RUN_ARTIFACT_DIR=${runDir}`].join("\n");
}

async function compileProgram(programPath: string, programText: string, runDir: string): Promise<string> {
  const lines = programText.split(/\r?\n/);
  const agentCount = lines.filter((line) => /^agent\s+[^:]+:/i.test(line.trim())).length;
  const inputCount = lines.filter((line) => /^input\s+[^:]+:/i.test(line.trim())).length;
  const parallelCount = lines.filter((line) => /^parallel:/i.test(line.trim())).length;
  const summary = {
    status: "ok",
    mode: "compile",
    programPath,
    stats: {
      lines: lines.length,
      agents: agentCount,
      inputs: inputCount,
      parallelBlocks: parallelCount,
    },
  };
  await fs.writeFile(path.join(runDir, "compile.json"), `${JSON.stringify(summary, null, 2)}\n`, "utf8");
  return [
    "OpenProse compile completed.",
    `- program: ${programPath}`,
    `- agents: ${agentCount}`,
    `- inputs: ${inputCount}`,
    `- parallel blocks: ${parallelCount}`,
    `- run: ${runDir}`,
  ].join("\n");
}

function getBuiltInInvestmentProgramPath(): string {
  return path.join(process.env.HOME ?? "", ".openclaw", "extensions", "open-prose", "skills", "prose", "investment_research_pipeline.prose");
}

async function listExamples(): Promise<string> {
  const examplesDir = path.join(process.env.HOME ?? "", ".openclaw", "extensions", "open-prose", "skills", "prose", "examples");
  const entries = await fs.readdir(examplesDir, { withFileTypes: true }).catch(() => []);
  const examples = entries
    .filter((entry) => entry.isFile() && entry.name.endsWith(".prose"))
    .map((entry) => entry.name)
    .sort();

  if (examples.length === 0) {
    return "No bundled examples found.";
  }

  return ["Bundled OpenProse examples:", "", ...examples.map((name) => `- ${name}`)].join("\n");
}

export default function register(api: OpenClawPluginApi) {
  // #region agent log
  fetch('http://127.0.0.1:7383/ingest/5e67e177-188a-465c-80d1-54ac3658e0c5',{method:'POST',headers:{'Content-Type':'application/json','X-Debug-Session-Id':'e182aa'},body:JSON.stringify({sessionId:'e182aa',runId:'plugin-load',hypothesisId:'H7',location:'skill/open-prose/index.ts:1207',message:'open-prose register invoked',data:{plugin:'open-prose'},timestamp:Date.now()})}).catch(()=>{});
  // #endregion
  api.registerCommand({
    name: "prose",
    description: "Run or compile OpenProse programs with auditable run artifacts.",
    acceptsArgs: true,
    handler: async (ctx) => {
      const action = parseAction(ctx.args ?? "");
      // #region agent log
      fetch('http://127.0.0.1:7383/ingest/5e67e177-188a-465c-80d1-54ac3658e0c5',{method:'POST',headers:{'Content-Type':'application/json','X-Debug-Session-Id':'e182aa'},body:JSON.stringify({sessionId:'e182aa',runId:'pre-run',hypothesisId:'H2',location:'skill/open-prose/index.ts:1207',message:'open-prose command handler entered',data:{channel:ctx.channel,accountId:ctx.accountId ?? null,to:ctx.to ?? null,args:ctx.args ?? '',actionKind:action.kind},timestamp:Date.now()})}).catch(()=>{});
      // #endregion
      if (action.kind === "help") {
        return { text: COMMAND_HELP };
      }
      if (action.kind === "examples") {
        return { text: await listExamples() };
      }
      if (action.kind === "invalid") {
        return { text: `${action.reason}\n\n${COMMAND_HELP}` };
      }

      try {
        let programPath: string;
        let runRequest: string | undefined;
        if (action.kind === "invest") {
          programPath = getBuiltInInvestmentProgramPath();
          runRequest = action.request;
          if (!(await fileExists(programPath))) {
            return {
              text: `OpenProse invest 需要内置程序文件，未找到: ${programPath}\n请执行 skill/open-prose/install-openclaw.sh 完成安装。`,
            };
          }
        } else {
          programPath = await resolveProgramPath(action.target);
          runRequest = action.request;
        }
        const programText = await fs.readFile(programPath, "utf8");
        const runId = buildRunId();
        const runDir = await ensureRunDir(programPath, runId);
        await fs.writeFile(path.join(runDir, "program.prose"), programText, "utf8");
        // #region agent log
        fetch('http://127.0.0.1:7383/ingest/5e67e177-188a-465c-80d1-54ac3658e0c5',{method:'POST',headers:{'Content-Type':'application/json','X-Debug-Session-Id':'e182aa'},body:JSON.stringify({sessionId:'e182aa',runId,hypothesisId:'H3',location:'skill/open-prose/index.ts:1233',message:'open-prose resolved program branch',data:{actionKind:action.kind,programPath,runDir,request:runRequest ?? null,isInvestmentProgram:isInvestmentResearchProgram(programPath, programText),hasDeterministicPlan:Boolean(parseDeterministicWrite(programText))},timestamp:Date.now()})}).catch(()=>{});
        // #endregion

        const meta: RunMeta = {
          id: runId,
          action: action.kind,
          target: action.kind === "invest" ? "investment_research_pipeline.prose" : action.target,
          source: programPath,
          createdAt: new Date().toISOString(),
          channel: ctx.channel,
          senderId: ctx.senderId,
          mode: action.kind === "compile" ? "compile" : "agent-bridge",
        };

        if (action.kind === "compile") {
          meta.mode = "compile";
          await writeRunMeta(runDir, meta);
          return { text: await compileProgram(programPath, programText, runDir) };
        }

        const deterministicPlan = parseDeterministicWrite(programText);
        if (deterministicPlan) {
          meta.mode = "deterministic-file-write";
          await writeRunMeta(runDir, meta);
          return {
            text: await executeDeterministicWrite({
              plan: deterministicPlan,
              runDir,
              programPath,
            }),
          };
        }

        if (action.kind === "invest" || isInvestmentResearchProgram(programPath, programText)) {
          meta.mode = "staged-agent-bridge";
          await writeRunMeta(runDir, meta);
          return {
            text: await executeInvestmentWorkflow({
              ctx,
              runId,
              runDir,
              programPath,
              programText,
              request: runRequest,
            }),
          };
        }

        meta.mode = "agent-bridge";
        await writeRunMeta(runDir, meta);
        return {
          text: await executeViaAgentBridge({
            api,
            runDir,
            programPath,
            programText,
            request: action.request,
          }),
        };
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        return { text: `OpenProse command failed: ${message}` };
      }
    },
  });
}
