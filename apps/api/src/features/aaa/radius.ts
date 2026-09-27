import { createHash, createHmac, randomBytes } from 'node:crypto';
import dgram from 'node:dgram';

/**
 * F-aaa: a minimal RADIUS Access-Request client (RFC 2865) with PAP and the Message-Authenticator attribute
 * (RFC 3579), implemented over UDP with node crypto — no shell, no external RADIUS library. Authenticates a
 * (username, password) against one server and returns Accept/Reject/unreachable plus the class/Filter-Id or
 * reply groups. Accounting and CHAP are out of scope for this increment.
 */

const ACCESS_REQUEST = 1;
const ACCESS_ACCEPT = 2;
const ACCESS_REJECT = 3;
const ATTR_USER_NAME = 1;
const ATTR_USER_PASSWORD = 2;
const ATTR_NAS_IDENTIFIER = 32;
const ATTR_MESSAGE_AUTHENTICATOR = 80;
const ATTR_CLASS = 25;
const ATTR_FILTER_ID = 11;
const ATTR_REPLY_MESSAGE = 18;

export interface RadiusServer {
  address: string;
  authPort: number;
  secret: string;
  timeoutMs: number;
  nasId?: string;
}

export type RadiusResult =
  | { status: 'accept'; groups: string[] }
  | { status: 'reject'; message?: string }
  | { status: 'unreachable'; error: string };

/** Encrypt the password per RFC 2865 §5.2 (the User-Password hiding, 16-byte blocks XORed with MD5(secret+prev)). */
function hidePassword(password: string, secret: string, authenticator: Buffer): Buffer {
  const pw = Buffer.from(password, 'utf8');
  const padLen = Math.ceil((pw.length || 1) / 16) * 16;
  const padded = Buffer.alloc(padLen);
  pw.copy(padded);
  const secretBuf = Buffer.from(secret, 'utf8');
  const out = Buffer.alloc(padLen);
  let prev = authenticator;
  for (let i = 0; i < padLen; i += 16) {
    const b = createHash('md5').update(secretBuf).update(prev).digest();
    const block = Buffer.alloc(16);
    for (let j = 0; j < 16; j++) block[j] = (padded[i + j] ?? 0) ^ (b[j] ?? 0);
    block.copy(out, i);
    prev = block;
  }
  return out;
}

function attr(type: number, value: Buffer): Buffer {
  const a = Buffer.alloc(2 + value.length);
  a[0] = type;
  a[1] = 2 + value.length;
  value.copy(a, 2);
  return a;
}

function buildRequest(
  server: RadiusServer,
  username: string,
  password: string,
): { packet: Buffer; authenticator: Buffer; id: number } {
  const id = randomBytes(1)[0] ?? 0;
  const authenticator = randomBytes(16);
  const attrs: Buffer[] = [
    attr(ATTR_USER_NAME, Buffer.from(username, 'utf8')),
    attr(ATTR_USER_PASSWORD, hidePassword(password, server.secret, authenticator)),
    attr(ATTR_NAS_IDENTIFIER, Buffer.from(server.nasId ?? 'vrx', 'utf8')),
    // Message-Authenticator: 16 zero bytes as a placeholder, filled in below (RFC 3579 §3.2)
    attr(ATTR_MESSAGE_AUTHENTICATOR, Buffer.alloc(16)),
  ];
  const body = Buffer.concat(attrs);
  const header = Buffer.alloc(20);
  header[0] = ACCESS_REQUEST;
  header[1] = id;
  header.writeUInt16BE(20 + body.length, 2);
  authenticator.copy(header, 4);
  const packet = Buffer.concat([header, body]);
  // fill Message-Authenticator = HMAC-MD5(secret, whole packet with the field zeroed)
  const maOffset = 20 + attrs[0]!.length + attrs[1]!.length + attrs[2]!.length + 2;
  const mac = createHmac('md5', server.secret).update(packet).digest();
  mac.copy(packet, maOffset);
  return { packet, authenticator, id };
}

/** Parse Class (25) and Filter-Id (11) reply attributes into group strings (used by the role map). */
function parseGroups(reply: Buffer): string[] {
  const groups: string[] = [];
  let i = 20;
  while (i + 2 <= reply.length) {
    const type = reply[i] ?? 0;
    const len = reply[i + 1] ?? 0;
    if (len < 2 || i + len > reply.length) break;
    const val = reply.subarray(i + 2, i + len);
    if (type === ATTR_CLASS || type === ATTR_FILTER_ID) groups.push(val.toString('utf8'));
    i += len;
  }
  return groups;
}

function parseReplyMessage(reply: Buffer): string | undefined {
  let i = 20;
  while (i + 2 <= reply.length) {
    const type = reply[i] ?? 0;
    const len = reply[i + 1] ?? 0;
    if (len < 2 || i + len > reply.length) break;
    if (type === ATTR_REPLY_MESSAGE) return reply.subarray(i + 2, i + len).toString('utf8');
    i += len;
  }
  return undefined;
}

/** Authenticate against one RADIUS server over PAP. */
export function radiusAuthenticate(
  server: RadiusServer,
  username: string,
  password: string,
): Promise<RadiusResult> {
  return new Promise((resolve) => {
    let settled = false;
    const socket = dgram.createSocket('udp4');
    const { packet, id } = buildRequest(server, username, password);
    const done = (r: RadiusResult) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      socket.close();
      resolve(r);
    };
    const timer = setTimeout(
      () => done({ status: 'unreachable', error: `no answer within ${server.timeoutMs} ms` }),
      server.timeoutMs,
    );
    socket.on('message', (msg) => {
      if (msg.length < 20 || msg[1] !== id) return;
      const code = msg[0];
      if (code === ACCESS_ACCEPT) {
        done({ status: 'accept', groups: parseGroups(msg) });
      } else if (code === ACCESS_REJECT) {
        const message = parseReplyMessage(msg);
        done(message ? { status: 'reject', message } : { status: 'reject' });
      }
    });
    socket.on('error', (e) => done({ status: 'unreachable', error: e.message }));
    socket.send(packet, server.authPort, server.address, (e) => {
      if (e) done({ status: 'unreachable', error: e.message });
    });
  });
}
