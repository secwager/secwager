import { useState } from 'react'
import { useBuilderStore } from '../../store/builderStore'
import { usePlayers } from '../../hooks/usePlayers'
import { usePropTypes } from '../../hooks/usePropTypes'
import { League, type Leg, type Position } from '../../grpc/registry'
import { Outcome, PropType, Comparator } from '../../gen/registry/registry'
import { rejectReason } from '../../lib/legValidation'

const OUTCOMES: { value: Outcome; label: string }[] = [
  { value: Outcome.HOME_WIN,  label: 'Home Win' },
  { value: Outcome.HOME_LOSS, label: 'Away Win' },
  { value: Outcome.DRAW,      label: 'Draw' },
]

const COMPARATORS: { value: Comparator; label: string }[] = [
  { value: Comparator.GT,  label: '>'  },
  { value: Comparator.GTE, label: '>=' },
  { value: Comparator.LT,  label: '<'  },
  { value: Comparator.LTE, label: '<=' },
  { value: Comparator.EQ,  label: '='  },
]

// Use Partial so UNRECOGNIZED is not required
const PROP_LABELS: Partial<Record<PropType, string>> = {
  [PropType.HOMERUNS]:        'Home Runs',
  [PropType.STRIKEOUTS]:      'Strikeouts',
  [PropType.HITS]:            'Hits',
  [PropType.RBIS]:            'RBIs',
  [PropType.WALKS]:           'Walks',
  [PropType.EARNED_RUNS]:     'Earned Runs',
  [PropType.GOALS]:           'Goals',
  [PropType.ASSISTS]:         'Assists',
  [PropType.SAVES]:           'Saves',
  [PropType.YELLOW_CARDS]:    'Yellow Cards',
  [PropType.PASSING_YARDS]:   'Passing Yards',
  [PropType.RUSHING_YARDS]:   'Rushing Yards',
  [PropType.RECEIVING_YARDS]: 'Receiving Yards',
  [PropType.TOUCHDOWNS]:      'Touchdowns',
  [PropType.INTERCEPTIONS]:   'Interceptions',
}

type LegType = 'outcome' | 'prop'

export function LegBuilder() {
  const { selectedLeague, selectedGameId, legs, addLeg } = useBuilderStore()
  const [legType, setLegType] = useState<LegType>('outcome')
  const [outcome, setOutcome] = useState<Outcome>(Outcome.HOME_WIN)
  const [playerId, setPlayerId] = useState('')
  const [playerPosition, setPlayerPosition] = useState<Position | null>(null)
  const [propType, setPropType] = useState<PropType>(PropType.PROP_UNSPECIFIED)
  const [comparator, setComparator] = useState<Comparator>(Comparator.GT)
  const [threshold, setThreshold] = useState('')

  const { data: playersData } = usePlayers(selectedGameId)
  const { data: propTypesData } = usePropTypes(selectedLeague, playerPosition)

  const isSoccer = selectedLeague === League.EPL
    || selectedLeague === League.LA_LIGA
    || selectedLeague === League.MLS

  const players = playersData?.players ?? []
  const allowedProps = propTypesData?.propTypes ?? []

  const candidateLeg: Leg | null = selectedGameId
    ? legType === 'outcome'
      ? { gameId: selectedGameId, gameOutcome: { outcome } }
      : (playerId && propType !== PropType.PROP_UNSPECIFIED && threshold)
        ? { gameId: selectedGameId, playerProp: { playerId, propType, comparator, threshold: parseInt(threshold, 10) } }
        : null
    : null

  const rejection = candidateLeg ? rejectReason(legs, candidateLeg) : null

  function handlePlayerChange(id: string) {
    setPlayerId(id)
    const p = players.find((pl) => pl.id === id)
    setPlayerPosition(p?.positions?.[0] ?? null)
    setPropType(PropType.PROP_UNSPECIFIED)
  }

  function handleAdd() {
    if (!candidateLeg || rejection) return
    addLeg(candidateLeg)
    if (legType === 'outcome') {
      setLegType('prop')
    } else {
      setPlayerId('')
      setPlayerPosition(null)
      setPropType(PropType.PROP_UNSPECIFIED)
      setThreshold('')
    }
  }

  if (!selectedGameId) {
    return <p className="text-sm text-gray-400 italic">Select a game above to add legs.</p>
  }

  return (
    <div className="space-y-4 bg-gray-50 rounded-lg p-4">
      <div className="flex gap-3">
        <label className="flex items-center gap-1.5 text-sm cursor-pointer">
          <input
            type="radio"
            name="legType"
            checked={legType === 'outcome'}
            onChange={() => setLegType('outcome')}
          />
          Game Outcome
        </label>
        <label className="flex items-center gap-1.5 text-sm cursor-pointer">
          <input
            type="radio"
            name="legType"
            checked={legType === 'prop'}
            onChange={() => setLegType('prop')}
          />
          Player Prop
        </label>
      </div>

      {legType === 'outcome' && (
        <select
          value={outcome}
          onChange={(e) => setOutcome(e.target.value as Outcome)}
          className="w-full rounded border border-gray-300 px-3 py-2 text-sm"
        >
          {OUTCOMES.filter((o) => isSoccer || o.value !== Outcome.DRAW).map((o) => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
      )}

      {legType === 'prop' && (
        <div className="space-y-2">
          <select
            value={playerId}
            onChange={(e) => handlePlayerChange(e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-2 text-sm"
          >
            <option value="">Select player…</option>
            {players.map((p) => (
              <option key={p.id} value={p.id}>
                {p.lineupConfirmed ? `★ ${p.name}` : p.name}
              </option>
            ))}
          </select>

          {playerId && (
            <select
              value={propType}
              onChange={(e) => setPropType(e.target.value as PropType)}
              className="w-full rounded border border-gray-300 px-3 py-2 text-sm"
            >
              <option value={PropType.PROP_UNSPECIFIED}>Select prop…</option>
              {allowedProps.map((pt) => (
                <option key={pt} value={pt}>{PROP_LABELS[pt] ?? pt}</option>
              ))}
            </select>
          )}

          {propType !== PropType.PROP_UNSPECIFIED && (
            <div className="flex gap-2">
              <select
                value={comparator}
                onChange={(e) => setComparator(e.target.value as Comparator)}
                className="rounded border border-gray-300 px-3 py-2 text-sm w-20"
              >
                {COMPARATORS.map((c) => (
                  <option key={c.value} value={c.value}>{c.label}</option>
                ))}
              </select>
              <input
                type="number"
                min={0}
                value={threshold}
                onChange={(e) => setThreshold(e.target.value)}
                placeholder="threshold"
                className="flex-1 rounded border border-gray-300 px-3 py-2 text-sm"
              />
            </div>
          )}
        </div>
      )}

      {rejection && (
        <p className="text-xs text-amber-600">
          {rejection === 'duplicate'
            ? 'This leg is already in the parlay.'
            : rejection === 'game-outcome-conflict'
            ? 'A game outcome leg for this game is already added.'
            : 'A more restrictive version of this leg is already in the parlay.'}
        </p>
      )}
      <button
        onClick={handleAdd}
        disabled={!candidateLeg || !!rejection}
        className="w-full rounded bg-indigo-600 text-white px-4 py-2 text-sm font-medium
                   hover:bg-indigo-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
      >
        + Add Leg
      </button>
    </div>
  )
}
