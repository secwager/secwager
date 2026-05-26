import { useState, useEffect } from 'react'
import { useBuilderStore } from '../../store/builderStore'
import { useAuthStore } from '../../store/authStore'
import { createInstrument } from '../../grpc/registry'

interface Props {
  onCreated(instrumentId: string, alreadyExisted: boolean): void
}

export function SubmitBar({ onCreated }: Props) {
  const { legs, reset } = useBuilderStore()
  const { jwt } = useAuthStore()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => { if (jwt) setError(null) }, [jwt])

  async function handleSubmit() {
    if (!jwt) { setError('Sign in to create instruments.'); return }
    if (legs.length === 0) { setError('Add at least one leg.'); return }
    setError(null)
    setLoading(true)
    try {
      const resp = await createInstrument(legs, jwt)
      reset()
      onCreated(resp.instrumentId, resp.alreadyExisted)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to create instrument.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-2">
      {error && <p className="text-sm text-red-500">{error}</p>}
      <button
        onClick={handleSubmit}
        disabled={loading || legs.length === 0}
        className="w-full rounded bg-emerald-600 text-white px-4 py-2.5 text-sm font-semibold
                   hover:bg-emerald-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
      >
        {loading ? 'Creating…' : 'Create Instrument'}
      </button>
    </div>
  )
}
