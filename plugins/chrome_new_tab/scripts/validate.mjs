import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const extensionRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

/** Read a required extension file relative to the package root. */
function readExtensionFile(relativePath) {
  return readFile(resolve(extensionRoot, relativePath), "utf8");
}

/** Read one binary extension asset without coercing its PNG bytes to text. */
function readExtensionAsset(relativePath) {
  return readFile(resolve(extensionRoot, relativePath));
}

/** Validate PNG signature and IHDR dimensions without adding image dependencies. */
function assertPngDimensions(buffer, expectedSize, relativePath) {
  const pngSignature = "89504e470d0a1a0a";
  assert.equal(buffer.subarray(0, 8).toString("hex"), pngSignature, `${relativePath} is not PNG`);
  assert.equal(buffer.readUInt32BE(16), expectedSize, `${relativePath} width is incorrect`);
  assert.equal(buffer.readUInt32BE(20), expectedSize, `${relativePath} height is incorrect`);
}

/**
 * Validate the deployment contract that Chrome relies on. These checks remain
 * deliberately dependency-free so the extension can be audited and installed
 * without running npm install.
 */
async function validateExtension() {
  const sources = await Promise.all([
    readExtensionFile("manifest.json"),
    readExtensionFile("package.json"),
    readExtensionFile("newtab.html"),
    readExtensionFile("newtab.js"),
    readExtensionFile("popup.html"),
    readExtensionFile("content.js"),
    readExtensionFile("chatgpt-theme.css"),
  ]);
  const [
    manifestSource,
    packageSource,
    newTabHtml,
    newTabScript,
    popupHtml,
    contentScript,
    themeCss,
  ] = sources;
  const manifest = JSON.parse(manifestSource);
  const packageMetadata = JSON.parse(packageSource);

  assert.equal(manifest.manifest_version, 3, "Manifest V3 is required");
  assert.equal(packageMetadata.version, manifest.version, "Package and manifest versions must match");
  assert.equal(manifest.chrome_url_overrides?.newtab, "newtab.html");
  assert.deepEqual(manifest.permissions, ["storage"], "Only local storage permission is expected");
  assert.equal(manifest.action?.default_popup, "popup.html");
  assert.deepEqual(manifest.content_scripts?.[0]?.matches, ["https://chatgpt.com/*"]);
  assert.deepEqual(manifest.content_scripts?.[0]?.js, ["settings.js", "content.js"]);
  assert.deepEqual(manifest.content_scripts?.[0]?.css, ["chatgpt-theme.css"]);
  assert.match(newTabHtml, /<script src="newtab\.js" defer><\/script>/);
  assert.match(popupHtml, /<script src="settings\.js" defer><\/script>/);
  assert.match(popupHtml, /<script src="popup\.js" defer><\/script>/);
  for (const html of [newTabHtml, popupHtml]) {
    assert.doesNotMatch(html, /<script(?:\s[^>]*)?>\s*[^<\s]/i, "Inline scripts violate CSP");
  }
  assert.match(newTabScript, /https:\/\/chatgpt\.com\//);
  assert.match(newTabScript, /location\.replace/);
  assert.match(contentScript, /chrome\.storage\.onChanged/);
  assert.match(contentScript, /attachShadow\(\{ mode: "closed" \}\)/);
  assert.match(contentScript, /querySelectorAll\("main"\)/);
  assert.match(contentScript, /new ResizeObserver/);
  assert.doesNotMatch(contentScript, /--cgnt-wallpaper-image/);
  assert.match(themeCss, /data-cgnt-wallpaper/);
  assert.doesNotMatch(themeCss, /bg-token-sidebar-surface/);
}

/** Ensure every manifest icon is a real square PNG at its declared size. */
async function validateIcons() {
  for (const size of [16, 32, 48, 128]) {
    const relativePath = `icons/icon-${size}.png`;
    assertPngDimensions(await readExtensionAsset(relativePath), size, relativePath);
  }
}

/** Validate the shared migration boundary with malformed and future data. */
async function validateSettingsNormalization() {
  await import(new URL("../settings.js", import.meta.url));
  const { normalizePreferences } = globalThis.ChatGptNewTabSettings;
  const normalized = normalizePreferences({
    enabled: false,
    dimming: 999,
    blur: -5,
    surfaceOpacity: "not-a-number",
    fit: "unsupported",
  });

  assert.deepEqual(normalized, {
    enabled: false,
    dimming: 80,
    blur: 0,
    surfaceOpacity: 78,
    fit: "contain",
  });
}

await validateExtension();
await validateIcons();
await validateSettingsNormalization();
console.log("Chrome extension validation passed.");
