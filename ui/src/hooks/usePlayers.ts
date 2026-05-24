import { useQuery } from '@tanstack/react-query'
import { listPlayersByGame } from '../grpc/registry'

export function usePlayers(gameId: string | null) {
  return useQuery({
    queryKey: ['players', gameId],
    queryFn: () => listPlayersByGame(gameId!),
    enabled: gameId !== null,
    staleTime: 10 * 60 * 1000,
  })
}
