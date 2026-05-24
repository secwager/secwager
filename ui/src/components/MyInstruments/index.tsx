import { useQuery } from '@tanstack/react-query'
import { useAuthStore } from '../../store/authStore'
import { useBuilderStore } from '../../store/builderStore'
import { listInstruments } from '../../grpc/registry'
import { League, type Instrument } from '../../gen/registry/registry'
import { InstrumentRow } from './InstrumentRow'

interface Props {
  onEditInstrument(): void
}

export function MyInstruments({ onEditInstrument }: Props) {
  const { userId } = useAuthStore()
  const { loadLegs } = useBuilderStore()

  const { data, isLoading } = useQuery({
    queryKey: ['myInstruments', userId],
    queryFn: () => listInstruments(userId!, League.LEAGUE_UNSPECIFIED, false),
    enabled: userId !== null,
  })

  function handleEdit(instrument: Instrument) {
    loadLegs(instrument.legs)
    onEditInstrument()
  }

  if (!userId) return null
  if (isLoading) return <p className="text-sm text-gray-400">Loading instruments…</p>

  const instruments = data?.instruments ?? []
  if (instruments.length === 0)
    return <p className="text-sm text-gray-400 italic">No instruments yet.</p>

  return (
    <div className="space-y-2">
      {instruments.map((inst) => (
        <InstrumentRow key={inst.instrumentId} instrument={inst} onEdit={handleEdit} />
      ))}
    </div>
  )
}
