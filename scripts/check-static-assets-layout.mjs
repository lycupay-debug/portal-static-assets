/**
 * Read-only validation for a Vite static site output directory.
 * Checks HTML entry points and local HTML/CSS asset references without touching source files.
 */
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve, sep } from 'node:path';

const root = resolve(process.argv[2] ?? 'frontend/dist');
if (!existsSync(root) || !statSync(root).isDirectory()) {
  console.error(`Build output directory does not exist: ${root}`);
  process.exit(2);
}

const files = [];
let hasSymlink = false;
function walk(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isSymbolicLink()) {
      console.error(`Unexpected symlink in build output: ${relative(root, full)}`);
      hasSymlink = true;
    } else if (entry.isDirectory()) walk(full);
    else if (entry.isFile()) files.push(full);
  }
}
walk(root);
const rel = (path) => relative(root, path).split(sep).join('/');
const htmlFiles = files.filter((f) => f.endsWith('.html'));
if (hasSymlink) process.exitCode = 1;
if (htmlFiles.length === 0) {
  console.error(`No HTML entry file found under ${root}`);
  process.exit(1);
}

const missing = new Set();
function checkReference(owner, reference, ownerDirectory) {
  if (/^(?:[a-z][a-z\d+.-]*:|\/\/|#)/i.test(reference)) return;
  let pathPart;
  try {
    pathPart = decodeURIComponent(reference.split(/[?#]/, 1)[0]);
  } catch {
    missing.add(`${rel(owner)} -> malformed encoded reference: ${reference}`);
    return;
  }
  if (!pathPart) return;
  const target = resolve(pathPart.startsWith('/') ? root : ownerDirectory, pathPart.replace(/^\//, ''));
  if (target !== root && !target.startsWith(root + sep)) {
    missing.add(`${rel(owner)} -> reference escapes output: ${reference}`);
  } else if (!existsSync(target)) {
    missing.add(`${rel(owner)} -> missing: ${reference}`);
  }
}

for (const html of htmlFiles) {
  const source = readFileSync(html, 'utf8');
  for (const match of source.matchAll(/(?:src|href)\s*=\s*["']([^"']+)["']/gi)) {
    checkReference(html, match[1], dirname(html));
  }
}
for (const css of files.filter((f) => f.endsWith('.css'))) {
  const source = readFileSync(css, 'utf8');
  for (const match of source.matchAll(/url\(\s*(["']?)(.*?)\1\s*\)/gi)) {
    checkReference(css, match[2].trim(), dirname(css));
  }
}

if (missing.size) {
  for (const item of missing) console.error(item);
  process.exitCode = 1;
} else if (!hasSymlink) {
  console.log(`Static asset layout check passed: ${files.length} files, ${htmlFiles.length} HTML files, local HTML/CSS references resolved under ${root}`);
}
