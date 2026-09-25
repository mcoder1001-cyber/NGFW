// TD-2 #2: a consumer type-checks against the BUILT package (dist, through package.json `exports`/`types`), not
// against src. When dist ships no generated schema, `paths` degrades to `any` and every assertion below fails.
import { createApiClient, type paths } from '@ngfw/api-client';

type IsAny<T> = 0 extends 1 & T ? true : false;
export const pathsAreTyped: IsAny<paths> = false;

const api = createApiClient('');

export async function probe(): Promise<void> {
  // TD-2 #3: /health has a response schema
  const health = await api.GET('/api/v1/health');
  if (health.data) {
    const status: 'ok' = health.data.status;
    const version: string = health.data.version;
    void status;
    void version;
  }
  // @ts-expect-error — unknown routes do not type-check
  await api.GET('/api/v1/no-such-route');
  // TD-2 #5: the lock names the API key that holds it
  const lock = await api.GET('/api/v1/config/lock');
  const keyId: string | null | undefined = lock.data?.ownerKeyId;
  void keyId;
  // TD-2 #6: redacted secret changes in the diff
  const diff = await api.GET('/api/v1/config/diff');
  const redacted: true | undefined = diff.data?.changes[0]?.redacted;
  void redacted;
  // TD-2 #1: password set
  await api.POST('/api/v1/users/{name}/password', {
    params: { path: { name: 'alice' } },
    body: { password: 'x'.repeat(12) },
  });
  // @ts-expect-error — the body is typed (password is required)
  await api.POST('/api/v1/users/{name}/password', { params: { path: { name: 'a' } }, body: {} });
}
