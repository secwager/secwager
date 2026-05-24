import { useEffect, useState } from 'react'
import { fetchAuthSession, getCurrentUser } from 'aws-amplify/auth'
import { Hub } from 'aws-amplify/utils'
import { useAuthStore } from './store/authStore'
import { getUser } from './grpc/userregistration'
import { Header } from './components/Header'
import { AccountSummary } from './components/Dashboard/AccountSummary'
import { InstrumentBuilder } from './components/InstrumentBuilder'
import { MyInstruments } from './components/MyInstruments'
import { InstrumentLookup } from './components/InstrumentLookup'

type Tab = 'builder' | 'my-instruments' | 'lookup'

export default function App() {
  const { setAuth, setBtcAddr, clearAuth, userId } = useAuthStore()
  const [tab, setTab] = useState<Tab>('builder')
  const [lookupId, setLookupId] = useState('')

  function openLookup(id: string) {
    setLookupId(id)
    setTab('lookup')
  }

  useEffect(() => {
    // Bootstrap auth on load
    bootstrapAuth()

    // Listen for Cognito auth events
    const unlisten = Hub.listen('auth', ({ payload }) => {
      if (payload.event === 'signedIn')  bootstrapAuth()
      if (payload.event === 'signedOut') clearAuth()
    })
    return unlisten
  }, [])

  async function bootstrapAuth() {
    try {
      const [session, user] = await Promise.all([fetchAuthSession(), getCurrentUser()])
      const jwt = session.tokens?.idToken?.toString()
      if (!jwt) return
      setAuth(jwt, user.userId, user.username)

      // Fetch BTC address from userregistration service
      const regResp = await getUser(user.username, jwt)
      setBtcAddr(regResp.btcAddr)
    } catch {
      // Not signed in
    }
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <Header />

      <main className="max-w-3xl mx-auto px-4 py-8 space-y-6">
        {userId && <AccountSummary />}

        <div className="flex gap-1 border-b border-gray-200">
          {([
            ['builder',        'Build Instrument'],
            ['my-instruments', 'My Instruments'],
            ['lookup',         'Lookup'],
          ] as [Tab, string][]).map(([t, label]) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={[
                'px-4 py-2 text-sm font-medium transition-colors',
                tab === t
                  ? 'border-b-2 border-indigo-600 text-indigo-600'
                  : 'text-gray-500 hover:text-gray-700',
              ].join(' ')}
            >
              {label}
            </button>
          ))}
        </div>

        <div className="bg-white rounded-xl border border-gray-200 p-6">
          {tab === 'builder' && <InstrumentBuilder onViewInstrument={openLookup} />}
          {tab === 'my-instruments' && (
            <MyInstruments onEditInstrument={() => setTab('builder')} />
          )}
          {tab === 'lookup' && <InstrumentLookup initialId={lookupId} />}
        </div>
      </main>
    </div>
  )
}
