/**
 * Package only files intended to be served by the independent static-resource origin.
 * Deliberately excludes index.html so the asset origin cannot become the portal document origin.
 */
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const projectRoot = resolveProjectRoot();
const siteDir = resolve(process.argv[2] ?? join(projectRoot, 'frontend/dist-static-origin'));
const artifactDir = resolve(process.argv[3] ?? join(projectRoot, 'artifacts'));
const origin = process.argv[4] ?? 'https://assets.lycupaynotion.top';
const commit = execFileSync('git', ['-C', projectRoot, 'rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();
const trackedChanges = execFileSync('git', ['-C', projectRoot, 'status', '--porcelain', '--untracked-files=no'], { encoding: 'utf8' }).trim();
if (trackedChanges) throw new Error('Refusing to package tracked changes that are not committed');
execFileSync('git', ['-C', projectRoot, 'merge-base', '--is-ancestor', '7d36218343da4a74a29f788b219231e65f3264f2', 'HEAD']);
execFileSync(process.execPath, [join(projectRoot, 'scripts/check-static-origin-build.mjs'), siteDir, origin], { stdio: 'inherit' });
if (!existsSync(join(siteDir, 'index.html'))) throw new Error(`Missing build output: ${siteDir}`);
const version = commit.slice(0, 12);
const archivePath = join(artifactDir, `static-origin-${version}.tar.gz`);
const staging = join(artifactDir, `.static-origin-staging-${version}`);
if (existsSync(archivePath) || existsSync(staging)) throw new Error(`Refusing to overwrite existing release output: ${archivePath}`);
if (!siteDir.startsWith(projectRoot + sep) || !artifactDir.startsWith(projectRoot + sep)) throw new Error('Build and artifact directories must be inside the project repository');

function walk(dir) {
  const result = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isSymbolicLink()) throw new Error(`Refusing to package symlink: ${full}`);
    if (entry.isDirectory()) result.push(...walk(full));
    else if (entry.isFile()) result.push(full);
  }
  return result;
}
function sha256(buffer) { return createHash('sha256').update(buffer).digest('hex'); }
const allowed = (path) => path.startsWith('assets/') || path.startsWith('icons/') || path === 'favicon.ico' || path === 'THIRD_PARTY_NOTICES.txt';
const allFiles = walk(siteDir).filter((path) => allowed(relative(siteDir, path).split(sep).join('/')));
if (!allFiles.some((path) => relative(siteDir, path).startsWith(`assets${sep}index-`) && path.endsWith('.js'))) throw new Error('No JS entry bundle found for packaging');
mkdirSync(artifactDir, { recursive: true });
mkdirSync(join(staging, 'site'), { recursive: true });
try {
  const files = [];
  for (const source of allFiles.sort((a, b) => relative(siteDir, a).localeCompare(relative(siteDir, b)))) {
    const path = relative(siteDir, source).split(sep).join('/');
    const bytes = readFileSync(source);
    const destination = join(staging, 'site', path);
    mkdirSync(dirname(destination), { recursive: true });
    copyFileSync(source, destination);
    files.push({ path: `site/${path}`, size: bytes.length, sha256: sha256(bytes) });
  }
  const manifest = { schemaVersion: 1, project: 'portal-static-assets', packageType: 'static-origin-only', staticOrigin: origin, sourceRepository: 'https://github.com/lycupay-debug/portal-static-assets', sourceCommit: commit, baselineCommit: '7d36218343da4a74a29f788b219231e65f3264f2', createdAt: new Date().toISOString(), excludesPortalDocument: true, files };
  writeFileSync(join(staging, 'manifest.json'), `${JSON.stringify(manifest, null, 2)}\n`, { flag: 'wx' });
  execFileSync('tar', ['-czf', archivePath, '-C', staging, 'site', 'manifest.json'], { stdio: 'inherit' });
  const archiveHash = sha256(readFileSync(archivePath));
  writeFileSync(`${archivePath}.sha256`, `${archiveHash}  ${archivePath.split(sep).pop()}\n`, { flag: 'wx' });
  console.log(`Created ${archivePath}`);
  console.log(`Source commit: ${commit}`);
  console.log(`Packaged files: ${files.length} (index.html excluded)`);
  console.log(`Archive SHA-256: ${archiveHash}`);
} catch (error) {
  rmSync(archivePath, { force: true });
  rmSync(`${archivePath}.sha256`, { force: true });
  throw error;
} finally {
  rmSync(staging, { recursive: true, force: true });
}
function resolveProjectRoot() { return resolve(dirname(fileURLToPath(import.meta.url)), '..'); }
