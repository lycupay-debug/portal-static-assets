/**
 * Validate a Vite build intended for the dedicated static-resource origin.
 * This is a local read-only check; it does not contact or modify production.
 */
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';

const root = resolve(process.argv[2] ?? 'frontend/dist-static-origin');
const origin = process.argv[3] ?? 'https://assets.lycupaynotion.top';
const originPrefix = origin.endsWith('/') ? origin : `${origin}/`;
if (!existsSync(join(root, 'index.html'))) throw new Error(`Missing build entry HTML in ${root}`);
const html = readFileSync(join(root, 'index.html'), 'utf8');
const entryUrls = [...html.matchAll(/(?:src|href)="(https?:\/\/[^"]+)"/g)].map((m) => m[1]);
if (entryUrls.length < 2) throw new Error('Expected absolute entry JavaScript and stylesheet URLs');
for (const url of entryUrls) {
  if (!url.startsWith(originPrefix)) throw new Error(`Entry URL is not on configured static origin: ${url}`);
  const parsed = new URL(url);
  const local = join(root, decodeURIComponent(parsed.pathname.replace(/^\//, '')));
  if (!existsSync(local) || !statSync(local).isFile()) throw new Error(`Entry asset missing from build output: ${parsed.pathname}`);
}
const assets = join(root, 'assets');
const jsFiles = readdirSync(assets).filter((name) => name.endsWith('.js'));
const cssFiles = readdirSync(assets).filter((name) => name.endsWith('.css'));
if (!jsFiles.some((name) => name.startsWith('index-'))) throw new Error('Missing hashed JS entry bundle');
if (!cssFiles.some((name) => name.startsWith('index-'))) throw new Error('Missing hashed CSS entry bundle');
let iconReferences = 0;
for (const name of cssFiles) {
  const source = readFileSync(join(assets, name), 'utf8');
  for (const match of source.matchAll(/url\(\s*(['"]?)(.*?)\1\s*\)/g)) {
    const reference = match[2].trim();
    if (reference.includes('/icons/')) {
      if (!reference.startsWith(originPrefix)) throw new Error(`Icon URL does not use static origin: ${reference}`);
      const parsed = new URL(reference);
      const local = join(root, decodeURIComponent(parsed.pathname.replace(/^\//, '')));
      if (!existsSync(local) || !statSync(local).isFile()) throw new Error(`Icon file missing: ${parsed.pathname}`);
      iconReferences += 1;
    }
  }
}
const entryJs = readFileSync(join(assets, jsFiles.find((name) => name.startsWith('index-'))), 'utf8');
if (!entryJs.includes(origin)) throw new Error('JS entry does not contain the configured static origin for lazy chunks');
if (!entryJs.includes('/api')) throw new Error('Expected API paths to remain on the page origin (root-relative /api)');
console.log(`Static-origin build passed: ${entryUrls.length} entry URLs, ${jsFiles.length} JS bundles, ${cssFiles.length} CSS bundles, ${iconReferences} icon URLs; API paths remain root-relative`);
