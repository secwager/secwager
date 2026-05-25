import { useState } from 'react'

interface Props {
  btcAddr: string
  onClose(): void
}

export function DepositModal({ btcAddr, onClose }: Props) {
  const [copied, setCopied] = useState(false)

  function handleCopy() {
    navigator.clipboard.writeText(btcAddr)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={onClose}
    >
      <div
        className="bg-white rounded-xl shadow-xl w-full max-w-sm p-6 space-y-4"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold text-gray-800">Deposit BTC</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-lg">✕</button>
        </div>
        <p className="text-sm text-gray-500">
          Send bitcoin to the address below. Your balance updates after 1 on-chain confirmation.
        </p>
        <div className="rounded bg-gray-50 border border-gray-200 px-4 py-3 font-mono text-xs break-all text-gray-700">
          {btcAddr}
        </div>
        <button
          onClick={handleCopy}
          className="w-full rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50 transition-colors"
        >
          {copied ? 'Copied!' : 'Copy address'}
        </button>
        <p className="text-center text-xs text-gray-400">Click outside to close</p>
      </div>
    </div>
  )
}
