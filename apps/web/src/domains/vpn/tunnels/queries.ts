import { useQuery } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
export const tunnelKeys = { state: ['state', 'tunnels'] as const };
export function useTunnelsState() {
  return useQuery({
    queryKey: tunnelKeys.state,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/state/tunnels', { signal }))).data,
    refetchInterval: 3_000,
  });
}
