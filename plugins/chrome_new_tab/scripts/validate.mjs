import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const extensionRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

/** Read a required extension file relative to the package root. */
function readExtensionFile(relativePath) {
  return readFile(resolve(extensionRoot, relativePath), "utf8");
}

/**
 * Validate the deployment contract that Chrome relies on. These checks remain
 * deliberately dependency-free so the extension can be audited and installed
 * without running npm install.
 */
async function validateExtension() {
  const [manifestSource, html, script] = await Promise.all([
    readExtensionFile("manifest.json"),
    readExtensionFile("newtab.html"),
    readExtensionFile("newtab.js"),
  ]);
  const manifest = JSON.parse(manifestSource);

  assert.equal(manifest.manifest_version, 3, "Manifest V3 is required");
  assert.equal(manifest.chrome_url_overrides?.newtab, "newtab.html");
  assert.match(html, /<script src="newtab\.js" defer><\/script>/);
  assert.doesNotMatch(html, /<script(?:\s[^>]*)?>\s*[^<\s]/i, "Inline scripts violate extension CSP");
  assert.match(script, /https:\/\/chatgpt\.com\//);
  assert.match(script, /location\.replace/);
}

await validateExtension();
console.log("Chrome extension validation passed.");
