/**
 * Regression tests for the static asset layout checker using isolated temporary fixtures.
 * No project build files or production paths are modified.
 */
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const checker = fileURLToPath(new URL('./check-static-assets-layout.mjs', import.meta.url));
const temp = mkdtempSync(join(tmpdir(), 'static-assets-check-'));
function write(root, path, content) {
  const file = join(root, path);
  mkdirSync(join(file, '..'), { recursive: true });
  writeFileSync(file, content);
}
function run(root) {
  return spawnSync(process.execPath, [checker, root], { encoding: 'utf8' });
}
try {
  const valid = join(temp, 'valid');
  write(valid, 'index.html', '<html><head><link rel="stylesheet" href="/assets/app.css"></head><body><script src="/assets/app.js"></script></body></html>');
  write(valid, 'assets/app.css', '.icon { mask-image: url("/icons/check.svg"); }');
  write(valid, 'assets/app.js', 'console.log("ok")');
  write(valid, 'icons/check.svg', '<svg xmlns="http://www.w3.org/2000/svg"></svg>');
  const good = run(valid);
  assert.equal(good.status, 0, `valid fixture should pass: ${good.stderr}`);

  const missing = join(temp, 'missing');
  write(missing, 'index.html', '<html><head><link rel="stylesheet" href="/assets/missing.css"></head></html>');
  const badHtml = run(missing);
  assert.notEqual(badHtml.status, 0, 'missing HTML asset must fail');
  assert.match(badHtml.stderr, /missing/);

  const missingCss = join(temp, 'missing-css');
  write(missingCss, 'index.html', '<html><head></head><body></body></html>');
  write(missingCss, 'assets/app.css', '.icon { background: url("/icons/missing.svg"); }');
  const badCss = run(missingCss);
  assert.notEqual(badCss.status, 0, 'missing CSS asset must fail');
  assert.match(badCss.stderr, /missing/);

  const escape = join(temp, 'escape');
  write(escape, 'index.html', '<html><body><script src="../../outside.js"></script></body></html>');
  const badEscape = run(escape);
  assert.notEqual(badEscape.status, 0, 'path traversal reference must fail');
  assert.match(badEscape.stderr, /escapes output/);

  console.log('Static asset checker tests passed: valid references, missing HTML asset, missing CSS asset, and path traversal');
} finally {
  rmSync(temp, { recursive: true, force: true });
}
