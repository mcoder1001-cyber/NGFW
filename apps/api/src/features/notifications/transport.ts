import { createHmac } from 'node:crypto';
import { lookup } from 'node:dns/promises';
import https from 'node:https';
import { createConnection, isIP, type Socket } from 'node:net';
import nodemailer from 'nodemailer';
import { parseIpv4, parseIpv6, type NotificationChannel } from '@ngfw/schema';

/** Never include provider errors, URLs, recipients or secret material in public delivery history. */
export class DeliveryError extends Error {
  constructor(public readonly reason: string) {
    super(reason);
  }
}

/** Canonical bytes, including expanded/mapped IPv6. Only ordinary global unicast webhook destinations. */
export function publicAddress(address: string): boolean {
  const family = isIP(address);
  if (family === 4) {
    const p = address.split('.').map(Number);
    const a = p[0] ?? 0,
      b = p[1] ?? 0;
    return !(
      a === 0 ||
      a === 10 ||
      a === 127 ||
      a >= 224 ||
      (a === 100 && b >= 64 && b <= 127) ||
      (a === 169 && b === 254) ||
      (a === 172 && b >= 16 && b <= 31) ||
      (a === 192 && (b === 168 || b === 0 || b === 2)) ||
      (a === 198 && (b === 18 || b === 19 || b === 51)) ||
      (a === 203 && b === 0)
    );
  }
  if (family !== 6 || address.includes('%')) return false;
  const u = new URL(`https://[${address}]/`).hostname.slice(1, -1).toLowerCase();
  // URL normalizes expanded zeros; mapped addresses are rejected, even a public mapped IPv4.
  if (u.startsWith('::')) return false;
  const first = Number.parseInt(u.split(':')[0] ?? '', 16);
  return (
    first >= 0x2000 &&
    first <= 0x3fff &&
    !u.startsWith('2001:db8:') &&
    !u.startsWith('2002:') &&
    !(first === 0x2001 && Number.parseInt(u.split(':')[1] || '0', 16) < 0x200)
  );
}

/** SMTP permits private relays, but never unspecified, loopback, link-local or multicast destinations. */
export function smtpAddress(address: string): boolean {
  if (address.includes('%') || !isIP(address)) return false;
  const allowedV4 = (v: bigint): boolean =>
    v !== 0n && v >> 24n !== 127n && v >> 16n !== 0xa9fen && v >> 28n < 14n;
  const v4 = parseIpv4(address);
  if (v4 !== undefined) return allowedV4(v4);
  const v6 = parseIpv6(address);
  if (v6 === undefined) return false;
  // IPv4-mapped addresses must obey the embedded IPv4 policy, regardless of spelling.
  if (v6 >> 32n === 0xffffn) return allowedV4(v6 & 0xffffffffn);
  return v6 !== 0n && v6 !== 1n && v6 >> 118n !== 0x3fan && v6 >> 120n !== 0xffn;
}

export async function resolvePublic(host: string): Promise<{ address: string; family: number }> {
  const h = host.replace(/^\[|\]$/g, '');
  const rows = isIP(h)
    ? [{ address: h, family: isIP(h) }]
    : await lookup(h, { all: true, verbatim: true });
  if (!rows.length || rows.some((r) => !publicAddress(r.address)))
    throw new DeliveryError('destination-rejected');
  return rows[0]!;
}

async function webhook(
  url: string,
  body: string,
  secret: string,
  signal: AbortSignal,
): Promise<void> {
  const u = new URL(url);
  if (u.protocol !== 'https:' || u.username || u.password || u.hash)
    throw new DeliveryError('destination-rejected');
  const pinned = await resolvePublic(u.hostname);
  if (signal.aborted) throw new DeliveryError('delivery-timeout');
  await new Promise<void>((resolve, reject) => {
    const req = https.request(
      u,
      {
        method: 'POST',
        signal,
        agent: false,
        rejectUnauthorized: true,
        lookup: (_host, _opts, cb) => cb(null, pinned.address, pinned.family),
        headers: {
          'content-type': 'application/json',
          'content-length': Buffer.byteLength(body),
          'x-vrx-signature': `sha256=${createHmac('sha256', secret).update(body).digest('hex')}`,
        },
      },
      (res) => {
        // No redirects, response bodies are neither buffered nor logged.
        const status = res.statusCode ?? 0;
        res.destroy();
        if (status >= 200 && status < 300) resolve();
        else reject(new DeliveryError(`http-${status}`));
      },
    );
    req.on('error', () =>
      reject(new DeliveryError(signal.aborted ? 'delivery-timeout' : 'transport-failed')),
    );
    req.end(body);
  });
}

export async function sendNotification(
  channel: NotificationChannel,
  body: string,
  readSecret: (ref: string) => Promise<string>,
  signal: AbortSignal,
): Promise<void> {
  if (channel.type === 'webhook' && channel.webhook) {
    return webhook(channel.webhook.url, body, await readSecret(channel.webhook.secretRef), signal);
  }
  const e = channel.email;
  if (channel.type !== 'email' || !e) throw new DeliveryError('channel-invalid');
  // SMTP may be an on-premise relay. DNS is pinned, TLS hostname and certificate validation remain original.
  const rows = await lookup(e.smtpHost, { all: true, verbatim: true });
  const pinned = rows[0];
  if (!pinned || rows.some((r) => !smtpAddress(r.address)))
    throw new DeliveryError('destination-rejected');
  const password = e.passwordRef ? await readSecret(e.passwordRef) : undefined;
  if (signal.aborted) throw new DeliveryError('delivery-timeout');
  let socket: Socket | undefined;
  const transport = nodemailer.createTransport({
    getSocket: (
      _options: unknown,
      done: (err: Error | null, options?: { connection: Socket }) => void,
    ) => {
      if (signal.aborted) {
        done(new DeliveryError('delivery-timeout'));
        return;
      }
      const owned = createConnection({ host: pinned.address, port: e.port });
      socket = owned;
      // TLS upgrade may remove nodemailer's plain-socket listeners; cancellation must never
      // surface an unhandled error on the raw socket. Connection errors are handled below.
      owned.on('error', () => undefined);
      const failed = (error: Error) => done(error);
      owned.once('error', failed);
      owned.once('connect', () => {
        owned.removeListener('error', failed);
        if (signal.aborted) {
          owned.destroy();
          done(new DeliveryError('delivery-timeout'));
          return;
        }
        // Nodemailer still performs TLS/STARTTLS with the original server name.
        done(null, { connection: owned });
      });
    },
    host: pinned.address,
    port: e.port,
    secure: e.tls === 'tls',
    requireTLS: e.tls === 'starttls',
    tls: { servername: e.smtpHost, rejectUnauthorized: true },
    ...(e.username && password ? { auth: { user: e.username, pass: password } } : {}),
    connectionTimeout: 5000,
    greetingTimeout: 5000,
    socketTimeout: 5000,
    pool: false,
  });
  const abort = () => {
    socket?.destroy(new DeliveryError('delivery-timeout'));
    transport.close();
  };
  signal.addEventListener('abort', abort, { once: true });
  try {
    await transport.sendMail({
      from: e.from,
      to: e.to,
      subject: 'VRX notification',
      text: body,
      disableFileAccess: true,
      disableUrlAccess: true,
    });
    if (signal.aborted) throw new DeliveryError('delivery-timeout');
  } catch {
    throw new DeliveryError(signal.aborted ? 'delivery-timeout' : 'smtp-failed');
  } finally {
    signal.removeEventListener('abort', abort);
    socket?.destroy();
    transport.close();
  }
}
