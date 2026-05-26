import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { InstrumentLookup } from './index'
import * as registry from '../../grpc/registry'
import { Outcome, PropType, Comparator } from '../../gen/registry/registry'

vi.mock('../../grpc/registry')

function wrap(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>)
}

const FAKE_ID = 'abc123def456'

const fakeInstrument = {
  instrumentId: FAKE_ID,
  expiryUnix: 1893456000, // 2030-01-01
  legs: [
    {
      gameId: 'MLB::NYY::BOS::20260601',
      gameOutcome: { outcome: Outcome.HOME_WIN },
    },
    {
      gameId: 'MLB::NYY::BOS::20260601',
      playerProp: {
        playerId: 'MLB::NYY::JUDGE',
        propType: PropType.HOMERUNS,
        comparator: Comparator.GT,
        threshold: 1,
      },
    },
  ],
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('InstrumentLookup', () => {
  it('renders input and disabled Look up button when empty', () => {
    wrap(<InstrumentLookup />)
    expect(screen.getByRole('textbox', { name: /instrument id/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /look up/i })).toBeDisabled()
  })

  it('enables Look up button once input has text', async () => {
    const user = userEvent.setup()
    wrap(<InstrumentLookup />)
    await user.type(screen.getByRole('textbox'), FAKE_ID)
    expect(screen.getByRole('button', { name: /look up/i })).toBeEnabled()
  })

  it('shows loading state while fetching', async () => {
    vi.mocked(registry.getInstrument).mockReturnValue(new Promise(() => {})) // never resolves
    wrap(<InstrumentLookup initialId={FAKE_ID} />)
    // initialId pre-populates both inputId and searchId, so query fires immediately
    await waitFor(() => {
      expect(screen.getByText(/looking up instrument/i)).toBeInTheDocument()
    })
  })

  it('shows instrument card on success', async () => {
    vi.mocked(registry.getInstrument).mockResolvedValue({ instrument: fakeInstrument } as any)
    wrap(<InstrumentLookup initialId={FAKE_ID} />)

    await waitFor(() => {
      expect(screen.getByText(FAKE_ID)).toBeInTheDocument()
    })

    expect(screen.getByText(/home win/i)).toBeInTheDocument()
    expect(screen.getByText(/MLB::NYY::JUDGE HRs > 1/)).toBeInTheDocument()
    expect(screen.getByText(/202[90]/)).toBeInTheDocument()

    expect(screen.getByRole('button', { name: /place order/i })).toBeDisabled()
  })

  it('shows error when instrument not found', async () => {
    vi.mocked(registry.getInstrument).mockRejectedValue(new Error('not found'))
    wrap(<InstrumentLookup initialId={FAKE_ID} />)

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(/instrument not found/i)
    })
  })

  it('triggers search on Enter key', async () => {
    vi.mocked(registry.getInstrument).mockResolvedValue({ instrument: fakeInstrument } as any)
    const user = userEvent.setup()
    wrap(<InstrumentLookup />)

    await user.type(screen.getByRole('textbox'), `${FAKE_ID}{Enter}`)

    await waitFor(() => {
      expect(registry.getInstrument).toHaveBeenCalledWith(FAKE_ID)
    })
  })

  it('triggers search on button click', async () => {
    vi.mocked(registry.getInstrument).mockResolvedValue({ instrument: fakeInstrument } as any)
    const user = userEvent.setup()
    wrap(<InstrumentLookup />)

    await user.type(screen.getByRole('textbox'), FAKE_ID)
    await user.click(screen.getByRole('button', { name: /look up/i }))

    await waitFor(() => {
      expect(registry.getInstrument).toHaveBeenCalledWith(FAKE_ID)
    })
  })

  it('trims whitespace from input before searching', async () => {
    vi.mocked(registry.getInstrument).mockResolvedValue({ instrument: fakeInstrument } as any)
    const user = userEvent.setup()
    wrap(<InstrumentLookup />)

    await user.type(screen.getByRole('textbox'), `  ${FAKE_ID}  `)
    await user.click(screen.getByRole('button', { name: /look up/i }))

    await waitFor(() => {
      expect(registry.getInstrument).toHaveBeenCalledWith(FAKE_ID)
    })
  })
})
