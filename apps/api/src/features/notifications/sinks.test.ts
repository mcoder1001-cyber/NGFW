import { execFileSync } from 'node:child_process'; // ALLOW: fixed-argument openssl creates temporary test-only TLS certificate; no runtime exec, shell or committed key
import { createHmac } from 'node:crypto';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import net, { type Socket } from 'node:net';
import tls, { type TLSSocket } from 'node:tls';
import https from 'node:https';
import type * as Net from 'node:net';
import type * as Https from 'node:https';
import type * as Nodemailer from 'nodemailer';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { NotificationsSchema, type NotificationChannel } from '@ngfw/schema';
import { sendNotification } from './transport.js';

const boundary = vi.hoisted(() => ({
  ca: '',
  trust: true,
  lookup: vi.fn(),
  connections: vi.fn(),
  pins: vi.fn(),
}));
vi.mock('node:dns/promises', () => ({ lookup: boundary.lookup }));
vi.mock('node:net', async () => {
  const actual = await vi.importActual<typeof Net>('node:net');
  return {
    ...actual,
    default: actual,
    createConnection: (options: Net.NetConnectOpts) => {
      boundary.connections(options);
      // Production resolved/pinned address is asserted; test routing alone
      // reaches the loopback sink instead of a real recipient or relay.
      return actual.createConnection({ ...options, host: '127.0.0.1' });
    },
  };
});
vi.mock('node:https', async () => {
  const actual = await vi.importActual<typeof Https>('node:https');
  return {
    ...actual,
    default: {
      ...actual,
      request: (
        url: URL,
        options: Https.RequestOptions,
        callback: Parameters<typeof actual.request>[2],
      ) =>
        actual.request(
          url,
          {
            ...options,
            ...(boundary.trust ? { ca: boundary.ca } : {}),
            lookup: (host, lookupOptions, callback) => {
              options.lookup!(host, lookupOptions, (error, address, family) => {
                boundary.pins(address);
                if (error) return callback(error, []);
                if (lookupOptions.all) {
                  if (!Array.isArray(address))
                    return callback(new Error('invalid pinned lookup shape'), []);
                  return callback(
                    null,
                    address.map((row) => ({ ...row, address: '127.0.0.1' })),
                  );
                }
                if (typeof address !== 'string')
                  return callback(new Error('invalid pinned lookup shape'), '');
                return callback(null, '127.0.0.1', family);
              });
            },
          },
          callback,
        ),
    },
  };
});
vi.mock('nodemailer', async () => {
  const actual = await vi.importActual<typeof Nodemailer>('nodemailer');
  return {
    default: {
      ...actual.default,
      createTransport: (options: Record<string, unknown>) => {
        const tlsOptions = options['tls'] as Record<string, unknown>;
        return actual.default.createTransport({
          ...options,
          tls: {
            ...tlsOptions,
            ...(boundary.trust ? { ca: boundary.ca } : {}),
          },
        });
      },
    },
  };
});

let directory: string;
let key: Buffer;
let cert: Buffer;
const cleanup: Array<() => Promise<void>> = [];
beforeAll(() => {
  directory = mkdtempSync(join(tmpdir(), 'vrx-notification-sink-'));
  const keyPath = join(directory, 'key.pem');
  const certPath = join(directory, 'cert.pem');
  execFileSync( // ALLOW: fixed openssl arguments and locally generated temporary paths only; test certificate, no shell or runtime execution
    'openssl',
    [
      'req',
      '-x509',
      '-newkey',
      'rsa:2048',
      '-nodes',
      '-days',
      '1',
      '-subj',
      '/CN=relay.example.test',
      '-addext',
      'subjectAltName=DNS:relay.example.test,DNS:sink.example.test',
      '-keyout',
      keyPath,
      '-out',
      certPath,
    ],
    { stdio: 'ignore' },
  );
  key = readFileSync(keyPath);
  cert = readFileSync(certPath);
  boundary.ca = cert.toString();
});
afterEach(async () => {
  for (const close of cleanup.splice(0)) await close();
  vi.clearAllMocks();
  boundary.trust = true;
});
afterAll(() => rmSync(directory, { recursive: true, force: true }));

async function listen(server: net.Server): Promise<number> {
  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => resolve());
  });
  const sockets = new Set<Socket>();
  server.on('connection', (socket: Socket) => {
    sockets.add(socket);
    socket.once('close', () => sockets.delete(socket));
    socket.on('error', () => undefined);
  });
  cleanup.push(async () => {
    for (const socket of sockets) socket.destroy();
    await new Promise<void>((resolve) => server.close(() => resolve()));
  });
  return (server.address() as net.AddressInfo).port;
}

async function smtp(mode: 'tls' | 'starttls', authFailure = false, silent = false) {
  const messages: string[] = [];
  const authSecure: boolean[] = [];
  const session = (socket: Socket | TLSSocket, secured: boolean, greeting = true) => {
    socket.on('error', () => undefined);
    if (silent) return;
    if (greeting) socket.write('220 relay.example.test fixture\r\n');
    let buffer = '',
      data = false,
      message = '';
    const receive = (chunk: Buffer) => {
      buffer += chunk.toString();
      let end: number;
      while ((end = buffer.indexOf('\r\n')) >= 0) {
        const line = buffer.slice(0, end);
        buffer = buffer.slice(end + 2);
        if (data) {
          if (line === '.') {
            messages.push(message);
            data = false;
            socket.write('250 queued\r\n');
          } else message += line + '\n';
        } else if (line.startsWith('EHLO')) {
          socket.write(
            secured
              ? '250-relay.example.test\r\n250 AUTH PLAIN\r\n'
              : '250-relay.example.test\r\n250 STARTTLS\r\n',
          );
        } else if (line === 'STARTTLS') {
          socket.write('220 upgrade\r\n');
          socket.removeListener('data', receive);
          const upgraded = new tls.TLSSocket(socket, {
            isServer: true,
            secureContext: tls.createSecureContext({ key, cert }),
          });
          session(upgraded, true, false);
          return;
        } else if (line.startsWith('AUTH PLAIN ')) {
          authSecure.push(secured);
          socket.write(
            authFailure
              ? '535 authentication failed VRX_TEST_PSK_provider\r\n'
              : '235 authenticated\r\n',
          );
        } else if (line.startsWith('MAIL FROM:') || line.startsWith('RCPT TO:'))
          socket.write('250 ok\r\n');
        else if (line === 'DATA') {
          data = true;
          message = '';
          socket.write('354 message\r\n');
        } else if (line === 'QUIT') {
          socket.end('221 bye\r\n');
        } else socket.write('500 unknown\r\n');
      }
    };
    socket.on('data', receive);
  };
  const server =
    mode === 'tls'
      ? tls.createServer({ key, cert }, (socket) => session(socket, true))
      : net.createServer((socket) => session(socket, false));
  server.on('tlsClientError', () => undefined);
  return { port: await listen(server), messages, authSecure };
}

function email(port: number, mode: 'tls' | 'starttls'): NotificationChannel {
  return NotificationsSchema.parse({
    channels: [
      {
        name: 'relay',
        type: 'email',
        email: {
          smtpHost: 'relay.example.test',
          port,
          tls: mode,
          username: 'fixture-user',
          passwordRef: 'password/fixture',
          from: 'sender@example.invalid',
          to: ['recipient@example.invalid'],
        },
      },
    ],
    rules: [],
  }).channels[0]!;
}
function webhook(port: number): NotificationChannel {
  return NotificationsSchema.parse({
    channels: [
      {
        name: 'hook',
        type: 'webhook',
        webhook: {
          url: `https://sink.example.test:${port}/hook`,
          secretRef: 'token/fixture',
        },
      },
    ],
    rules: [],
  }).channels[0]!;
}
function resolveSink() {
  boundary.lookup.mockResolvedValue([{ address: '9.9.9.9', family: 4 }]);
}

describe('actual local notification sinks with test-only routing and trusted CA', () => {
  it.each(['tls', 'starttls'] as const)(
    'SMTP %s receives message and authenticates only after TLS',
    async (mode) => {
      const sink = await smtp(mode);
      resolveSink();
      await sendNotification(
        email(sink.port, mode),
        '{"kind":"alarm"}',
        async () => 'VRX_TEST_PSK_fixture',
        new AbortController().signal,
      );
      expect(sink.messages).toHaveLength(1);
      expect(sink.messages[0]).toContain('kind');
      expect(sink.authSecure).toEqual([true]);
      expect(boundary.connections.mock.calls[0]?.[0]).toMatchObject({
        host: '9.9.9.9',
        port: sink.port,
      });
    },
  );
  it('SMTP auth failure returns fixed error without provider secret', async () => {
    const sink = await smtp('starttls', true);
    resolveSink();
    await expect(
      sendNotification(
        email(sink.port, 'starttls'),
        '{}',
        async () => 'VRX_TEST_PSK_fixture',
        new AbortController().signal,
      ),
    ).rejects.toMatchObject({ reason: 'smtp-failed', message: 'smtp-failed' });
    expect(sink.messages).toHaveLength(0);
  });
  it('SMTP cancellation closes a silent greeting connection', async () => {
    const sink = await smtp('starttls', false, true);
    resolveSink();
    const abort = new AbortController();
    const timer = setTimeout(() => abort.abort(), 50);
    try {
      await expect(
        sendNotification(
          email(sink.port, 'starttls'),
          '{}',
          async () => 'VRX_TEST_PSK_fixture',
          abort.signal,
        ),
      ).rejects.toMatchObject({ reason: 'delivery-timeout' });
    } finally {
      clearTimeout(timer);
    }
  });
  it('SMTP rejects an untrusted TLS certificate', async () => {
    const sink = await smtp('tls');
    resolveSink();
    boundary.trust = false;
    await expect(
      sendNotification(
        email(sink.port, 'tls'),
        '{}',
        async () => 'VRX_TEST_PSK_fixture',
        new AbortController().signal,
      ),
    ).rejects.toMatchObject({ reason: 'smtp-failed' });
    expect(sink.authSecure).toHaveLength(0);
    expect(sink.messages).toHaveLength(0);
  });
  it.each([200, 503, 302])(
    'HTTPS signed POST gets real sink status %i without redirect following',
    async (status) => {
      const received: Array<{ body: string; signature: string | undefined }> = [];
      const server = https.createServer({ key, cert }, (request, response) => {
        let body = '';
        request.on('data', (chunk: Buffer) => {
          body += chunk.toString();
        });
        request.on('end', () => {
          received.push({
            body,
            signature: request.headers['x-vrx-signature'] as string | undefined,
          });
          response.writeHead(status, { location: 'https://example.invalid/redirect' });
          response.end('VRX_TEST_PSK_provider');
        });
      });
      resolveSink();
      const port = await listen(server);
      const send = sendNotification(
        webhook(port),
        '{"kind":"commit"}',
        async () => 'VRX_TEST_PSK_fixture',
        new AbortController().signal,
      );
      if (status === 200) await send;
      else
        await expect(send).rejects.toMatchObject({
          reason: `http-${status}`,
          message: `http-${status}`,
        });
      expect(boundary.pins).toHaveBeenCalledWith([{ address: '9.9.9.9', family: 4 }]);
      expect(received).toEqual([
        {
          body: '{"kind":"commit"}',
          signature:
            'sha256=' +
            createHmac('sha256', 'VRX_TEST_PSK_fixture').update('{"kind":"commit"}').digest('hex'),
        },
      ]);
    },
  );
  it('nondefault VRF and rejected webhook destination never read secrets or open sockets', async () => {
    resolveSink();
    const secret = vi.fn(async () => 'VRX_TEST_PSK_fixture');
    const unsupported = { ...webhook(443), vrf: 'management' } as unknown as NotificationChannel;
    await expect(
      sendNotification(unsupported, '{}', secret, new AbortController().signal),
    ).rejects.toMatchObject({ reason: 'management-vrf-unsupported' });
    expect(boundary.lookup).not.toHaveBeenCalled();
    boundary.lookup.mockResolvedValue([{ address: '127.0.0.1', family: 4 }]);
    await expect(
      sendNotification(webhook(443), '{}', secret, new AbortController().signal),
    ).rejects.toMatchObject({ reason: 'destination-rejected' });
    expect(secret).not.toHaveBeenCalled();
    expect(boundary.connections).not.toHaveBeenCalled();
  });
});
