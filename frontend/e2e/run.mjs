// End-to-end check of the whole app: the real Go server (dataset, query API,
// ADK agent) serving the built frontend, a scripted fake Groq, and Microsoft
// Edge driven by Playwright. Walks every tab at desktop and phone widths in
// both themes, fails on console errors or failed requests, and saves
// screenshots to e2e/output.
//
// Prerequisites: `npm run build` (frontend/dist) and Go on PATH.
import { spawn, spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, rmSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright-core'
import { startFakeGroq } from './fake-groq.mjs'

const here = path.dirname(fileURLToPath(import.meta.url))
const frontend = path.resolve(here, '..')
const backend = path.resolve(frontend, '../backend')
const out = path.join(here, 'output')
const PORT = 8091
const GROQ_PORT = 8797
const BASE = `http://127.0.0.1:${PORT}`

if (!existsSync(path.join(frontend, 'dist', 'index.html'))) {
  console.error('frontend/dist is missing: run `npm run build` first.')
  process.exit(1)
}
rmSync(out, { recursive: true, force: true })
mkdirSync(out, { recursive: true })

console.log('building the Go server…')
const exe = path.join(backend, 'bin', process.platform === 'win32' ? 'glucolens-e2e.exe' : 'glucolens-e2e')
const build = spawnSync('go', ['build', '-o', exe, './cmd/server'], { cwd: backend, stdio: 'inherit' })
if (build.status !== 0) process.exit(1)

const groq = await startFakeGroq(GROQ_PORT)
const server = spawn(exe, ['-env', 'none.env'], {
  cwd: backend,
  env: {
    ...process.env,
    PORT: String(PORT),
    STATIC_DIR: path.join(frontend, 'dist'),
    GROQ_API_KEY: 'e2e-fake-key',
    GROQ_BASE_URL: `http://127.0.0.1:${GROQ_PORT}`,
    GROQ_MODEL: 'openai/gpt-oss-120b',
  },
  stdio: ['ignore', 'pipe', 'pipe'],
})
let serverLog = ''
server.stdout.on('data', (d) => (serverLog += d))
server.stderr.on('data', (d) => (serverLog += d))


const assertNoOverflow = async (page, tag, where, problems) => {
  const extra = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  if (extra > 1) problems.push(`[${tag}] ${where}: page is ${extra}px wider than the screen`)
}
const problems = []
let browser
try {
  for (let i = 0; i < 60; i++) {
    try {
      const r = await fetch(`${BASE}/api/v1/health`)
      if (r.ok) break
    } catch {
      /* not up yet */
    }
    await new Promise((r) => setTimeout(r, 500))
  }
  const health = await (await fetch(`${BASE}/api/v1/health`)).json()
  if (!health.assistant?.ready) throw new Error('assistant not ready in e2e server: ' + JSON.stringify(health.assistant))

  browser = await chromium.launch({ channel: 'msedge', headless: true })

  for (const theme of ['light', 'dark']) {
    for (const vp of [
      { name: 'desktop', width: 1360, height: 900 },
      { name: 'phone', width: 390, height: 844 },
    ]) {
      const tag = `${theme}-${vp.name}`
      const context = await browser.newContext({ viewport: { width: vp.width, height: vp.height }, colorScheme: theme, deviceScaleFactor: 1 })
      await context.addInitScript((t) => {
        try {
          // Seed only once per context so values entered during the run survive navigation.
          if (!localStorage.getItem('glucolens-settings')) {
            localStorage.setItem('glucolens-settings', JSON.stringify({ state: { theme: t, values: {}, useValues: true }, version: 1 }))
          }
        } catch {
          /* ignore */
        }
      }, theme)
      const page = await context.newPage()
      page.on('console', (m) => m.type() === 'error' && problems.push(`[${tag}] console: ${m.text()}`))
      page.on('pageerror', (e) => problems.push(`[${tag}] page error: ${e.message}`))
      page.on('requestfailed', (r) => {
        const why = r.failure()?.errorText ?? ''
        if (!why.includes('ERR_ABORTED')) problems.push(`[${tag}] request failed: ${r.url()} ${why}`)
      })
      page.on('response', (r) => r.status() >= 400 && problems.push(`[${tag}] HTTP ${r.status()} ${r.url()}`))
      const shot = (name) => page.screenshot({ path: path.join(out, `${tag}-${name}.png`), fullPage: true })
      const expectText = async (text, where) => {
        try {
          await page.getByText(text, { exact: false }).first().waitFor({ timeout: 15000 })
        } catch {
          problems.push(`[${tag}] ${where}: did not find "${text}"`)
        }
      }

      // Ask: hero, then a full streamed answer with evidence.
      await page.goto(`${BASE}/`)
      await expectText('Understand your checkup', 'ask hero')
      await page.waitForTimeout(800)
      await assertNoOverflow(page, tag, 'ask-hero', problems)
      await shot('ask-hero')
      await page.getByRole('textbox', { name: 'Your question' }).fill('Does smoking change diabetes rates in this dataset?')
      await page.keyboard.press('Enter')
      await expectText('Data behind this answer', 'ask answer')
      await expectText('No meaningful difference', 'ask answer text')
      await page.locator('article svg').first().waitFor({ timeout: 15000 }).catch(() => problems.push(`[${tag}] ask: evidence chart missing`))
      await page.waitForTimeout(900)
      await assertNoOverflow(page, tag, 'ask-answer', problems)
      await shot('ask-answer')

      // My values: load a sample patient.
      await page.getByRole('button', { name: /My values/ }).click()
      await page.getByRole('button', { name: 'Load sample patient' }).click()
      await page.waitForFunction(() => {
        try {
          return Object.keys(JSON.parse(localStorage.getItem('glucolens-settings') || '{}').state?.values ?? {}).length > 10
        } catch {
          return false
        }
      }, null, { timeout: 10000 }).catch(() => problems.push(`[${tag}] values: sample patient not loaded`))
      await page.waitForTimeout(400)
      await assertNoOverflow(page, tag, 'values-drawer', problems)
      await shot('values-drawer')
      await page.keyboard.press('Escape')

      // A personal question now that values exist.
      await page.getByRole('textbox', { name: 'Your question' }).fill('Where does my HbA1c sit?')
      await page.keyboard.press('Enter')
      await page.waitForFunction(() => document.querySelectorAll('article').length >= 2, null, { timeout: 15000 })
      await expectText('Placing your values among everyone', 'ask personal')
      await page.waitForTimeout(1500)

      // Insights: sections, with the sample patient's values marked.
      await page.goto(`${BASE}/insights?section=glucose`)
      await expectText('HbA1c', 'insights')
      await expectText('Your glucose & insulin values', 'insights profile')
      await page.locator('svg text', { hasText: 'You ·' }).first().waitFor({ timeout: 15000 }).catch(() => problems.push(`[${tag}] insights: no You marker`))
      await page.waitForTimeout(1200)
      await assertNoOverflow(page, tag, 'insights-glucose', problems)
      await shot('insights-glucose')
      await page.goto(`${BASE}/insights?section=lifestyle&split=gender`)
      await expectText('Smoking', 'insights split')
      await page.waitForTimeout(1200)
      await assertNoOverflow(page, tag, 'insights-split', problems)
      await shot('insights-split')
      await page.goto(`${BASE}/insights?view=relationships`)
      await expectText('How values move together', 'relationships')
      await page.waitForTimeout(1200)
      await assertNoOverflow(page, tag, 'insights-relationships', problems)
      await shot('insights-relationships')

      // Risk Explorer: ranking, factor deep dive with strata, your picture.
      await page.goto(`${BASE}/risk?factor=family_history_diabetes&strat=age`)
      await expectText('What matters most', 'risk')
      await expectText('Within each Age group', 'risk strata')
      await expectText('People like you', 'risk your picture')
      await expectText('closest people in this dataset', 'risk similar details')
      await expectText('Values to talk about', 'risk flagged values')
      await page.waitForTimeout(1500)
      await assertNoOverflow(page, tag, 'risk', problems)
      await shot('risk')

      await context.close()
    }
  }
  if (groq.requests.length === 0) problems.push('the fake Groq server never received a request')
  const sys = groq.requests[0]?.messages?.[0]?.content ?? ''
  if (!String(sys).includes('GlucoLens')) problems.push('system prompt did not reach the model')
} catch (err) {
  problems.push(`run failed: ${err.stack ?? err}`)
} finally {
  await browser?.close()
  server.kill()
  groq.server.close()
}

if (problems.length > 0) {
  console.error(`\n${problems.length} problem(s):\n- ` + problems.join('\n- '))
  if (serverLog) console.error('\nserver log tail:\n' + serverLog.split('\n').slice(-15).join('\n'))
  process.exit(1)
}
console.log(`\nAll checks passed. ${groq.requests.length} model calls. Screenshots in ${out}`)
