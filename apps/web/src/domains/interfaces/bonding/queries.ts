import { useQuery } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';

export const bondKeys = {
  state: ['state', 'interfaces', 'bonds'] as const,
};

/** Live bond table refresh (members and LACP states change without a commit). */
export const BONDS_POLL_MS = 3_000;

export async function fetchBondsState(signal?: AbortSignal) {
  return (await call(api.GET('/api/v1/state/interfaces/bonds', signal ? { signal } : {}))).data;
}

export function useBondsState() {
  return useQuery({ queryKey: bondKeys.state, queryFn: ({ signal }) => fetchBondsState(signal), refetchInterval: BONDS_POLL_MS });
}
