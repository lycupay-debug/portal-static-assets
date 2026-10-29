/**
 * Responsibility: enforce measured transfer-size budgets for the initial route and lazy features.
 * Dependencies: the Vite output directory and Node's gzip implementation; no network access.
 * Collaboration: run after normal or static-origin builds to catch regressions in initial, editor,
 * and Markdown payloads, including separately emitted vendor chunks.
 * Boundaries: measures built artifacts only; it never changes thresholds or production files.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const dist = resolve(process.argv[2] ?? fileURLToPath(new URL('../dist/', import.meta.url)))
const assetsDir = join(dist, 'assets')
const html = readFileSync(join(dist, 'index.html'), 'utf8')
const assets = readdirSync(assetsDir)

function sizes(path) {
  const source = readFileSync(path)
  return { raw: statSync(path).size, gzip: gzipSync(source).byteLength }
}

function assertBudget(label, actual, limit, unit) {
  if (actual > limit) throw new Error(`${label} is ${actual} bytes; budget is ${limit} bytes (${unit})`)
  console.log(`✓ ${label}: ${(actual / 1024).toFixed(2)} KiB / ${(limit / 1024).toFixed(2)} KiB ${unit}`)
}

const entryMatch = html.match(/<script[^>]+src="([^"]+\.js)"/)
if (!entryMatch) throw new Error('Could not find the initial JavaScript entry in dist/index.html')
const localBuildPath = (url) => join(dist, decodeURIComponent(new URL(url, 'http://local.invalid').pathname.replace(/^\//, '')))
const entryPath = localBuildPath(entryMatch[1])
const cssPaths = [...html.matchAll(/<link[^>]+rel="stylesheet"[^>]+href="([^"]+\.css)"/g)]
  .map((match) => localBuildPath(match[1]))
if (!cssPaths.length) throw new Error('Could not find initial CSS assets in dist/index.html')

const initialJsPaths = new Set([entryPath])
for (const match of html.matchAll(/<link[^>]+rel="modulepreload"[^>]+href="([^"]+\.js)"/g)) {
  initialJsPaths.add(localBuildPath(match[1]))
}
const initialJs = [...initialJsPaths].reduce((total, path) => {
  const size = sizes(path)
  return { raw: total.raw + size.raw, gzip: total.gzip + size.gzip }
}, { raw: 0, gzip: 0 })
// Maintainability guardrail, not a minimization target; do not fragment architecture to shave bytes.
assertBudget('Initial JavaScript including modulepreloads (gzip)', initialJs.gzip, 150 * 1024, 'gzip bytes')
// Keep a small raw-size allowance for whitespace-insensitive table search.
assertBudget('Initial JavaScript including modulepreloads (raw)', initialJs.raw, 191 * 1024, 'raw bytes')
const initialCssGzip = cssPaths.reduce((total, path) => total + sizes(path).gzip, 0)
assertBudget('Initial CSS (gzip)', initialCssGzip, 85 * 1024, 'gzip bytes')

function aggregateChunks(patterns, label) {
  const paths = assets.filter((name) => patterns.some((pattern) => pattern.test(name))).map((name) => join(assetsDir, name))
  if (!assets.some((name) => patterns[0].test(name))) {
    throw new Error(`Could not find the primary ${label} chunk`)
  }
  return paths.reduce((total, path) => {
    const size = sizes(path)
    return { raw: total.raw + size.raw, gzip: total.gzip + size.gzip }
  }, { raw: 0, gzip: 0 })
}

const editor = aggregateChunks([/^ContentDocumentEditor-.+\.js$/, /^editor-vendor-.+\.js$/], 'lazy editor JavaScript')
assertBudget('Lazy editor JavaScript including vendor chunks (gzip)', editor.gzip, 230 * 1024, 'gzip bytes')
assertBudget('Lazy editor JavaScript including vendor chunks (raw)', editor.raw, 720 * 1024, 'raw bytes')

const markdown = aggregateChunks([/^markdown-.+\.js$/, /^markdown-vendor-.+\.js$/], 'lazy Markdown renderer JavaScript')
assertBudget('Lazy Markdown renderer including vendor chunks (gzip)', markdown.gzip, 30 * 1024, 'gzip bytes')

const initialMarkup = html.match(/<link[^>]+rel="modulepreload"[^>]+href="([^"]+)"/g) || []
if (initialMarkup.some((tag) => /ContentDocumentEditor|markdown-|tiptap-|prosemirror-|editor-vendor-/i.test(tag))) {
  throw new Error('Initial HTML preloads editor or article-rendering dependencies; keep these route-lazy')
}
console.log('✓ Editor and Markdown renderer remain outside the initial HTML preload list')
