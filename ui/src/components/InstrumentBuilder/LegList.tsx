import { useBuilderStore } from '../../store/builderStore'
import { describeLeg } from '../../lib/legDescription'

export function LegList() {
  const { legs, removeLeg } = useBuilderStore()

  if (legs.length === 0)
    return <p className="text-sm text-gray-400 italic">No legs added yet.</p>

  return (
    <div className="space-y-2">
      <h3 className="text-sm font-semibold text-gray-600">
        Parlay legs ({legs.length})
      </h3>
      {legs.map((leg, i) => (
        <div
          key={i}
          className="flex items-center justify-between bg-white border border-gray-200
                     rounded px-3 py-2 text-sm"
        >
          <span className="text-gray-700 truncate">{describeLeg(leg)}</span>
          <button
            onClick={() => removeLeg(i)}
            className="ml-4 text-gray-400 hover:text-red-500 transition-colors flex-shrink-0"
            aria-label="Remove leg"
          >
            ✕
          </button>
        </div>
      ))}
    </div>
  )
}
