// Test helpers of the F-pki unit tests (imported by *.test.ts only; never by product code). The host's openssl is a
// VERIFIER here — an independent implementation that checks what our DER codec produces and produces what it parses.
import { execFileSync } from 'node:child_process'; // ALLOW: test-only verifier (openssl), never product code
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

/** A scratch directory under $VRX_PKI_TEST_DIR (the slot's /run/vrx-test/w<N>) or the OS temp dir; removed by `done`. */
export function scratch(): {
  dir: string;
  file: (name: string, content: string | Buffer) => string;
  done: () => void;
} {
  const base = process.env['VRX_PKI_TEST_DIR'] ?? tmpdir();
  mkdirSync(base, { recursive: true });
  const dir = mkdtempSync(join(base, 'vrx-pki-'));
  return {
    dir,
    file: (name, content) => {
      const p = join(dir, name);
      writeFileSync(p, content, { mode: 0o600 });
      return p;
    },
    done: () => rmSync(dir, { recursive: true, force: true }),
  };
}

/** Runs openssl with args (no shell); returns stdout+stderr; throws with the output on a non-zero exit. */
export function openssl(args: string[], input?: Buffer | string): string {
  try {
    const options = { input, stdio: 'pipe' as const, encoding: 'utf8' as const };
    return execFileSync('openssl', args, options); // ALLOW: test-only fixed-argv verifier
  } catch (e) {
    const err = e as { stdout?: string; stderr?: string; status?: number };
    throw new Error(
      `openssl ${args[0]} exited ${err.status}: ${err.stdout ?? ''}${err.stderr ?? ''}`,
    );
  }
}

/** Runs openssl and returns raw stdout bytes (DER outputs). */
export function opensslBytes(args: string[], input?: Buffer | string): Buffer {
  return execFileSync('openssl', args, { input, stdio: ['pipe', 'pipe', 'pipe'] }); // ALLOW: test-only verifier
}

describe('F-pki test kit', () => {
  it('has the host openssl as an independent verifier', () => {
    expect(openssl(['version'])).toMatch(/^OpenSSL 3\./);
  });
});
