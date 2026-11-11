// A scripted stand-in for Groq's OpenAI-compatible Chat Completions API, so
// the end-to-end test can drive the real Go server, ADK agent and UI without
// an API key. It answers each question with one tool call, then a streamed
// markdown answer.
import http from 'node:http'

function chunk(res, id, delta, finish = null) {
  const body = { id, object: 'chat.completion.chunk', created: 1700000000, model: 'fake-groq', choices: [{ index: 0, delta, finish_reason: finish }] }
  res.write(`data: ${JSON.stringify(body)}\n\n`)
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

function pickTool(question) {
  const q = question.toLowerCase()
  if (q.includes('smok')) return { name: 'analyze_risk', args: { outcome: 'diagnosed_diabetes', factor: 'smoking_status', stratify_by: 'age' } }
  if (q.includes('like me')) return { name: 'find_similar_people', args: {} }
  if (q.includes('my ')) return { name: 'get_percentile', args: {} }
  if (q.includes('factor')) return { name: 'rank_risk_factors', args: { outcome: 'diagnosed_diabetes' } }
  return { name: 'compare_groups', args: { metric: 'hba1c', group_by: 'family_history_diabetes' } }
}

export function startFakeGroq(port) {
  const requests = []
  const server = http.createServer(async (req, res) => {
    // The static site calls Groq straight from the browser, so answer CORS
    // the way api.groq.com does.
    res.setHeader('Access-Control-Allow-Origin', '*')
    res.setHeader('Access-Control-Allow-Headers', req.headers['access-control-request-headers'] || '*')
    res.setHeader('Access-Control-Allow-Methods', 'POST, OPTIONS')
    if (req.method === 'OPTIONS') {
      res.writeHead(204).end()
      return
    }
    if (req.method !== 'POST' || !req.url.endsWith('/chat/completions')) {
      res.writeHead(404).end()
      return
    }
    let raw = ''
    for await (const part of req) raw += part
    const body = JSON.parse(raw)
    requests.push(body)
    const messages = body.messages ?? []
    const last = messages[messages.length - 1]
    const id = `chatcmpl-${requests.length}`
    res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' })

    if (last?.role === 'user') {
      const question = typeof last.content === 'string' ? last.content : JSON.stringify(last.content)
      const tool = pickTool(question)
      const args = JSON.stringify(tool.args)
      chunk(res, id, { role: 'assistant', tool_calls: [{ index: 0, id: `call_${requests.length}`, type: 'function', function: { name: tool.name, arguments: args.slice(0, 8) } }] })
      await sleep(150)
      chunk(res, id, { tool_calls: [{ index: 0, function: { arguments: args.slice(8) } }] })
      chunk(res, id, {}, 'tool_calls')
    } else {
      const toolMsg = messages.filter((m) => m.role === 'tool').pop()
      const result = String(toolMsg?.content ?? '')
      const verdict = (result.match(/"(?:verdict|summary)":"([^"]+)"/) ?? [])[1] ?? 'The figures are shown below.'
      const parts = [
        'In this dataset, ',
        `**${verdict.replace(/\.$/, '')}**.\n\n`,
        '- The chart below shows each group with its confidence interval.\n',
        '- A link in this data does not prove cause.\n\n',
        'Talk your own results through with a clinician.',
      ]
      for (const [i, p] of parts.entries()) {
        chunk(res, id, i === 0 ? { role: 'assistant', content: p } : { content: p })
        await sleep(120)
      }
      chunk(res, id, {}, 'stop')
    }
    res.write(`data: ${JSON.stringify({ id, object: 'chat.completion.chunk', created: 1700000000, model: 'fake-groq', choices: [], usage: { prompt_tokens: 1000, completion_tokens: 50, total_tokens: 1050 } })}\n\n`)
    res.write('data: [DONE]\n\n')
    res.end()
  })
  return new Promise((resolve) => server.listen(port, '127.0.0.1', () => resolve({ server, requests })))
}
