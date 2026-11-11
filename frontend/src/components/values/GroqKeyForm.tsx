import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Key01Icon } from '@hugeicons/core-free-icons'
import { Button, Icon } from '@/components/ui/primitives'
import { Select } from '@/components/ui/controls'
import { GROQ_MODELS, applyGroqSettings } from '@/lib/runtime'
import { useSettings } from '@/lib/store'

/**
 * Static site only: the visitor enters their own Groq key. It is stored in
 * this browser (localStorage) and sent only to Groq, straight from the page.
 */
export function GroqKeyForm({ onDone }: { onDone?: () => void }) {
  const qc = useQueryClient()
  const savedKey = useSettings((s) => s.groqKey)
  const savedModel = useSettings((s) => s.groqModel)
  const setGroq = useSettings((s) => s.setGroq)
  const [key, setKey] = useState(savedKey)
  const [model, setModel] = useState(savedModel || GROQ_MODELS[0].value)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const save = async () => {
    const k = key.trim()
    if (!k) return
    setSaving(true)
    setError(null)
    setGroq(k, model)
    try {
      await applyGroqSettings()
      await qc.invalidateQueries({ queryKey: ['health'] })
      onDone?.()
    } catch (err) {
      console.error('[GlucoLens] could not start the assistant', err)
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault()
        void save()
      }}
    >
      <label className="block">
        <span className="mb-1 block text-xs font-medium text-ink-2">Groq API key</span>
        <div className="flex h-10 items-center gap-2 rounded-xl bg-surface-2 px-3 ring-1 ring-line focus-within:ring-2 focus-within:ring-accent">
          <Icon icon={Key01Icon} size={16} className="text-muted" />
          <input
            type="password"
            autoComplete="off"
            spellCheck={false}
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder="gsk_…"
            aria-label="Groq API key"
            className="min-w-0 flex-1 bg-transparent font-mono text-sm text-ink outline-none placeholder:text-muted"
          />
        </div>
      </label>
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs font-medium text-ink-2">Model</span>
        <Select options={GROQ_MODELS} value={model} onChange={setModel} ariaLabel="Groq model" size="sm" menuWidth={280} />
      </div>
      <p className="text-xs leading-relaxed text-muted">
        Your key is saved only in this browser and sent only to Groq (api.groq.com), directly from this page. Get a free key at
        console.groq.com/keys.
      </p>
      {error && <p className="text-xs text-alert-text">{error}</p>}
      <Button type="submit" variant="primary" disabled={!key.trim() || saving}>
        {saving ? 'Starting the assistant…' : 'Save and start'}
      </Button>
    </form>
  )
}

/** A one-line reminder of the saved key, with change and remove actions. */
export function GroqKeyStatus({ onChange }: { onChange: () => void }) {
  const qc = useQueryClient()
  const model = useSettings((s) => s.groqModel)
  const setGroq = useSettings((s) => s.setGroq)
  return (
    <p className="flex flex-wrap items-center justify-center gap-x-3 gap-y-1 text-xs text-muted">
      <span>
        Using your Groq key ({GROQ_MODELS.find((m) => m.value === model)?.label ?? model}), saved in this browser
      </span>
      <button type="button" className="font-medium text-accent hover:underline" onClick={onChange}>
        Change
      </button>
      <button
        type="button"
        className="font-medium text-accent hover:underline"
        onClick={async () => {
          setGroq('', model)
          await applyGroqSettings().catch(() => {})
          await qc.invalidateQueries({ queryKey: ['health'] })
        }}
      >
        Remove
      </button>
    </p>
  )
}
