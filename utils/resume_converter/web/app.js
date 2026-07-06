const SAMPLE = `# 张三

北京 | zhang@example.com | 138 0000 0000

GitHub: https://github.com/example

## 个人定位

5 年后端研发经验，关注高并发服务、数据正确性与工程效率，能够将复杂业务能力沉淀为稳定、可观测、可演进的基础设施。

## 核心技术栈

**后端工程：** Go、gRPC、MySQL、Redis、Kubernetes、OpenTelemetry。

**平台治理：** 灰度发布、熔断降级、分布式限流、配置热更新。

## 工作经历

### 示例科技有限公司 | 高级后端工程师

**2022.06 - 至今 | 北京**

- 负责核心服务架构与交付，通过链路优化将接口 P99 降低 40%。
- 建设可观测与故障诊断体系，显著缩短线上问题定位时间。
- 推动配置治理与灰度发布能力落地，降低高风险变更的上线影响。
- 设计缓存一致性与数据正确性保障机制，补齐数据契约校验和回滚策略。
- 建设统一鉴权中间件，沉淀签名校验、日志、限流与权限控制能力。

### 云平台有限公司 | 后端工程师

**2020.07 - 2022.05 | 杭州**

- 参与容器节点管理与任务分发链路建设，提升资源调度稳定性。
- 优化镜像启动和网络配置流程，降低开发环境准备时间。
- 建设通知与告警框架，统一处理多渠道消息投递和失败重试。

## 项目经历

### 实时数据平台

- 设计实时增量计算链路，保障数据正确性、可追溯性与服务稳定性。
- 拆分多源查询依赖并引入分层并发流水线，降低下游 I/O 峰值负载。

## 教育经历

示例大学 | 软件工程 | 本科`;

const PRESETS = {
  auto: {
    lineHeight: 1.38,
    listFactor: 0.08,
    listLineHeight: 1.15,
    marginMm: 6,
    minFontPt: 8,
    maxFontPt: 10.8,
    paragraphFactor: 0.9,
    sectionFactor: 1,
    subheadingFactor: 0.95,
  },
  relaxed: {
    lineHeight: 1.52,
    listFactor: 0.24,
    listLineHeight: 1.22,
    marginMm: 12,
    minFontPt: 9.5,
    maxFontPt: 11,
    paragraphFactor: 1.3,
    sectionFactor: 1.45,
    subheadingFactor: 1.3,
  },
  balanced: {
    lineHeight: 1.44,
    listFactor: 0.16,
    listLineHeight: 1.18,
    marginMm: 9,
    minFontPt: 9,
    maxFontPt: 10.5,
    paragraphFactor: 1.12,
    sectionFactor: 1.2,
    subheadingFactor: 1.12,
  },
  compact: {
    lineHeight: 1.34,
    listFactor: 0.08,
    listLineHeight: 1.15,
    marginMm: 6,
    minFontPt: 8,
    maxFontPt: 9.5,
    paragraphFactor: 0.88,
    sectionFactor: 1,
    subheadingFactor: 0.92,
  },
};

const state = {
  filename: "未命名简历.md",
  markdown: "",
  options: {
    accent: "#215fbd",
    ...PRESETS.auto,
  },
  layout: null,
  preset: "auto",
  step: 1,
};

const $ = (selector) => document.querySelector(selector);
const elements = {
  dialog: $("#editor-dialog"),
  editor: $("#markdown-editor"),
  empty: $("#empty-state"),
  fileInput: $("#file-input"),
  panel: $("#panel-content"),
  paper: $("#preview-paper"),
  preview: $("#resume-preview"),
  previewWrap: $("#preview-wrap"),
  toast: $("#toast"),
};

let previewTimer;
let requestVersion = 0;

/** Wire every visible control so the selected Product Design target is fully interactive. */
function initialize() {
  document.querySelectorAll(".step").forEach((button) => button.addEventListener("click", () => setStep(Number(button.dataset.step))));
  $("#file-input").addEventListener("change", (event) => loadFile(event.target.files[0]));
  $("#upload-hero").addEventListener("click", openFilePicker);
  $("#sample-button").addEventListener("click", () => setMarkdown(SAMPLE, "示例简历.md"));
  $("#edit-button").addEventListener("click", openEditor);
  $("#refresh-button").addEventListener("click", refreshPreview);
  $("#fit-preview-button").addEventListener("click", fitPreviewToWindow);
  $("#apply-editor").addEventListener("click", () => setMarkdown(elements.editor.value, state.filename));
  $("#previous-button").addEventListener("click", () => setStep(state.step - 1));
  $("#next-button").addEventListener("click", handleNext);
  window.addEventListener("dragover", (event) => event.preventDefault());
  window.addEventListener("drop", (event) => {
    event.preventDefault();
    loadFile(event.dataTransfer.files[0]);
  });
  new ResizeObserver(fitPreviewToWindow).observe(elements.previewWrap);
  renderPanel();
}

/** Read local Markdown in-browser; no upload occurs until local preview rendering. */
async function loadFile(file) {
  if (!file) return;
  if (!/\.(md|markdown)$/i.test(file.name)) return showToast("请选择 .md 或 .markdown 文件");
  setMarkdown(await file.text(), file.name);
}

function setMarkdown(markdown, filename) {
  state.markdown = markdown;
  state.filename = filename;
  elements.dialog.close();
  $("#filename-label").textContent = filename;
  updateStructure();
  setStep(Math.max(state.step, 2));
  schedulePreview();
}

function openFilePicker() {
  elements.fileInput.click();
}

function openEditor() {
  elements.editor.value = state.markdown || SAMPLE;
  elements.dialog.showModal();
  setTimeout(() => elements.editor.focus(), 0);
}

/** Keep workflow context explicit while preserving the same central preview. */
function setStep(step) {
  state.step = Math.min(4, Math.max(1, step));
  document.querySelectorAll(".step").forEach((button) => {
    const value = Number(button.dataset.step);
    button.classList.toggle("is-active", value === state.step);
    button.classList.toggle("is-complete", value < state.step && Boolean(state.markdown));
  });
  renderPanel();
}

function handleNext() {
  if (state.step === 4) return exportPdf();
  if (!state.markdown && state.step === 1) return openFilePicker();
  setStep(state.step + 1);
}

/** Render only controls relevant to the current step, avoiding a dense settings wall. */
function renderPanel() {
  const renderers = [renderImportPanel, renderStructurePanel, renderDesignPanel, renderExportPanel];
  elements.panel.innerHTML = renderers[state.step - 1]();
  bindPanelActions();
  $("#previous-button").style.visibility = state.step === 1 ? "hidden" : "visible";
  $("#next-button").innerHTML = state.step === 4 ? '<i class="ph ph-download-simple"></i> 导出 PDF' : '下一步 <i class="ph ph-arrow-right"></i>';
}

function renderImportPanel() {
  return `<div class="panel-heading"><h2>导入内容</h2><p>选择已有 Markdown 简历，或直接编辑示例内容。</p></div>
  <div class="field-group"><button class="drop-zone" data-action="upload"><i class="ph ph-upload-simple"></i><strong>${state.markdown ? "替换 Markdown" : "拖入或选择 Markdown"}</strong><span>支持 .md 与 .markdown，最大 5MB</span></button>
  ${state.markdown ? `<div class="file-meta"><strong>${escapeHtml(state.filename)}</strong><span>解析成功</span></div>` : ""}</div>
  <button class="secondary-button" data-action="edit"><i class="ph ph-pencil-simple"></i> 编辑 Markdown</button>`;
}

function renderStructurePanel() {
  const headings = extractHeadings();
  return `<div class="panel-heading"><h2>检查结构</h2><p>确认姓名、主章节和子项目已被正确识别。</p></div>
  <div class="field-group"><strong>识别到 ${headings.length} 个标题</strong><div class="structure-list">${headings.map((item) => `<div class="structure-row"><strong>${escapeHtml(item.text)}</strong><span>H${item.level}</span></div>`).join("") || '<div class="diagnostic"><p>导入 Markdown 后显示结构。</p></div>'}</div></div>
  <button class="secondary-button" data-action="edit"><i class="ph ph-pencil-simple"></i> 修正 Markdown 结构</button>`;
}

function renderDesignPanel() {
  return `<div class="panel-heading"><h2>调整版式</h2><p>从阅读体验出发，再让内容适配一页。</p></div>
  <div class="field-group"><strong>版式预设</strong><div class="preset-grid">
    ${presetButton("auto", "智能", "优先贴近一页", state.preset)}
    ${presetButton("relaxed", "舒展", "阅读更从容", state.preset)}
    ${presetButton("balanced", "平衡", "层级清晰", state.preset)}
    ${presetButton("compact", "紧凑", "内容更多", state.preset)}
  </div></div>
  <div class="field-group"><div class="diagnostic"><strong>${layoutHeadline()}</strong><p>${layoutAdvice()}</p></div>${designPrimaryAction()}</div>
  <div class="field-group"><strong>主题色</strong><div class="swatches">${["#215fbd","#17365d","#168178","#426b46","#8a5f32","#684783"].map((color) => `<button class="swatch ${state.options.accent === color ? "is-active" : ""}" style="--color:${color}" data-accent="${color}" title="${color}"></button>`).join("")}</div></div>
  <div class="field-group">
    ${rangeControl("页面边距", "marginMm", state.options.marginMm, 4, 18, "mm")}
    ${rangeControl("最低可读字号", "minFontPt", state.options.minFontPt, 8, 12, "pt")}
    ${rangeControl("首选正文字号", "maxFontPt", state.options.maxFontPt, 9, 14, "pt")}
    ${rangeControl("正文行高", "lineHeight", state.options.lineHeight, 1.25, 1.65, "×", 0.01)}
  </div>
  <div class="field-group"><strong>排版节奏</strong>
    ${rangeControl("章节间距", "sectionFactor", state.options.sectionFactor, 0.7, 1.6, "×", 0.05)}
    ${rangeControl("子标题间距", "subheadingFactor", state.options.subheadingFactor, 0.7, 1.6, "×", 0.05)}
    ${rangeControl("段落间距", "paragraphFactor", state.options.paragraphFactor, 0.7, 1.5, "×", 0.05)}
    ${rangeControl("列表项间距", "listFactor", state.options.listFactor, 0, 1, "×", 0.02)}
    ${rangeControl("列表行高", "listLineHeight", state.options.listLineHeight, 1.12, 1.4, "×", 0.01)}
  </div>`;
}

function renderExportPanel() {
  const layout = state.layout;
  const fit = layout?.status === "fit";
  return `<div class="panel-heading"><h2>导出 PDF</h2><p>最终文件会经过严格单页校验后下载。</p></div>
  <div class="field-group"><div class="diagnostic"><strong>${fit ? "已准备好导出" : "尚未满足单页要求"}</strong><p>${fit ? `当前使用 ${layout.settings.fontPt.toFixed(2)}pt，页面利用率 ${usagePercent()}%。` : "返回调整版式，或精简占用最多的章节。"}</p></div></div>
  <div class="field-group"><strong>导出检查</strong><div class="structure-list"><div class="structure-row"><span>PDF 页数</span><strong>${fit ? "1 页" : "待适配"}</strong></div><div class="structure-row"><span>主题色</span><strong>${state.options.accent}</strong></div><div class="structure-row"><span>页面边距</span><strong>${state.options.marginMm} mm</strong></div></div></div>
  <button class="primary-button export-primary" data-action="export" ${fit ? "" : "disabled"}><i class="ph ph-download-simple"></i> 导出一页 PDF</button>`;
}

function bindPanelActions() {
  elements.panel.querySelectorAll("[data-action=upload]").forEach((button) => button.addEventListener("click", openFilePicker));
  elements.panel.querySelectorAll("[data-action=edit]").forEach((button) => button.addEventListener("click", openEditor));
  elements.panel.querySelectorAll("[data-action=export]").forEach((button) => button.addEventListener("click", exportPdf));
  elements.panel.querySelectorAll("[data-action=go-export]").forEach((button) => button.addEventListener("click", () => setStep(4)));
  elements.panel.querySelectorAll("[data-action=auto-fit]").forEach((button) => button.addEventListener("click", () => applyPreset("auto")));
  elements.panel.querySelectorAll("[data-preset]").forEach((button) => button.addEventListener("click", () => applyPreset(button.dataset.preset)));
  elements.panel.querySelectorAll("[data-accent]").forEach((button) => button.addEventListener("click", () => updateOption("accent", button.dataset.accent)));
  elements.panel.querySelectorAll("input[type=range]").forEach((input) => input.addEventListener("input", () => updateOption(input.name, Number(input.value))));
}

function applyPreset(preset) {
  state.preset = preset;
  Object.assign(state.options, PRESETS[preset]);
  renderPanel();
  schedulePreview();
}

function updateOption(key, value) {
  state.options[key] = value;
  state.preset = "custom";
  if (key === "minFontPt" && value > state.options.maxFontPt) state.options.maxFontPt = value;
  renderPanel();
  schedulePreview();
}

function schedulePreview() {
  clearTimeout(previewTimer);
  previewTimer = setTimeout(refreshPreview, 280);
}

/** Ask the local server for browser-measured layout, then display finalized HTML. */
async function refreshPreview() {
  if (!state.markdown) return;
  const version = ++requestVersion;
  document.body.classList.add("is-loading");
  try {
    const response = await fetch("/api/preview", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ markdown: state.markdown, options: state.options }) });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error);
    if (version !== requestVersion) return;
    state.layout = result.layout;
    elements.preview.srcdoc = result.html;
    elements.empty.hidden = true;
    elements.previewWrap.hidden = false;
    updateFitStatus();
    renderPanel();
    requestAnimationFrame(fitPreviewToWindow);
  } catch (error) {
    showToast(error.message);
  } finally {
    document.body.classList.remove("is-loading");
  }
}

/** Scale the complete A4 sheet into the visible stage without clipping its bottom. */
function fitPreviewToWindow() {
  if (elements.previewWrap.hidden) return;
  const styles = getComputedStyle(elements.previewWrap);
  const availableWidth =
    elements.previewWrap.clientWidth -
    Number.parseFloat(styles.paddingLeft) -
    Number.parseFloat(styles.paddingRight);
  const availableHeight =
    elements.previewWrap.clientHeight -
    Number.parseFloat(styles.paddingTop) -
    Number.parseFloat(styles.paddingBottom);
  const scale = Math.max(0.36, Math.min(1, availableWidth / 794, availableHeight / 1123));
  elements.paper.style.setProperty("--preview-scale", scale.toFixed(3));
  elements.previewWrap.scrollTo({ left: 0, top: 0, behavior: "smooth" });
}

function updateFitStatus() {
  const fit = state.layout?.status === "fit";
  document.body.classList.toggle("is-fit", fit);
  document.body.classList.toggle("is-overflow", !fit);
  $("#fit-title").textContent = fit ? "一页适配良好" : overflowTitle();
  $("#fit-description").textContent = fit ? "内容可完整显示在一页 A4 纸张。" : overflowDescription();
  $("#usage-value").textContent = `${usagePercent()}%`;
  const largest = state.layout.sections?.[0];
  $("#largest-title").textContent = largest ? `${largest.title}占用最多` : "尚未分析";
  $("#largest-description").textContent = largest ? `当前占用约 ${Math.round(largest.height)}px` : "系统会指出占用最多的章节";
}

function layoutHeadline() {
  if (!state.layout) return "等待自动适配";
  if (state.layout.status === "fit") return state.layout.isNearTarget ? "已接近满页" : "已安全适配一页";
  return overflowTitle();
}

function layoutAdvice() {
  if (!state.layout) return "导入 Markdown 后，系统会自动搜索接近一页的安全排版。";
  if (state.layout.status === "fit") {
    return `当前页面利用率 ${usagePercent()}%，已保留约 ${Math.round(state.layout.printSafetyPx)}px 打印安全区。`;
  }
  return `${overflowDescription()} 可尝试智能适配，若仍不行再精简占用最多的章节。`;
}

function designPrimaryAction() {
  if (state.layout?.status === "fit") {
    return '<button class="primary-button export-primary" data-action="go-export"><i class="ph ph-arrow-right"></i> 前往导出</button>';
  }
  return '<button class="secondary-button export-primary" data-action="auto-fit"><i class="ph ph-sparkle"></i> 使用智能适配</button>';
}

function overflowTitle() {
  return state.layout?.overflowPercent > 0 ? "内容超出一页" : "打印安全区不足";
}

function overflowDescription() {
  if (!state.layout) return "上传 Markdown 后开始一页适配";
  if (state.layout.overflowPercent > 0) return `超出 ${state.layout.overflowPercent}%，建议精简内容。`;
  return `利用率 ${usagePercent()}%，但距离纸张底部太近，导出可能分页。`;
}

async function exportPdf() {
  if (state.layout?.status !== "fit") return showToast("请先让内容适配一页");
  showToast("正在生成并校验 PDF…");
  const response = await fetch("/api/export", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ filename: state.filename, markdown: state.markdown, options: state.options }) });
  if (!response.ok) return showToast((await response.json()).error);
  const link = document.createElement("a");
  link.href = URL.createObjectURL(await response.blob());
  link.download = `${state.filename.replace(/\.(md|markdown)$/i, "")}.pdf`;
  link.click();
  URL.revokeObjectURL(link.href);
  showToast("PDF 已通过单页校验并开始下载");
}

function updateStructure() {
  const count = extractHeadings().length;
  $("#structure-summary").textContent = `已识别 ${count} 个标题模块`;
}

function extractHeadings() {
  return [...state.markdown.matchAll(/^(#{1,6})\s+(.+)$/gm)].map((match) => ({ level: match[1].length, text: match[2] }));
}

function usagePercent() {
  return state.layout ? Math.min(999, Math.round((state.layout.contentHeight / state.layout.availableHeight) * 100)) : 0;
}

function presetButton(key, title, description, active) {
  return `<button class="preset ${active === key ? "is-active" : ""}" data-preset="${key}"><strong>${title}</strong><span>${description}</span></button>`;
}

function rangeControl(label, name, value, min, max, unit, step = 0.5) {
  return `<div class="control"><label><span>${label}</span><strong>${value} ${unit}</strong></label><input type="range" name="${name}" value="${value}" min="${min}" max="${max}" step="${step}"></div>`;
}

function escapeHtml(value) {
  return value.replace(/[&<>"']/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[character]);
}

function showToast(message) {
  elements.toast.textContent = message;
  elements.toast.classList.add("is-visible");
  setTimeout(() => elements.toast.classList.remove("is-visible"), 2600);
}

initialize();
