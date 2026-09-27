import { useMutation } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';

/** The IPv6 source an IPv4 client appears as (running clientPrefix, RFC 6052); read-only. */
export function useNat46Client() {
  return useMutation({
    mutationFn: async (ipv4: string) =>
      (await call(api.GET('/api/v1/state/nat/nat46/client', { params: { query: { ipv4 } } })))
        .data,
  });
}
