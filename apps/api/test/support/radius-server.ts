import { createHash } from 'node:crypto';
import dgram from 'node:dgram';
import type { AddressInfo } from 'node:net';

/** One account the test server knows: the password it accepts and the groups it replies with. */
export interface RadiusAccount {
  password: string;
  /** Sent back as Class attributes (what `parseGroups` reads). */
  groups?: string[];
}

export interface RadiusTestServer {
  port: number;
  /** Usernames this server was asked about, in order — so a test can assert it was never consulted. */
  seen: string[];
  close(): void;
}

/** Undo RFC 2865 §5.2 User-Password hiding, so the fake server can compare the plain text. */
function decodePassword(enc: Buffer, secret: string, authenticator: Buffer): string {
  const out = Buffer.alloc(enc.length);
  let prev = authenticator;
  for (let i = 0; i < enc.length; i += 16) {
    const b = createHash('md5').update(secret).update(prev).digest();
    for (let j = 0; j < 16; j++) out[i + j] = (enc[i + j] ?? 0) ^ (b[j] ?? 0);
    prev = enc.subarray(i, i + 16);
  }
  return out.toString('utf8').replace(/\0+$/, '');
}

/**
 * A minimal RADIUS server on 127.0.0.1 for e2e tests: Access-Accept with Class groups for a known
 * (username, password), Access-Reject otherwise. It records every username it was asked about, which is how a test
 * proves the login walk did NOT reach the directory.
 */
export async function startRadius(
  secret: string,
  accounts: Record<string, RadiusAccount>,
): Promise<RadiusTestServer> {
  const seen: string[] = [];
  const socket = dgram.createSocket('udp4');
  socket.on('message', (msg, rinfo) => {
    const id = msg[1] ?? 0;
    const authenticator = msg.subarray(4, 20);
    let user = '';
    let password = '';
    let i = 20;
    while (i + 2 <= msg.length) {
      const type = msg[i] ?? 0;
      const len = msg[i + 1] ?? 0;
      if (len < 2) break;
      const val = msg.subarray(i + 2, i + len);
      if (type === 1) user = val.toString('utf8');
      else if (type === 2) password = decodePassword(val, secret, authenticator);
      i += len;
    }
    seen.push(user);
    const account = accounts[user];
    const ok = account !== undefined && account.password === password;
    const attrs = ok
      ? Buffer.concat(
          (account.groups ?? []).map((g) => {
            const v = Buffer.from(g, 'utf8');
            return Buffer.concat([Buffer.from([25, 2 + v.length]), v]);
          }),
        )
      : Buffer.alloc(0);
    const header = Buffer.alloc(20);
    header[0] = ok ? 2 : 3; // Access-Accept / Access-Reject
    header[1] = id;
    header.writeUInt16BE(20 + attrs.length, 2);
    createHash('md5')
      .update(Buffer.concat([header.subarray(0, 4), authenticator, attrs, Buffer.from(secret)]))
      .digest()
      .copy(header, 4);
    socket.send(Buffer.concat([header, attrs]), rinfo.port, rinfo.address);
  });
  await new Promise<void>((r) => socket.bind(0, '127.0.0.1', r));
  return {
    port: (socket.address() as AddressInfo).port,
    seen,
    close: () => socket.close(),
  };
}

/**
 * A UDP port on 127.0.0.1 with nothing listening — a server that never answers. Bind and immediately close, so the
 * port is (almost certainly) free and the client's send either draws ICMP port-unreachable or times out.
 */
export async function deadPort(): Promise<number> {
  const s = dgram.createSocket('udp4');
  await new Promise<void>((r) => s.bind(0, '127.0.0.1', r));
  const port = (s.address() as AddressInfo).port;
  await new Promise<void>((r) => s.close(() => r()));
  return port;
}
