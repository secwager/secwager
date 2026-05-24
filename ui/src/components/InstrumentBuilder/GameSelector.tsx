import { useBuilderStore } from '../../store/builderStore'
import { useGames } from '../../hooks/useGames'

function fmtUnix(unix: number): string {
  return new Date(unix * 1000).toLocaleString(undefined, {
    month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit',
  })
}

export function GameSelector() {
  const { selectedLeague, selectedGameId, setGameId } = useBuilderStore()
  const { data, isLoading, isError } = useGames(selectedLeague)

  if (isLoading) return <p className="text-sm text-gray-400 py-4">Loading games…</p>
  if (isError)   return <p className="text-sm text-red-500 py-4">Failed to load games.</p>

  const games = data?.games ?? []
  if (games.length === 0)
    return <p className="text-sm text-gray-400 py-4">No upcoming games found.</p>

  return (
    <div className="space-y-1 max-h-64 overflow-y-auto pr-1">
      {games.map((g) => (
        <button
          key={g.id}
          onClick={() => setGameId(g.id)}
          className={[
            'w-full text-left px-4 py-2 rounded text-sm transition-colors',
            g.id === selectedGameId
              ? 'bg-indigo-50 border border-indigo-300 text-indigo-800'
              : 'bg-gray-50 hover:bg-gray-100 text-gray-700',
          ].join(' ')}
        >
          <span className="font-medium">{g.awayTeamId}</span>
          <span className="mx-2 text-gray-400">@</span>
          <span className="font-medium">{g.homeTeamId}</span>
          <span className="ml-3 text-gray-400 text-xs">{fmtUnix(g.scheduledUnix)}</span>
        </button>
      ))}
    </div>
  )
}
