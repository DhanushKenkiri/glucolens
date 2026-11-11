// Builds the static site and publishes it to the repository's gh-pages branch
// from this machine. Use it when the GitHub Actions workflow cannot run (for
// example while Actions is unavailable on the account).
// Usage: node scripts/deploy-pages.mjs [--skip-build]
import { execFileSync } from 'node:child_process'
import { existsSync, rmSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const site = path.join(root, 'frontend', 'dist-pages')
const git = (args, cwd = root) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim()

if (!process.argv.includes('--skip-build')) {
  execFileSync(process.execPath, [path.join(root, 'scripts', 'build-pages.mjs')], { stdio: 'inherit' })
}
if (!existsSync(path.join(site, 'index.html'))) throw new Error('frontend/dist-pages is missing')

const remote = git(['remote', 'get-url', 'origin'])
const sha = git(['rev-parse', '--short', 'HEAD'])
const identity = ['-c', `user.name=${git(['config', 'user.name'])}`, '-c', `user.email=${git(['config', 'user.email'])}`]

rmSync(path.join(site, '.git'), { recursive: true, force: true })
git(['init', '-q', '-b', 'gh-pages'], site)
git([...identity, 'add', '-A'], site)
git([...identity, 'commit', '-q', '-m', `Deploy ${sha}`], site)
execFileSync('git', ['push', '-f', remote, 'gh-pages'], { cwd: site, stdio: 'inherit' })
rmSync(path.join(site, '.git'), { recursive: true, force: true })
console.log(`\nPublished ${sha} to gh-pages. GitHub Pages updates in a minute or two.`)
