// End-to-end check of the static GitHub Pages build (frontend/dist-pages):
// serves it the way GitHub Pages does (under /glucolens/, 404.html fallback),
// points the in-browser ADK agent at a scripted fake Groq, and drives
// Microsoft Edge through every tab. Run `node scripts/build-pages.mjs` first.
import http from 'node:http'
import { existsSync, mkdirSync, readFileSync, rmSync, statSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright-core'
import { startFakeGroq } from './fake-groq.mjs'

const here = path.dirname(fileURLToPath(import.meta.url))
const site = path.resolve(here, '..', 'dist-pages')
const out = path.join(here, 'output-static')
const PORT = 8093
const GROQ_PORT = 8797
const BASE = `http://127.0.0.1:${PORT}/glucolens/`

if (!existsSync(path.join(site, 'index.html'))) {
  console.error('frontend/dist-pages is missing: run `node scripts/build-pages.mjs` first.')
  process.exit(1)
}
rmSync(out, { recursive: true, force: true })
mkdirSync(out, { recursive: true })

const types = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.json': 'application/json',
  '.wasm': 'application/wasm',
  '.svg': 'image/svg+xml',
}
const pages = http.createServer((req, res) => {
  const url = new URL(req.url, 'http://x')
  if (!url.pathname.startsWith('/glucolens/')) {
    res.writeHead(404).end('not found')
    return
  }
  const rel = decodeURIComponent(url.pathname.slice('/glucolens/'.length)) || 'index.html'
  let file = path.join(site, rel)
  let status = 200
  if (!file.startsWith(site) || !existsSync(file) || statSync(file).isDirectory()) {
    file = path.join(site, '404.html') // GitHub Pages' fallback for unknown paths
    status = 404
  }
  res.writeHead(status, { 'Content-Type': types[path.extname(file)] ?? 'application/octet-stream' })
  res.end(readFileSync(file))
})
await new Promise((resolve) => pages.listen(PORT, '127.0.0.1', resolve))
const groq = await startFakeGroq(GROQ_PORT)


const assertNoOverflow = async (page, tag, where, problems) => {
  const extra = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  if (extra > 1) problems.push(`[${tag}] ${where}: page is ${extra}px wider than the screen`)
}
const problems = []
let browser
try {
  browser = await chromium.launch({ channel: 'msedge', headless: true })
  for (const vp of [
    { name: 'desktop', width: 1360, height: 900 },
    { name: 'phone', width: 390, height: 844 },
  ]) {
    const tag = `static-${vp.name}`
    const context = await browser.newContext({ viewport: { width: vp.width, height: vp.height }, colorScheme: 'light' })
    const page = await context.newPage()
    page.on('console', (m) => {
      // Deep links are served by 404.html with status 404, exactly like GitHub Pages.
      if (m.type() === 'error' && !m.text().includes('status of 404')) problems.push(`[${tag}] console: ${m.text()}`)
    })
    page.on('pageerror', (e) => problems.push(`[${tag}] page error: ${e.message}`))
    page.on('requestfailed', (r) => {
      const why = r.failure()?.errorText ?? ''
      if (!why.includes('ERR_ABORTED')) problems.push(`[${tag}] request failed: ${r.url()} ${why}`)
    })
    page.on('response', (r) => {
      const deepLink = r.status() === 404 && r.request().resourceType() === 'document'
      if (r.status() >= 400 && !deepLink) problems.push(`[${tag}] HTTP ${r.status()} ${r.url()}`)
    })
    const shot = (name) => page.screenshot({ path: path.join(out, `${tag}-${name}.png`), fullPage: true })
    const expectText = async (text, where, timeout = 60000) => {
      try {
        await page.getByText(text, { exact: false }).first().waitFor({ timeout })
      } catch {
        problems.push(`[${tag}] ${where}: did not find "${text}"`)
      }
    }

    // 1. No key yet: the Ask tab offers the key form; the data tabs work.
    await page.goto(BASE)
    await expectText('Understand your checkup', 'hero')
    await expectText('The assistant needs a Groq API key', 'key form')
    await assertNoOverflow(page, tag, 'ask-no-key', problems)
      await shot('ask-no-key')

    // Deep link straight into Insights (served through 404.html).
    await page.goto(`${BASE}insights?section=glucose`)
    await expectText('Fasting glucose', 'insights deep link')
    await page.locator('section svg, div svg').first().waitFor({ timeout: 60000 }).catch(() => problems.push(`[${tag}] insights: no chart`))
    await expectText('Diabetes range', 'insights bands')
    await page.waitForTimeout(800)
    await assertNoOverflow(page, tag, 'insights', problems)
      await shot('insights')

    await page.goto(`${BASE}risk?factor=family_history_diabetes`)
    await expectText('What matters most', 'risk')
    await expectText('Moderate link', 'risk verdict')
    await page.waitForTimeout(600)
    await assertNoOverflow(page, tag, 'risk', problems)
      await shot('risk')

    // 2. Enter a key through the form (pointed at the fake Groq for the test).
    await page.goto(BASE)
    await page.evaluate((url) => {
      const raw = JSON.parse(localStorage.getItem('glucolens-settings') || '{"state":{},"version":1}')
      raw.state.groqBaseURL = url
      localStorage.setItem('glucolens-settings', JSON.stringify(raw))
    }, `http://127.0.0.1:${GROQ_PORT}`)
    await page.reload()
    await page.getByLabel('Groq API key').fill('gsk_e2e_fake')
    await page.getByRole('button', { name: 'Save and start' }).click()
    await page.getByRole('textbox', { name: 'Your question' }).waitFor({ timeout: 120000 })
    await expectText('Using your Groq key', 'key status')

    // 3. Ask: the ADK agent runs in the browser and calls "Groq" directly.
    await page.getByRole('textbox', { name: 'Your question' }).fill('Does smoking change diabetes rates in this dataset?')
    await page.keyboard.press('Enter')
    await expectText('Data behind this answer', 'answer evidence', 120000)
    await expectText('No meaningful difference', 'answer text')
    await page.locator('article svg').first().waitFor({ timeout: 30000 }).catch(() => problems.push(`[${tag}] answer: chart missing`))
    await page.waitForTimeout(800)
    await assertNoOverflow(page, tag, 'ask-answer', problems)
      await shot('ask-answer')

    // Once loaded, the agent build also serves the data tabs.
    await page.goto(`${BASE}risk?factor=age`)
    await expectText('What matters most', 'risk after agent')
    await context.close()
  }
  if (groq.requests.length === 0) problems.push('the in-browser agent never called the (fake) Groq API')
  const sys = String(groq.requests[0]?.messages?.[0]?.content ?? '')
  if (!sys.includes('GlucoLens')) problems.push('system prompt did not reach the model')
} catch (err) {
  problems.push(`run failed: ${err.stack ?? err}`)
} finally {
  await browser?.close()
  pages.close()
  groq.server.close()
}

if (problems.length > 0) {
  console.error(`\n${problems.length} problem(s):\n- ` + problems.join('\n- '))
  process.exit(1)
}
console.log(`\nStatic site checks passed. ${groq.requests.length} model calls from the browser. Screenshots in ${out}`)
