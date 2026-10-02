import { afterEach, describe, expect, it, vi } from 'vitest';
import { NotificationsSchema } from '@ngfw/schema';
import { sendNotification } from './transport.js';

const mocks = vi.hoisted(() => ({
  lookup: vi.fn(),
  createTransport: vi.fn(),
  sendMail: vi.fn(async () => undefined),
  close: vi.fn(),
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
