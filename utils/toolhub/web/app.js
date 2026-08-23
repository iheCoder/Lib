const state = {
  tools: [],
  toolSignature: "",
  query: "",
  activeTask: null,
  activeLogs: null,
  toastTimer: null,
  logTimer: null,
};

const statusLabels = Object.freeze({
  stopped: "未运行",
  starting: "启动中",
  running: "运行中",
  external: "外部运行",
  unhealthy: "运行异常",
  stopping: "正在停止",
  succeeded: "已完成",
  failed: "失败",
});

const activeStatuses = new Set(["starting", "running", "unhealthy", "stopping"]);

/** Wire stable page controls once; cards use explicit listeners when rendered. */
function initialize() {
  document.querySelector("#tool-search").addEventListener("input", (event) => {
    state.query = event.target.value.trim().toLocaleLowerCase();
    renderTools();
  });
  document.querySelector("#clear-search").addEventListener("click", clearSearch);
  document.querySelector("#task-form").addEventListener("submit", submitTask);
  document.querySelector("#refresh-logs").addEventListener("click", loadLogs);
  document.querySelectorAll("[data-close-dialog]").forEach((button) => {
    button.addEventListener("click", () => document.querySelector(`#${button.dataset.closeDialog}`).close());
  });
  document.querySelector("#logs-dialog").addEventListener("close", stopLogPolling);
  loadTools();
  window.setInterval(loadTools, 2000);
}

/** Fetch status snapshots without replacing good data on a transient failure. */
async function loadTools() {
  try {
    const response = await fetch("/api/tools", { cache: "no-store" });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const payload = await response.json();
    const signature = JSON.stringify(payload.tools);
    const changed = signature !== state.toolSignature;
    state.tools = payload.tools;
    state.toolSignature = signature;
    document.querySelector("#refresh-state").textContent = `刚刚更新 · ${formatClock(new Date())}`;
    // Stable cards preserve keyboard focus and open-dialog triggers across the
    // two-second status poll. Rebuild only when observable tool state changed.
    if (changed) {
      renderTools();
      renderMetrics();
    }
  } catch (error) {
    document.querySelector("#refresh-state").textContent = "连接中断，正在重试";
    if (state.tools.length === 0) showToast(`无法读取工具状态：${error.message}`);
  }
}

/** Render the filtered catalog using DOM text nodes so tool metadata is inert. */
function renderTools() {
  const container = document.querySelector("#tool-list");
  const tools = state.tools.filter(matchesQuery);
  container.replaceChildren(...tools.map(createToolCard));
  container.setAttribute("aria-busy", "false");
  document.querySelector("#empty-state").hidden = tools.length !== 0 || state.tools.length === 0;
}

/** Match against user-facing fields rather than hidden command details. */
function matchesQuery(tool) {
  if (!state.query) return true;
  return [tool.name, tool.description, tool.category, tool.kind]
    .some((value) => (value || "").toLocaleLowerCase().includes(state.query));
}

/** Build one complete status card with actions appropriate to its lifecycle. */
function createToolCard(tool) {
  const card = element("article", "tool-card");
  card.dataset.status = tool.status;
  const top = element("div", "card-top");
  top.append(
    textElement("span", "tool-type", `${tool.category || "其他"} / ${tool.kind === "service" ? "WEB 服务" : "本地任务"}`),
    textElement("span", `status-badge ${tool.status}`, statusLabels[tool.status] || tool.status),
  );
  card.append(top, textElement("h3", "", tool.name), textElement("p", "tool-description", tool.description));
  if (tool.error) card.append(textElement("p", "tool-error", tool.error));
  const metadata = createMetadata(tool);
  if (metadata.childElementCount) card.append(metadata);
  card.append(createActions(tool));
  return card;
}

/** Runtime metadata uses tabular values to avoid reflow during polling. */
function createMetadata(tool) {
  const metadata = element("div", "runtime-meta");
  if (tool.pid) metadata.append(textElement("span", "", `PID ${tool.pid}`));
  if (tool.startedAt) metadata.append(textElement("span", "", `启动于 ${formatClock(new Date(tool.startedAt))}`));
  if (tool.exitCode !== undefined && tool.exitCode !== null) {
    metadata.append(textElement("span", "", `退出码 ${tool.exitCode}`));
  }
  return metadata;
}

/** Keep the primary action singular: start/run when idle, open when available. */
function createActions(tool) {
  const actions = element("div", "card-actions");
  if (tool.kind === "service") appendServiceActions(actions, tool);
  else appendTaskActions(actions, tool);
  actions.append(actionButton("查看日志", "secondary", () => openLogs(tool)));
  return actions;
}

/** Service actions make webpage navigation a first-class component. */
function appendServiceActions(actions, tool) {
  const available = ["running", "external", "unhealthy"].includes(tool.status);
  if (available) {
    const link = textElement("a", "button primary", "打开工具");
    link.href = tool.url;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    actions.append(link);
  } else {
    const start = actionButton("启动工具", "primary", () => startTool(tool));
    start.disabled = activeStatuses.has(tool.status) || tool.owned;
    if (tool.status === "starting") start.textContent = "正在启动";
    actions.append(start);
  }
  if (tool.owned && activeStatuses.has(tool.status)) {
    const stop = actionButton("停止", "danger", () => stopTool(tool));
    stop.disabled = tool.status === "stopping";
    actions.append(stop);
  }
}

/** Tasks open a generated parameter form and can be cancelled while active. */
function appendTaskActions(actions, tool) {
  if (!activeStatuses.has(tool.status)) {
    actions.append(actionButton(tool.status === "failed" || tool.status === "succeeded" ? "再次运行" : "运行任务", "primary", () => openTask(tool)));
  }
  if (tool.owned && activeStatuses.has(tool.status)) {
    const cancel = actionButton("取消任务", "danger", () => stopTool(tool));
    cancel.disabled = tool.status === "stopping";
    actions.append(cancel);
  }
}

/** Open the task dialog and derive controls exclusively from central metadata. */
function openTask(tool) {
  state.activeTask = tool;
  document.querySelector("#task-dialog-title").textContent = tool.name;
  const fields = tool.inputs.map(createInputField);
  document.querySelector("#task-fields").replaceChildren(...fields);
  document.querySelector("#task-dialog").showModal();
  document.querySelector("#task-fields input, #task-fields select")?.focus();
}

/** Map supported input types to accessible native controls with visible labels. */
function createInputField(input) {
  if (input.type === "boolean") return createBooleanField(input);
  const label = element("label", "field");
  label.append(textElement("span", "", input.label + (input.required ? " · 必填" : "")));
  const control = input.type === "select" ? document.createElement("select") : document.createElement("input");
  control.name = input.id;
  control.required = Boolean(input.required);
  if (input.type === "select") {
    control.append(...input.options.map((option) => {
      const node = textElement("option", "", option);
      node.value = option;
      node.selected = option === input.default;
      return node;
    }));
  } else {
    control.type = "text";
    control.value = input.default || "";
    control.placeholder = input.placeholder || "";
    control.autocomplete = "off";
  }
  label.append(control);
  return label;
}

/** Boolean flags use a native checkbox so keyboard and screen-reader behavior
 * remains predictable without custom switch semantics. */
function createBooleanField(input) {
  const label = element("label", "checkbox-field");
  const control = document.createElement("input");
  control.type = "checkbox";
  control.name = input.id;
  control.checked = input.default === "true";
  label.append(control, textElement("span", "", input.label));
  return label;
}

/** Submit structured values; the server converts them to literal argv entries. */
async function submitTask(event) {
  event.preventDefault();
  if (!state.activeTask) return;
  const submit = document.querySelector("#task-submit");
  submit.disabled = true;
  submit.textContent = "正在启动";
  const form = new FormData(event.currentTarget);
  const inputs = Object.fromEntries(state.activeTask.inputs.map((input) => [
    input.id,
    input.type === "boolean" ? String(form.has(input.id)) : String(form.get(input.id) || ""),
  ]));
  const succeeded = await mutateTool(state.activeTask.id, "start", { inputs });
  submit.disabled = false;
  submit.textContent = "开始运行";
  if (succeeded) document.querySelector("#task-dialog").close();
}

/** Service start has no form but shares the same strict JSON action contract. */
async function startTool(tool) {
  await mutateTool(tool.id, "start", { inputs: {} });
}

/** Stop requests are asynchronous; polling reflects stopping and final state. */
async function stopTool(tool) {
  await mutateTool(tool.id, "stop", {});
}

/** Centralize mutation errors so every card offers consistent recovery text. */
async function mutateTool(id, action, payload) {
  try {
    const response = await fetch(`/api/tools/${encodeURIComponent(id)}/${action}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || `HTTP ${response.status}`);
    await loadTools();
    return true;
  } catch (error) {
    showToast(error.message);
    return false;
  }
}

/** Logs remain available after exit and refresh more frequently while visible. */
function openLogs(tool) {
  state.activeLogs = tool;
  document.querySelector("#logs-dialog-title").textContent = `${tool.name} · 日志`;
  document.querySelector("#logs-dialog").showModal();
  loadLogs();
  stopLogPolling();
  state.logTimer = window.setInterval(loadLogs, 1500);
}

/** Fetch the bounded tail and preserve whitespace for command diagnostics. */
async function loadLogs() {
  if (!state.activeLogs) return;
  try {
    const response = await fetch(`/api/tools/${encodeURIComponent(state.activeLogs.id)}/logs`, { cache: "no-store" });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const logs = await response.json();
    document.querySelector("#logs-content").textContent = logs.content || "这个工具尚未产生输出。";
    document.querySelector("#logs-updated").textContent = logs.updatedAt ? `更新于 ${formatClock(new Date(logs.updatedAt))}` : "尚无输出";
  } catch (error) {
    document.querySelector("#logs-content").textContent = `读取日志失败：${error.message}`;
  }
}

/** Stop dialog-only polling to avoid background requests after dismissal. */
function stopLogPolling() {
  if (state.logTimer) window.clearInterval(state.logTimer);
  state.logTimer = null;
}

/** Summaries use text plus counts; color never carries status by itself. */
function renderMetrics() {
  const running = state.tools.filter((tool) => ["running", "external", "starting"].includes(tool.status)).length;
  const attention = state.tools.filter((tool) => ["failed", "unhealthy"].includes(tool.status)).length;
  document.querySelector("#metric-total").textContent = state.tools.length;
  document.querySelector("#metric-running").textContent = running;
  document.querySelector("#metric-attention").textContent = attention;
}

/** Clear both the visible input and its derived lowercase query. */
function clearSearch() {
  document.querySelector("#tool-search").value = "";
  state.query = "";
  renderTools();
  document.querySelector("#tool-search").focus();
}

/** Toasts announce transient errors without stealing keyboard focus. */
function showToast(message) {
  const toast = document.querySelector("#toast");
  toast.textContent = message;
  toast.classList.add("visible");
  if (state.toastTimer) window.clearTimeout(state.toastTimer);
  state.toastTimer = window.setTimeout(() => toast.classList.remove("visible"), 4500);
}

/** Create a semantic element with optional classes and safe text content. */
function textElement(tag, className, text) {
  const node = element(tag, className);
  node.textContent = text;
  return node;
}

/** Keep generic element creation deliberately small and side-effect free. */
function element(tag, className) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  return node;
}

/** Buttons receive explicit labels and native disabled behavior. */
function actionButton(label, variant, handler) {
  const button = textElement("button", `button ${variant}`, label);
  button.type = "button";
  button.addEventListener("click", handler);
  return button;
}

/** Localized time keeps status metadata compact without a date that rarely helps. */
function formatClock(date) {
  return new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" }).format(date);
}

initialize();
