import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AgentClient } from '../../agent/agent.client.js';
import { ProblemError } from '../../common/problem.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { FakeAgent } from '../../testing/fake-agent.js';
import { ADMIN, TEST_HASH, testEnv } from '../../testing/fixtures.js';
import { MemoryConfigRepo } from '../../testing/memory-repo.js';
import { HostStackController } from './host-stack.controller.js';

async function problem(
  p: Promise<unknown>,
): Promise<{ status: number; body: Record<string, unknown> }> {
  try {
    await p;
  } catch (e) {
    expect(e).toBeInstanceOf(ProblemError);
    return { status: (e as ProblemError).getStatus(), body: (e as ProblemError).body() };
  }
  throw new Error('expected a problem');
}

const HS = {
  enabled: true,
  namespaces: { 'w1-app': { vrf: 'default' } },
  sessionRules: [
    {
      tag: 'w1-deny',
      transport: 'tcp',
      local: '10.1.1.0/24',
      localPort: 3190,
      remote: '10.1.2.0/24',
      action: 'deny',
    },
  ],
};

describe('F-host-stack API (fake agent over gRPC, in-memory datastore)', () => {
  const dir = mkdtempSync(join(tmpdir(), 'vrx-hs-'));
  const env = testEnv({
    VRX_AGENT_SOCKET: join(dir, 'agent.sock'),
    VRX_AGENT_OWNER: 'w1',
    VRX_AGENT_TIMEOUT_MS: '5000',
  });
  let fake: FakeAgent;
  let agent: AgentClient;
  let ds: DatastoreService;

  beforeAll(async () => {
    fake = new FakeAgent({ owner: 'w1' });
    await fake.start(env.VRX_AGENT_SOCKET);
    agent = new AgentClient(env);
    const repo = new MemoryConfigRepo();
    repo.addUser('admin', 'admin', TEST_HASH);
    ds = new DatastoreService(repo, env);
  });
  afterAll(async () => {
    agent.close();
    await fake.stop();
    rmSync(dir, { recursive: true, force: true });
  });

  it('GET /state/host-stack maps the HostStackState RPC', async () => {
    fake.reset({ services: { hostStack: HS } });
    const out = await new HostStackController(agent).state();
    expect(out).toMatchObject({
      sessionEnabled: true,
      sessionDetail: '',
      namespaces: ['w1-app'],
      ruleCount: 1,
      ruleCountTotal: 1,
      rules: [
        { tag: 'w1-deny', scope: 'global', transport: 'tcp', localPort: 3190, action: 'deny' },
      ],
    });
    expect(typeof out.retrievedAt).toBe('string');
  });

  it('accepts the host stack via the pointer route', async () => {
    await ds.patchCandidate(ADMIN, '/services/hostStack', HS);
    const cand = (await ds.getCandidate()) as { services: { hostStack: typeof HS } };
    expect(cand.services.hostStack.sessionRules[0]?.tag).toBe('w1-deny');
  });

  it('inline secret → 400 problem with pointer (D-051)', async () => {
    const r = await problem(
      ds.patchCandidate(ADMIN, '/services/hostStack', {
        ...HS,
        namespaces: { 'w1-app': { vrf: 'default', secretRef: 'hunter2' } },
      }),
    );
    expect(r.status).toBe(400);
    expect(JSON.stringify(r.body)).toContain('/services/hostStack/namespaces/w1-app/secretRef');
  });

  it("'..' in wwwRootPath → 400 problem with pointer (D-049)", async () => {
    const r = await problem(
      ds.patchCandidate(ADMIN, '/services/hostStack', {
        ...HS,
        httpStatic: {
          enabled: true,
          wwwRootPath: '/var/lib/vrx/www/../../etc',
          uri: 'tcp://10.1.1.1/80',
        },
      }),
    );
    expect(r.status).toBe(400);
    expect(JSON.stringify(r.body)).toContain('/services/hostStack/httpStatic/wwwRootPath');
  });
});
