import { previewConfig, previewState } from './fixtures';

// OFFLINE_UI_PREVIEW_MARKER: only loaded by the explicitly enabled Vite dev entry.
const sampleSession = { accessToken: 'offline-preview', tokenType: 'Bearer', expiresIn: 86400,
  user: { id: 1, username: 'UI preview', role: 'admin' } };
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
  status, headers: { 'content-type': status >= 400 ? 'application/problem+json' : 'application/json' },
});

/** No request handled here can reach a gateway; actions intentionally stay unavailable. */
export async function previewResponse(request: Request): Promise<Response> {
  const path = new URL(request.url).pathname;
  if (['/api/v1/auth/refresh', '/api/v1/auth/login'].includes(path)) return json(sampleSession);
  if (path === '/api/v1/auth/logout') return new Response(null, { status: 204 });
  if (request.method !== 'GET' && request.method !== 'HEAD') return json({
    status: 403, title: 'Preview only', detail: 'Sample data preview: changes and device actions are disabled.',
  }, 403);
  if (path === '/api/v1/config/diff') return json({ baseRevision: 1, changes: [] });
  if (path === '/api/v1/config/lock') return json({ locked: false, owner: null, ownerId: null, lockedAt: null, lastActivity: null, expiresAt: null });
  if (path === '/api/v1/config/commit/pending') return json({ pending: null });
  if (path === '/api/v1/config/revisions') return json({ items: [], total: 0 });
  if (path === '/api/v1/config') return json(previewConfig);
  const prefix = ['/api/v1/config/candidate/', '/api/v1/config/running/'].find(p => path.startsWith(p));
  if (prefix) {
    const segments = decodeURIComponent(path.slice(prefix.length)).split('/').filter(Boolean);
    let value: unknown = previewConfig;
    for (const segment of segments) value = value && typeof value === 'object' ? (value as Record<string, unknown>)[segment] : undefined;
    return value === undefined ? json({ status: 404, title: 'No sample for this configuration path' }, 404) : json(value);
  }
  const state = previewState[path];
  if (state !== undefined) return json(state);
  return json({ status: 503, title: 'No sample data', detail: 'This device capability has no offline sample. Navigation and forms remain available.' }, 503);
}

export function installPreviewTransport(): void {
  const fetchAssets = globalThis.fetch.bind(globalThis);
  globalThis.fetch = async (input, init) => {
    const request = input instanceof Request ? new Request(input, init) : new Request(new URL(String(input), location.href), init);
    const url = new URL(request.url);
    if (url.pathname.startsWith('/api/')) return previewResponse(request);
    // No remote calls in offline preview, including action routes outside /api.
    if (url.origin !== location.origin || !['GET', 'HEAD'].includes(request.method)) return json({ status: 403, title: 'Offline preview blocks network actions' }, 403);
    return fetchAssets(request);
  };
}
