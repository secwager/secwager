import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getInstrument } from '../../grpc/registry'
import { describeLeg, fmtExpiry } from '../../lib/legDescription'

interface Props {
  initialId?: string
}

export function InstrumentLookup({ initialId = '' }: Props) {
  const [inputId, setInputId] = useState(initialId)
  const [searchId, setSearchId] = useState(initialId)

  const { data, isLoading, isError, isFetched } = useQuery({
    queryKey: ['instrument', searchId],
    queryFn: () => getInstrument(searchId),
    enabled: searchId.length > 0,
    retry: false,
  })

  function handleSearch() {
    const trimmed = inputId.trim()
    if (trimmed) setSearchId(trimmed)
  }

  const instrument = data?.instrument

  return (
    <div className="space-y-4">
      <div className="flex gap-2">
        <input
          value={inputId}
          onChange={(e) => setInputId(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
          placeholder="Paste instrument ID…"
          className="flex-1 rounded border border-gray-300 px-3 py-2 text-sm font-mono
                     focus:outline-none focus:ring-2 focus:ring-indigo-300"
          aria-label="Instrument ID"
        />
        <button
          onClick={handleSearch}
          disabled={!inputId.trim()}
          className="rounded bg-indigo-600 text-white px-4 py-2 text-sm font-medium
                     hover:bg-indigo-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          Look up
        </button>
      </div>

      {isLoading && (
        <p className="text-sm text-gray-400">Looking up instrument…</p>
      )}

      {isError && (
        <p className="text-sm text-red-500" role="alert">
          Instrument not found.
        </p>
      )}

      {instrument && (
        <div className="rounded-lg border border-gray-200 bg-white divide-y divide-gray-100">
          <div className="px-4 py-3 space-y-1">
            <p className="text-xs text-gray-400">Instrument ID</p>
            <p className="font-mono text-xs text-gray-700 break-all">{instrument.instrumentId}</p>
          </div>
          <div className="px-4 py-3 space-y-1">
            <p className="text-xs text-gray-400">Expires</p>
            <p className="text-sm text-gray-700">{fmtExpiry(instrument.expiryUnix)}</p>
          </div>
          <div className="px-4 py-3 space-y-2">
            <p className="text-xs text-gray-400">
              Legs ({instrument.legs.length})
            </p>
            {instrument.legs.map((leg, i) => (
              <div
                key={i}
                className="rounded bg-gray-50 border border-gray-200 px-3 py-2 text-sm text-gray-700"
              >
                {describeLeg(leg)}
              </div>
            ))}
          </div>
          <div className="px-4 py-3">
            <button
              disabled
              className="w-full rounded bg-indigo-600 text-white px-4 py-2 text-sm font-medium
                         opacity-40 cursor-not-allowed"
              title="Order entry coming soon"
            >
              Place Order
            </button>
          </div>
        </div>
      )}

      {isFetched && !isLoading && !isError && !instrument && searchId && (
        <p className="text-sm text-gray-400">No instrument found for that ID.</p>
      )}
    </div>
  )
}
