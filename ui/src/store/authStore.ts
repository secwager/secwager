import { create } from 'zustand'

interface AuthState {
  jwt: string | null
  userId: string | null
  username: string | null
  btcAddr: string | null
  setAuth(jwt: string, userId: string, username: string): void
  setBtcAddr(addr: string): void
  clearAuth(): void
}

export const useAuthStore = create<AuthState>((set) => ({
  jwt: null,
  userId: null,
  username: null,
  btcAddr: null,

  setAuth: (jwt, userId, username) => set({ jwt, userId, username }),
  setBtcAddr: (btcAddr) => set({ btcAddr }),
  clearAuth: () => set({ jwt: null, userId: null, username: null, btcAddr: null }),
}))
