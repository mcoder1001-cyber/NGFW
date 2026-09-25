import type { paths } from '@ngfw/api-client';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';

export const licenseKey = ['state', 'license'] as const;

/** GET /api/v1/state/license (generated client types). */
export function useLicenseState() {
  return useQuery({
    queryKey: licenseKey,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/state/license', { signal }))).data,
    staleTime: 60_000,
    retry: false,
  });
}

export type LicenseState = NonNullable<ReturnType<typeof useLicenseState>['data']>;
type LicenseFile = paths['/api/v1/system/license']['put']['requestBody']['content']['application/json'];

/** PUT /api/v1/system/license with the `.vrxlic` file content (admin). */
export function useUploadLicense() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (text: string) => {
      let body: LicenseFile;
      try {
        body = JSON.parse(text) as LicenseFile;
      } catch {
        throw new SyntaxError('not-json');
      }
      return (await call(api.PUT('/api/v1/system/license', { body }))).data;
    },
    onSuccess: (data) => qc.setQueryData(licenseKey, data),
  });
}
