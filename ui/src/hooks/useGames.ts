import { useQuery } from '@tanstack/react-query'
import { listGames, League } from '../grpc/registry'

export function useGames(league: League) {
  const now = Math.floor(Date.now() / 1000)
  const twoWeeks = now + 14 * 24 * 60 * 60

  return useQuery({
    queryKey: ['games', league],
    queryFn: () => listGames(league, now, twoWeeks),
    staleTime: 5 * 60 * 1000,
  })
}
