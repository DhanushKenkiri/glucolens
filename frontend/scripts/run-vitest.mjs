// Runs Vitest with every path spelled the same way. On Windows a lower-case
// drive letter (g:\ vs G:\) makes Vitest load its runner twice, and every
// suite then fails with "Cannot read properties of undefined (reading 'config')".
import { realpathSync } from 'node:fs'
import { spawnSync } from 'node:child_process'
import { createRequire } from 'node:module'
import path from 'node:path'

const canonical = (p) => realpathSync.native(p).replace(/^[a-z]:/, (d) => d.toUpperCase())
const require = createRequire(import.meta.url)
const cwd = canonical(process.cwd())
const bin = canonical(path.join(path.dirname(require.resolve('vitest/package.json')), 'vitest.mjs'))
const result = spawnSync(process.execPath, [bin, 'run', ...process.argv.slice(2)], {
  cwd,
  stdio: 'inherit',
  env: { ...process.env, INIT_CWD: cwd },
})
process.exit(result.status ?? 1)
