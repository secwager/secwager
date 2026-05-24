import { useQuery } from '@tanstack/react-query'
import { useAuthStore } from '../../store/authStore'
import { checkAvailable } from '../../grpc/cashier'
import { useState } from 'react'
import { DepositModal } from '../modals/DepositModal'

function fmtSats(sats: number): string {
  return sats.toLocaleString() + ' sats'
}

export function AccountSummary() {
  const { userId, jwt, btcAddr } = useAuthStore()
  const [depositOpen, setDepositOpen] = useState(false)

  const { data } = useQuery({
    queryKey: ['balance', userId],
    queryFn: () => checkAvailable(userId!, jwt!),
    enabled: !!(userId && jwt),
    refetchInterval: 30_000,
  })

  if (!userId) return null

  const gross = data?.grossBalance ?? 0
  const escrowed = data?.escrowed ?? 0
  const available = gross - escrowed

  return (
    <>
      <div className="rounded-lg border border-gray-200 bg-white px-5 py-4 space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-semibold text-gray-700">Account</h3>
          <button
            onClick={() => setDepositOpen(true)}
            className="text-xs text-indigo-600 hover:text-indigo-800 font-medium transition-colors"
          >
            Deposit
          </button>
        </div>
        <div className="grid grid-cols-3 gap-4 text-center">
          <div>
            <p className="text-xs text-gray-400">Balance</p>
            <p className="text-sm font-semibold text-gray-800">{fmtSats(gross)}</p>
          </div>
          <div>
            <p className="text-xs text-gray-400">Escrowed</p>
            <p className="text-sm font-semibold text-yellow-600">{fmtSats(escrowed)}</p>
          </div>
          <div>
            <p className="text-xs text-gray-400">Available</p>
            <p className="text-sm font-semibold text-emerald-600">{fmtSats(available)}</p>
          </div>
        </div>
      </div>

      {depositOpen && btcAddr && (
        <DepositModal btcAddr={btcAddr} onClose={() => setDepositOpen(false)} />
      )}
    </>
  )
}
