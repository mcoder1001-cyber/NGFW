import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { existsSync, mkdirSync, rmSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { AgentClient } from '../../src/agent/agent.client.js';
import { loadEnv } from '../../src/config.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/** Passwords of this run only — generated, never literal (gitleaks, 00-CONTEXT secrets rule). */
const PW = { ro: runSecret() };

/**
 * P06 §10 agent e2e against the REAL agent (P05) and the real VPP on this host. Runs only with VRX_INTEGRATION=1,
 * under the shared lab lock (tools/ci.sh full holds it; by hand: `tools/lab lock shared pnpm test:integration`), and
 * only when the agent binary (VRX_AGENT_BIN, else apps/agent/bin/vrx-agent) actually serves `vrx.v1.Dataplane`: the
 * probe below starts it as VRX_OWNER=<prefix> on the slot socket and calls Health. Before P05 merges, main's binary is
 * a skeleton that never opens the socket → the suite is skipped with that reason (the fake-agent e2e covers the API
 * side meanwhile). Objects: loop<slot>xx / 10.<slot>.0.0/16 (shared-host rules).
 */
const REPO = resolve(dirname(fileURLToPath(import.meta.url)), '../../../..');
const BIN = process.env['VRX_AGENT_BIN'] ?? resolve(REPO, 'apps/agent/bin/vrx-agent');
const PREFIX = process.env['VRX_TEST_PREFIX'] ?? 'w1';
const SOCKET = process.env['VRX_AGENT_SOCKET'] ?? `/run/vrx-test/${PREFIX}/agent.sock`;
const STATE_DIR = process.env['VRX_AGENT_STATE_DIR'] ?? `${dirname(SOCKET)}/agent-state`;
const SLOT = Number(/(\d+)$/.exec(PREFIX)?.[1] ?? '1');

function startAgent(): ChildProcess {
  rmSync(SOCKET, { force: true });
  mkdirSync(dirname(SOCKET), { recursive: true });
  return spawn(BIN, (process.env['VRX_AGENT_ARGS'] ?? '').split(' ').filter(Boolean), {
    env: {
      ...process.env,
      VRX_OWNER: PREFIX,
      VRX_AGENT_SOCKET: SOCKET,
      // slot-scoped state and metrics (shared-host rules): never /var/lib/vrx/agent or :9101
      VRX_AGENT_STATE_DIR: STATE_DIR,
      VRX_METRICS_PORT: process.env['VRX_METRICS_PORT'] ?? String(9100 + 10 * SLOT + 1),
    },
    stdio: ['ignore', 'inherit', 'inherit'],
  });
}

async function waitForSocket(ms: number): Promise<boolean> {
  for (const t0 = Date.now(); Date.now() - t0 < ms; await new Promise((r) => setTimeout(r, 200))) {
    if (existsSync(SOCKET)) return true;
  }
  return false;
}

/** Why the suite cannot run, or undefined when a real agent answered Health on the slot socket. */
async function probe(): Promise<string | undefined> {
  if (process.env['VRX_INTEGRATION'] !== '1') return 'VRX_INTEGRATION is not 1';
  if (!existsSync(BIN)) return `no agent binary at ${BIN} (set VRX_AGENT_BIN)`;
  const child = startAgent();
  try {
    if (!(await waitForSocket(10_000))) {
      return `${BIN} did not open ${SOCKET} within 10 s — not a vrx.v1.Dataplane server yet (P05 not merged)`;
    }
    const client = new AgentClient(loadEnv({ VRX_AGENT_SOCKET: SOCKET, VRX_AGENT_OWNER: PREFIX }));
    try {
      const h = await client.health(3000);
      return h.owner === PREFIX
        ? undefined
        : `agent serves owner '${h.owner}', expected '${PREFIX}'`;
    } catch (e) {
      return `agent on ${SOCKET} does not answer Health: ${(e as Error).message}`;
    } finally {
      client.close();
    }
  } finally {
    child.kill('SIGTERM');
    await new Promise((r) => (child.exitCode === null ? child.once('exit', r) : r(undefined)));
  }
}

const reason = await probe();
const enabled = reason === undefined;
if (!enabled) console.warn(`agent integration test skipped: ${reason}`);

describe.skipIf(!enabled)('agent integration (real vrx-agent + VPP)', () => {
  let h: Harness;
  let agent: ChildProcess | undefined;
  let admin: string;
  let ro: string;
  const prefix = PREFIX;
  const socket = SOCKET;
  const slot = Number(/(\d+)$/.exec(prefix)?.[1] ?? '1');
  const IF = `loop${slot}01`;
  const addr = (n: number) => `10.${slot}.101.${n}/24`;
  const vppAddrs = () => execFileSync('vppctl', ['show', 'int', 'addr'], { encoding: 'utf8' });

  beforeAll(async () => {
    agent = startAgent();
    await vi.waitFor(() => expect(existsSync(socket)).toBe(true), {
      timeout: 20_000,
      interval: 200,
    });
    h = await startHarness({ VRX_AGENT_SOCKET: socket, VRX_AGENT_OWNER: prefix });
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'ro9', role: 'readonly', password: PW.ro }]);
    ro = await h.login('ro9', PW.ro);
  }, 60_000);

  afterAll(async () => {
    if (h) {
      // leave nothing behind on VPP: an authoritative empty interfaces domain deletes our loopback
      await h.call(admin, 'PUT', '/api/v1/config/interfaces', {});
      await h.call(admin, 'POST', '/api/v1/config/commit?comment=cleanup');
      await h.close();
    }
    if (agent?.pid && agent.exitCode === null) {
      const exited = new Promise((r) => agent!.once('exit', r));
      agent.kill('SIGTERM');
      await exited;
    }
    // the agent's slot state (desired.pb …) goes with it; the next run starts clean
    rmSync(STATE_DIR, { recursive: true, force: true });
  });

  it('patch → diff → commit → Retrieve + vppctl → rollback → reverted', async () => {
    await h.call(admin, 'PUT', `/api/v1/config/interfaces/${IF}`, {
      ipv4: [addr(1)],
      enabled: true,
    });
    const d = await h.call(admin, 'GET', '/api/v1/config/diff');
    expect(d.body.changes).toContainEqual(
      expect.objectContaining({ pointer: `/interfaces/${IF}` }),
    );
    const c1 = await h.call(admin, 'POST', '/api/v1/config/commit');
    expect(c1.status).toBe(200);
    await h.call(admin, 'PATCH', `/api/v1/config/interfaces/${IF}`, { ipv4: [addr(2)] });
    const c2 = await h.call(admin, 'POST', '/api/v1/config/commit');
    expect(c2.status).toBe(200);
    const st = await h.call(ro, 'GET', '/api/v1/state/interfaces');
    expect(st.body.items.find((i: { name: string }) => i.name === IF).config.ipv4).toEqual([
      addr(2),
    ]);
    expect(vppAddrs()).toContain(addr(2).split('/')[0]);
    const rb = await h.call(admin, 'POST', `/api/v1/config/rollback/${c1.body.revision.id}`);
    expect(rb.status).toBe(200);
    expect(vppAddrs()).toContain(addr(1).split('/')[0]);
    expect(vppAddrs()).not.toContain(addr(2).split('/')[0]);
    expect((await h.call(ro, 'GET', '/api/v1/state/drift')).body.changes).toEqual([]);
  });

  it('commit ?confirm=5 without confirm is reverted after 5 s', async () => {
    await h.call(admin, 'PATCH', `/api/v1/config/interfaces/${IF}`, { ipv4: [addr(3)] });
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?confirm=5')).body.status).toBe(
      'pending',
    );
    expect(vppAddrs()).toContain(addr(3).split('/')[0]);
    await vi.waitFor(
      async () =>
        expect((await h.call(ro, 'GET', '/api/v1/config/commit/pending')).body.pending).toBeNull(),
      { timeout: 15_000, interval: 500 },
    );
    expect(vppAddrs()).not.toContain(addr(3).split('/')[0]);
    await h.call(admin, 'POST', '/api/v1/config/discard');
  });

  it('overlapping IPs → 400 with pointer; readonly PATCH → 403', async () => {
    await h.call(admin, 'PUT', `/api/v1/config/interfaces/loop${slot}02`, {
      ipv4: [`10.${slot}.101.200/25`],
    });
    const c = await h.call(admin, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.body.errors).toContainEqual(
      expect.objectContaining({ pointer: `/interfaces/loop${slot}02/ipv4/0` }),
    );
    await h.call(admin, 'POST', '/api/v1/config/discard');
    expect(
      (await h.call(ro, 'PATCH', `/api/v1/config/interfaces/${IF}`, { mtu: 1500 })).status,
    ).toBe(403);
  });
});
