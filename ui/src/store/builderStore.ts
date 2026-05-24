import { create } from 'zustand'
import { League, type Leg } from '../grpc/registry'

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
  addLeg: (leg) => set((s) => ({ legs: [...s.legs, leg] })),
  removeLeg: (idx) => set((s) => ({ legs: s.legs.filter((_, i) => i !== idx) })),
  loadLegs: (legs) => set({ legs }),
  reset: () => set({ selectedGameId: null, legs: [] }),
}))
