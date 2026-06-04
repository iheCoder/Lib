import fs from "node:fs/promises";
import { buildResumeHtml } from "./template.js";
import { exportPdf } from "./exporter.js";
import { HELP, parseOptions } from "./options.js";

/** Coordinate the user-facing conversion flow and report its final decision. */
export async function run(argv) {
  const options = parseOptions(argv);
  if (options.help) {
    process.stdout.write(HELP);
    return;
  }

  const markdown = await readMarkdown(options.inputPath);
  const layout = await exportPdf(buildResumeHtml(markdown, options), options);
  if (layout.status === "overflow") {
    throw new Error(formatOverflow(layout, options));
  }

  console.log(
    `已导出：${options.outputPath}\n` +
      `排版：${layout.settings.fontPt.toFixed(2)}pt，内容占页面可用高度的 ${(
        (layout.contentHeight / layout.availableHeight) *
        100
      ).toFixed(1)}%`,
  );
}

/** Read only regular files so directory mistakes fail with a useful message. */
async function readMarkdown(inputPath) {
  try {
    const stat = await fs.stat(inputPath);
    if (!stat.isFile()) {
      throw new Error("不是文件");
    }
    return await fs.readFile(inputPath, "utf8");
  } catch (error) {
    throw new Error(`无法读取 Markdown 文件：${inputPath}（${error.message}）`);
  }
}

/** Turn layout evidence into an actionable one-page overflow diagnosis. */
function formatOverflow(layout, options) {
  const largest = layout.sections[0];
  const suggestions = layout.suggestions.map((item) => `  - ${item.text}`).join("\n");
  return `无法在最低可读字号下生成单页 PDF，未写入输出文件。
当前内容高度：${layout.contentHeight}px
单页可用高度：${layout.availableHeight}px
超出比例：${layout.overflowPercent}%
最低字号：${options.minFontPt}pt
占用最大章节：${largest?.title ?? "无法识别"}（${largest?.height ?? 0}px）
建议优先精简：
${suggestions || "  - 未找到可精简的段落或列表项"}`;
}
