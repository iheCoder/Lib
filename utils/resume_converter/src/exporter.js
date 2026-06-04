import fs from "node:fs/promises";
import path from "node:path";
import { chromium } from "playwright";
import { PDFDocument } from "pdf-lib";
import { fitOnePage } from "./layout.js";

/**
 * Own the browser lifecycle and artifact writes so callers either receive a
 * verified one-page PDF or a diagnostic result with no misleading PDF left.
 */
export async function exportPdf(html, options) {
  const browser = await launchBrowser();
  const page = await browser.newPage({ viewport: { width: 1200, height: 1400 } });
  let layout;
  try {
    await preparePage(page, html);
    layout = await fitOnePage(page, options);

    if (options.debug) {
      await writeDebugArtifacts(page, layout, options);
    }
    if (layout.status === "overflow") {
      await fs.rm(options.outputPath, { force: true });
      return layout;
    }

    await fs.mkdir(path.dirname(options.outputPath), { recursive: true });
    await page.pdf({
      path: options.outputPath,
      format: "A4",
      printBackground: true,
      preferCSSPageSize: true,
    });
    await verifySinglePage(options.outputPath);
    return layout;
  } finally {
    await browser.close();
  }
}

/**
 * Fit a document exactly as export would, then return the browser-finalized
 * HTML for the web studio's live preview without creating a PDF artifact.
 */
export async function renderPreview(html, options) {
  const browser = await launchBrowser();
  const page = await browser.newPage({ viewport: { width: 1200, height: 1400 } });
  try {
    await preparePage(page, html);
    const layout = await fitOnePage(page, options);
    // The DOM is already semantically enhanced. Removing the bootstrap script
    // prevents the iframe from wrapping the finalized structure a second time.
    await page.evaluate(() => {
      document.documentElement.classList.add("preview-mode");
      document.querySelectorAll("script").forEach((script) => script.remove());
    });
    return { html: await page.content(), layout };
  } finally {
    await browser.close();
  }
}

/** Prefer the reproducible bundled browser, with a practical desktop fallback. */
async function launchBrowser() {
  try {
    return await chromium.launch({ headless: true });
  } catch (error) {
    if (!/Executable doesn't exist/.test(error.message)) {
      throw error;
    }
  }

  // The bundled revision is the reproducible default. A system Chrome fallback
  // keeps the CLI useful immediately after npm install on common desktops.
  try {
    return await chromium.launch({ channel: "chrome", headless: true });
  } catch (error) {
    throw new Error(
      "找不到可用的 Chromium。请运行 `npx playwright install chromium`，或安装 Google Chrome。\n" +
        `浏览器启动错误：${error.message}`,
    );
  }
}

/** Reject and remove any artifact that escaped the layout guard as multiple pages. */
async function verifySinglePage(pdfPath) {
  const document = await PDFDocument.load(await fs.readFile(pdfPath));
  if (document.getPageCount() !== 1) {
    await fs.rm(pdfPath, { force: true });
    throw new Error(`PDF 二次校验失败：预期 1 页，实际 ${document.getPageCount()} 页`);
  }
}

/** Preserve enough evidence to inspect visual output and tuning decisions. */
async function writeDebugArtifacts(page, layout, options) {
  const debugDir = `${options.outputPath}.debug`;
  await fs.mkdir(debugDir, { recursive: true });
  await Promise.all([
    fs.writeFile(path.join(debugDir, "resume.html"), await page.content()),
    fs.writeFile(path.join(debugDir, "layout.json"), `${JSON.stringify(layout, null, 2)}\n`),
    page.screenshot({ path: path.join(debugDir, "resume.png"), fullPage: true }),
  ]);
}

/** Establish the same print rendering context for preview and final export. */
async function preparePage(page, html) {
  await page.setContent(html, { waitUntil: "networkidle" });
  await page.emulateMedia({ media: "print" });
  await page.evaluate(() => document.fonts.ready);
}
