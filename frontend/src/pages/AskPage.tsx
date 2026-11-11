import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  Alert02Icon,
  ArrowUp02Icon,
  ArrowUpRight01Icon,
  DatabaseIcon,
  Key01Icon,
  RefreshIcon,
  StopIcon,
  UserCircleIcon,
} from '@hugeicons/core-free-icons'
import SoftAurora from '@/components/reactbits/SoftAurora'
import SplitText from '@/components/reactbits/SplitText'
import RotatingText from '@/components/reactbits/RotatingText'
import ShinyText from '@/components/reactbits/ShinyText'
import StarBorder from '@/components/reactbits/StarBorder'
import ThoughtLine from '@/components/reactbits/ThoughtLine'
import LatticeLoader from '@/components/reactbits/LatticeLoader'
import StatusMark from '@/components/reactbits/StatusMark'
import FadeContent from '@/components/reactbits/FadeContent'
import BorderGlow from '@/components/reactbits/BorderGlow'
import { Button, Card, Icon } from '@/components/ui/primitives'
import { Switch } from '@/components/ui/controls'
import { ViewRenderer } from '@/components/views/ViewRenderer'
import { GroqKeyForm, GroqKeyStatus } from '@/components/values/GroqKeyForm'
import { STATIC } from '@/lib/runtime'
import { ApiError } from '@/lib/api'
import { streamChat } from '@/lib/sse'
import { linkForView } from '@/lib/links'
import { useColumns, useHealth, useSchema } from '@/lib/schema'
import { useChat, useSettings, useUi, type ToolStep, type Turn } from '@/lib/store'
import { fmtAny, fmtNumber } from '@/lib/format'
import { literal, useResolvedTheme } from '@/lib/theme'
import type { AssistantStatus, ChatEvent, Column, Values } from '@/lib/types'

/** Mirrors assistant problems into the browser console for debugging. */
function logChatEvent(question: string, ev: ChatEvent) {
  if (ev.type === 'error') {
    console.error('[GlucoLens] assistant error', { question, code: ev.data.code, message: ev.data.message })
  } else if (ev.type === 'tool_result' && !ev.data.ok) {
    console.warn('[GlucoLens] tool call failed (the assistant was told and may retry)', { tool: ev.data.name, error: ev.data.error })
  }
}

const GENERIC_QUESTIONS = [
  'Does smoking change diabetes rates in this dataset?',
  'Which factors are most linked to diabetes?',
  'How does HbA1c differ with a family history of diabetes?',
  'How does blood pressure change with age?',
]

function personalQuestions(values: Values, cols: Map<string, Column>): string[] {
  const out: string[] = []
  for (const name of ['hba1c', 'glucose_fasting', 'bmi', 'systolic_bp']) {
    const v = values[name]
    const c = cols.get(name)
    if (v !== undefined && c) {
      out.push(`Where does my ${c.label} of ${fmtAny(c, v)} sit compared with everyone?`)
      break
    }
  }
  out.push('What is the diabetes rate among people like me?')
  out.push('Which of my values are outside the typical range?')
  return out
}

export function AskPage() {
  const turns = useChat((s) => s.turns)
  const sessionId = useChat((s) => s.sessionId)
  const addTurn = useChat((s) => s.addTurn)
  const apply = useChat((s) => s.apply)
  const fail = useChat((s) => s.fail)
  const reset = useChat((s) => s.reset)
  const values = useSettings((s) => s.values)
  const useValues = useSettings((s) => s.useValues)
  const health = useHealth()
  const { data: schema } = useSchema()
  const cols = useColumns(schema)
  const abortRef = useRef<AbortController | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const [editKey, setEditKey] = useState(false)

  const status = health.data?.assistant
  const streaming = turns.some((t) => t.status === 'streaming')
  const hasValues = Object.keys(values).length > 0
  const suggestions = useMemo(
    () => (hasValues && useValues ? [...personalQuestions(values, cols), ...GENERIC_QUESTIONS.slice(0, 1)] : GENERIC_QUESTIONS),
    [hasValues, useValues, values, cols],
  )

  const ask = useCallback(
    async (question: string) => {
      const q = question.trim()
      if (!q || streaming) return
      const id = typeof crypto.randomUUID === 'function' ? crypto.randomUUID() : String(Date.now())
      addTurn({ id, question: q, segments: [], segmentOpen: false, steps: [], status: 'streaming', startedAt: Date.now() })
      const ctrl = new AbortController()
      abortRef.current = ctrl
      try {
        await streamChat(
          { sessionId, message: q, values, useValues },
          (ev) => {
            logChatEvent(q, ev)
            apply(id, ev)
          },
          ctrl.signal,
        )
        const t = useChat.getState().turns.find((x) => x.id === id)
        if (t?.status === 'streaming') apply(id, { type: 'done', data: {} })
      } catch (err) {
        if ((err as Error).name === 'AbortError') {
          fail(id, { code: 'stopped', message: 'Stopped.' })
        } else if (err instanceof ApiError) {
          console.error('[GlucoLens] chat request failed', { question: q, status: err.status, code: err.code, message: err.message })
          fail(id, { code: err.code, message: err.message })
          if (err.code === 'assistant_unavailable') health.refetch()
        } else {
          console.error('[GlucoLens] chat stream interrupted', { question: q, error: err })
          fail(id, { code: 'network', message: 'The answer stream was interrupted. Try again.' })
        }
      } finally {
        abortRef.current = null
      }
    },
    [streaming, addTurn, sessionId, values, useValues, apply, fail, health],
  )

  useEffect(() => {
    if (turns.length > 0) bottomRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
  }, [turns.length])

  useEffect(() => () => abortRef.current?.abort(), [])

  if (turns.length === 0) {
    return <Hero status={status} statusLoading={health.isPending} onAsk={ask} suggestions={suggestions} />
  }

  return (
    <div className="mx-auto flex max-w-[860px] flex-col px-4 pt-6">
      <div className="mb-4 flex items-center justify-between gap-3">
        <p className="text-sm text-ink-2">
          Answers use {schema ? fmtNumber(schema.dataset.rows) : ''} checkup records
          {hasValues && useValues ? ' and your values' : ''}.
        </p>
        <Button size="sm" variant="ghost" icon={RefreshIcon} onClick={() => { abortRef.current?.abort(); reset() }}>
          New chat
        </Button>
      </div>
      <div className="space-y-10 pb-6">
        {turns.map((t) => (
          <TurnView key={t.id} turn={t} onRetry={() => ask(t.question)} />
        ))}
      </div>
      <div ref={bottomRef} />
      <div className="sticky bottom-0 z-10 -mx-4 bg-gradient-to-t from-[var(--page)] via-[var(--page)] to-transparent px-4 pt-6 pb-5">
        {STATIC && (editKey || (status && !status.ready)) ? (
          <div className="card p-4">
            <GroqKeyForm onDone={() => setEditKey(false)} />
          </div>
        ) : (
          <>
            <Composer onAsk={ask} streaming={streaming} onStop={() => abortRef.current?.abort()} disabled={status ? !status.ready : false} compact />
            {STATIC && (
              <div className="mt-2">
                <GroqKeyStatus onChange={() => setEditKey(true)} />
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

function Hero({ status, statusLoading, onAsk, suggestions }: { status?: AssistantStatus; statusLoading: boolean; onAsk: (q: string) => void; suggestions: string[] }) {
  const mode = useResolvedTheme()
  const lit = literal[mode]
  const openValues = useUi((s) => s.openValues)
  const values = useSettings((s) => s.values)
  const useValues = useSettings((s) => s.useValues)
  const setUseValues = useSettings((s) => s.setUseValues)
  const count = Object.keys(values).length
  const [editingKey, setEditingKey] = useState(false)
  const notReady = (status && !status.ready) || editingKey

  return (
    <div className="relative isolate overflow-hidden">
      <div className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-[520px] opacity-80 [mask-image:linear-gradient(to_bottom,black_40%,transparent)]">
        <SoftAurora
          color1={lit.auroraA}
          color2={lit.auroraB}
          lightMode={mode === 'light'}
          brightness={mode === 'light' ? 0.9 : 1}
          speed={0.4}
          enableMouseInteraction={false}
          opacity={mode === 'light' ? 0.55 : 0.75}
          className="h-full w-full"
        />
      </div>
      <div className="mx-auto flex max-w-[820px] flex-col items-center px-4 pt-16 pb-16 text-center sm:pt-24">
        <span className="mb-5 inline-flex items-center gap-2 rounded-full bg-surface/70 px-3 py-1 text-xs font-medium text-ink-2 ring-1 ring-line backdrop-blur">
          <span className="size-1.5 rounded-full bg-ok" aria-hidden="true" />
          <ShinyText text="Powered by Google's Agent Development Kit" color={lit.shineBase} shineColor={lit.shine} speed={3} />
        </span>
        <SplitText
          text="Understand your checkup"
          tag="h1"
          className="text-[38px] leading-[1.08] font-semibold tracking-tight text-ink sm:text-[56px]"
          splitType="words, chars"
          delay={18}
          duration={0.7}
          trigger="load"
        />
        <div className="mt-4 flex flex-wrap items-center justify-center gap-x-2 text-lg text-ink-2 sm:text-xl">
          <span>Ask anything about</span>
          <RotatingText
            texts={['HbA1c', 'blood pressure', 'cholesterol', 'smoking', 'BMI', 'your risk']}
            mainClassName="overflow-hidden rounded-lg bg-accent px-2 py-0.5 font-semibold text-white"
            rotationInterval={2200}
            staggerDuration={0.02}
            splitLevelClassName="overflow-hidden"
          />
        </div>
        <p className="mt-4 max-w-xl text-[15px] text-ink-2">
          Answers come from 100,000 anonymised checkups, analysed live. The assistant shows every chart it used.
        </p>

        <div className="mt-8 w-full">
          {notReady && status ? (
            <SetupCard status={editingKey ? { ...status, ready: false, reason: 'Update the key or model the assistant uses.' } : status} onDone={() => setEditingKey(false)} />
          ) : (
            <>
              <Composer onAsk={onAsk} streaming={false} disabled={statusLoading && !status} />
              {STATIC && status?.ready && (
                <div className="mt-3">
                  <GroqKeyStatus onChange={() => setEditingKey(true)} />
                </div>
              )}
            </>
          )}
        </div>

        <div className="mt-5 flex w-full flex-wrap items-center justify-center gap-2">
          {suggestions.map((s, i) => (
            <FadeContent key={s} trigger="mount" delay={0.35 + i * 0.07} duration={0.5} blur={false}>
              <button
                type="button"
                disabled={!!notReady}
                onClick={() => onAsk(s)}
                className="rounded-full bg-surface/80 px-3.5 py-2 text-left text-[13px] text-ink ring-1 ring-line backdrop-blur transition-colors hover:bg-surface-2 disabled:opacity-50"
              >
                {s}
              </button>
            </FadeContent>
          ))}
        </div>

        <div className="mt-8 flex flex-wrap items-center justify-center gap-4 text-sm">
          {count > 0 ? (
            <Switch id="hero-use-values" checked={useValues} onChange={setUseValues} label={`Use my ${count} value${count === 1 ? '' : 's'} in answers`} />
          ) : (
            <button type="button" onClick={() => openValues(true)} className="inline-flex items-center gap-1.5 font-medium text-accent">
              <Icon icon={UserCircleIcon} size={16} /> Add your checkup values for personal answers
            </button>
          )}
        </div>

        <div className="mt-14 grid w-full gap-3 text-left sm:grid-cols-3">
          {[
            ['Ask', 'Questions answered with live analysis of the dataset, never invented numbers.'],
            ['Insights', 'Every value charted, with yours marked and clinical ranges shaded.'],
            ['Risk Explorer', 'Which factors go with an outcome, how strongly, and for people like you.'],
          ].map(([t, d], i) => (
            <Link key={t} to={i === 0 ? '/' : i === 1 ? '/insights' : '/risk'} className="card group p-4 transition-colors hover:bg-surface-2">
              <p className="flex items-center justify-between text-sm font-semibold text-ink">
                {t}
                {i > 0 && <Icon icon={ArrowUpRight01Icon} size={15} className="text-muted transition-transform group-hover:translate-x-0.5" />}
              </p>
              <p className="mt-1 text-[13px] text-ink-2">{d}</p>
            </Link>
          ))}
        </div>
      </div>
    </div>
  )
}

function SetupCard({ status, onDone }: { status: AssistantStatus; onDone?: () => void }) {
  const mode = useResolvedTheme()
  const lit = literal[mode]
  return (
    <BorderGlow backgroundColor={lit.surface} glowColor={lit.glowHsl} colors={[...lit.glowColors]} borderRadius={20} className="w-full text-left">
      <div className="p-5 sm:p-6">
        <div className="flex items-start gap-3">
          <span className="grid size-10 shrink-0 place-items-center rounded-xl bg-accent-soft text-accent">
            <Icon icon={Key01Icon} size={20} />
          </span>
          <div className="min-w-0">
            <p className="text-[15px] font-semibold text-ink">The assistant needs a Groq API key</p>
            <p className="mt-1 text-sm text-ink-2">{status.reason}</p>
          </div>
        </div>
        {STATIC ? (
          <div className="mt-4">
            <GroqKeyForm onDone={onDone} />
          </div>
        ) : (
        <ol className="mt-4 space-y-2 text-sm text-ink-2">
          <li>
            1. Get a key at <span className="font-medium text-ink">console.groq.com/keys</span>.
          </li>
          <li>
            2. Open <code className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-[12.5px] text-ink">backend/.env</code> and set{' '}
            <code className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-[12.5px] text-ink">GROQ_API_KEY=your-key</code>.
          </li>
          <li>3. Restart the server. This card turns into the search box.</li>
        </ol>
        )}
        <p className="mt-4 flex flex-wrap gap-x-4 gap-y-1 text-sm">
          <span className="text-ink-2">Meanwhile everything else works:</span>
          <Link to="/insights" className="font-medium text-accent">Insights</Link>
          <Link to="/risk" className="font-medium text-accent">Risk Explorer</Link>
        </p>
      </div>
    </BorderGlow>
  )
}

function Composer({ onAsk, streaming, onStop, disabled, compact }: { onAsk: (q: string) => void; streaming: boolean; onStop?: () => void; disabled?: boolean; compact?: boolean }) {
  const mode = useResolvedTheme()
  const lit = literal[mode]
  const [text, setText] = useState('')
  const ref = useRef<HTMLTextAreaElement>(null)
  const submit = () => {
    if (!text.trim() || streaming || disabled) return
    onAsk(text)
    setText('')
  }
  useEffect(() => {
    const el = ref.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(160, el.scrollHeight)}px`
  }, [text])

  const box = (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
      className="flex w-full items-end gap-2 rounded-[22px] bg-surface p-2 pl-4 shadow-[var(--shadow-card)] ring-1 ring-line focus-within:ring-2 focus-within:ring-accent"
    >
      <textarea
        ref={ref}
        rows={1}
        value={text}
        maxLength={2000}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault()
            submit()
          }
        }}
        placeholder={compact ? 'Ask a follow-up…' : 'Ask about a value, a risk factor, or yourself…'}
        aria-label="Your question"
        disabled={disabled}
        className="min-h-[40px] flex-1 resize-none bg-transparent py-2.5 text-[15px] text-ink outline-none placeholder:text-muted disabled:opacity-60"
      />
      {streaming ? (
        <button type="button" onClick={onStop} aria-label="Stop" className="grid size-10 shrink-0 place-items-center rounded-full bg-surface-3 text-ink">
          <Icon icon={StopIcon} size={18} />
        </button>
      ) : (
        <button
          type="submit"
          aria-label="Ask"
          disabled={!text.trim() || disabled}
          className="grid size-10 shrink-0 place-items-center rounded-full bg-accent text-white transition-colors hover:bg-accent-hover disabled:bg-surface-3 disabled:text-muted"
        >
          <Icon icon={ArrowUp02Icon} size={19} strokeWidth={2.2} />
        </button>
      )}
    </form>
  )
  if (compact) return box
  return (
    <StarBorder as="div" theme={mode} color={lit.accent} backgroundColor="transparent" radius={24} thickness={1.5} duration={7} className="block w-full p-0! text-left">
      {box}
    </StarBorder>
  )
}

function TurnView({ turn, onRetry }: { turn: Turn; onRetry: () => void }) {
  const { data: schema } = useSchema()
  const working = turn.status === 'streaming'
  const running = turn.steps.find((s) => s.status === 'running')
  const answer = turn.segments.join('\n\n').trim()
  const evidence = turn.steps.filter((s) => s.status === 'done' && s.view)
  const failedSteps = turn.steps.filter((s) => s.status === 'failed' && s.error)

  return (
    <article className="min-w-0 space-y-4" aria-busy={working}>
      <div className="flex justify-end">
        <p className="max-w-[85%] rounded-[20px] rounded-br-md bg-accent px-4 py-2.5 text-[15px] text-white">{turn.question}</p>
      </div>

      <div className="space-y-3">
        {turn.steps.length > 0 ? (
          <ThoughtLine
            label={running ? `${running.label}…` : 'Writing the answer…'}
            doneLabel="Worked for"
            steps={turn.steps.map((s) => (s.status === 'failed' ? `${s.label} (retried)` : s.label))}
            working={working}
            elapsed={turn.elapsedMs !== undefined ? turn.elapsedMs / 1000 : undefined}
            color="var(--ink-2)"
            glyphColor="var(--accent)"
            fontSize={13.5}
          />
        ) : working && !answer ? (
          <LatticeLoader label="Thinking" status="working" pattern="spiral" color="var(--accent)" fontSize={13.5} />
        ) : null}

        {answer && (
          <div className="answer">
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ a: ({ href, children }) => <a href={href} target="_blank" rel="noreferrer noopener">{children}</a> }}>
              {answer}
            </ReactMarkdown>
          </div>
        )}

        {turn.status === 'error' && turn.error && turn.error.code !== 'stopped' && (
          <div role="alert" className="flex items-start gap-3 rounded-2xl bg-surface-2 p-4 text-sm ring-1 ring-line">
            <Icon icon={Alert02Icon} size={18} className="mt-0.5 shrink-0 text-alert" />
            <div className="min-w-0 flex-1">
              <p className="font-medium text-ink">{errorTitle(turn.error.code)}</p>
              <p className="mt-0.5 text-ink-2">{turn.error.message}</p>
            </div>
            <Button size="sm" onClick={onRetry} icon={RefreshIcon}>
              Retry
            </Button>
          </div>
        )}
        {turn.status === 'error' && turn.error?.code === 'stopped' && <p className="text-xs text-muted">Stopped.</p>}
      </div>

      {evidence.length > 0 && (
        <div className="space-y-3">
          <p className="flex items-center gap-1.5 text-xs font-semibold tracking-wide text-muted uppercase">
            <Icon icon={DatabaseIcon} size={14} /> Data behind this answer
          </p>
          {/* min-w-0 lets cards shrink below their content (long titles, measured charts). */}
          <div className="grid min-w-0 gap-3 [&>*]:min-w-0">
            {evidence.map((s) => (
              <EvidenceCard key={s.id} step={s} schemaLink={s.view ? linkForView(s.view, schema) : undefined} />
            ))}
          </div>
        </div>
      )}
      {!working && failedSteps.length > 0 && evidence.length === 0 && turn.status !== 'error' && (
        <p className="text-xs text-muted">Some lookups failed and were retried by the assistant.</p>
      )}
    </article>
  )
}

function EvidenceCard({ step, schemaLink }: { step: ToolStep; schemaLink?: { to: string; label: string } }) {
  return (
    <FadeContent trigger="mount" duration={0.45} blur={false}>
      <Card className="min-w-0 p-4 sm:p-5">
        <div className="mb-3 flex items-center justify-between gap-3">
          <p className="flex min-w-0 items-center gap-2 text-sm font-medium text-ink">
            <StatusMark status="done" size={16} doneColor="var(--ok)" />
            <span className="truncate">{step.label}</span>
          </p>
          {schemaLink && (
            <Link to={schemaLink.to} className="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-accent hover:underline">
              {schemaLink.label} <Icon icon={ArrowUpRight01Icon} size={13} />
            </Link>
          )}
        </div>
        {step.view && <ViewRenderer view={step.view} />}
        {step.result && (
          <details className="mt-3 text-xs text-ink-2">
            <summary className="cursor-pointer text-muted select-none hover:text-ink">What the assistant received</summary>
            <pre className="scroll-thin mt-2 max-h-60 overflow-auto rounded-xl bg-surface-2 p-3 font-mono text-[11.5px] leading-relaxed whitespace-pre-wrap text-ink-2 ring-1 ring-line">
              {JSON.stringify(step.result, null, 2)}
            </pre>
          </details>
        )}
      </Card>
    </FadeContent>
  )
}

function errorTitle(code: string): string {
  switch (code) {
    case 'rate_limited':
      return 'Groq rate limit reached'
    case 'invalid_api_key':
      return 'The Groq API key was rejected'
    case 'assistant_unavailable':
      return 'The assistant is not set up'
    case 'api_unreachable':
      return 'Can’t reach the GlucoLens API'
    case 'model_not_found':
      return 'Model not available'
    case 'tool_call_rejected':
      return 'The assistant could not form a valid data request'
    default:
      return 'The answer could not be completed'
  }
}

