import { afterEach, describe, expect, it, vi } from 'vitest';
import { Socket } from 'node:net';
import type * as Net from 'node:net';
import { NotificationsSchema } from '@ngfw/schema';
import { sendNotification } from './transport.js';

const mocks = vi.hoisted(() => ({
  lookup: vi.fn(),
  createConnection: vi.fn(),
  createTransport: vi.fn(),
  sendMail: vi.fn(async () => undefined),
  close: vi.fn(),
}));
vi.mock('node:net', async () => ({
  ...(await vi.importActual<typeof Net>('node:net')),
  createConnection: mocks.createConnection,
}));
vi.mock('node:dns/promises', () => ({ lookup: mocks.lookup }));
vi.mock('nodemailer', () => ({ default: { createTransport: mocks.createTransport } }));
afterEach(() => vi.clearAllMocks());
function channel(host = 'relay.example.com') {
  return NotificationsSchema.parse({
    channels: [
      {
        name: 'relay',
        type: 'email',
        email: {
          smtpHost: host,
          port: 587,
          tls: 'starttls',
          from: 'admin@example.com',
          to: ['ops@example.com'],
        },
      },
    ],
    rules: [],
  }).channels[0]!;
}
async function deliver(addresses: string[], host?: string) {
  mocks.lookup.mockResolvedValue(
    addresses.map((address) => ({ address, family: address.includes(':') ? 6 : 4 })),
  );
  mocks.createTransport.mockReturnValue({ sendMail: mocks.sendMail, close: mocks.close });
  await sendNotification(channel(host), '{}', vi.fn(), new AbortController().signal);
}
describe('SMTP destination policy before transport creation', () => {
  it.each([
    '127.0.0.1',
    '127.255.255.254',
    '0.0.0.0',
    '169.254.169.254',
    '::',
    '::1',
    '0:0:0:0:0:0:0:1',
    '::ffff:127.0.0.1',
    '::ffff:7f00:1',
    '::ffff:0.0.0.0',
    '::ffff:169.254.169.254',
    'fe80::1',
    'febf::1',
    'fe90::1',
    'fe80:0:0:0:0:0:0:1',
    'ff02::1',
    '224.0.0.1',
  ])('rejects schema-valid destination %s', async (address) => {
    await expect(deliver([address], address)).rejects.toThrow('destination-rejected');
    expect(mocks.createTransport).not.toHaveBeenCalled();
  });
  it('rejects a mixed DNS answer set before opening the approved first address', async () => {
    await expect(deliver(['10.1.2.3', '::ffff:127.0.0.1'])).rejects.toThrow('destination-rejected');
    expect(mocks.createTransport).not.toHaveBeenCalled();
  });
  it.each([
    '10.1.2.3',
    '172.16.1.2',
    '192.168.1.2',
    'fd00::25',
    '::ffff:192.168.1.2',
    '2606:4700::1111',
  ])('preserves relay %s and pins its approved DNS address', async (address) => {
    await deliver([address]);
    expect(mocks.createTransport).toHaveBeenCalledWith(
      expect.objectContaining({
        host: address,
        requireTLS: true,
        tls: { servername: 'relay.example.com', rejectUnauthorized: true },
      }),
    );
    expect(mocks.sendMail).toHaveBeenCalledTimes(1);
  });
});

describe('SMTP authentication configuration', () => {
  it.each(['username', 'passwordRef'] as const)('rejects partial auth containing only %s before DNS or transport', async (present) => {
    const relay = channel();
    if (present === 'username') relay.email!.username = 'relay-user';
    else relay.email!.passwordRef = 'password/relay';
    const readSecret = vi.fn();
    await expect(sendNotification(relay, '{}', readSecret, new AbortController().signal)).rejects.toThrow('channel-invalid');
    expect(mocks.lookup).not.toHaveBeenCalled();
    expect(readSecret).not.toHaveBeenCalled();
    expect(mocks.createTransport).not.toHaveBeenCalled();
  });
  it('passes both configured credentials to SMTP', async () => {
    const relay = channel();
    relay.email!.username = 'relay-user';
    relay.email!.passwordRef = 'password/relay';
    mocks.lookup.mockResolvedValue([{ address: '10.1.2.3', family: 4 }]);
    mocks.createTransport.mockReturnValue({ sendMail: mocks.sendMail, close: mocks.close });
    const readSecret = vi.fn(async () => 'NGFW_TEST_PSK_SMTP');
    await sendNotification(relay, '{}', readSecret, new AbortController().signal);
    expect(readSecret).toHaveBeenCalledWith('password/relay');
    expect(mocks.createTransport).toHaveBeenCalledWith(expect.objectContaining({
      auth: { user: 'relay-user', pass: 'NGFW_TEST_PSK_SMTP' },
    }));
  });
  it('omits auth for an explicitly unauthenticated relay', async () => {
    await deliver(['10.1.2.3']);
    expect(mocks.createTransport.mock.calls[0]![0]).not.toHaveProperty('auth');
  });
});

describe('SMTP abort owns the active TCP socket', () => {
  it.each([false, true])('destroys socket when aborted (connected=%s)', async (connected) => {
    const socket = new Socket();
    mocks.createConnection.mockReturnValue(socket);
    const controller = new AbortController();
    let started!: () => void;
    const ready = new Promise<void>((resolve) => {
      started = resolve;
    });
    mocks.lookup.mockResolvedValue([{ address: '10.1.2.3', family: 4 }]);
    mocks.createTransport.mockImplementation(
      (options: {
        getSocket: (
          opts: unknown,
          done: (error: Error | null, result?: { connection: Socket }) => void,
        ) => void;
      }) => ({
        close: mocks.close,
        sendMail: () =>
          new Promise<void>((_resolve, reject) => {
            options.getSocket({}, (error, result) => {
              if (error) {
                reject(error);
                return;
              }
              result!.connection.once('error', reject);
            });
            if (connected) socket.emit('connect');
            started();
          }),
      }),
    );
    const result = sendNotification(channel(), '{}', vi.fn(), controller.signal);
    const rejection = expect(result).rejects.toThrow('delivery-timeout');
    await ready;
    expect(socket.destroyed).toBe(false);
    controller.abort();
    await rejection;
    expect(socket.destroyed).toBe(true);
  });
});
