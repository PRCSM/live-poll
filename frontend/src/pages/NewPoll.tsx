import { useRef, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError, request } from '../lib/api'
import { serverNow } from '../lib/time'
import type { OwnerPoll, Visibility } from '../lib/types'
import { Field } from './Auth'

const DURATIONS = [
  { label: '5 min', ms: 5 * 60_000 },
  { label: '15 min', ms: 15 * 60_000 },
  { label: '1 hour', ms: 3_600_000 },
  { label: '1 day', ms: 86_400_000 },
  { label: '1 week', ms: 7 * 86_400_000 },
] as const

const VISIBILITY: { value: Visibility; label: string; hint: string }[] = [
  { value: 'always', label: 'Always', hint: 'Everyone sees results live.' },
  { value: 'after_vote', label: 'After voting', hint: 'Voters see results once they vote, so early numbers do not sway them.' },
  { value: 'after_close', label: 'After close', hint: 'Results stay hidden until the poll closes. You always see them.' },
]

const MAX_OPTIONS = 10

// datetime-local needs local time without a zone.
function toLocalInput(ms: number) {
  const d = new Date(ms)
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset())
  return d.toISOString().slice(0, 16)
}

export function NewPoll() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [question, setQuestion] = useState('')
  const nextId = useRef(2)
  const [options, setOptions] = useState([
    { id: 0, text: '' },
    { id: 1, text: '' },
  ])
  const [multiChoice, setMultiChoice] = useState(false)
  const [visibility, setVisibility] = useState<Visibility>('always')
  const [requireLogin, setRequireLogin] = useState(false)
  const [duration, setDuration] = useState<number | 'custom'>(DURATIONS[2].ms)
  const [custom, setCustom] = useState(() => toLocalInput(serverNow() + 86_400_000))
  const [fields, setFields] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const setOption = (id: number, text: string) => setOptions((o) => o.map((x) => (x.id === id ? { ...x, text } : x)))
  const removeOption = (id: number) => setOptions((o) => o.filter((x) => x.id !== id))
  const addOption = () => setOptions((o) => [...o, { id: nextId.current++, text: '' }])

  const closesAt = () => (duration === 'custom' ? new Date(custom).toISOString() : new Date(serverNow() + duration).toISOString())

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setFields({})
    const filled = options.map((o) => o.text.trim()).filter(Boolean)
    if (!question.trim()) return setFields({ question: 'Write a question.' })
    if (filled.length < 2) return setFields({ options: 'Add at least two options.' })
    if (duration === 'custom' && Number.isNaN(Date.parse(custom))) return setFields({ closesAt: 'Pick a closing time.' })

    setBusy(true)
    try {
      const { poll } = await request<{ poll: OwnerPoll }>('/api/my/polls', {
        method: 'POST',
        body: { question, options: filled, multiChoice, resultsVisibility: visibility, requireLogin, closesAt: closesAt() },
      })
      qc.invalidateQueries({ queryKey: ['my-polls'] })
      navigate(`/p/${poll.slug}`, { state: { created: true } })
    } catch (err) {
      if (err instanceof ApiError && Object.keys(err.fields).length) setFields(err.fields)
      else setError(err instanceof Error ? err.message : 'Could not create the poll.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section style={{ maxWidth: 620 }}>
      <h1>New poll</h1>
      <form className="stack" onSubmit={submit} noValidate>
        {error && (
          <div className="banner" role="alert">
            {error}
          </div>
        )}

        <Field id="question" label="Question" error={fields.question} hint={`${question.trim().length}/280`}>
          <textarea
            id="question"
            maxLength={280}
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            aria-invalid={!!fields.question}
            placeholder="Where should the team offsite be?"
            autoFocus
          />
        </Field>

        <fieldset className="field">
          <legend>Options</legend>
          <div className="stack" style={{ gap: 8 }}>
            {options.map((o, i) => {
              // Server errors index the non-empty options that were sent.
              const sentIndex = options.slice(0, i).filter((x) => x.text.trim()).length
              const err = o.text.trim() ? fields[`options.${sentIndex}`] : undefined
              return (
                <div key={o.id}>
                  <div className="option-row">
                    <span className="idx">{String(i + 1).padStart(2, '0')}</span>
                    <input
                      type="text"
                      value={o.text}
                      maxLength={100}
                      onChange={(e) => setOption(o.id, e.target.value)}
                      aria-label={`Option ${i + 1}`}
                      aria-invalid={!!err}
                    />
                    <button type="button" className="btn ghost small" onClick={() => removeOption(o.id)} disabled={options.length <= 2} aria-label={`Remove option ${i + 1}`}>
                      Remove
                    </button>
                  </div>
                  {err && (
                    <span className="error small" role="alert" style={{ marginLeft: 36, color: 'var(--accent)' }}>
                      {err}
                    </span>
                  )}
                </div>
              )
            })}
          </div>
          {fields.options && (
            <span className="error" role="alert">
              {fields.options}
            </span>
          )}
          <div>
            <button type="button" className="btn ghost small" onClick={addOption} disabled={options.length >= MAX_OPTIONS}>
              Add option
            </button>
            <span className="hint" style={{ marginLeft: 10 }}>
              {options.length}/{MAX_OPTIONS}
            </span>
          </div>
        </fieldset>

        <fieldset className="field">
          <legend>Closes after</legend>
          <div className="segmented">
            {DURATIONS.map((d) => (
              <label key={d.label}>
                <input type="radio" name="duration" checked={duration === d.ms} onChange={() => setDuration(d.ms)} />
                {d.label}
              </label>
            ))}
            <label>
              <input type="radio" name="duration" checked={duration === 'custom'} onChange={() => setDuration('custom')} />
              Pick a time
            </label>
          </div>
          {duration === 'custom' && (
            <input type="datetime-local" value={custom} onChange={(e) => setCustom(e.target.value)} aria-label="Closing time" aria-invalid={!!fields.closesAt} />
          )}
          {fields.closesAt && (
            <span className="error" role="alert">
              {fields.closesAt}
            </span>
          )}
        </fieldset>

        <fieldset className="field">
          <legend>Who sees results</legend>
          <div className="segmented">
            {VISIBILITY.map((v) => (
              <label key={v.value}>
                <input type="radio" name="visibility" checked={visibility === v.value} onChange={() => setVisibility(v.value)} />
                {v.label}
              </label>
            ))}
          </div>
          <span className="hint">{VISIBILITY.find((v) => v.value === visibility)?.hint}</span>
        </fieldset>

        <div className="stack" style={{ gap: 10 }}>
          <label className="choice">
            <input type="checkbox" checked={multiChoice} onChange={(e) => setMultiChoice(e.target.checked)} />
            <span>
              Allow picking several options
              <span className="hint" style={{ display: 'block' }}>
                Each voter still votes once.
              </span>
            </span>
          </label>
          <label className="choice">
            <input type="checkbox" checked={requireLogin} onChange={(e) => setRequireLogin(e.target.checked)} />
            <span>
              Require sign-in to vote
              <span className="hint" style={{ display: 'block' }}>
                One vote per account instead of per device. Stops repeat votes from private windows.
              </span>
            </span>
          </label>
        </div>

        <div>
          <button className="btn" disabled={busy}>
            {busy ? 'Creating…' : 'Create poll'}
          </button>
        </div>
      </form>
    </section>
  )
}
