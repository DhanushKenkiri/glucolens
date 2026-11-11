// Rebuilds only the frontend of the static site, keeping the existing
// WebAssembly binaries and dataset in frontend/dist-pages (faster iteration).
import { execFileSync } from 'node:child_process'
import { copyFileSync, mkdirSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const frontend = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'frontend')
const out = path.join(frontend, 'dist-pages')
const keep = path.join(frontend, '.pages-keep')
const runtime = ['dataset.json', 'glucolens-core.wasm', 'glucolens-agent.wasm', 'wasm_exec.js']
rmSync(keep, { recursive: true, force: true })
mkdirSync(keep)
for (const f of runtime) copyFileSync(path.join(out, f), path.join(keep, f))
execFileSync('npx', ['vite', 'build', '--base', process.env.PAGES_BASE || '/glucolens/', '--outDir', 'dist-pages', '--emptyOutDir'], {
  cwd: frontend,
  stdio: 'inherit',
  shell: process.platform === 'win32',
  env: { ...process.env, VITE_STATIC: '1' },
})
for (const f of readdirSync(keep)) copyFileSync(path.join(keep, f), path.join(out, f))
copyFileSync(path.join(out, 'index.html'), path.join(out, '404.html'))
writeFileSync(path.join(out, '.nojekyll'), '')
rmSync(keep, { recursive: true, force: true })
