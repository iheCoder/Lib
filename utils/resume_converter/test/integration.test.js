import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { PDFDocument } from "pdf-lib";
import { run } from "../src/cli.js";
import { exportPdf } from "../src/exporter.js";
import { DEFAULT_OPTIONS } from "../src/options.js";
import { createServer } from "../src/server.js";
import { buildResumeHtml } from "../src/template.js";

const integration = process.env.MD_RESUME_PDF_INTEGRATION === "1" ? test : test.skip;

integration("exports a one-page PDF from a path containing spaces and Chinese", async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), "md-resume-pdf-中文 "));
  const input = path.join(directory, "张三 简历.md");
  const output = path.join(directory, "张三 简历.pdf");
  await fs.writeFile(input, "# 张三\n\n北京 | zhang@example.com\n\n## 个人定位\n\n后端工程师。\n\n## 工作经历\n\n### 示例公司\n\n- 交付稳定系统。\n");

  await run([input]);

  const pdf = await PDFDocument.load(await fs.readFile(output));
  assert.equal(pdf.getPageCount(), 1);
});

integration("expands spacing for a short resume while keeping the preferred font size", async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), "md-resume-pdf-spacing-"));
  const outputPath = path.join(directory, "short.pdf");
  const markdown = "# 张三\n\n北京 | zhang@example.com\n\n## 个人定位\n\n后端工程师。\n\n## 工作经历\n\n- 交付稳定系统。\n";
  const options = { ...DEFAULT_OPTIONS, outputPath };

  const layout = await exportPdf(buildResumeHtml(markdown, options), options);

  assert.ok(layout.settings.fontPt >= DEFAULT_OPTIONS.maxFontPt);
  assert.ok(layout.settings.sectionFactor > 1.3);
  assert.ok(layout.settings.subheadingFactor > 1.3);
  assert.ok(layout.settings.paragraphFactor > 1.2);
  assert.ok(layout.settings.listFactor < layout.settings.sectionFactor);
  assert.ok(layout.settings.listFactor <= 0.28);
  assert.ok(layout.settings.listLineHeight <= 1.24);
});

integration("rejects overflow and leaves no PDF", async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), "md-resume-pdf-overflow-"));
  const input = path.join(directory, "long.md");
  const output = path.join(directory, "long.pdf");
  const paragraphs = Array.from({ length: 180 }, (_, index) => `- 第 ${index + 1} 项：需要保留的大段项目经历和成果说明。`).join("\n");
  await fs.writeFile(input, `# 张三\n\n## 工作经历\n\n${paragraphs}\n`);

  await assert.rejects(() => run([input]), /无法在最低可读字号下生成单页 PDF/);
  await assert.rejects(() => fs.access(output));
});

integration("auto-style settings expand dense resumes toward a full safe page", async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), "md-resume-pdf-auto-fit-"));
  const outputPath = path.join(directory, "auto.pdf");
  const bullets = Array.from(
    { length: 16 },
    (_, index) => `- 第 ${index + 1} 项：负责跨系统链路建设、数据一致性治理、性能优化和可观测能力沉淀，支撑复杂业务稳定落地。`,
  ).join("\n");
  const markdown = `# 张三\n\n北京 | zhang@example.com\n\n## 个人定位\n\n后端工程师，关注复杂系统稳定性、数据正确性和工程效率。\n\n## 工作经历\n\n### 示例公司\n\n${bullets}\n\n## 教育经历\n\n示例大学 | 软件工程 | 本科`;
  const options = {
    ...DEFAULT_OPTIONS,
    lineHeight: 1.38,
    listFactor: 0.08,
    listLineHeight: 1.15,
    marginMm: 6,
    maxFontPt: 10.8,
    minFontPt: 8,
    outputPath,
    paragraphFactor: 0.9,
    sectionFactor: 1,
    subheadingFactor: 0.95,
  };

  const layout = await exportPdf(buildResumeHtml(markdown, options), options);
  const pdf = await PDFDocument.load(await fs.readFile(outputPath));

  assert.equal(layout.status, "fit");
  assert.equal(pdf.getPageCount(), 1);
  assert.ok(layout.contentHeight / layout.availableHeight >= 0.95);
});

integration("web studio previews and exports a Chinese filename", async () => {
  const server = createServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address();
  const payload = {
    filename: "张三 简历.md",
    markdown: "# 张三\n\n北京\n\n## 工作经历\n\n- 交付稳定系统。",
    // 0.08 is the compact preset's real value. This specifically guards
    // against silently rejecting it and falling back to the looser default.
    options: { listFactor: 0.08, listLineHeight: 1.15, sectionFactor: 1.3, subheadingFactor: 1.2 },
  };
  try {
    const preview = await fetch(`http://127.0.0.1:${port}/api/preview`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    assert.equal(preview.status, 200);
    const previewPayload = await preview.json();
    assert.equal(previewPayload.layout.status, "fit");
    assert.ok(previewPayload.layout.settings.sectionFactor > previewPayload.layout.settings.listFactor);
    assert.ok(previewPayload.layout.settings.listFactor <= 0.28);
    assert.ok(previewPayload.layout.settings.listLineHeight <= 1.24);

    const exported = await fetch(`http://127.0.0.1:${port}/api/export`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    assert.equal(exported.status, 200);
    assert.match(exported.headers.get("content-disposition"), /filename\*=UTF-8''/);
    assert.ok((await exported.arrayBuffer()).byteLength > 1_000);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
});
