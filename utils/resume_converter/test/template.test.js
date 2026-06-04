import assert from "node:assert/strict";
import test from "node:test";
import { buildResumeHtml } from "../src/template.js";
import { DEFAULT_OPTIONS } from "../src/options.js";

test("renders semantic Markdown and bounded theme variables", () => {
  const html = buildResumeHtml("# 张三\n\n联系方式\n\n## 工作经历\n\n- 完成项目", DEFAULT_OPTIONS);
  assert.match(html, /<h1>张三<\/h1>/);
  assert.match(html, /<h2>工作经历<\/h2>/);
  assert.match(html, /--font-size: 10.5pt/);
  assert.match(html, /--accent: #17365d/);
});

test("removes executable raw HTML from public Markdown input", () => {
  const html = buildResumeHtml(
    "# 张三\n\n<script>globalThis.compromised = true</script>\n\n## 经历",
    DEFAULT_OPTIONS,
  );
  assert.doesNotMatch(html, /globalThis\.compromised/);
  assert.doesNotMatch(html, /<script>globalThis/);
});
