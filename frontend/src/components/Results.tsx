import { useEffect, useRef, useState, type CSSProperties } from 'react'
import type { Option } from '../lib/types'

type Props = {
  options: Option[]
  counts: Record<string, number>
  total: number
  mine?: string[]
  big?: boolean
  mode?: 'pct' | 'count'
}

/**
 * Hairline bar chart in the style of the reference's water tile. Bars are scaled to
 * the share of voters, so multi-choice polls can add up to more than 100%.
 */
export function ResultBars({ options, counts, total, mine = [], big = false, mode = 'pct' }: Props) {
  const max = Math.max(0, ...options.map((o) => counts[o.id] ?? 0))
  const pct = (n: number) => (total === 0 ? 0 : (n / total) * 100)

  // Options whose count just went up get a brief pulse on their bar's end dot.
  const prev = useRef(counts)
  const [bumped, setBumped] = useState<string[]>([])
  useEffect(() => {
    const changed = options.filter((o) => (counts[o.id] ?? 0) > (prev.current[o.id] ?? 0)).map((o) => o.id)
    prev.current = counts
    if (!changed.length) return
    setBumped(changed)
    const t = setTimeout(() => setBumped([]), 650)
    return () => clearTimeout(t)
  }, [counts, options])

  return (
    <div>
      <ol className={big ? 'chart big' : 'chart'} aria-label="Results">
        {options.map((o) => {
          const n = counts[o.id] ?? 0
          const cls = ['chart-row', n > 0 && n === max && 'lead', bumped.includes(o.id) && 'bump'].filter(Boolean).join(' ')
          return (
            <li key={o.id} className={cls}>
              <span className="name">
                {o.text}
                {mine.includes(o.id) && <span className="mine">your vote</span>}
              </span>
              <span className="track">
                <span className="bar" style={{ '--w': `${pct(n)}%` } as CSSProperties} />
              </span>
              <span className="val">{mode === 'pct' ? `${Math.round(pct(n))}%` : n.toLocaleString()}</span>
            </li>
          )
        })}
      </ol>
      <div className="chart-axis" aria-hidden="true">
        <span className="label-mono">{mode === 'pct' ? 'Share' : 'Votes'}</span>
        <span className="ticks label-mono">
          <span>0</span>
          <span>25</span>
          <span>50</span>
          <span>75</span>
          <span>100%</span>
        </span>
        <span />
      </div>
    </div>
  )
}
