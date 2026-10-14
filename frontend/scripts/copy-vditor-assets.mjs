import { cpSync, mkdirSync, rmSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const scriptDirectory = dirname(fileURLToPath(import.meta.url))
const frontendDirectory = dirname(scriptDirectory)
const source = join(frontendDirectory, 'node_modules', 'vditor', 'dist')
const target = join(frontendDirectory, 'public', 'vditor', 'dist')

rmSync(target, { recursive: true, force: true })
mkdirSync(dirname(target), { recursive: true })
for (const directory of ['js', 'images', 'css']) {
  cpSync(join(source, directory), join(target, directory), { recursive: true })
}
console.log('Copied Vditor runtime assets from the locked npm dependency.')
