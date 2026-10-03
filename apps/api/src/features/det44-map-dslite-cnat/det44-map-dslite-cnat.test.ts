import { DesiredState, type ApplyRequest } from '@ngfw/proto';
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
import { Det44LookupBody, Det44SessionsQuery } from './dto.js';
import { det44MapFake } from './fake.js';
import { det44PortsPerHost, Det44MapDsliteCnatService } from './service.js';

/**
 * F-det44-map-dslite-cnat API with the fake agent over gRPC and the in-memory repo: CGNAT/MAP/CNAT/PNAT configuration
 * through the commit engine (the semantic 400 with a pointer for a CNAT SNAT policy without addresses, before the agent
 * is asked), DET44 sessions / lookup / close and CNAT sessions / purge through the service. The PostgreSQL path (routes,
 * RBAC, audit) is test/e2e/det44-map-dslite-cnat.e2e.test.ts. Slot 8 names and addresses (10.8.0.0/16).
 */
describe('F-det44-map-dslite-cnat (fake agent over gRPC)', () => {
  const dir = mkdtempSync(join(tmpdir(), 'ngfw-det44-'));
  const socket = join(dir, 'agent.sock');
  const env = testEnv({
    NGFW_AGENT_SOCKET: socket,
    NGFW_AGENT_OWNER: 'w8',
    NGFW_AGENT_TIMEOUT_MS: '5000',
  });
  let fake: FakeAgent;
  let agent: AgentClient;
  let repo: MemoryConfigRepo;
  let ds: DatastoreService;
  let commits: CommitService;
  let svc: Det44MapDsliteCnatService;

  beforeAll(async () => {
    fake = new FakeAgent({ owner: 'w8' });
    await fake.start(socket);
    agent = new AgentClient(env);
    svc = new Det44MapDsliteCnatService(agent);
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
    repo = new MemoryConfigRepo();
    repo.addUser('admin', 'admin', TEST_HASH);
    ds = new DatastoreService(repo, env);
    commits = new CommitService(
      repo,
      new ValidationService(repo, agent),
      agent,
      { record: vi.fn(async () => undefined) } as unknown as SystemEventsService,
      new Bus(),
      env,
      {
        revokeUser: vi.fn(async () => ({ persisted: true, families: 0 })),
      } as unknown as TokensService,
      { write: vi.fn(async () => undefined) } as unknown as AuditService,
    );
  });
  afterEach(() => commits.onApplicationShutdown());

  const interfaces = {
    'host-w8l0': { enabled: true, ipv4: ['10.8.1.1/24'] },
    'host-w8w0': { enabled: true, ipv4: ['10.8.2.1/24'] },
  };
  const semantic400 = async (): Promise<Record<string, unknown>> => {
    let err: unknown;
    try {
      await commits.commit(ADMIN, {});
    } catch (e) {
      err = e;
    }
    expect(err).toBeInstanceOf(ProblemError);
    const p = err as ProblemError;
    expect(p.getStatus()).toBe(400);
    return p.body();
  };

  it('commits DET44 + CNAT + PNAT: the agent receives nat.det44 / cnat / pnat', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces', interfaces);
    await ds.patchCandidate(ADMIN, '/nat', {
      det44: {
        enabled: true,
        inside: ['host-w8l0'],
        outside: ['host-w8w0'],
        mappings: [{ inside: '10.8.1.0/24', outside: '10.8.2.200/30' }],
      },
      cnat: {
        translations: [
          {
            name: 'web',
            protocol: 'tcp',
            vip: { ip: '10.8.2.100', port: 80 },
            backends: [
              { ip: '10.8.1.2', port: 8080 },
              { ip: '10.8.1.3', port: 8080 },
            ],
          },
        ],
        snat: { addresses: { ipv4: '10.8.2.1' } },
      },
      pnat: {
        bindings: [
          {
            name: 'dns',
            match: { proto: 'udp', dst: '10.8.2.53', dport: 53 },
            rewrite: { dst: '10.8.1.53' },
          },
        ],
        attachments: [{ binding: 'dns', interface: 'host-w8w0', point: 'input' }],
      },
    });
    const r = await commits.commit(ADMIN, { comment: 'cgnat' });
    expect(r.status).toBe('applied');
    const apply = fake.calls.filter((c) => c.method === 'Apply').at(-1)?.request as ApplyRequest;
    expect(apply.desiredState?.nat?.det44?.mappings).toEqual([
      { inside: '10.8.1.0/24', outside: '10.8.2.200/30' },
    ]);
    expect(apply.desiredState?.nat?.cnat?.translations?.[0]?.backends).toHaveLength(2);
    expect(apply.desiredState?.nat?.pnat?.attachments).toEqual([
      { binding: 'dns', interface: 'host-w8w0', point: 'input' },
    ]);
  });

  it('CNAT SNAT policy without addresses → 400 with the pointer, the agent is not asked', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces', interfaces);
    await ds.patchCandidate(ADMIN, '/nat', { cnat: { snat: { policy: 'interface' } } });
    const body = await semantic400();
    expect(body['tier']).toBe('semantic');
    expect(body['errors']).toContainEqual(
      expect.objectContaining({ pointer: '/nat/cnat/snat/addresses' }),
    );
    expect(fake.calls.filter((c) => c.method === 'Apply' || c.method === 'DryRun')).toEqual([]);
  });

  it('DET44 sessions, lookup and close through the agent', async () => {
    fake.current = {
      ...fake.current,
      nat: { det44: { mappings: [{ inside: '10.8.1.0/24', outside: '10.8.2.200/30' }] } },
    };
    const st = det44MapFake(fake);
    st.det44Sessions.set(
      '10.8.1.5',
      [0, 1, 2].map((i) => ({
        insidePort: 40000 + i,
        outsidePort: 6064 + i,
        externalAddress: '10.8.2.2',
        externalPort: 80,
        state: 'tcp-established',
        expire: 100,
      })),
    );
    const page = await svc.det44Sessions(
      Det44SessionsQuery.parse({ user: '10.8.1.5', pageSize: '2' }),
    );
    expect(page).toMatchObject({
      outsideAddress: '10.8.2.200',
      portLo: 6064,
      portHi: 7071,
      total: 3,
    });
    expect(page.items).toHaveLength(2);
    await expect(
      svc.det44Sessions(Det44SessionsQuery.parse({ user: '10.9.9.9' })),
    ).rejects.toMatchObject({
      status: 404,
    });
    expect(await svc.det44Lookup(Det44LookupBody.parse({ inside: '10.8.1.70' }))).toEqual({
      inside: '10.8.1.70',
      outside: '10.8.2.201',
      portLo: 1024 + 1008 * 6,
      portHi: 1024 + 1008 * 7 - 1,
    });
    expect(
      await svc.det44Lookup(Det44LookupBody.parse({ outside: '10.8.2.201', port: 7100 })),
    ).toMatchObject({
      inside: '10.8.1.70',
      portLo: null,
    });
    const close = {
      direction: 'in',
      address: '10.8.1.5',
      port: 40000,
      externalAddress: '10.8.2.2',
      externalPort: 80,
    } as const;
    expect(await svc.det44Close(close)).toMatchObject({ summary: expect.stringMatching(/closed/) });
    await expect(svc.det44Close(close)).rejects.toMatchObject({ status: 404 });
    expect(st.det44Sessions.get('10.8.1.5')).toHaveLength(2);
  });

  it('CNAT sessions page; the purge is refused for a slot agent (403)', async () => {
    const st = det44MapFake(fake);
    st.cnatSessions = [0, 1, 2].map((i) => ({
      dstAddress: '10.8.2.100',
      dstPort: 80,
      srcAddress: `10.8.1.${10 + i}`,
      srcPort: 40000,
      protocol: 'tcp',
      translationIndex: 0,
      flags: 0,
    }));
    const r = await svc.cnatSessions({ page: 2, pageSize: 2 });
    expect(r).toMatchObject({ total: 3, truncated: false });
    expect(r.items.map((s) => s.srcAddress)).toEqual(['10.8.1.12']);
    await expect(svc.cnatPurge()).rejects.toMatchObject({ status: 403 });
    expect(st.purges).toBe(0);
  });

  it('computes DET44 ports per host like VPP', () => {
    expect(det44PortsPerHost(24, 30)).toBe(1008);
    expect(det44PortsPerHost(24, 24)).toBe(64512);
    expect(det44PortsPerHost(16, 26)).toBe(63);
  });
});
