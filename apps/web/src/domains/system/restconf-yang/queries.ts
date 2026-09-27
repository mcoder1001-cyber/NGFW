import type { paths } from '@ngfw/api-client';
import { useQuery } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';

type Ok<P extends keyof paths, M extends 'get'> = paths[P][M] extends {
  responses: { 200: { content: { 'application/json': infer T } } };
}
  ? T
  : never;

export type YangModules = Ok<'/api/v1/system/yang', 'get'>;
export type YangModule = Ok<'/api/v1/system/yang/{name}', 'get'>;

export const yangKeys = { list: ['system', 'yang'] as const };

export function useYangModules() {
  return useQuery({
    queryKey: yangKeys.list,
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/system/yang', { signal }))).data as YangModules,
  });
}

/** Fetch one module's text (on demand, for download). */
export async function fetchYangModule(name: string): Promise<YangModule> {
  return (await call(api.GET('/api/v1/system/yang/{name}', { params: { path: { name } } })))
    .data as YangModule;
}
