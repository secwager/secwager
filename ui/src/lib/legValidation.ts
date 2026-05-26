import type { Leg } from '../gen/registry/registry'
import { Comparator } from '../gen/registry/registry'

function isLowerBound(c: Comparator): boolean {
  return c === Comparator.GT || c === Comparator.GTE
}
function isUpperBound(c: Comparator): boolean {
  return c === Comparator.LT || c === Comparator.LTE
}

// Effective integer lower-bound: GT n → n+1, GTE n → n
function effectiveLower(c: Comparator, t: number): number {
  return c === Comparator.GT ? t + 1 : t
}
// Effective integer upper-bound: LT n → n-1, LTE n → n
function effectiveUpper(c: Comparator, t: number): number {
  return c === Comparator.LT ? t - 1 : t
}

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

export type LegRejectionReason = 'duplicate' | 'game-outcome-conflict' | 'degenerate-prop'

/**
 * Returns the reason the candidate leg cannot be added, or null if it is allowed.
 * Mirrors the server-side validateCrossLeg rules that are cheaply checked client-side.
 */
export function rejectReason(existing: Leg[], candidate: Leg): LegRejectionReason | null {
  for (const leg of existing) {
    if (legsEqual(leg, candidate)) return 'duplicate'
    if (leg.gameOutcome && candidate.gameOutcome && leg.gameId === candidate.gameId) {
      return 'game-outcome-conflict'
    }
    if (leg.playerProp && candidate.playerProp && leg.gameId === candidate.gameId) {
      const e = leg.playerProp
      const c = candidate.playerProp
      if (e.playerId === c.playerId && e.propType === c.propType) {
        // Candidate lower bound is weaker (less restrictive) than existing lower bound
        if (isLowerBound(e.comparator) && isLowerBound(c.comparator)
            && effectiveLower(c.comparator, c.threshold) <= effectiveLower(e.comparator, e.threshold)) {
          return 'degenerate-prop'
        }
        // Candidate upper bound is weaker (less restrictive) than existing upper bound
        if (isUpperBound(e.comparator) && isUpperBound(c.comparator)
            && effectiveUpper(c.comparator, c.threshold) >= effectiveUpper(e.comparator, e.threshold)) {
          return 'degenerate-prop'
        }
      }
    }
  }
  return null
}

export { isLowerBound, isUpperBound, effectiveLower, effectiveUpper }
