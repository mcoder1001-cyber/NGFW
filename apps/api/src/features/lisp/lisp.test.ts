import { DesiredState, type ApplyRequest } from '@ngfw/proto';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
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
import { LispController } from './lisp.controller.js';

/** The full example (packages/proto/test/fixtures/lisp-full.json). */
const FULL = JSON.parse(
  readFileSync(
    new URL('../../../../../packages/proto/test/fixtures/lisp-full.json', import.meta.url),
    'utf8',
  ),
) as Record<string, Record<string, unknown>>;

/**
 * F-lisp API: `tunnels.lisp` through the generic pointer routes and the commit engine (semantic 400 with a pointer
 * before the agent is asked), and `GET /api/v1/state/lisp` through the LispState RPC of the fake agent (its real fake
 * behaviour, features/lisp/fake.ts). Persistence is the in-memory repo; the PostgreSQL path is test/e2e/lisp.e2e.test.ts.
 */
describe('F-lisp (fake agent over gRPC)', () => {
  const dir = mkdtempSync(join(tmpdir(), 'vrx-lisp-'));
  const socket = join(dir, 'agent.sock');
  const env = testEnv({
    VRX_AGENT_SOCKET: socket,
    VRX_AGENT_OWNER: 'w11',
    VRX_AGENT_TIMEOUT_MS: '5000',
  });
  let fake: FakeAgent;
  let agent: AgentClient;
  let repo: MemoryConfigRepo;
  let ds: DatastoreService;
  let commits: CommitService;

  beforeAll(async () => {
    fake = new FakeAgent({ owner: 'w11' });
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

  const load = async () => {
    await ds.patchCandidate(ADMIN, '/vrfs', FULL['vrfs']);
    await ds.patchCandidate(ADMIN, '/interfaces', FULL['interfaces']);
    await ds.patchCandidate(ADMIN, '/tunnels/lisp', FULL['tunnels']!['lisp']);
  };

  it('commits the full example: the agent receives tunnels.lisp; GET state/lisp reports it', async () => {
    await load();
    const r = await commits.commit(ADMIN, { comment: 'lisp' });
    expect(r.status).toBe('applied');
    const apply = fake.calls.filter((c) => c.method === 'Apply').at(-1)?.request as ApplyRequest;
    expect(apply.desiredState?.tunnels?.lisp?.localEids).toEqual([
      { vni: 1100, eid: '10.11.100.0/24', locatorSet: 'w11-rloc' },
    ]);
    const st = await new LispController(agent).state();
    expect(st).toMatchObject({
      enabled: true,
      gpeEnabled: true,
      pitr: 'w11-rloc',
      locatorSets: [
        { name: 'w11-rloc', locators: [{ interface: 'host-w11-eth0', priority: 1, weight: 1 }] },
      ],
      adjacencies: [{ vni: 1100, reid: '10.11.200.0/24', leid: '10.11.100.0/24' }],
      eidTables: [{ vni: 1100, dpTable: 1100, isL2: false }],
      mapResolvers: ['10.11.1.254'],
      mapServers: ['10.11.1.253'],
      gpeVnis: [1101],
    });
    expect(st.mappings.map((m) => [m.eid, m.local])).toEqual([
      ['10.11.100.0/24', true],
      ['10.11.200.0/24', false],
    ]);
  });

  it('duplicate EID in one VNI → 400 problem+json with a pointer, agent not asked', async () => {
    await load();
    const lisp = structuredClone(FULL['tunnels']!['lisp']) as { localEids: unknown[] };
    lisp.localEids.push({ vni: 1100, eid: '10.11.100.0/24', locatorSet: 'w11-rloc' });
    await ds.patchCandidate(ADMIN, '/tunnels/lisp', { localEids: lisp.localEids });
    let err: unknown;
    try {
      await commits.commit(ADMIN, {});
    } catch (e) {
      err = e;
    }
    expect(err).toBeInstanceOf(ProblemError);
    const p = err as ProblemError;
    expect(p.getStatus()).toBe(400);
    const body = p.body();
    expect(body['tier']).toBe('semantic');
    expect(body['errors']).toContainEqual({
      pointer: '/tunnels/lisp/localEids/1/eid',
      message: 'EID 10.11.100.0/24 in VNI 1100 is already configured at /tunnels/lisp/localEids/0',
    });
    expect(fake.calls.filter((c) => c.method === 'Apply' || c.method === 'DryRun')).toEqual([]);
  });

  it('an agent without LispState answers 501', async () => {
    const orig = fake.failAllWith;
    fake.failAllWith = 12; // UNIMPLEMENTED
    try {
      await expect(new LispController(agent).state()).rejects.toMatchObject({ status: 501 });
    } finally {
      fake.failAllWith = orig;
    }
  });
});
