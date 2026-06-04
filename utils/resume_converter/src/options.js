import path from "node:path";

export const DEFAULT_OPTIONS = Object.freeze({
  accent: "#17365d",
  debug: false,
  marginMm: 8,
  maxFontPt: 10.5,
  minFontPt: 9,
  paragraphFactor: 1,
  sectionFactor: 1,
  subheadingFactor: 1,
  listFactor: 0.38,
  listLineHeight: 1.32,
  lineHeight: 1.44,
  theme: "classic",
});

const VALUE_FLAGS = new Map([
  ["-o", "output"],
  ["--output", "output"],
  ["--theme", "theme"],
  ["--accent", "accent"],
  ["--margin", "margin"],
  ["--min-font-size", "minFontSize"],
  ["--max-font-size", "maxFontSize"],
]);

/**
 * Parse the deliberately small public CLI surface. Keeping validation here
 * prevents rendering code from having to reason about malformed dimensions.
 */
export function parseOptions(argv, cwd = process.cwd()) {
  if (argv.includes("--help") || argv.includes("-h")) {
    return { help: true };
  }

  const { input, values } = collectArguments(argv);
  return validateOptions(input, values, cwd);
}

/**
 * Collect positional and named arguments without mixing syntax handling with
 * semantic validation.
 */
function collectArguments(argv) {
  const values = { ...DEFAULT_OPTIONS };
  let input;

  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--debug") {
      values.debug = true;
      continue;
    }
    if (argument.startsWith("-")) {
      const key = VALUE_FLAGS.get(argument);
      if (!key) {
        throw new Error(`未知参数：${argument}`);
      }
      const value = argv[index + 1];
      if (!value || value.startsWith("-")) {
        throw new Error(`参数 ${argument} 缺少值`);
      }
      values[key] = value;
      index += 1;
      continue;
    }
    if (input) {
      throw new Error(`只能指定一个 Markdown 输入文件：${argument}`);
    }
    input = argument;
  }

  if (!input) {
    throw new Error("缺少 Markdown 输入文件。运行 md-resume-pdf --help 查看用法。");
  }
  return { input, values };
}

/**
 * Resolve paths and enforce the public readability/configuration boundaries.
 */
function validateOptions(input, values, cwd) {
  const inputPath = path.resolve(cwd, input);
  const outputPath = values.output
    ? path.resolve(cwd, values.output)
    : path.join(path.dirname(inputPath), `${path.parse(inputPath).name}.pdf`);

  const options = {
    ...values,
    inputPath,
    outputPath,
    marginMm: parseDimension(values.margin, "页面边距", 4, 18, DEFAULT_OPTIONS.marginMm),
    minFontPt: parseDimension(values.minFontSize, "最小字号", 8, 12, DEFAULT_OPTIONS.minFontPt),
    maxFontPt: parseDimension(values.maxFontSize, "最大字号", 8, 14, DEFAULT_OPTIONS.maxFontPt),
  };

  if (options.theme !== "classic") {
    throw new Error(`不支持的主题：${options.theme}。当前可用主题：classic`);
  }
  if (!/^#[0-9a-f]{6}$/i.test(options.accent)) {
    throw new Error(`强调色必须是六位十六进制颜色，例如 #17365d：${options.accent}`);
  }
  if (options.minFontPt > options.maxFontPt) {
    throw new Error("最小字号不能大于最大字号");
  }

  return options;
}

/** Parse a unit-bearing CLI value while keeping accepted ranges explicit. */
function parseDimension(rawValue, label, minimum, maximum, fallback) {
  if (rawValue === undefined) {
    return fallback;
  }
  const value = Number.parseFloat(String(rawValue).replace(/(?:mm|pt)$/i, ""));
  if (!Number.isFinite(value) || value < minimum || value > maximum) {
    throw new Error(`${label}必须在 ${minimum} 到 ${maximum} 之间：${rawValue}`);
  }
  return value;
}

export const HELP = `用法：
  md-resume-pdf <resume.md> [选项]

选项：
  -o, --output <path>       输出 PDF；默认与 Markdown 同目录同名
  --theme classic           简历主题
  --accent "#17365d"        六位十六进制强调色
  --margin 8mm              A4 页面边距，范围 4-18mm
  --min-font-size 9pt       最低可读字号，范围 8-12pt
  --max-font-size 10.5pt    首选正文字号，范围 8-14pt
  --debug                    保留 HTML、截图和布局诊断 JSON
  -h, --help                显示帮助
`;
