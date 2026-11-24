import type { Option } from '../lib/types'

type Props = {
  options: Option[]
  counts: Record<string, number>
  total: number
  mine?: string[]
  big?: boolean
}

export function ResultBars({ options, counts, total, mine = [], big = false }: Props) {
  const max = Math.max(0, ...options.map((o) => counts[o.id] ?? 0))
  // Bars are scaled to the share of voters, so multi-choice polls can exceed 100% in sum.
  const pct = (n: number) => (total === 0 ? 0 : Math.round((n / total) * 100))

  return (
    <ol className={big ? 'results big' : 'results'} aria-label="Results">
      {options.map((o) => {
        const n = counts[o.id] ?? 0
        const leading = n > 0 && n === max
        const cls = ['result', leading && 'leading', mine.includes(o.id) && 'mine'].filter(Boolean).join(' ')
        return (
          <li key={o.id} className={cls}>
            <div className="label">
              <span className="text">{o.text}</span>
              <span className="num">
                {pct(n)}% · {n}
              </span>
            </div>
            <div className="track" aria-hidden="true">
              <div className="fill" style={{ width: `${pct(n)}%` }} />
            </div>
          </li>
        )
      })}
    </ol>
  )
}
