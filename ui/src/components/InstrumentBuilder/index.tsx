import { useState } from 'react'
import { LeagueTabs } from '../LeagueTabs'
import { GameSelector } from './GameSelector'
import { LegBuilder } from './LegBuilder'
import { LegList } from './LegList'
import { SubmitBar } from './SubmitBar'

interface Props {
  onViewInstrument?(id: string): void
}

export function InstrumentBuilder({ onViewInstrument }: Props) {
  const [result, setResult] = useState<{ id: string; alreadyExisted: boolean } | null>(null)

  if (result) {
    return (
      <div className="rounded-lg border border-emerald-200 bg-emerald-50 p-6 text-center space-y-3">
        <p className="text-emerald-700 font-semibold">
          {result.alreadyExisted ? 'Instrument already exists' : 'Instrument created'}
        </p>
        <p className="font-mono text-xs text-gray-600 break-all">{result.id}</p>
        <div className="flex gap-3 justify-center">
          <button
            onClick={() => setResult(null)}
            className="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50 transition-colors"
          >
            Build another
          </button>
          <button
            onClick={() => onViewInstrument?.(result.id)}
            className="rounded border border-indigo-300 text-indigo-600 px-4 py-2 text-sm hover:bg-indigo-50 transition-colors"
          >
            View Instrument
          </button>
          {/* Order entry stub — wired once order_gateway is available */}
          <button
            disabled
            className="rounded bg-indigo-600 text-white px-4 py-2 text-sm opacity-40 cursor-not-allowed"
            title="Order entry coming soon"
          >
            Place Order
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-base font-semibold text-gray-800 mb-3">League</h2>
        <LeagueTabs />
      </div>

      <div>
        <h2 className="text-base font-semibold text-gray-800 mb-3">Game</h2>
        <GameSelector />
      </div>

      <div>
        <h2 className="text-base font-semibold text-gray-800 mb-3">Add Leg</h2>
        <LegBuilder />
      </div>

      <div>
        <LegList />
      </div>

      <SubmitBar onCreated={(id, alreadyExisted) => setResult({ id, alreadyExisted })} />
    </div>
  )
}
