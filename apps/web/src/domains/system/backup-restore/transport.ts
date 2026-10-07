import { session } from '../../../auth/session';
import { ApiError, type ApiProblemBody } from '../../../api-problem';
import { fetchWithTimeout } from '../../../net';

/** Binary endpoints share bearer refresh rules, with upload deadlines sized for update bundles. */
export async function request(path: string, init: RequestInit = {}): Promise<Response> {
  const url = new URL(`/api/v1${path}`, location.origin);
  const replayable =
    init.body === undefined ||
    init.body === null ||
    typeof init.body === 'string' ||
    init.body instanceof Blob;
  if (!replayable) throw new ApiError(400, { title: 'non-replayable-upload' }, false);
  const send = (token: string | null) => {
    const headers = new Headers(init.headers);
    if (token) headers.set('authorization', `Bearer ${token}`);
    return fetchWithTimeout(
      (r) => fetch(r),
      new Request(url, { credentials: 'same-origin', ...init, headers }),
      path === '/actions/upgrade-upload'
        ? 3_600_000
        : path.startsWith('/actions/')
          ? 300_000
          : 15_000,
    );
  };
  try {
    const token = session.accessToken;
    let res = await send(token);
    if (res.status === 401 && token && (await session.handleUnauthorized(token)))
      res = await send(session.accessToken);
    if (!res.ok) {
      const json = (res.headers.get('content-type') ?? '').includes('json');
      const body = json ? ((await res.json()) as ApiProblemBody) : { status: res.status };
      throw new ApiError(res.status, body, !json && res.status >= 500);
    }
    return res;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    throw new ApiError(0, { title: 'unreachable' }, true);
  }
}
export async function json<T>(
  path: string,
  body?: unknown,
  method = body === undefined ? 'GET' : 'POST',
): Promise<T> {
  const response = await request(path, {
    method,
    ...(body === undefined
      ? {}
      : { headers: { 'content-type': 'application/json' }, body: JSON.stringify(body) }),
  });
  return response.json() as Promise<T>;
}
export async function download(path: string, body: unknown, fallback: string): Promise<void> {
  const res = await request(path, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  });
  const blob = await res.blob();
  const suggested = /filename="?([^";]+)"?/.exec(res.headers.get('content-disposition') ?? '')?.[1];
  const name = suggested && /^[A-Za-z0-9._-]+$/.test(suggested) ? suggested : fallback;
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export async function archiveBase64(file: File): Promise<string> {
  if (file.size > 32 * 1024 * 1024) throw new ApiError(413, { title: 'archive-too-large' }, false);
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = '';
  for (let i = 0; i < bytes.length; i += 8192)
    binary += String.fromCharCode(...bytes.subarray(i, i + 8192));
  return btoa(binary);
}
