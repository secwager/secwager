import type { Instrument } from '../../gen/registry/registry'

interface Props {
  instrument: Instrument
  onEdit(instrument: Instrument): void
}

function fmtExpiry(unix: number): string {
  return new Date(unix * 1000).toLocaleDateString(undefined, {
    month: 'short', day: 'numeric', year: 'numeric',
  })
}

export function InstrumentRow({ instrument, onEdit }: Props) {
  // has_orders is a deferred backend field; treat as immutable for now if legs are non-empty
  // and the server would return it. Until the field is added we always show Edit.
  const isDraft = true

  return (
    <div className="flex items-center justify-between rounded border border-gray-200 bg-white px-4 py-3">
      <div className="space-y-0.5">
        <p className="font-mono text-xs text-gray-500 truncate max-w-xs">
          {instrument.instrumentId}
        </p>
        <p className="text-xs text-gray-400">
          {instrument.legs.length} leg{instrument.legs.length !== 1 ? 's' : ''} &middot; expires {fmtExpiry(instrument.expiryUnix)}
        </p>
      </div>
      <div className="flex items-center gap-3 flex-shrink-0">
        <span
          className={[
            'text-xs font-medium px-2 py-0.5 rounded-full',
            isDraft
              ? 'bg-yellow-100 text-yellow-700'
              : 'bg-green-100 text-green-700',
          ].join(' ')}
        >
          {isDraft ? 'Draft' : 'Active'}
        </span>
        {isDraft && (
          <button
            onClick={() => onEdit(instrument)}
            className="text-xs text-indigo-600 hover:text-indigo-800 font-medium transition-colors"
          >
            Edit
          </button>
        )}
      </div>
    </div>
  )
}
