import { League } from '../grpc/registry'
import { useBuilderStore } from '../store/builderStore'

const LEAGUES: { league: League; label: string; abbr: string }[] = [
  { league: League.MLB,     label: 'MLB',     abbr: 'MLB' },
  { league: League.NFL,     label: 'NFL',     abbr: 'NFL' },
  { league: League.EPL,     label: 'Premier League', abbr: 'EPL' },
  { league: League.LA_LIGA, label: 'La Liga', abbr: 'La Liga' },
  { league: League.MLS,     label: 'MLS',     abbr: 'MLS' },
]

export function LeagueTabs() {
  const { selectedLeague, setLeague } = useBuilderStore()

  return (
    <div className="flex border-b border-gray-200">
      {LEAGUES.map(({ league, label, abbr }) => {
        const active = league === selectedLeague
        return (
          <button
            key={league}
            onClick={() => setLeague(league)}
            className={[
              'px-5 py-3 text-sm font-medium transition-colors',
              active
                ? 'border-b-2 border-indigo-600 text-indigo-600'
                : 'text-gray-500 hover:text-gray-700',
            ].join(' ')}
            title={label}
          >
            {abbr}
          </button>
        )
      })}
    </div>
  )
}
