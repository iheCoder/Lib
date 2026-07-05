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

test("normalizes loose Markdown list paragraphs inside resume bullets", () => {
  const html = buildResumeHtml(
    "# 张三\n\n## 工作经历\n\n- 第一条较长经历。\n\n- 第二条较长经历。\n",
    DEFAULT_OPTIONS,
  );

  // Blank lines between Markdown bullets create <li><p>...</p></li>. The
  // resume theme must cancel paragraph margins there so loose and tight lists
  // render with the same compact rhythm.
  assert.match(html, /<li><p>第一条较长经历。<\/p>\s*<\/li>/);
  assert.match(html, /li > p \{\n  margin: 0;/);
});
