import { ENVOY_HOST, getMetadata } from './transport'
import {
  GrpcWebImpl,
  RegistryServiceClientImpl,
  League,
  ListGamesRequest,
  ListTeamsRequest,
  ListPlayersByGameRequest,
  GetAllowedPropTypesRequest,
  CreateInstrumentRequest,
  GetInstrumentRequest,
  ListInstrumentsRequest,
  type Leg,
  type Position,
} from '../gen/registry/registry'

function makeClient() {
  return new RegistryServiceClientImpl(new GrpcWebImpl(ENVOY_HOST, {}))
}

const client = makeClient()

export function listGames(league: League, fromUnix: number, toUnix: number) {
  return client.ListGames(ListGamesRequest.fromPartial({ league, fromUnix, toUnix }))
}

export function listTeams(league: League) {
  return client.ListTeams(ListTeamsRequest.fromPartial({ league }))
}

export function listPlayersByGame(gameId: string) {
  return client.ListPlayersByGame(ListPlayersByGameRequest.fromPartial({ gameId }))
}

export function getAllowedPropTypes(league: League, position: Position) {
  return client.GetAllowedPropTypes(
    GetAllowedPropTypesRequest.fromPartial({ league, position }),
  )
}

export function createInstrument(legs: Leg[], jwt: string) {
  return client.CreateInstrument(
    CreateInstrumentRequest.fromPartial({ legs }),
    getMetadata(jwt),
  )
}

export function getInstrument(instrumentId: string) {
  return client.GetInstrument(GetInstrumentRequest.fromPartial({ instrumentId }))
}

export function listInstruments(entityId: string, league: League, activeOnly: boolean) {
  return client.ListInstruments(
    ListInstrumentsRequest.fromPartial({ entityId, league, activeOnly, pageSize: 50 }),
  )
}

export { League, type Leg, type Position }
