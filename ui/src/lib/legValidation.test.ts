import { describe, it, expect } from 'vitest'
import { rejectReason } from './legValidation'
import { Outcome, PropType, Comparator } from '../gen/registry/registry'
import type { Leg } from '../gen/registry/registry'

const outcomeWin  = (): Leg => ({ gameId: 'g1', gameOutcome: { outcome: Outcome.HOME_WIN  } })
const outcomeLoss = (): Leg => ({ gameId: 'g1', gameOutcome: { outcome: Outcome.HOME_LOSS } })
const outcomeDraw = (): Leg => ({ gameId: 'g1', gameOutcome: { outcome: Outcome.DRAW      } })

const prop = (overrides: Partial<{
  gameId: string; playerId: string; propType: PropType; comparator: Comparator; threshold: number
}> = {}): Leg => ({
  gameId: overrides.gameId ?? 'g1',
  playerProp: {
    playerId:   overrides.playerId   ?? 'MLB::OAK::ROOKER',
    propType:   overrides.propType   ?? PropType.HOMERUNS,
    comparator: overrides.comparator ?? Comparator.GT,
    threshold:  overrides.threshold  ?? 1,
  },
})

describe('rejectReason — no conflict', () => {
  it('returns null for first leg', () => {
    expect(rejectReason([], outcomeWin())).toBeNull()
  })

  it('returns null for outcome leg on a different game', () => {
    expect(rejectReason([outcomeWin()], { ...outcomeWin(), gameId: 'g2' })).toBeNull()
  })

  it('returns null for prop leg alongside an outcome leg on the same game', () => {
    expect(rejectReason([outcomeWin()], prop())).toBeNull()
  })

  it('returns null for same player, different prop type', () => {
    expect(rejectReason([prop()], prop({ propType: PropType.HITS }))).toBeNull()
  })

  it('returns null for same player+prop, different comparator', () => {
    expect(rejectReason([prop()], prop({ comparator: Comparator.GTE }))).toBeNull()
  })

  it('returns null for same player+prop+comparator, different threshold', () => {
    expect(rejectReason([prop()], prop({ threshold: 2 }))).toBeNull()
  })

  it('returns null for different players, same prop on same game', () => {
    expect(rejectReason([prop()], prop({ playerId: 'MLB::OAK::BLACKBURN' }))).toBeNull()
  })
})

describe('rejectReason — duplicate', () => {
  it('rejects an identical outcome leg', () => {
    expect(rejectReason([outcomeWin()], outcomeWin())).toBe('duplicate')
  })

  it('rejects an identical player prop leg', () => {
    expect(rejectReason([prop()], prop())).toBe('duplicate')
  })

  it('rejects even when other non-duplicate legs are present', () => {
    expect(rejectReason([outcomeDraw(), prop()], prop())).toBe('duplicate')
  })
})

describe('rejectReason — game-outcome-conflict', () => {
  it('rejects a second outcome leg for the same game (different outcome)', () => {
    expect(rejectReason([outcomeWin()], outcomeLoss())).toBe('game-outcome-conflict')
  })

  it('rejects DRAW as a second outcome for the same game', () => {
    expect(rejectReason([outcomeWin()], outcomeDraw())).toBe('game-outcome-conflict')
  })

  it('allows outcome legs for two different games', () => {
    const g2win: Leg = { gameId: 'g2', gameOutcome: { outcome: Outcome.HOME_WIN } }
    expect(rejectReason([outcomeWin()], g2win)).toBeNull()
  })
})
