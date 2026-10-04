import { EventEmitter } from 'node:events';
import { createHash } from 'node:crypto';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { sendPinned } from './transport.js';
const fixture = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock('node:https', () => ({ request: fixture.request }));

describe('pinned TLS transport', () => {
  let req: EventEmitter & {
    end: ReturnType<typeof vi.fn>;
    destroy: ReturnType<typeof vi.fn>;
    setTimeout: ReturnType<typeof vi.fn>;
  };
  let socket: EventEmitter & { getPeerCertificate: ReturnType<typeof vi.fn> };
  const raw = Buffer.from('test-leaf-certificate'),
    pin = createHash('sha256').update(raw).digest('hex');
  beforeEach(() => {
    req = Object.assign(new EventEmitter(), {
      end: vi.fn(),
      setTimeout: vi.fn(),
      destroy: vi.fn((error: Error) => req.emit('error', error)),
    });
    socket = Object.assign(new EventEmitter(), {
      getPeerCertificate: vi.fn(() => ({ raw, valid_from: '2020-01-01', valid_to: '2099-01-01' })),
    });
    fixture.request.mockReset();
    fixture.request.mockReturnValue(req);
  });
  const send = (expectedPin = pin) =>
    sendPinned(
      '192.0.2.1',
      4370,
      expectedPin,
      'NGFW_TEST_PSK_cluster_auth_fixture',
      'node-a',
      1,
      {},
      new AbortController().signal,
    );
  it('never writes payload or authentication bytes before validating the pinned TLS leaf', async () => {
    const result = send();
    req.emit('socket', socket);
    expect(req.end).not.toHaveBeenCalled();
    socket.emit('secureConnect');
    expect(req.end).toHaveBeenCalledTimes(1);
    const [url, options, onResponse] = fixture.request.mock.calls[0]!;
    expect(url).toMatch(/^https:/);
    expect(options.agent).toBe(false);
    expect(options.minVersion).toBe('TLSv1.2');
    const res = Object.assign(new EventEmitter(), { statusCode: 201 });
    onResponse(res);
    res.emit('data', Buffer.from('{"revision":{"id":9},"role":"backup"}'));
    res.emit('end');
    await expect(result).resolves.toEqual({ revision: 9, role: 'backup' });
  });
  it('rejects wrong pins before request bytes are written', async () => {
    const result = send('f'.repeat(64));
    req.emit('socket', socket);
    socket.emit('secureConnect');
    await expect(result).rejects.toThrow('pin mismatch');
    expect(req.end).not.toHaveBeenCalled();
  });
  it('rejects expired pinned certificates', async () => {
    socket.getPeerCertificate.mockReturnValue({
      raw,
      valid_from: '2020-01-01',
      valid_to: '2021-01-01',
    });
    const result = send();
    req.emit('socket', socket);
    socket.emit('secureConnect');
    await expect(result).rejects.toThrow('validity');
    expect(req.end).not.toHaveBeenCalled();
  });
  it('rejects absent pins without opening a connection', async () => {
    await expect(send('')).rejects.toThrow('pin');
    expect(fixture.request).not.toHaveBeenCalled();
  });
});
