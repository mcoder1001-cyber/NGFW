import { DesiredState } from '@ngfw/proto';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { AgentClient } from '../../agent/agent.client.js';
import type { AuditService } from '../../audit/audit.service.js';
import type { SystemEventsService } from '../../audit/system-events.service.js';
import type { TokensService } from '../../auth/tokens.service.js';
import { CommitService } from '../../commit/commit.service.js';
import { ValidationService } from '../../commit/validation.service.js';
import { ProblemError } from '../../common/problem.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { emptyDocument } from '../../datastore/documents.js';
import { Bus } from '../../infra/bus.js';
import { FakeAgent } from '../../testing/fake-agent.js';
import { ADMIN, TEST_HASH, testEnv } from '../../testing/fixtures.js';
import { MemoryConfigRepo } from '../../testing/memory-repo.js';
import { IpfixSflowController, IpfixStateOut } from './ipfix-sflow.controller.js';

/** F-ipfix-sflow: services.ipfix through the commit engine (semantic 400s) and GET /state/ipfix over the fake agent. */
describe('ipfix-sflow (fake agent over gRPC)', () => {
  const dir = mkdtempSync(join(tmpdir(), 'vrx-ipfix-'));
  const socket = join(dir, 'agent.sock');
  const env = testEnv({
    VRX_AGENT_SOCKET: socket,
    VRX_AGENT_OWNER: 'w1',
    VRX_AGENT_TIMEOUT_MS: '5000',
  });
  let fake: FakeAgent;
  let agent: AgentClient;
  let ds: DatastoreService;
  let commits: CommitService;
  const events = { record: vi.fn(async () => undefined) };
  const tokens = { revokeUser: vi.fn(async () => ({ persisted: true, families: 0 })) };
  const audit = { write: vi.fn(async () => undefined) };

  beforeAll(async () => {
    fake = new FakeAgent({ owner: 'w1' });
    await fake.start(socket);
    agent = new AgentClient(env);
  });
  afterAll(async () => {
    agent.close();
    await fake.stop();
    rmSync(dir, { recursive: true, force: true });
  });
  beforeEach(() => {
    fake.reset(
      DesiredState.toJSON(ValidationService.desiredState(emptyDocument())) as Record<
        string,
        unknown
      >,
    );
    const repo = new MemoryConfigRepo();
    repo.addUser('admin', 'admin', TEST_HASH);
    ds = new DatastoreService(repo, env);
    commits = new CommitService(
      repo,
      new ValidationService(repo, agent),
      agent,
      events as unknown as SystemEventsService,
      new Bus(),
      env,
      tokens as unknown as TokensService,
      audit as unknown as AuditService,
    );
  });
  afterEach(() => commits.onApplicationShutdown());

  const ifaces = { 'host-w1a': { ipv4: ['10.1.1.1/24'] }, 'host-w1b': { ipv4: ['10.1.2.1/24'] } };
  const probe = { interfaces: [{ interface: 'host-w1a', direction: 'rx', ip4: true, ip6: false }] };

  it('flowprobe interfaces without an enabled exporter are a 400 with a pointer', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces', ifaces);
    await ds.patchCandidate(ADMIN, '/services/ipfix/flowprobe', probe);
    let err: unknown;
    try {
      await commits.commit(ADMIN, {});
    } catch (e) {
      err = e;
    }
    expect(err).toBeInstanceOf(ProblemError);
    const p = err as ProblemError;
    expect(p.getStatus()).toBe(400);
    expect(p.body()['tier']).toBe('semantic');
    expect(p.body()['errors']).toContainEqual({
      pointer: '/services/ipfix/flowprobe/interfaces',
      message:
        'flowprobe records are sent through IPFIX exporter 0 only: enable an exporter with an IPv4 collector',
    });
    expect(fake.calls.filter((c) => c.method === 'Apply')).toEqual([]);
  });

  it('commits exporters, flowprobe and sFlow; GET /state/ipfix reports them', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces', ifaces);
    await ds.patchCandidate(ADMIN, '/services/ipfix', {
      exporters: {
        lan: { collector: { address: '10.1.1.9', port: 3171 }, sourceAddress: '10.1.1.1' },
      },
      flowprobe: probe,
      sflow: {
        enabled: true,
        samplingN: 1000,
        collectors: [{ address: '10.1.1.9', port: 3172 }],
        interfaces: ['host-w1b'],
      },
    });
    const r = await commits.commit(ADMIN, { comment: 'flows' });
    expect(r.status).toBe('applied');
    const out = await new IpfixSflowController(agent).ipfix();
    expect(IpfixStateOut.parse(out)).toEqual(out);
    expect(out.exporters).toEqual([
      expect.objectContaining({
        name: 'lan',
        defaultExporter: true,
        collector: '10.1.1.9',
        collectorPort: 3171,
        statIndex: null,
      }),
    ]);
    expect(out.flowprobe.interfaces).toEqual([
      { interface: 'host-w1a', which: 'ip4', direction: 'rx' },
    ]);
    expect(out.sflow.interfaces).toEqual([{ interface: 'host-w1b', hwIfIndex: 100 }]);
    expect(out.sflow.global?.samplingN).toBe(1000);
    expect(out.sflow.exportsToCollectors).toBe(false);
    expect(out.sflow.counters[0]?.value).toBe('1000');
  });

  it('sFlow header bytes not a multiple of 32 are a 400 at the leaf', async () => {
    await ds.patchCandidate(ADMIN, '/services/ipfix/sflow', {
      enabled: true,
      headerBytes: 100,
      collectors: [{ address: '10.1.1.9' }],
    });
    await expect(commits.commit(ADMIN, {})).rejects.toSatisfy((e: unknown) => {
      const errors = (e as ProblemError).body()['errors'] as { pointer: string }[];
      return errors.some((i) => i.pointer === '/services/ipfix/sflow/headerBytes');
    });
  });
});
