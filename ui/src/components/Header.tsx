import { useState } from 'react'
import { signOut } from 'aws-amplify/auth'
import { useAuthStore } from '../store/authStore'
import { AuthModal } from './modals/AuthModal'
import { DepositModal } from './modals/DepositModal'

export function Header() {
  const { username, btcAddr, clearAuth } = useAuthStore()
  const [authOpen, setAuthOpen] = useState(false)
  const [depositOpen, setDepositOpen] = useState(false)

  async function handleSignOut() {
    await signOut()
    clearAuth()
  }

  return (
    <>
      <header className="flex items-center justify-between px-6 py-3 border-b border-gray-200 bg-white">
        <span className="font-bold text-lg tracking-tight text-indigo-700">secwager</span>

        <div className="flex items-center gap-4 text-sm">
          {username ? (
            <>
              {btcAddr && (
                <button
                  onClick={() => setDepositOpen(true)}
                  className="text-gray-500 hover:text-indigo-600 transition-colors font-mono text-xs truncate max-w-[140px]"
                  title={btcAddr}
                >
                  {btcAddr.slice(0, 10)}…
                </button>
              )}
              <span className="text-gray-700 font-medium">{username}</span>
              <button
                onClick={handleSignOut}
                className="text-gray-400 hover:text-gray-600 transition-colors"
              >
                Sign out
              </button>
            </>
          ) : (
            <button
              onClick={() => setAuthOpen(true)}
              className="rounded bg-indigo-600 text-white px-4 py-1.5 font-medium hover:bg-indigo-700 transition-colors"
            >
              Sign in / Register
            </button>
          )}
        </div>
      </header>

      {authOpen && <AuthModal onClose={() => setAuthOpen(false)} />}
      {depositOpen && btcAddr && (
        <DepositModal btcAddr={btcAddr} onClose={() => setDepositOpen(false)} />
      )}
    </>
  )
}
