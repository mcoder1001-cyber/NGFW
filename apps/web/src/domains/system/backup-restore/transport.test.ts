import { Blob as NodeBlob } from 'node:buffer';
import { session } from '../../../auth/session';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { archiveBase64, json, request } from './transport';
import { parseStatus } from './UpgradePage';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
describe('backup and upgrade transport', () => {
  it('keeps update uploads binary with the protocol filename header', async () => {
    vi.stubGlobal('Blob', NodeBlob);
    let sent: Request | undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (req: Request) => {
        sent = req;
        return new Response(JSON.stringify({ bundle: '/data/updates/safe.tar' }));
      }),
    );
    await request('/actions/upgrade-upload', {
      method: 'POST',
      headers: {
        'content-type': 'application/vnd.ngfw.update',
        'x-ngfw-filename': 'ngfw-update-1.2.3.tar',
      },
      body: new Blob(['update-bytes']),
    });
    expect(sent?.headers.get('content-type')).toBe('application/vnd.ngfw.update');
    expect(sent?.headers.get('x-ngfw-filename')).toBe('ngfw-update-1.2.3.tar');
    expect(await sent?.text()).toBe('update-bytes');
  });
  it('replays the binary blob after one authenticated session refresh', async () => {
    vi.stubGlobal('Blob', NodeBlob);
    let token = 'old-token';
    vi.spyOn(session, 'accessToken', 'get').mockImplementation(() => token);
    const refresh = vi.spyOn(session, 'handleUnauthorized').mockImplementation(async () => {
      token = 'fresh-token';
      return true;
    });
    const headers: string[] = [];
    const bodies: string[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (req: Request) => {
        headers.push(req.headers.get('authorization') ?? '');
        bodies.push(await req.text());
        return new Response('{}', { status: headers.length === 1 ? 401 : 200 });
      }),
    );
    await request('/actions/upgrade-upload', { method: 'POST', body: new Blob(['replay-bytes']) });
    expect(refresh).toHaveBeenCalledOnce();
    expect(headers).toEqual(['Bearer old-token', 'Bearer fresh-token']);
    expect(bodies).toEqual(['replay-bytes', 'replay-bytes']);
  });
  it('preserves problem details and field pointers for a rejected restore', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              status: 400,
              errors: [{ pointer: '/passphrase', message: 'invalid' }],
            }),
            { status: 400, headers: { 'content-type': 'application/problem+json' } },
          ),
      ),
    );
    await expect(
      json('/actions/restore', { archive: 'AA==', passphrase: 'test-only-phrase' }),
    ).rejects.toMatchObject({ status: 400, body: { errors: [{ pointer: '/passphrase' }] } });
  });
  it('rejects oversized archives before reading them', async () => {
    const arrayBuffer = vi.fn();
    await expect(
      archiveBase64({ size: 32 * 1024 * 1024 + 1, arrayBuffer } as unknown as File),
    ).rejects.toMatchObject({ status: 413 });
    expect(arrayBuffer).not.toHaveBeenCalled();
  });
  it('refuses an unsuccessful or malformed status instead of inventing slot values', () => {
    expect(() =>
      parseStatus({ lines: ['{}'], done: { exitCode: 0, summary: '', stats: {} } }),
    ).toThrow();
    expect(() =>
      parseStatus({ lines: [], done: { exitCode: 1, summary: 'unavailable', stats: {} } }),
    ).toThrow('unavailable');
    expect(
      parseStatus({
        lines: [
          JSON.stringify({
            active_slot: 'B',
            default_slot: 'A',
            versions: { A: '1.0.0', B: '1.1.0' },
            pending_slot: 'B',
            staged_slot: 'B',
            confirmed: false,
          }),
        ],
        done: { exitCode: 0, summary: '', stats: {} },
      }).active_slot,
    ).toBe('B');
  });
});
