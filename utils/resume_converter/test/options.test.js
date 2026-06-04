import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";
import { parseOptions } from "../src/options.js";

test("uses the Markdown directory and basename for default output", () => {
  const options = parseOptions(["资料/中文 简历.md"], "/workspace");
  assert.equal(options.inputPath, path.resolve("/workspace/资料/中文 简历.md"));
  assert.equal(options.outputPath, path.resolve("/workspace/资料/中文 简历.pdf"));
});

test("parses stable visual options", () => {
  const options = parseOptions([
    "resume.md",
    "-o",
    "dist/resume.pdf",
    "--accent",
    "#224466",
    "--margin",
    "9mm",
    "--min-font-size",
    "9.2pt",
    "--max-font-size",
    "11pt",
    "--debug",
  ]);
  assert.equal(options.accent, "#224466");
  assert.equal(options.marginMm, 9);
  assert.equal(options.minFontPt, 9.2);
  assert.equal(options.maxFontPt, 11);
  assert.equal(options.debug, true);
});

test("rejects an unreadable font range", () => {
  assert.throws(
    () => parseOptions(["resume.md", "--min-font-size", "11pt", "--max-font-size", "9pt"]),
    /最小字号不能大于最大字号/,
  );
});
