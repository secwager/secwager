import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { LegBuilder } from './LegBuilder'
import { useBuilderStore } from '../../store/builderStore'
import { League } from '../../grpc/registry'
import { PropType, Comparator, Outcome, Position } from '../../gen/registry/registry'

vi.mock('../../hooks/usePlayers')
vi.mock('../../hooks/usePropTypes')

import { usePlayers } from '../../hooks/usePlayers'
import { usePropTypes } from '../../hooks/usePropTypes'
const mockUsePlayers = vi.mocked(usePlayers)
const mockUsePropTypes = vi.mocked(usePropTypes)

const PLAYERS = [
  { id: 'MLB::OAK::ROOKER', name: 'Brent Rooker', teamId: 'MLB::OAK', positions: [Position.MLB_BATTER], lineupConfirmed: false },
  { id: 'MLB::OAK::BLACKBURN', name: 'Paul Blackburn', teamId: 'MLB::OAK', positions: [Position.MLB_PITCHER], lineupConfirmed: false },
]

beforeEach(() => {
  useBuilderStore.setState({ selectedLeague: League.MLB, selectedGameId: null, legs: [] })
  mockUsePlayers.mockReturnValue({ data: { players: PLAYERS }, isLoading: false } as ReturnType<typeof usePlayers>)
  mockUsePropTypes.mockReturnValue({ data: { propTypes: [PropType.HOMERUNS, PropType.HITS] } } as ReturnType<typeof usePropTypes>)
  vi.clearAllMocks()
})

describe('LegBuilder — no game selected', () => {
  it('shows prompt to select a game', () => {
    render(<LegBuilder />)
    expect(screen.getByText(/select a game above/i)).toBeInTheDocument()
  })
})

describe('LegBuilder — outcome mode', () => {
  beforeEach(() => {
    useBuilderStore.setState({ selectedLeague: League.MLB, selectedGameId: 'g1', legs: [] })
    mockUsePlayers.mockReturnValue({ data: { players: PLAYERS }, isLoading: false } as ReturnType<typeof usePlayers>)
    mockUsePropTypes.mockReturnValue({ data: { propTypes: [] } } as unknown as ReturnType<typeof usePropTypes>)
  })

  it('shows outcome select by default', () => {
    render(<LegBuilder />)
    expect(screen.getByText('Home Win')).toBeInTheDocument()
    expect(screen.getByText('Away Win')).toBeInTheDocument()
  })

  it('does not show Draw option for MLB', () => {
    render(<LegBuilder />)
    expect(screen.queryByText('Draw')).not.toBeInTheDocument()
  })

  it('shows Draw option for EPL', () => {
    useBuilderStore.setState({ selectedLeague: League.EPL, selectedGameId: 'g1', legs: [] })
    render(<LegBuilder />)
    expect(screen.getByText('Draw')).toBeInTheDocument()
  })

  it('shows Draw option for La Liga', () => {
    useBuilderStore.setState({ selectedLeague: League.LA_LIGA, selectedGameId: 'g1', legs: [] })
    render(<LegBuilder />)
    expect(screen.getByText('Draw')).toBeInTheDocument()
  })

  it('shows Draw option for MLS', () => {
    useBuilderStore.setState({ selectedLeague: League.MLS, selectedGameId: 'g1', legs: [] })
    render(<LegBuilder />)
    expect(screen.getByText('Draw')).toBeInTheDocument()
  })

  it('does not show Draw option for NFL', () => {
    useBuilderStore.setState({ selectedLeague: League.NFL, selectedGameId: 'g1', legs: [] })
    render(<LegBuilder />)
    expect(screen.queryByText('Draw')).not.toBeInTheDocument()
  })

  it('Add Leg button is enabled in outcome mode', () => {
    render(<LegBuilder />)
    expect(screen.getByRole('button', { name: /add leg/i })).not.toBeDisabled()
  })

  it('disables Add Leg and shows duplicate message when exact outcome already exists', () => {
    // Default select value is HOME_WIN; existing leg is also HOME_WIN → duplicate
    useBuilderStore.setState({
      selectedLeague: League.MLB,
      selectedGameId: 'g1',
      legs: [{ gameId: 'g1', gameOutcome: { outcome: Outcome.HOME_WIN } }],
    })
    render(<LegBuilder />)
    expect(screen.getByRole('button', { name: /add leg/i })).toBeDisabled()
    expect(screen.getByText(/already in the parlay/i)).toBeInTheDocument()
  })

  it('disables Add Leg and shows conflict message when a different outcome for same game exists', async () => {
    // Existing: HOME_WIN; user selects AWAY_WIN → conflict
    useBuilderStore.setState({
      selectedLeague: League.MLB,
      selectedGameId: 'g1',
      legs: [{ gameId: 'g1', gameOutcome: { outcome: Outcome.HOME_WIN } }],
    })
    render(<LegBuilder />)
    await userEvent.selectOptions(screen.getByRole('combobox'), Outcome.HOME_LOSS)
    expect(screen.getByRole('button', { name: /add leg/i })).toBeDisabled()
    expect(screen.getByText(/game outcome leg for this game is already added/i)).toBeInTheDocument()
  })

  it('adds a game outcome leg on click', async () => {
    render(<LegBuilder />)
    await userEvent.click(screen.getByRole('button', { name: /add leg/i }))
    const legs = useBuilderStore.getState().legs
    expect(legs).toHaveLength(1)
    expect(legs[0].gameOutcome?.outcome).toBe(Outcome.HOME_WIN)
  })
})

describe('LegBuilder — prop mode', () => {
  beforeEach(() => {
    useBuilderStore.setState({ selectedLeague: League.MLB, selectedGameId: 'g1', legs: [] })
    mockUsePlayers.mockReturnValue({ data: { players: PLAYERS }, isLoading: false } as ReturnType<typeof usePlayers>)
    mockUsePropTypes.mockReturnValue({ data: { propTypes: [PropType.HOMERUNS, PropType.HITS] } } as ReturnType<typeof usePropTypes>)
  })

  async function switchToPropMode() {
    await userEvent.click(screen.getByLabelText(/player prop/i))
  }

  it('Add Leg button is disabled before player is selected', async () => {
    render(<LegBuilder />)
    await switchToPropMode()
    expect(screen.getByRole('button', { name: /add leg/i })).toBeDisabled()
  })

  it('Add Leg button is disabled after player selected but no prop type', async () => {
    render(<LegBuilder />)
    await switchToPropMode()
    await userEvent.selectOptions(screen.getByRole('combobox'), 'MLB::OAK::ROOKER')
    expect(screen.getByRole('button', { name: /add leg/i })).toBeDisabled()
  })

  it('Add Leg button is disabled after prop selected but no threshold', async () => {
    render(<LegBuilder />)
    await switchToPropMode()
    await userEvent.selectOptions(screen.getAllByRole('combobox')[0], 'MLB::OAK::ROOKER')
    await userEvent.selectOptions(screen.getAllByRole('combobox')[1], PropType.HOMERUNS)
    expect(screen.getByRole('button', { name: /add leg/i })).toBeDisabled()
  })

  it('adds a player prop leg when all fields are filled', async () => {
    render(<LegBuilder />)
    await switchToPropMode()
    await userEvent.selectOptions(screen.getAllByRole('combobox')[0], 'MLB::OAK::ROOKER')
    await userEvent.selectOptions(screen.getAllByRole('combobox')[1], PropType.HOMERUNS)
    // comparator select (index 2) defaults to GT
    await userEvent.type(screen.getByPlaceholderText(/threshold/i), '1')
    await userEvent.click(screen.getByRole('button', { name: /add leg/i }))

    const legs = useBuilderStore.getState().legs
    expect(legs).toHaveLength(1)
    expect(legs[0].playerProp?.playerId).toBe('MLB::OAK::ROOKER')
    expect(legs[0].playerProp?.propType).toBe(PropType.HOMERUNS)
    expect(legs[0].playerProp?.comparator).toBe(Comparator.GT)
    expect(legs[0].playerProp?.threshold).toBe(1)
  })

  it('is disabled when the same prop leg already exists', async () => {
    useBuilderStore.setState({
      selectedLeague: League.MLB,
      selectedGameId: 'g1',
      legs: [{
        gameId: 'g1',
        playerProp: { playerId: 'MLB::OAK::ROOKER', propType: PropType.HOMERUNS, comparator: Comparator.GT, threshold: 1 },
      }],
    })
    render(<LegBuilder />)
    await switchToPropMode()
    await userEvent.selectOptions(screen.getAllByRole('combobox')[0], 'MLB::OAK::ROOKER')
    await userEvent.selectOptions(screen.getAllByRole('combobox')[1], PropType.HOMERUNS)
    await userEvent.type(screen.getByPlaceholderText(/threshold/i), '1')
    expect(screen.getByRole('button', { name: /add leg/i })).toBeDisabled()
    expect(screen.getByText(/already in the parlay/i)).toBeInTheDocument()
  })

  it('resets player/prop/threshold fields after adding a leg', async () => {
    render(<LegBuilder />)
    await switchToPropMode()
    await userEvent.selectOptions(screen.getAllByRole('combobox')[0], 'MLB::OAK::ROOKER')
    await userEvent.selectOptions(screen.getAllByRole('combobox')[1], PropType.HOMERUNS)
    await userEvent.type(screen.getByPlaceholderText(/threshold/i), '2')
    await userEvent.click(screen.getByRole('button', { name: /add leg/i }))

    // Player select should be back to empty
    expect((screen.getAllByRole('combobox')[0] as HTMLSelectElement).value).toBe('')
  })
})
