const TYPOGRAPHY_AND_HEADER = `
* { box-sizing: border-box; }
html, body { margin: 0; padding: 0; background: #fff; }
body {
  color: #252b33;
  font-family: "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans CJK SC", Arial, sans-serif;
  font-size: var(--font-size);
  font-variant-numeric: tabular-nums;
  line-height: var(--line-height);
  orphans: 1;
  text-rendering: optimizeLegibility;
  widows: 1;
}
#resume { width: var(--page-width); margin: 0 auto; }
h1, h2, h3, p, ul, ol { margin-top: 0; }
a { color: inherit; text-decoration: none; }
strong { color: #202831; font-weight: 650; }
.resume-header {
  margin-bottom: calc(5.5px * var(--section-factor));
  text-align: center;
}
.resume-name {
  margin: 0 0 calc(3px * var(--space-factor));
  color: #17202b;
  font-size: calc(var(--font-size) * 2.05);
  font-weight: 720;
  letter-spacing: .08em;
  line-height: 1.08;
}
.resume-header p {
  margin-bottom: calc(1.8px * var(--space-factor));
  color: #4d5661;
  font-size: calc(var(--font-size) * .93);
  line-height: 1.35;
}
`;

const SECTION_AND_CONTENT = `
.resume-section {
  margin-top: calc(7px * var(--section-factor));
  break-inside: auto;
}
.resume-section:first-of-type { margin-top: 0; }
.section-title {
  margin: 0 0 calc(4px * var(--space-factor));
  padding-bottom: calc(1.8px * var(--space-factor));
  border-bottom: 1px solid var(--accent);
  color: var(--accent);
  font-size: calc(var(--font-size) * 1.34);
  font-weight: 720;
  letter-spacing: .035em;
  line-height: 1.15;
}
.resume-section > p {
  margin-bottom: calc(4.5px * var(--space-factor));
  text-align: justify;
}
.resume-section > p:last-child { margin-bottom: 0; }
.resume-section h2:not(.section-title) {
  margin: calc(5px * var(--space-factor)) 0 calc(1.5px * var(--space-factor));
  color: #202a35;
  font-size: calc(var(--font-size) * 1.12);
  line-height: 1.2;
}
.resume-section h3 {
  margin: calc(4.5px * var(--space-factor)) 0 calc(1.5px * var(--space-factor));
  color: var(--accent);
  font-size: calc(var(--font-size) * 1.02);
  line-height: 1.2;
}
.resume-section h2 + p, .resume-section h3 + p {
  margin-bottom: calc(3px * var(--space-factor));
  color: #414a55;
}
ul, ol {
  margin-bottom: calc(4px * var(--space-factor));
  padding-left: 1.25em;
}
li {
  margin-bottom: calc(2px * var(--space-factor));
  padding-left: .12em;
}
li:last-child { margin-bottom: 0; }
p + ul, p + ol { margin-top: calc(-1px * var(--space-factor)); }
blockquote, pre, table, img { max-width: 100%; }
blockquote { margin: 4px 0; padding-left: 8px; border-left: 2px solid var(--accent); }
pre { white-space: pre-wrap; }
table { border-collapse: collapse; }
td, th { padding: 2px 4px; border: 1px solid #ccd2d8; }
`;

/**
 * Assemble the classic theme around a small set of adaptive CSS variables.
 * Layout code changes only these variables, leaving visual hierarchy stable.
 */
export function classicTheme(options) {
  return `
@page { size: A4; margin: ${options.marginMm}mm; }
:root {
  --accent: ${options.accent};
  --font-size: ${options.maxFontPt}pt;
  --line-height: 1.44;
  --space-factor: 1;
  --section-factor: 1;
  --page-width: ${210 - options.marginMm * 2}mm;
}
${TYPOGRAPHY_AND_HEADER}
${SECTION_AND_CONTENT}
@media screen {
  body { padding: 24px 0; background: #e9edf1; }
  #resume {
    min-height: ${297 - options.marginMm * 2}mm;
    background: #fff;
    box-shadow: 0 2px 18px rgba(23, 32, 43, .12);
  }
}`;
}
