import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { LegList } from './LegList'
import { useBuilderStore } from '../../store/builderStore'
import { Outcome, PropType, Comparator } from '../../gen/registry/registry'
import { League } from '../../grpc/registry'

beforeEach(() => {
  useBuilderStore.setState({ selectedLeague: League.MLB, selectedGameId: null, legs: [] })
})

describe('LegList — empty state', () => {
  it('shows placeholder when no legs', () => {
    render(<LegList />)
    expect(screen.getByText(/no legs added yet/i)).toBeInTheDocument()
  })
})

describe('LegList — leg descriptions', () => {
  it('renders a game outcome leg with Home Win label', () => {
    useBuilderStore.setState({
      legs: [{ gameId: 'MLB::OAK@PIT::20240320', gameOutcome: { outcome: Outcome.HOME_WIN } }],
    })
    render(<LegList />)
    expect(screen.getByText(/Home Win/)).toBeInTheDocument()
    expect(screen.getByText(/MLB::OAK@PIT::20240320/)).toBeInTheDocument()
  })

  it('renders a game outcome leg with Away Win label', () => {
    useBuilderStore.setState({
      legs: [{ gameId: 'g1', gameOutcome: { outcome: Outcome.HOME_LOSS } }],
    })
    render(<LegList />)
    expect(screen.getByText(/Away Win/)).toBeInTheDocument()
  })

  it('renders a game outcome leg with Draw label', () => {
    useBuilderStore.setState({
      legs: [{ gameId: 'g1', gameOutcome: { outcome: Outcome.DRAW } }],
    })
    render(<LegList />)
    expect(screen.getByText(/Draw/)).toBeInTheDocument()
  })

  it('renders a player prop leg with prop abbreviation and comparator', () => {
    useBuilderStore.setState({
      legs: [{
        gameId: 'g1',
        playerProp: {
          playerId: 'MLB::OAK::ROOKER',
          propType: PropType.HOMERUNS,
          comparator: Comparator.GT,
          threshold: 1,
        },
      }],
    })
    render(<LegList />)
    // The full leg description is rendered as a single span
    expect(screen.getByText(/MLB::OAK::ROOKER HRs > 1/)).toBeInTheDocument()
  })

  it('renders a passing yards prop with >= comparator', () => {
    useBuilderStore.setState({
      legs: [{
        gameId: 'g1',
        playerProp: {
          playerId: 'NFL::KC::MAHOMES',
          propType: PropType.PASSING_YARDS,
          comparator: Comparator.GTE,
          threshold: 300,
        },
      }],
    })
    render(<LegList />)
    expect(screen.getByText(/Pass Yds/)).toBeInTheDocument()
    expect(screen.getByText(/>=/)).toBeInTheDocument()
  })
})

describe('LegList — multiple legs', () => {
  it('shows leg count in header', () => {
    useBuilderStore.setState({
      legs: [
        { gameId: 'g1', gameOutcome: { outcome: Outcome.HOME_WIN } },
        { gameId: 'g2', gameOutcome: { outcome: Outcome.HOME_LOSS } },
      ],
    })
    render(<LegList />)
    // The h3 renders "Parlay legs (N)" with N in a separate text node
    const heading = screen.getByRole('heading', { level: 3 })
    expect(heading).toHaveTextContent('Parlay legs (2)')
  })

  it('shows singular "leg" for one leg', () => {
    useBuilderStore.setState({
      legs: [{ gameId: 'g1', gameOutcome: { outcome: Outcome.HOME_WIN } }],
    })
    render(<LegList />)
    const heading = screen.getByRole('heading', { level: 3 })
    expect(heading).toHaveTextContent('Parlay legs (1)')
  })

  it('renders a remove button for each leg', () => {
    useBuilderStore.setState({
      legs: [
        { gameId: 'g1', gameOutcome: { outcome: Outcome.HOME_WIN } },
        { gameId: 'g2', gameOutcome: { outcome: Outcome.HOME_LOSS } },
      ],
    })
    render(<LegList />)
    expect(screen.getAllByRole('button', { name: /remove leg/i })).toHaveLength(2)
  })

  it('removes a leg when its remove button is clicked', async () => {
    useBuilderStore.setState({
      legs: [
        { gameId: 'g1', gameOutcome: { outcome: Outcome.HOME_WIN } },
        { gameId: 'g2', gameOutcome: { outcome: Outcome.HOME_LOSS } },
      ],
    })
    render(<LegList />)
    await userEvent.click(screen.getAllByRole('button', { name: /remove leg/i })[0])
    expect(useBuilderStore.getState().legs).toHaveLength(1)
    expect(useBuilderStore.getState().legs[0].gameId).toBe('g2')
  })
})
