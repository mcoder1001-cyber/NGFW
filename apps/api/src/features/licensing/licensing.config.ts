import type { Entitlements } from './entitlements.js';
/**
 * Licensing configuration (kept in the feature directory; apps/api/src/config.ts is shared).
 *
 * PRODUCT_PUBLIC_KEYS: the Ed25519 public key(s) the API build trusts. The key below is a PLACEHOLDER generated for
 * development; its private half was discarded, so no licence verifies against it. Release engineering replaces it
 * with the product key whose private half lives offline with support (docs/user/system/licensing.md). Several keys
 * may be listed for key rotation. VRX_LICENSE_PUBLIC_KEYS (comma-separated PEMs, `\n` escapes allowed) replaces
 * this list when set.
 */
export const PRODUCT_PUBLIC_KEYS: readonly string[] = [
  `-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEALR/JBpNQfFbGNAdIRgUMaDpPzrkPZkbavNmpQC7CP2o=
-----END PUBLIC KEY-----
`,
];

export const LICENSING_OPTIONS = Symbol('VRX_LICENSING_OPTIONS');

export interface LicensingOptions {
  /** Where the uploaded `.vrxlic` is stored (VRX_LICENSE_FILE). */
  file: string;
  /**
   * Extra trusted public key (PEM file path, VRX_LICENSE_PUBKEY_FILE) — development and tests only; production uses
   * PRODUCT_PUBLIC_KEYS. Not a tamper-resistance boundary (out of scope, D-059).
   */
  extraPublicKeyFile?: string | undefined;
  /** Serial override (VRX_LICENSE_SERIAL); otherwise /sys/class/dmi/id/product_serial. */
  serial?: string | undefined;
  machineIdFile: string;
  dmiSerialFile: string;
  /** Re-evaluation period for the expiry event (ms). */
  checkIntervalMs: number;
  now: () => Date;
  /** Trusted product keys (VRX_LICENSE_PUBLIC_KEYS); replaces PRODUCT_PUBLIC_KEYS when set. */
  publicKeys?: readonly string[] | undefined;
  /** Community entitlements; default COMMUNITY (no gated feature). Tests may pass SAMPLE_COMMUNITY. */
  community?: Entitlements | undefined;
}

/** Parse VRX_LICENSE_PUBLIC_KEYS: comma-separated PEM public keys; undefined when unset or empty. */
export function parsePublicKeys(raw: string | undefined): string[] | undefined {
  if (!raw) return undefined;
  const keys = raw
    .split(',')
    .map((k) => k.replace(/\\n/g, '\n').trim())
    .filter((k) => k.length > 0)
    .map((k) => `${k}\n`);
  return keys.length > 0 ? keys : undefined;
}

export function licensingOptionsFromEnv(env: NodeJS.ProcessEnv = process.env): LicensingOptions {
  return {
    file: env.VRX_LICENSE_FILE || '/var/lib/vrx/license.vrxlic',
    extraPublicKeyFile: env.VRX_LICENSE_PUBKEY_FILE || undefined,
    serial: env.VRX_LICENSE_SERIAL || undefined,
    publicKeys: parsePublicKeys(env.VRX_LICENSE_PUBLIC_KEYS),
    machineIdFile: '/etc/machine-id',
    dmiSerialFile: '/sys/class/dmi/id/product_serial',
    checkIntervalMs: 3_600_000,
    now: () => new Date(),
  };
}
