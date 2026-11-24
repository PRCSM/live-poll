import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import { Link } from 'react-router-dom'

/** Animates between numbers with an ease-out curve, like the readouts in the reference. */
export function Counter({ value, className }: { value: number; className?: string }) {
  const [shown, setShown] = useState(value)
  const from = useRef(value)

  useEffect(() => {
    const start = from.current
    if (start === value || matchMedia('(prefers-reduced-motion: reduce)').matches) {
      from.current = value
      setShown(value)
      return
    }
    const t0 = performance.now()
    const duration = 650
    let raf = 0
    const tick = (t: number) => {
      const p = Math.min(1, (t - t0) / duration)
      const eased = 1 - Math.pow(1 - p, 3)
      const v = Math.round(start + (value - start) * eased)
      from.current = v
      setShown(v)
      if (p < 1) raf = requestAnimationFrame(tick)
    }
    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [value])

  return <span className={className}>{shown.toLocaleString()}</span>
}

type SegOption<T extends string> = { value: T; label: string }

/** Pill segmented control whose knob slides to the selected option. */
export function Segmented<T extends string>({
  options,
  value,
  onChange,
  label,
}: {
  options: SegOption<T>[]
  value: T
  onChange: (v: T) => void
  label: string
}) {
  const refs = useRef<Record<string, HTMLButtonElement | null>>({})
  const [knob, setKnob] = useState({ x: 0, w: 0 })

  useLayoutEffect(() => {
    const el = refs.current[value]
    if (el) setKnob({ x: el.offsetLeft, w: el.offsetWidth })
  }, [value, options.length])

  return (
    <div className="seg" role="group" aria-label={label}>
      <span className="knob" style={{ transform: `translateX(${knob.x}px)`, width: knob.w }} aria-hidden="true" />
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          ref={(el) => {
            refs.current[o.value] = el
          }}
          aria-pressed={o.value === value}
          onClick={() => onChange(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

/**
 * Tick strip from the thermostat tile. `fraction` of the ticks are lit, the tick at the
 * boundary is the "now" marker, and an optional tag sits underneath it.
 */
export function Ruler({ fraction, ticks = 44, tag }: { fraction: number; ticks?: number; tag?: ReactNode }) {
  const f = Math.max(0, Math.min(1, fraction))
  const nowIdx = Math.min(ticks - 1, Math.round(f * (ticks - 1)))
  return (
    <div className="ruler" aria-hidden="true">
      {Array.from({ length: ticks }, (_, k) => (
        <i key={k} className={k === nowIdx ? 'now' : k < nowIdx ? 'on' : ''} style={{ '--k': k } as CSSProperties} />
      ))}
      {tag && (
        <span className="tag" style={{ left: `${(nowIdx / (ticks - 1)) * 100}%` }}>
          {tag}
        </span>
      )}
    </div>
  )
}

export function Pill({ tone, live, children }: { tone?: 'green' | 'red' | 'amber'; live?: boolean; children: ReactNode }) {
  return <span className={['pill', tone, live && 'live'].filter(Boolean).join(' ')}>{children}</span>
}

export function Card({
  i = 0,
  className = '',
  children,
  expandTo,
  expandLabel,
  style,
}: {
  i?: number
  className?: string
  children: ReactNode
  expandTo?: string
  expandLabel?: string
  style?: CSSProperties
}) {
  return (
    <section className={`card ${className}`} style={{ '--i': i, ...style } as CSSProperties}>
      {children}
      {expandTo && (
        <Link className="expand" to={expandTo} target="_blank" rel="noopener" aria-label={expandLabel ?? 'Open full screen'}>
          <Icon name="expand" />
        </Link>
      )}
    </section>
  )
}

export function Switch({
  checked,
  onChange,
  title,
  hint,
}: {
  checked: boolean
  onChange: (v: boolean) => void
  title: string
  hint?: string
}) {
  return (
    <label className="switch">
      <span>
        <span style={{ display: 'block', fontSize: 14 }}>{title}</span>
        {hint && <span className="small muted">{hint}</span>}
      </span>
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span className="track" aria-hidden="true" />
    </label>
  )
}

const paths: Record<string, ReactNode> = {
  home: <path d="M4 11.5 12 5l8 6.5V19a1 1 0 0 1-1 1h-4.5v-5h-5v5H5a1 1 0 0 1-1-1z" />,
  polls: (
    <>
      <path d="M5 19V11" />
      <path d="M12 19V5" />
      <path d="M19 19v-5" />
    </>
  ),
  plus: (
    <>
      <path d="M12 5v14" />
      <path d="M5 12h14" />
    </>
  ),
  screen: (
    <>
      <rect x="3.5" y="5" width="17" height="11" rx="2" />
      <path d="M9 20h6" />
      <path d="M12 16v4" />
    </>
  ),
  user: (
    <>
      <circle cx="12" cy="8.5" r="3.5" />
      <path d="M5 20c1.2-3.4 4-5 7-5s5.8 1.6 7 5" />
    </>
  ),
  out: (
    <>
      <path d="M10 5H6a1 1 0 0 0-1 1v12a1 1 0 0 0 1 1h4" />
      <path d="M14 8l4 4-4 4" />
      <path d="M18 12H9" />
    </>
  ),
  expand: (
    <>
      <path d="M14 5h5v5" />
      <path d="M10 19H5v-5" />
      <path d="M19 5l-6 6" />
      <path d="M5 19l6-6" />
    </>
  ),
  close: (
    <>
      <path d="M6 6l12 12" />
      <path d="M18 6 6 18" />
    </>
  ),
  copy: (
    <>
      <rect x="8" y="8" width="11" height="11" rx="2" />
      <path d="M16 8V6a1 1 0 0 0-1-1H6a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h2" />
    </>
  ),
}

export function Icon({ name }: { name: keyof typeof paths | string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      {paths[name]}
    </svg>
  )
}
