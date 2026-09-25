/**
 * Licensing configuration (kept in the feature directory; apps/api/src/config.ts is shared).
 *
 * PRODUCT_PUBLIC_KEYS: the Ed25519 public key(s) the API build trusts. The key below is a PLACEHOLDER generated for
 * development; its private half was discarded, so no licence verifies against it. Release engineering replaces it
 * with the product key whose private half lives offline with support (docs/user/system/licensing.md). Several keys
 * may be listed for key rotation.
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
}

export function licensingOptionsFromEnv(env: NodeJS.ProcessEnv = process.env): LicensingOptions {
  return {
    file: env.VRX_LICENSE_FILE || '/var/lib/vrx/license.vrxlic',
    extraPublicKeyFile: env.VRX_LICENSE_PUBKEY_FILE || undefined,
    serial: env.VRX_LICENSE_SERIAL || undefined,
    machineIdFile: '/etc/machine-id',
    dmiSerialFile: '/sys/class/dmi/id/product_serial',
    checkIntervalMs: 3_600_000,
    now: () => new Date(),
  };
}
