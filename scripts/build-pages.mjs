// Builds the static GitHub Pages site into frontend/dist-pages:
//   - dataset.json converted from the CSV by the Go server code,
//   - glucolens-core.wasm (data API) and glucolens-agent.wasm (data API +
//     ADK agent), compiled from the same Go packages as the server,
//   - Go's wasm_exec.js loader,
//   - the React app built in static mode with the repository's base path.
// Usage: node scripts/build-pages.mjs [--base /glucolens/]
import { execFileSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdirSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const backend = path.join(root, 'backend')
const frontend = path.join(root, 'frontend')
const out = path.join(frontend, 'dist-pages')
const stage = path.join(frontend, '.pages-stage')
const baseArg = process.argv.indexOf('--base')
const base = baseArg > -1 ? process.argv[baseArg + 1] : process.env.PAGES_BASE || '/glucolens/'

const run = (cmd, args, opts = {}) => {
  console.log(`$ ${cmd} ${args.join(' ')}`)
  // npx is a .cmd shim on Windows and needs a shell; go must not get one, or
  // the shell would split '-ldflags=-s -w' into two arguments.
  execFileSync(cmd, args, { stdio: 'inherit', shell: process.platform === 'win32' && cmd === 'npx', ...opts })
}
const mb = (file) => `${(statSync(file).size / 1024 / 1024).toFixed(1)} MB`

rmSync(stage, { recursive: true, force: true })
mkdirSync(stage, { recursive: true })

// 1. Convert the CSV once into the JSON the app reads.
run('go', ['run', './cmd/server', '-convert-only', '-env', 'none.env'], { cwd: backend })
copyFileSync(path.join(backend, 'data', 'dataset.json'), path.join(stage, 'dataset.json'))

// 2. The two WebAssembly builds.
const wasmEnv = { ...process.env, GOOS: 'js', GOARCH: 'wasm' }
for (const [cmd, file] of [
  ['./cmd/wasm-core', 'glucolens-core.wasm'],
  ['./cmd/wasm-agent', 'glucolens-agent.wasm'],
]) {
  run('go', ['build', '-trimpath', '-ldflags=-s -w', '-o', path.join(stage, file), cmd], { cwd: backend, env: wasmEnv })
  console.log(`  ${file}: ${mb(path.join(stage, file))}`)
}

// 3. Go's JavaScript loader, matching the Go version that built the binaries.
const goroot = execFileSync('go', ['env', 'GOROOT'], { encoding: 'utf8' }).trim()
const loader = [path.join(goroot, 'lib', 'wasm', 'wasm_exec.js'), path.join(goroot, 'misc', 'wasm', 'wasm_exec.js')].find(existsSync)
if (!loader) throw new Error('wasm_exec.js not found in GOROOT')
copyFileSync(loader, path.join(stage, 'wasm_exec.js'))

// 4. The frontend in static mode.
run('npx', ['tsc', '-b'], { cwd: frontend })
run('npx', ['vite', 'build', '--base', base, '--outDir', 'dist-pages', '--emptyOutDir'], {
  cwd: frontend,
  env: { ...process.env, VITE_STATIC: '1' },
})

// 5. Assemble: runtime files, SPA fallback for deep links, no Jekyll.
for (const f of readdirSync(stage)) copyFileSync(path.join(stage, f), path.join(out, f))
copyFileSync(path.join(out, 'index.html'), path.join(out, '404.html'))
writeFileSync(path.join(out, '.nojekyll'), '')
rmSync(stage, { recursive: true, force: true })

console.log(`\nStatic site ready in ${path.relative(root, out)} (base ${base})`)
for (const f of readdirSync(out).sort()) {
  const p = path.join(out, f)
  if (statSync(p).isFile()) console.log(`  ${f.padEnd(26)} ${mb(p)}`)
}
