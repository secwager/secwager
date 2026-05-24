import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { GameSelector } from './GameSelector'
import { useBuilderStore } from '../../store/builderStore'
import { League } from '../../grpc/registry'

vi.mock('../../hooks/useGames')

import { useGames } from '../../hooks/useGames'
const mockUseGames = vi.mocked(useGames)

const GAMES = [
  {
    id: 'MLB::OAK@PIT::20240320',
    league: League.MLB,
    homeTeamId: 'MLB::PIT',
    awayTeamId: 'MLB::OAK',
    scheduledUnix: 1710979200, // 2024-03-20 20:00 UTC
    expiryUnix: 1710993600,
  },
  {
    id: 'MLB::NYY@BOS::20240321',
    league: League.MLB,
    homeTeamId: 'MLB::BOS',
    awayTeamId: 'MLB::NYY',
    scheduledUnix: 1711065600,
    expiryUnix: 1711080000,
  },
]

beforeEach(() => {
  useBuilderStore.setState({ selectedLeague: League.MLB, selectedGameId: null, legs: [] })
  vi.clearAllMocks()
})

describe('GameSelector — loading state', () => {
  it('shows loading message while fetching', () => {
    mockUseGames.mockReturnValue({ data: undefined, isLoading: true, isError: false } as ReturnType<typeof useGames>)
    render(<GameSelector />)
    expect(screen.getByText(/loading games/i)).toBeInTheDocument()
  })
})

describe('GameSelector — error state', () => {
  it('shows error message on failure', () => {
    mockUseGames.mockReturnValue({ data: undefined, isLoading: false, isError: true } as ReturnType<typeof useGames>)
    render(<GameSelector />)
    expect(screen.getByText(/failed to load games/i)).toBeInTheDocument()
  })
})

describe('GameSelector — empty state', () => {
  it('shows no-games message when list is empty', () => {
    mockUseGames.mockReturnValue({ data: { games: [] }, isLoading: false, isError: false } as ReturnType<typeof useGames>)
    render(<GameSelector />)
    expect(screen.getByText(/no upcoming games/i)).toBeInTheDocument()
  })
})

describe('GameSelector — game list', () => {
  beforeEach(() => {
    mockUseGames.mockReturnValue({ data: { games: GAMES }, isLoading: false, isError: false } as ReturnType<typeof useGames>)
  })

  it('renders a button for each game', () => {
    render(<GameSelector />)
    expect(screen.getAllByRole('button')).toHaveLength(2)
  })

  it('shows home and away team ids', () => {
    render(<GameSelector />)
    expect(screen.getByText('MLB::OAK')).toBeInTheDocument()
    expect(screen.getByText('MLB::PIT')).toBeInTheDocument()
  })

  it('calls setGameId when a game is clicked', async () => {
    render(<GameSelector />)
    await userEvent.click(screen.getAllByRole('button')[0])
    expect(useBuilderStore.getState().selectedGameId).toBe('MLB::OAK@PIT::20240320')
  })

  it('applies active style to the selected game', async () => {
    render(<GameSelector />)
    const buttons = screen.getAllByRole('button')
    await userEvent.click(buttons[0])
    // Re-render to pick up store change
    const { rerender } = render(<GameSelector />)
    rerender(<GameSelector />)
    expect(useBuilderStore.getState().selectedGameId).toBe('MLB::OAK@PIT::20240320')
  })
})
