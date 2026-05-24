import { describe, it, expect, beforeEach } from 'vitest'
import { useBuilderStore } from './builderStore'
import { League } from '../grpc/registry'
import { Outcome, PropType, Comparator } from '../gen/registry/registry'

const outcomeleg = () => ({
  gameId: 'g1',
  gameOutcome: { outcome: Outcome.HOME_WIN },
})
const propLeg = () => ({
  gameId: 'g1',
  playerProp: {
    playerId: 'MLB::OAK::ROOKER',
    propType: PropType.HOMERUNS,
    comparator: Comparator.GT,
    threshold: 1,
  },
})

beforeEach(() => {
  useBuilderStore.setState({
    selectedLeague: League.MLB,
    selectedGameId: null,
    legs: [],
  })
})

describe('setLeague', () => {
  it('updates the league', () => {
    useBuilderStore.getState().setLeague(League.NFL)
    expect(useBuilderStore.getState().selectedLeague).toBe(League.NFL)
  })

  it('resets selectedGameId when league changes', () => {
    useBuilderStore.setState({ selectedGameId: 'game-1' })
    useBuilderStore.getState().setLeague(League.EPL)
    expect(useBuilderStore.getState().selectedGameId).toBeNull()
  })
})

describe('setGameId', () => {
  it('sets the game id', () => {
    useBuilderStore.getState().setGameId('game-42')
    expect(useBuilderStore.getState().selectedGameId).toBe('game-42')
  })

  it('accepts null to deselect', () => {
    useBuilderStore.setState({ selectedGameId: 'game-42' })
    useBuilderStore.getState().setGameId(null)
    expect(useBuilderStore.getState().selectedGameId).toBeNull()
  })
})

describe('addLeg', () => {
  it('appends a game outcome leg', () => {
    useBuilderStore.getState().addLeg(outcomeleg())
    expect(useBuilderStore.getState().legs).toHaveLength(1)
    expect(useBuilderStore.getState().legs[0].gameId).toBe('g1')
  })

  it('appends a player prop leg', () => {
    useBuilderStore.getState().addLeg(propLeg())
    expect(useBuilderStore.getState().legs[0].playerProp?.propType).toBe(PropType.HOMERUNS)
  })

  it('accumulates multiple legs in order', () => {
    useBuilderStore.getState().addLeg(outcomeleg())
    useBuilderStore.getState().addLeg(propLeg())
    const legs = useBuilderStore.getState().legs
    expect(legs).toHaveLength(2)
    expect(legs[0].gameOutcome).toBeDefined()
    expect(legs[1].playerProp).toBeDefined()
  })
})

describe('removeLeg', () => {
  it('removes a leg by index', () => {
    useBuilderStore.setState({ legs: [outcomeleg(), propLeg()] })
    useBuilderStore.getState().removeLeg(0)
    const legs = useBuilderStore.getState().legs
    expect(legs).toHaveLength(1)
    expect(legs[0].playerProp).toBeDefined()
  })

  it('removes the last leg', () => {
    useBuilderStore.setState({ legs: [outcomeleg(), propLeg()] })
    useBuilderStore.getState().removeLeg(1)
    expect(useBuilderStore.getState().legs).toHaveLength(1)
    expect(useBuilderStore.getState().legs[0].gameOutcome).toBeDefined()
  })

  it('is a no-op for an out-of-range index', () => {
    useBuilderStore.setState({ legs: [outcomeleg()] })
    useBuilderStore.getState().removeLeg(5)
    expect(useBuilderStore.getState().legs).toHaveLength(1)
  })
})

describe('loadLegs', () => {
  it('replaces existing legs', () => {
    useBuilderStore.setState({ legs: [outcomeleg()] })
    const newLegs = [propLeg()]
    useBuilderStore.getState().loadLegs(newLegs)
    expect(useBuilderStore.getState().legs).toHaveLength(1)
    expect(useBuilderStore.getState().legs[0].playerProp).toBeDefined()
  })

  it('accepts an empty array to clear legs', () => {
    useBuilderStore.setState({ legs: [outcomeleg()] })
    useBuilderStore.getState().loadLegs([])
    expect(useBuilderStore.getState().legs).toHaveLength(0)
  })
})

describe('reset', () => {
  it('clears legs and gameId but preserves league', () => {
    useBuilderStore.setState({
      selectedLeague: League.EPL,
      selectedGameId: 'game-1',
      legs: [outcomeleg()],
    })
    useBuilderStore.getState().reset()
    const state = useBuilderStore.getState()
    expect(state.legs).toHaveLength(0)
    expect(state.selectedGameId).toBeNull()
    expect(state.selectedLeague).toBe(League.EPL)
  })
})
