/**
 * Package a validated static-site build for a later, explicitly selected deployment target.
 * Does not upload files, change server configuration, or contact production services.
 */
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const siteDir = resolve(process.argv[2] ?? join(projectRoot, 'frontend/dist'));
const artifactDir = resolve(process.argv[3] ?? join(projectRoot, 'artifacts'));
const repoRoot = execFileSync('git', ['-C', projectRoot, 'rev-parse', '--show-toplevel'], { encoding: 'utf8' }).trim();
const commit = execFileSync('git', ['-C', projectRoot, 'rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();
const version = commit.slice(0, 12);
const archivePath = join(artifactDir, `static-assets-${version}.tar.gz`);
const staging = join(artifactDir, `.static-assets-staging-${version}`);

if (!existsSync(siteDir) || !statSync(siteDir).isDirectory()) throw new Error(`Build output directory does not exist: ${siteDir}`);
if (!existsSync(join(siteDir, 'index.html'))) throw new Error(`Missing entry file: ${join(siteDir, 'index.html')}`);
if (existsSync(archivePath) || existsSync(staging)) throw new Error(`Refusing to overwrite existing release output: ${archivePath}`);
if (siteDir === repoRoot || siteDir.startsWith(repoRoot + sep) === false) throw new Error(`Build output must be inside the project repository: ${siteDir}`);
if (artifactDir === repoRoot || artifactDir.startsWith(repoRoot + sep) === false) throw new Error(`Artifact directory must be inside the project repository: ${artifactDir}`);

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

mkdirSync(artifactDir, { recursive: true });
mkdirSync(join(staging, 'site'), { recursive: true });
try {
  const entries = walk(siteDir).sort((a, b) => relative(siteDir, a).localeCompare(relative(siteDir, b)));
  const files = [];
  for (const source of entries) {
    const path = relative(siteDir, source).split(sep).join('/');
    const bytes = readFileSync(source);
    const destination = join(staging, 'site', path);
    mkdirSync(dirname(destination), { recursive: true });
    copyFileSync(source, destination);
    files.push({ path: `site/${path}`, size: bytes.length, sha256: sha256(bytes) });
  }
  const manifest = {
    schemaVersion: 1,
    project: 'portal-static-assets',
    sourceRepository: 'https://github.com/lycupay-debug/portal-static-assets',
    sourceCommit: commit,
    baselineCommit: '7d36218343da4a74a29f788b219231e65f3264f2',
    createdAt: new Date().toISOString(),
    files,
  };
  writeFileSync(join(staging, 'manifest.json'), `${JSON.stringify(manifest, null, 2)}\n`, { flag: 'wx' });
  execFileSync('tar', ['-czf', archivePath, '-C', staging, 'site', 'manifest.json'], { stdio: 'inherit' });
  const archiveHash = sha256(readFileSync(archivePath));
  writeFileSync(`${archivePath}.sha256`, `${archiveHash}  ${archivePath.split(sep).pop()}\n`, { flag: 'wx' });
  console.log(`Created ${archivePath}`);
  console.log(`Source commit: ${commit}`);
  console.log(`Packaged files: ${files.length}`);
  console.log(`Archive SHA-256: ${archiveHash}`);
} catch (error) {
  rmSync(archivePath, { force: true });
  rmSync(`${archivePath}.sha256`, { force: true });
  throw error;
} finally {
  rmSync(staging, { recursive: true, force: true });
}
