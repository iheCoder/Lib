import fs from "node:fs/promises";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { buildResumeHtml } from "./template.js";
import { exportPdf, renderPreview } from "./exporter.js";
import { DEFAULT_OPTIONS } from "./options.js";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const WEB_ROOT = path.join(ROOT, "web");
const ICON_ROOT = path.join(ROOT, "node_modules", "@phosphor-icons", "web", "src");
const PORT = Number(process.env.PORT || 4173);
const MAX_BODY_BYTES = 5 * 1024 * 1024;

/**
 * Serve the local studio and its small JSON API. Markdown never leaves this
 * process; preview and export both reuse the verified CLI rendering pipeline.
 */
export function createServer() {
  return http.createServer(async (request, response) => {
    try {
      if (request.method === "POST" && request.url === "/api/preview") {
        return await handlePreview(request, response);
      }
      if (request.method === "POST" && request.url === "/api/export") {
        return await handleExport(request, response);
      }
      return await serveStatic(request, response);
    } catch (error) {
      sendJson(response, 500, { error: error.message });
    }
  });
}

/** Render a fitted HTML preview and return diagnostics for the studio chrome. */
async function handlePreview(request, response) {
  const payload = await readJson(request);
  const options = normalizeWebOptions(payload.options);
  const preview = await renderPreview(buildResumeHtml(payload.markdown || "", options), options);
  sendJson(response, 200, preview);
}

/** Create a verified one-page PDF in a temporary location, then stream it. */
async function handleExport(request, response) {
  const payload = await readJson(request);
  const outputPath = path.join(os.tmpdir(), `md-resume-pdf-${crypto.randomUUID()}.pdf`);
  const options = { ...normalizeWebOptions(payload.options), outputPath };
  try {
    const layout = await exportPdf(buildResumeHtml(payload.markdown || "", options), options);
    if (layout.status === "overflow") {
      return sendJson(response, 422, { error: "内容无法在最低可读字号下放入一页。", layout });
    }
    const pdf = await fs.readFile(outputPath);
    const filename = `${safeFilename(payload.filename)}.pdf`;
    response.writeHead(200, {
      "Content-Disposition": `attachment; filename="resume.pdf"; filename*=UTF-8''${encodeURIComponent(filename)}`,
      "Content-Length": pdf.length,
      "Content-Type": "application/pdf",
    });
    response.end(pdf);
  } finally {
    await fs.rm(outputPath, { force: true });
  }
}

/** Bound and validate the stable visual options accepted from browser controls. */
function normalizeWebOptions(raw = {}) {
  const numberInRange = (value, fallback, min, max) => {
    const parsed = Number(value);
    return Number.isFinite(parsed) && parsed >= min && parsed <= max ? parsed : fallback;
  };
  const accent = /^#[0-9a-f]{6}$/i.test(raw.accent || "") ? raw.accent : DEFAULT_OPTIONS.accent;
  const minFontPt = numberInRange(raw.minFontPt, DEFAULT_OPTIONS.minFontPt, 8, 12);
  const maxFontPt = Math.max(
    minFontPt,
    numberInRange(raw.maxFontPt, DEFAULT_OPTIONS.maxFontPt, 8, 14),
  );
  return {
    ...DEFAULT_OPTIONS,
    accent,
    listFactor: numberInRange(raw.listFactor, DEFAULT_OPTIONS.listFactor, 0.1, 1.4),
    listLineHeight: numberInRange(raw.listLineHeight, DEFAULT_OPTIONS.listLineHeight, 1.2, 1.55),
    lineHeight: numberInRange(raw.lineHeight, DEFAULT_OPTIONS.lineHeight, 1.25, 1.65),
    marginMm: numberInRange(raw.marginMm, DEFAULT_OPTIONS.marginMm, 4, 18),
    maxFontPt,
    minFontPt,
    paragraphFactor: numberInRange(raw.paragraphFactor, DEFAULT_OPTIONS.paragraphFactor, 0.7, 1.5),
    sectionFactor: numberInRange(raw.sectionFactor, DEFAULT_OPTIONS.sectionFactor, 0.7, 1.6),
    subheadingFactor: numberInRange(raw.subheadingFactor, DEFAULT_OPTIONS.subheadingFactor, 0.7, 1.6),
  };
}

/** Read JSON with an explicit size limit because resumes may contain private data. */
async function readJson(request) {
  let size = 0;
  const chunks = [];
  for await (const chunk of request) {
    size += chunk.length;
    if (size > MAX_BODY_BYTES) {
      throw new Error("请求内容超过 5MB 限制");
    }
    chunks.push(chunk);
  }
  return JSON.parse(Buffer.concat(chunks).toString("utf8") || "{}");
}

/** Serve only known web and icon assets; arbitrary filesystem paths are rejected. */
async function serveStatic(request, response) {
  const urlPath = decodeURIComponent(new URL(request.url, "http://localhost").pathname);
  const isIcon = urlPath.startsWith("/icons/");
  const root = isIcon ? ICON_ROOT : WEB_ROOT;
  const relativePath = isIcon ? urlPath.slice("/icons/".length) : urlPath === "/" ? "index.html" : urlPath.slice(1);
  const filePath = path.resolve(root, relativePath);
  if (path.relative(root, filePath).startsWith("..")) {
    return sendJson(response, 403, { error: "禁止访问" });
  }
  try {
    const body = await fs.readFile(filePath);
    response.writeHead(200, { "Content-Type": contentType(filePath) });
    response.end(body);
  } catch {
    sendJson(response, 404, { error: "未找到资源" });
  }
}

function safeFilename(filename = "resume") {
  return path.parse(String(filename)).name.replace(/[^\p{L}\p{N}._-]+/gu, "-") || "resume";
}

function contentType(filePath) {
  return {
    ".css": "text/css; charset=utf-8",
    ".html": "text/html; charset=utf-8",
    ".js": "text/javascript; charset=utf-8",
    ".woff2": "font/woff2",
  }[path.extname(filePath)] || "application/octet-stream";
}

function sendJson(response, status, payload) {
  response.writeHead(status, { "Content-Type": "application/json; charset=utf-8" });
  response.end(JSON.stringify(payload));
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  createServer().listen(PORT, "127.0.0.1", () => {
    console.log(`Guided Studio 已启动：http://127.0.0.1:${PORT}`);
  });
}
