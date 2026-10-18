import { afterEach, describe, expect, it, vi } from 'vitest';
import { installPreviewTransport, previewResponse } from './transport';

afterEach(() => vi.unstubAllGlobals());

describe('offline UI preview isolation', () => {
  it('never forwards API requests or remote actions to the original fetch', async () => {
    const network = vi.fn(async () => new Response('asset'));
    vi.stubGlobal('fetch', network);
    installPreviewTransport();
    await fetch('http://gateway.invalid/api/v1/config/commit', { method: 'POST' });
    await fetch('http://gateway.invalid/api/v1/state/interfaces');
    await fetch('http://gateway.invalid/action', { method: 'POST' });
    expect(network).not.toHaveBeenCalled();
  });
  it('restores only a sample session', async () => {
    const response = await previewResponse(new Request('http://localhost/api/v1/auth/refresh', { method: 'POST' }));
    expect((await response.json()).user.username).toBe('UI preview');
  });
  it('blocks configuration writes and device actions', async () => {
    for (const [method, path] of [['PATCH', 'config/candidate/interfaces'], ['POST', 'config/commit'], ['POST', 'auth/password'], ['DELETE', 'state/interfaces']] as const) {
      const response = await previewResponse(new Request(`http://localhost/api/v1/${path}`, { method }));
      expect(response.status).toBe(403);
      expect((await response.json()).detail).toContain('disabled');
    }
  });
  it('reports missing samples instead of making a real API request', async () => {
    const response = await previewResponse(new Request('http://localhost/api/v1/not-sampled'));
    expect(response.status).toBe(503);
    expect((await response.json()).title).toBe('No sample data');
  });
});
