// Test helpers of the F-pki unit tests (imported by *.test.ts only; never by product code). The host's openssl is a
// VERIFIER here — an independent implementation that checks what our DER codec produces and produces what it parses.
import { execFileSync, spawnSync } from 'node:child_process'; // ALLOW: test-only verifier (openssl), never product code
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

/** A scratch directory under $NGFW_PKI_TEST_DIR (the slot's /run/ngfw-test/w<N>) or the OS temp dir; removed by `done`. */
export function scratch(): {
  dir: string;
  file: (name: string, content: string | Buffer) => string;
  done: () => void;
} {
  const base = process.env['NGFW_PKI_TEST_DIR'] ?? tmpdir();
  mkdirSync(base, { recursive: true });
  const dir = mkdtempSync(join(base, 'ngfw-pki-'));
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
  const options = { input, stdio: 'pipe' as const, encoding: 'utf8' as const };
  const result = spawnSync('openssl', args, options); // ALLOW: test-only fixed-argv verifier
  const output = `${result.stdout ?? ''}${result.stderr ?? ''}`;
  if (result.error !== undefined || result.status !== 0) {
    throw new Error(
      `openssl ${args[0]} exited ${result.status}: ${output}${result.error?.message ?? ''}${result.signal ? ` (signal ${result.signal})` : ''}`,
    );
  }
  return output;
}

/** Runs openssl and returns raw stdout bytes (DER outputs). */
export function opensslBytes(args: string[], input?: Buffer | string): Buffer {
  return execFileSync('openssl', args, { input, stdio: ['pipe', 'pipe', 'pipe'] }); // ALLOW: test-only verifier
}

describe('F-pki test kit', () => {
  it('retains successful command diagnostics written to stderr', () => {
    // OpenSSL help is written to stderr even though the command succeeds.
    expect(openssl(['req', '-help'])).toMatch(/Usage: req/);
  });
  it('throws on a failed verifier and retains its stderr diagnostic', () => {
    expect(() => openssl(['req', '-noout', '-verify'], 'not a CSR')).toThrow(
      /openssl req exited [1-9].*(?:Unable|unable|Could|could|Expecting|expecting)/s,
    );
  });
  it('has the host openssl as an independent verifier', () => {
    expect(openssl(['version'])).toMatch(/^OpenSSL 3\./);
  });
});
