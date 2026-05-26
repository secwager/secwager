import { create } from 'zustand'
import { League, type Leg } from '../grpc/registry'
import { isLowerBound, isUpperBound, effectiveLower, effectiveUpper } from '../lib/legValidation'

interface BuilderState {
  selectedLeague: League
  selectedGameId: string | null
  legs: Leg[]
  setLeague(league: League): void
  setGameId(gameId: string | null): void
  addLeg(leg: Leg): void
  removeLeg(idx: number): void
  loadLegs(legs: Leg[]): void
  reset(): void
}

export const useBuilderStore = create<BuilderState>((set) => ({
  selectedLeague: League.MLB,
  selectedGameId: null,
  legs: [],

  setLeague: (league) => set({ selectedLeague: league, selectedGameId: null }),
  setGameId: (gameId) => set({ selectedGameId: gameId }),
  addLeg: (leg) => set((s) => {
    if (!leg.playerProp) return { legs: [...s.legs, leg] }
    const { playerId, propType, comparator, threshold } = leg.playerProp
    const newIsLower = isLowerBound(comparator)
    const newIsUpper = isUpperBound(comparator)
    // Remove any existing same-direction prop leg that is weaker than the new one
    const filtered = s.legs.filter((existing) => {
      if (!existing.playerProp || existing.gameId !== leg.gameId) return true
      const e = existing.playerProp
      if (e.playerId !== playerId || e.propType !== propType) return true
      if (newIsLower && isLowerBound(e.comparator)
          && effectiveLower(comparator, threshold) > effectiveLower(e.comparator, e.threshold)) {
        return false
      }
      if (newIsUpper && isUpperBound(e.comparator)
          && effectiveUpper(comparator, threshold) < effectiveUpper(e.comparator, e.threshold)) {
        return false
      }
      return true
    })
    return { legs: [...filtered, leg] }
  }),
  removeLeg: (idx) => set((s) => ({ legs: s.legs.filter((_, i) => i !== idx) })),
  loadLegs: (legs) => set({ legs }),
  reset: () => set({ selectedGameId: null, legs: [] }),
}))
