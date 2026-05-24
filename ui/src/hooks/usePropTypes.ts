import { useQuery } from '@tanstack/react-query'
import { getAllowedPropTypes, League, type Position } from '../grpc/registry'

export function usePropTypes(league: League, position: Position | null) {
  return useQuery({
    queryKey: ['propTypes', league, position],
    queryFn: () => getAllowedPropTypes(league, position!),
    enabled: position !== null,
    staleTime: Infinity,
  })
}
