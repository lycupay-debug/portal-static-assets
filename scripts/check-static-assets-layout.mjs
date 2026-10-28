/**
 * Validate a Vite build output directory for basic static-resource integrity.
 * This check is intentionally read-only and only inspects the supplied build directory.
 */
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, resolve, sep } from 'node:path';

const root = resolve(process.argv[2] ?? 'frontend/dist');
if (!existsSync(root) || !statSync(root).isDirectory()) {
  console.error(`Build output directory does not exist: ${root}`);
  process.exit(2);
}

const files = [];
function walk(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isSymbolicLink()) {
      console.error(`Unexpected symlink in build output: ${relative(root, full)}`);
      process.exitCode = 1;
    } else if (entry.isDirectory()) walk(full);
    else if (entry.isFile()) files.push(full);
  }
}
walk(root);
const rel = (path) => relative(root, path).split(sep).join('/');
const htmlFiles = files.filter((f) => f.endsWith('.html'));
if (htmlFiles.length === 0) {
  console.error(`No HTML entry file found under ${root}`);
  process.exit(1);
}
const missing = new Set();
for (const html of htmlFiles) {
  const source = readFileSync(html, 'utf8');
  for (const match of source.matchAll(/(?:src|href)=["']([^"']+)["']/g)) {
    const ref = match[1];
    if (/^(?:[a-z]+:|\/\/|#|data:|mailto:|tel:)/i.test(ref)) continue;
    const pathPart = decodeURIComponent(ref.split(/[?#]/, 1)[0]);
    if (!pathPart) continue;
    const target = resolve(root, pathPart.replace(/^\//, ''));
    if (!target.startsWith(root + sep) && target !== root) {
      missing.add(`${rel(html)} -> reference escapes output: ${ref}`);
    } else if (!existsSync(target)) {
      missing.add(`${rel(html)} -> missing: ${ref}`);
    }
  }
}
if (missing.size) {
  for (const item of missing) console.error(item);
  process.exitCode = 1;
} else {
  console.log(`Static asset layout check passed: ${files.length} files, ${htmlFiles.length} HTML files under ${root}`);
}
