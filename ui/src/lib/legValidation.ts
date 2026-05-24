import type { Leg } from '../gen/registry/registry'

function legsEqual(a: Leg, b: Leg): boolean {
  if (a.gameId !== b.gameId) return false
  if (a.gameOutcome && b.gameOutcome) {
    return a.gameOutcome.outcome === b.gameOutcome.outcome
  }
  if (a.playerProp && b.playerProp) {
    const p = a.playerProp
    const q = b.playerProp
    return p.playerId === q.playerId
      && p.propType === q.propType
      && p.comparator === q.comparator
      && p.threshold === q.threshold
  }
  return false
}

export type LegRejectionReason = 'duplicate' | 'game-outcome-conflict'

/**
 * Returns the reason the candidate leg cannot be added, or null if it is allowed.
 * Mirrors the server-side validateCrossLeg rules that are cheaply checked client-side.
 */
export function rejectReason(existing: Leg[], candidate: Leg): LegRejectionReason | null {
  for (const leg of existing) {
    if (legsEqual(leg, candidate)) return 'duplicate'
    // Two outcome legs for the same game are always contradictory regardless of outcome value
    if (leg.gameOutcome && candidate.gameOutcome && leg.gameId === candidate.gameId) {
      return 'game-outcome-conflict'
    }
  }
  return null
}
