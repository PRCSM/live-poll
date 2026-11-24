import { useRef, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError, request } from '../lib/api'
import { formatRemaining, serverNow } from '../lib/time'
import type { OwnerPoll, Visibility } from '../lib/types'
import { Card, Icon, Pill, Ruler, Segmented, Switch } from '../components/ui'

type Duration = '5m' | '15m' | '1h' | '1d' | '1w' | 'custom'

const DURATIONS: { value: Duration; label: string; ms: number }[] = [
  { value: '5m', label: '5 min', ms: 5 * 60_000 },
  { value: '15m', label: '15 min', ms: 15 * 60_000 },
  { value: '1h', label: '1 hr', ms: 3_600_000 },
  { value: '1d', label: '1 day', ms: 86_400_000 },
  { value: '1w', label: '1 wk', ms: 7 * 86_400_000 },
  { value: 'custom', label: 'Pick', ms: 0 },
]

const VISIBILITY: { value: Visibility; label: string; hint: string }[] = [
  { value: 'always', label: 'Always', hint: 'Everyone sees results live.' },
  { value: 'after_vote', label: 'After vote', hint: 'Voters see results once they vote, so early numbers do not sway them.' },
  { value: 'after_close', label: 'After close', hint: 'Hidden until the poll closes. You always see them.' },
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
  const nextId = useRef(2)
  const [question, setQuestion] = useState('')
  const [options, setOptions] = useState([
    { id: 0, text: '' },
    { id: 1, text: '' },
  ])
  const [multiChoice, setMultiChoice] = useState(false)
  const [visibility, setVisibility] = useState<Visibility>('always')
  const [requireLogin, setRequireLogin] = useState(false)
  const [duration, setDuration] = useState<Duration>('1h')
  const [custom, setCustom] = useState(() => toLocalInput(serverNow() + 86_400_000))
  const [fields, setFields] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const setOption = (id: number, text: string) => setOptions((o) => o.map((x) => (x.id === id ? { ...x, text } : x)))
  const removeOption = (id: number) => setOptions((o) => o.filter((x) => x.id !== id))
  const addOption = () => setOptions((o) => [...o, { id: nextId.current++, text: '' }])

  const presetMs = DURATIONS.find((d) => d.value === duration)?.ms ?? 0
  const closesAt = () => (duration === 'custom' ? new Date(custom).toISOString() : new Date(serverNow() + presetMs).toISOString())
  const filled = options.map((o) => o.text.trim()).filter(Boolean)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setFields({})
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
      qc.invalidateQueries({ queryKey: ['my-polls-summary'] })
      navigate(`/p/${poll.slug}`, { state: { created: true } })
    } catch (err) {
      if (err instanceof ApiError && Object.keys(err.fields).length) setFields(err.fields)
      else setError(err instanceof Error ? err.message : 'Could not create the poll.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <header className="page-head">
        <div>
          <span className="label-mono">Create</span>
          <h1>New poll</h1>
        </div>
      </header>
      <form className="bento" onSubmit={submit} noValidate>
        <Card i={0} className="span-2 light">
          <div className="stack">
            {error && (
              <div className="banner" role="alert">
                {error}
              </div>
            )}
            <div className="field">
              <label htmlFor="question">Question</label>
              <textarea
                id="question"
                className="question-input"
                maxLength={280}
                value={question}
                onChange={(e) => setQuestion(e.target.value)}
                aria-invalid={!!fields.question}
                placeholder="Where should the team offsite be?"
                autoFocus
              />
              <span className="hint">{question.trim().length}/280</span>
              {fields.question && (
                <span className="error" role="alert">
                  {fields.question}
                </span>
              )}
            </div>

            <fieldset className="field">
              <legend className="field-label">
                Options · {options.length}/{MAX_OPTIONS}
              </legend>
              <div className="stack" style={{ gap: 6, marginTop: 6 }}>
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
                        <button type="button" className="icon-btn" onClick={() => removeOption(o.id)} disabled={options.length <= 2} aria-label={`Remove option ${i + 1}`}>
                          <Icon name="close" />
                        </button>
                      </div>
                      {err && (
                        <span className="error" role="alert" style={{ marginLeft: 32 }}>
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
              <div style={{ marginTop: 6 }}>
                <button type="button" className="btn chip ghost" onClick={addOption} disabled={options.length >= MAX_OPTIONS}>
                  + Add option
                </button>
              </div>
            </fieldset>
          </div>
        </Card>

        <Card i={1} className="sunrise" style={{ display: 'grid', alignContent: 'space-between', gap: 18 }}>
          <div className="spread">
            <span className="label-mono">Preview</span>
            <Pill tone="green">
              <span className="dot" />
              Open
            </Pill>
          </div>
          <div>
            <div style={{ fontSize: 20, letterSpacing: '-0.03em', marginBottom: 14, overflowWrap: 'anywhere' }}>
              {question.trim() || <span className="muted">Your question</span>}
            </div>
            <div className="stack" style={{ gap: 6 }}>
              {(filled.length ? filled : ['Option one', 'Option two']).map((t, i) => (
                <div key={i} className="small" style={{ padding: '8px 12px', background: 'rgba(255,255,255,.7)', borderRadius: 10, overflowWrap: 'anywhere' }}>
                  {t}
                </div>
              ))}
            </div>
          </div>
          <Ruler fraction={0.03} ticks={30} tag={duration === 'custom' ? 'custom' : formatRemaining(presetMs)} />
        </Card>

        <Card i={2} className="span-2">
          <div className="stack">
            <div className="field">
              <span className="field-label">Closes after</span>
              <div className="row">
                <Segmented<Duration> label="Closes after" value={duration} onChange={setDuration} options={DURATIONS} />
                {duration === 'custom' && (
                  <input
                    type="datetime-local"
                    value={custom}
                    onChange={(e) => setCustom(e.target.value)}
                    aria-label="Closing time"
                    aria-invalid={!!fields.closesAt}
                    style={{ width: 'auto' }}
                  />
                )}
              </div>
              {fields.closesAt && (
                <span className="error" role="alert">
                  {fields.closesAt}
                </span>
              )}
            </div>
            <div className="field">
              <span className="field-label">Who sees results</span>
              <div>
                <Segmented<Visibility> label="Who sees results" value={visibility} onChange={setVisibility} options={VISIBILITY} />
              </div>
              <span className="hint">{VISIBILITY.find((v) => v.value === visibility)?.hint}</span>
            </div>
          </div>
        </Card>

        <Card i={3}>
          <div className="stack">
            <Switch checked={multiChoice} onChange={setMultiChoice} title="Multiple choice" hint="Voters can pick several options." />
            <Switch checked={requireLogin} onChange={setRequireLogin} title="Require sign-in" hint="One vote per account, not per device." />
            <button className="btn" disabled={busy} style={{ marginTop: 6 }}>
              {busy ? 'Creating…' : 'Create poll'}
            </button>
          </div>
        </Card>
      </form>
    </>
  )
}
