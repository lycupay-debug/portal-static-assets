import { readdirSync, readFileSync, statSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const dist = fileURLToPath(new URL('../dist/', import.meta.url))
const assetsDir = join(dist, 'assets')
const html = readFileSync(join(dist, 'index.html'), 'utf8')
const assets = readdirSync(assetsDir)

function assetFile(pattern, label) {
  const matches = assets.filter((name) => pattern.test(name))
  if (matches.length !== 1) throw new Error(`Expected exactly one ${label} asset, found ${matches.length}`)
  return join(assetsDir, matches[0])
}

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
const entryPath = join(dist, entryMatch[1].replace(/^\//, ''))
const cssPaths = [...html.matchAll(/<link[^>]+rel="stylesheet"[^>]+href="([^"]+\.css)"/g)]
  .map((match) => join(dist, match[1].replace(/^\//, '')))
if (!cssPaths.length) throw new Error('Could not find initial CSS assets in dist/index.html')

const entry = sizes(entryPath)
assertBudget('Initial JavaScript (gzip)', entry.gzip, 65 * 1024, 'gzip bytes')
// Gzip remains the primary delivery-size guard. The raw limit allows small, intentional module-boundary refactors.
assertBudget('Initial JavaScript (raw)', entry.raw, 193 * 1024, 'raw bytes')
const initialCssGzip = cssPaths.reduce((total, path) => total + sizes(path).gzip, 0)
assertBudget('Initial CSS (gzip)', initialCssGzip, 85 * 1024, 'gzip bytes')

const editorPath = assetFile(/^ContentDocumentEditor-.+\.js$/, 'lazy editor JavaScript')
const editor = sizes(editorPath)
assertBudget('Lazy editor JavaScript (gzip)', editor.gzip, 230 * 1024, 'gzip bytes')
assertBudget('Lazy editor JavaScript (raw)', editor.raw, 720 * 1024, 'raw bytes')

const markdownPath = assetFile(/^markdown-.+\.js$/, 'lazy Markdown renderer JavaScript')
const markdown = sizes(markdownPath)
assertBudget('Lazy Markdown renderer (gzip)', markdown.gzip, 30 * 1024, 'gzip bytes')

const initialMarkup = html.match(/<link[^>]+rel="modulepreload"[^>]+href="([^"]+)"/g) || []
if (initialMarkup.some((tag) => /ContentDocumentEditor|markdown-|tiptap-|prosemirror-/i.test(tag))) {
  throw new Error('Initial HTML preloads editor or article-rendering dependencies; keep these route-lazy')
}
console.log('✓ Editor and Markdown renderer remain outside the initial HTML preload list')
