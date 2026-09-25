import { generateKeyPairSync, sign, type KeyObject } from 'node:crypto';
import { canonicalJson, LICENSE_FORMAT, type License } from './format.js';

/** Test helpers: keys are generated per test run in memory, never written into the repository. */
export function testKeys(): { privateKey: KeyObject; publicKey: KeyObject; publicPem: string } {
  const { privateKey, publicKey } = generateKeyPairSync('ed25519');
  return {
    privateKey,
    publicKey,
    publicPem: publicKey.export({ type: 'spki', format: 'pem' }).toString(),
  };
}

const DAY = 86_400_000;

export function sampleLicense(now: Date, over: Partial<License> = {}): License {
  return {
    version: 1,
    licenseId: 'LIC-TEST-0001',
    customer: 'Example Customer',
    issuedAt: new Date(now.getTime() - DAY).toISOString(),
    notBefore: new Date(now.getTime() - DAY).toISOString(),
    expiresAt: new Date(now.getTime() + 100 * DAY).toISOString(),
    binding: {},
    entitlements: { features: ['ipsec', 'ha', 'bgp'], limits: { ipsecTunnels: 10 } },
    ...over,
  };
}

export function signFile(lic: unknown, key: KeyObject): string {
  const signature = sign(null, Buffer.from(canonicalJson(lic), 'utf8'), key).toString('base64');
  return JSON.stringify({ format: LICENSE_FORMAT, license: lic, signature }, null, 2);
}

export const days = (n: number) => n * DAY;
