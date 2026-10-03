import { writeFileSync } from 'node:fs';
import { inject } from 'vitest';
import { sampleLicense, signFile, testKeys } from '../../src/features/licensing/testkit.js';

/**
 * A licence for one e2e file (E2E-red-main): a gated feature's e2e (BGP, WireGuard, …) commits configuration that the
 * community set refuses since F-licensing. The licence is signed with a key generated for this run (never written to
 * the repository), its public half is trusted through NGFW_LICENSE_PUBLIC_KEYS, and the file lives in the slot's run
 * dir. Call before startHarness (the licensing module reads the environment when the app is created); the returned
 * function restores the environment.
 */
export function useTestLicense(features: string[], limits: Record<string, number> = {}): () => void {
  const keys = testKeys();
  const file = `${inject('runDir')}/e2e-license-${process.pid}.ngfwlic`;
  const lic = sampleLicense(new Date(), {
    licenseId: 'LIC-E2E',
    entitlements: { features, limits },
  });
  writeFileSync(file, signFile(lic, keys.privateKey), { mode: 0o600 });
  const before = {
    NGFW_LICENSE_FILE: process.env['NGFW_LICENSE_FILE'],
    NGFW_LICENSE_PUBLIC_KEYS: process.env['NGFW_LICENSE_PUBLIC_KEYS'],
  };
  process.env['NGFW_LICENSE_FILE'] = file;
  process.env['NGFW_LICENSE_PUBLIC_KEYS'] = keys.publicPem.replace(/\n/g, '\\n');
  return () => {
    for (const [k, v] of Object.entries(before)) {
      if (v === undefined) delete process.env[k];
      else process.env[k] = v;
    }
  };
}
