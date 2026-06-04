import { marked } from "marked";
import sanitizeHtml from "sanitize-html";
import { enhanceResumeStructure } from "./structure.js";
import { classicTheme } from "./theme-classic.js";

const MAIN_SECTION_NAMES =
  /^(个人定位|个人简介|自我介绍|求职意向|核心技术栈|专业技能|技能清单|工作经历|项目经历|教育经历|实习经历|获奖经历|证书|开源经历|summary|profile|objective|skills|experience|work experience|projects?|education|awards?|certificates?)$/i;

/**
 * Markdown remains the content contract. The browser-side enhancement script
 * adds layout roles from heading semantics without rewriting source content.
 */
export function buildResumeHtml(markdown, options) {
  const contentHtml = sanitizeResumeHtml(marked.parse(markdown, { gfm: true, breaks: false }));
  return `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>简历</title>
<style>${classicTheme(options)}</style>
</head>
<body>
<main id="resume">${contentHtml}</main>
<script>
  window.MD_RESUME_MAIN_SECTION = ${MAIN_SECTION_NAMES.toString()};
  (${enhanceResumeStructure.toString()})();
</script>
</body>
</html>`;
}

/** Allow useful resume markup while preventing Markdown from executing code. */
function sanitizeResumeHtml(html) {
  return sanitizeHtml(html, {
    allowedTags: [
      "a",
      "blockquote",
      "br",
      "code",
      "del",
      "em",
      "h1",
      "h2",
      "h3",
      "h4",
      "h5",
      "h6",
      "hr",
      "img",
      "li",
      "ol",
      "p",
      "pre",
      "strong",
      "table",
      "tbody",
      "td",
      "th",
      "thead",
      "tr",
      "ul",
    ],
    allowedAttributes: {
      a: ["href", "title"],
      img: ["alt", "height", "src", "title", "width"],
    },
    allowedSchemes: ["data", "http", "https", "mailto", "tel"],
  });
}
