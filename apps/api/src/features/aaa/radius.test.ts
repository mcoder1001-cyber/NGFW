import dgram from 'node:dgram';
import { createHash } from 'node:crypto';
import type { AddressInfo } from 'node:net';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { radiusAuthenticate } from './radius.js';

/** A tiny RADIUS server that accepts one (user,password) and rejects the rest, for the codec round-trip test. */
const SECRET = 'testing123';
const GOOD_USER = 'alice';
const GOOD_PASS = 's3cr3t';

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

let server: dgram.Socket;
let port: number;

beforeAll(async () => {
  server = dgram.createSocket('udp4');
  server.on('message', (msg, rinfo) => {
    const id = msg[1] ?? 0;
    const authenticator = msg.subarray(4, 20);
    let user = '';
    let pass = '';
    let hasMA = false;
    let i = 20;
    while (i + 2 <= msg.length) {
      const type = msg[i] ?? 0;
      const len = msg[i + 1] ?? 0;
      if (len < 2) break;
      const val = msg.subarray(i + 2, i + len);
      if (type === 1) user = val.toString('utf8');
      else if (type === 2) pass = decodePassword(val, SECRET, authenticator);
      else if (type === 80) hasMA = true;
      i += len;
    }
    const accept = hasMA && user === GOOD_USER && pass === GOOD_PASS;
    const code = accept ? 2 : 3;
    const gval = Buffer.from('netadm');
    const attrs = accept
      ? Buffer.concat([Buffer.from([25, 2 + gval.length]), gval])
      : Buffer.alloc(0);
    const header = Buffer.alloc(20);
    header[0] = code;
    header[1] = id;
    header.writeUInt16BE(20 + attrs.length, 2);
    // Response Authenticator = MD5(code+id+len+reqAuth+attrs+secret)
    const ra = createHash('md5')
      .update(Buffer.concat([header.subarray(0, 4), authenticator, attrs, Buffer.from(SECRET)]))
      .digest();
    ra.copy(header, 4);
    server.send(Buffer.concat([header, attrs]), rinfo.port, rinfo.address);
  });
  await new Promise<void>((r) => server.bind(0, '127.0.0.1', r));
  port = (server.address() as AddressInfo).port;
});
afterAll(() => server.close());

describe('RADIUS PAP client', () => {
  const srv = () => ({ address: '127.0.0.1', authPort: port, secret: SECRET, timeoutMs: 1000 });

  it('accepts the right password and returns reply groups (Class)', async () => {
    const r = await radiusAuthenticate(srv(), GOOD_USER, GOOD_PASS);
    expect(r).toEqual({ status: 'accept', groups: ['netadm'] });
  });

  it('rejects a wrong password', async () => {
    const r = await radiusAuthenticate(srv(), GOOD_USER, 'wrong');
    expect(r.status).toBe('reject');
  });

  it('reports unreachable when no server answers', async () => {
    const r = await radiusAuthenticate(
      { address: '127.0.0.1', authPort: 1, secret: SECRET, timeoutMs: 300 },
      GOOD_USER,
      GOOD_PASS,
    );
    expect(r.status).toBe('unreachable');
  });

  it('handles a long password (multi-block hiding)', async () => {
    const long = 'x'.repeat(40);
    const r = await radiusAuthenticate(srv(), GOOD_USER, long);
    expect(r.status).toBe('reject'); // decoded correctly but not the good password
  });
});
