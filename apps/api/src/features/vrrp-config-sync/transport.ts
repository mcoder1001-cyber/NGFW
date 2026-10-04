import { createHash, createHmac, randomUUID, timingSafeEqual } from 'node:crypto';
import { request } from 'node:https';
import type { TLSSocket } from 'node:tls';
import type { Doc } from '../../datastore/repo.js';

export interface Envelope {
  origin: string;
  revision: number;
  timestamp: number;
  nonce: string;
  document: Doc;
}
export const signature = (body: string, key: string) =>
  createHmac('sha256', key).update(body).digest('hex');
export function validSignature(body: string, key: string, mac: string): boolean {
  const want = signature(body, key);
  return /^[a-f0-9]{64}$/.test(mac) && timingSafeEqual(Buffer.from(want), Buffer.from(mac));
}
export function sendPinned(
  address: string,
  port: number,
  pin: string,
  key: string,
  origin: string,
  revision: number,
  document: Doc,
  signal: AbortSignal,
): Promise<{ revision: number; role: string }> {
  const body = JSON.stringify({
    origin,
    revision,
    timestamp: Date.now(),
    nonce: randomUUID(),
    document,
  } satisfies Envelope);
  const host = address.includes(':') ? `[${address}]` : address;
  if (!/^[a-f0-9]{64}$/.test(pin))
    return Promise.reject(new Error('peer certificate pin is missing or invalid'));
  return new Promise((resolve, reject) => {
    // A pinned certificate is the trust anchor. No HTTP fallback, redirects or unpinned request bytes.
    const req = request(
      `https://${host}:${port}/api/v1/actions/ha/receive`,
      {
        method: 'POST',
        rejectUnauthorized: false,
        minVersion: 'TLSv1.2',
        agent: false,
        signal: AbortSignal.any([signal, AbortSignal.timeout(8000)]),
        headers: {
          'content-type': 'application/json',
          'content-length': Buffer.byteLength(body),
          'x-ngfw-cluster-signature': signature(body, key),
        },
      },
      (res) => {
        const chunks: Buffer[] = [];
        let size = 0;
        res.on('data', (part: Buffer) => {
          size += part.length;
          if (size > 65536) {
            req.destroy(new Error('peer response too large'));
            return;
          }
          chunks.push(part);
        });
        res.on('end', () => {
          if (res.statusCode !== 201 && res.statusCode !== 200) {
            reject(new Error(`peer refused sync (HTTP ${res.statusCode})`));
            return;
          }
          try {
            const result: unknown = JSON.parse(Buffer.concat(chunks).toString());
            const rev = (result as { revision?: { id?: unknown } }).revision?.id;
            if (typeof rev !== 'number') throw new Error('peer returned no committed revision');
            const role = (result as { role?: unknown }).role;
            resolve({
              revision: rev,
              role:
                typeof role === 'string' && ['master', 'backup', 'unknown'].includes(role)
                  ? role
                  : 'unknown',
            });
          } catch (e) {
            reject(e);
          }
        });
      },
    );
    req.setTimeout(8000, () => req.destroy(new Error('peer sync timeout')));
    req.on('socket', (socket) => {
      (socket as TLSSocket).once('secureConnect', () => {
        const cert = (socket as TLSSocket).getPeerCertificate();
        const now = Date.now(),
          start = Date.parse(cert.valid_from),
          end = Date.parse(cert.valid_to);
        if (!Number.isFinite(start) || !Number.isFinite(end) || start > now || end < now) {
          req.destroy(new Error('peer TLS certificate outside validity period'));
          return;
        }

        if (!cert.raw || createHash('sha256').update(cert.raw).digest('hex') !== pin) {
          req.destroy(new Error('peer TLS certificate pin mismatch'));
          return;
        }
        req.end(body);
      });
    });
    req.on('error', reject);
  });
}
