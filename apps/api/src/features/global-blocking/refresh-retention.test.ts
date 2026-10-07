import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AgentClient } from '../../agent/agent.client.js';
import type { SystemEventsService } from '../../audit/system-events.service.js';
import type { CommitService } from '../../commit/commit.service.js';
import type { Principal } from '../../common/principal.js';
import type { Env } from '../../config.js';
import type { DatastoreService } from '../../datastore/datastore.service.js';
import type { ConfigRepo, Doc } from '../../datastore/repo.js';
import type { Db } from '../../db/db.js';
import { Bus } from '../../infra/bus.js';
import type { Valkey } from '../../infra/valkey.js';
import type { SecretsService } from '../../secrets/secrets.service.js';
import type * as Fetch from './fetch.js';
import { FetchError } from './fetch.js';
import { GlobalBlockingService, type FetchState } from './global-blocking.service.js';

const download = vi.hoisted(() => vi.fn());
vi.mock('./fetch.js', async (original) => ({
  ...(await original<typeof Fetch>()),
  fetchList: download,
}));

// Exercise the production service/parser. Only transport and persistence are
// substituted: no DB, Valkey, daemon, timer, feed server or product agent is used.
const user: Principal = { id: 1, username: 'fixture', role: 'admin', via: 'jwt' };
const oldEntries = ['192.0.2.7/32', '2001:db8:1::/48'];
const goodState: FetchState = {
  lastFetchAt: '2026-10-07T00:00:00.000Z',
  lastResult: 'ok',
  lastError: null,
  etag: '"last-good"',
  lastModified: 'Wed, 07 Oct 2026 00:00:00 GMT',
  entries: 2,
};

function fixture() {
  let running: Doc = {
    acl: {
      globalBlocking: {
        lists: {
          feed: {
            enabled: true,
            interfaces: ['wan0'],
            entries: [...oldEntries],
            source: { kind: 'url', url: 'https://feed.example.test/list', refreshSec: 300 },
          },
        },
      },
    },
  };
  const candidate = structuredClone(running);
  const store = new Map<string, string>([['gb:state:feed', JSON.stringify(goodState)]]);
  const stage = vi.fn();
  const commit = vi.fn(async (change: (doc: Doc) => Doc | null) => {
    const next = change(structuredClone(running));
    if (next === null) return { status: 'unchanged' };
    running = next;
    return { status: 'applied' };
  });
  const record = vi.fn();
  const bus = new Bus();
  const publish = vi.spyOn(bus, 'publish');
  const svc = new GlobalBlockingService(
    { NGFW_GLOBAL_BLOCKING_CHECK_SEC: 0 } as Env,
    {
      getRunning: async () => ({ doc: structuredClone(running) }),
      putCandidate: stage,
    } as unknown as DatastoreService,
    { systemCommit: commit } as unknown as CommitService,
    {} as AgentClient,
    { record } as unknown as SystemEventsService,
    {} as SecretsService,
    {} as Db,
    {
      get: async (key: string) => store.get(key) ?? null,
      set: async (key: string, value: string, ...options: unknown[]) => {
        if (options.includes('NX') && store.has(key)) return null;
        store.set(key, value);
        return 'OK';
      },
      del: async (key: string) => Number(store.delete(key)),
    } as unknown as Valkey,
    { candidate: async () => ({ payload: structuredClone(candidate) }) } as unknown as ConfigRepo,
    bus,
  );
  let clock = Date.parse('2026-10-07T00:05:01.000Z');
  svc.now = () => new Date(clock);
  return {
    svc,
    store,
    stage,
    commit,
    record,
    publish,
    running: () => structuredClone(running),
    candidate: () => structuredClone(candidate),
    state: () => JSON.parse(store.get('gb:state:feed')!) as FetchState,
    advance: () => {
      clock += 301_000;
    },
  };
}

const failures = [
  { name: 'transport failure', error: new FetchError('connection refused') },
  { name: 'HTTP failure', error: new FetchError('HTTP 503') },
  { name: 'empty body', text: '' },
  { name: 'comments only', text: '# no entries\n\n' },
  { name: 'garbage only', text: 'not-an-ip\n' },
  { name: 'partially valid above threshold', text: '203.0.113.8\n2001:db8:2::1\ngarbage\n' },
];

describe('Global Blocking last-good refresh retention', () => {
  beforeEach(() => download.mockReset());

  it.each(failures)(
    'scheduled $name retains both families and validators, then recovers',
    async (bad) => {
      const f = fixture();
      const before = f.running();
      if (bad.error) download.mockRejectedValueOnce(bad.error);
      else download.mockResolvedValueOnce({ status: 'ok', text: bad.text, etag: '"refused"' });
      expect(await f.svc.refreshDue()).toEqual([
        { list: 'feed', result: 'failed', detail: expect.any(String) },
      ]);
      expect(f.running()).toEqual(before);
      expect(f.candidate()).toEqual(before);
      expect(f.commit).not.toHaveBeenCalled();
      expect(f.stage).not.toHaveBeenCalled();
      expect(f.state()).toMatchObject({
        ...goodState,
        lastFetchAt: f.svc.now().toISOString(),
        lastResult: 'failed',
        lastError: expect.any(String),
      });
      expect(f.record).toHaveBeenCalledWith(
        'warning',
        'global-blocking',
        'BLOCKLIST_FETCH_FAILED',
        expect.stringContaining('last good list is kept'),
        { list: 'feed', via: 'scheduled', reason: expect.any(String) },
      );
      expect(f.publish).toHaveBeenCalledWith('security.events', {
        type: 'global-blocking-fetch-failed',
        list: 'feed',
      });
      expect(f.store.has('gb:fetching:feed')).toBe(false);
      expect(download).toHaveBeenCalledWith('https://feed.example.test/list', {
        verifyTls: true,
        etag: goodState.etag,
        lastModified: goodState.lastModified,
      });

      f.advance();
      download.mockResolvedValueOnce({
        status: 'ok',
        text: '203.0.113.8\n2001:db8:2::1\n',
        etag: '"recovered"',
      });
      expect(await f.svc.refreshDue()).toEqual([
        { list: 'feed', result: 'ok', detail: '2 added, 2 removed' },
      ]);
      expect(f.commit).toHaveBeenCalledTimes(1);
      expect(await f.svc.exportText('feed', 'running')).toBe(
        '# ngfw block list feed (running, 2 entries)\n203.0.113.8/32\n2001:db8:2::1/128\n',
      );
      expect(f.state()).toMatchObject({
        lastResult: 'ok',
        lastError: null,
        etag: '"recovered"',
        entries: 2,
      });
      expect(f.record).toHaveBeenLastCalledWith(
        'info',
        'global-blocking',
        'BLOCKLIST_REFRESHED',
        expect.any(String),
        { list: 'feed', added: 2, removed: 2, entries: 2 },
      );
    },
  );

  it.each(failures)('manual $name never stages a partial replacement', async (bad) => {
    const f = fixture();
    const before = f.running();
    if (bad.error) download.mockRejectedValueOnce(bad.error);
    else download.mockResolvedValueOnce({ status: 'ok', text: bad.text });
    await expect(f.svc.fetchNow(user, 'feed', false)).rejects.toThrow('the list is unchanged');
    expect(f.running()).toEqual(before);
    expect(f.candidate()).toEqual(before);
    expect(f.stage).not.toHaveBeenCalled();
    expect(f.commit).not.toHaveBeenCalled();
    expect(f.state()).toMatchObject({ lastResult: 'failed', lastError: expect.any(String) });
    expect(f.record).toHaveBeenCalledWith(
      'warning',
      'global-blocking',
      'BLOCKLIST_FETCH_FAILED',
      expect.any(String),
      { list: 'feed', via: 'manual', reason: expect.any(String) },
    );
  });

  it('304 after a failed refresh retains last-good metadata and performs no commit', async () => {
    const f = fixture();
    download.mockRejectedValueOnce(new FetchError('HTTP 503'));
    await f.svc.refreshDue();
    const before = f.running();
    f.advance();
    download.mockResolvedValueOnce({ status: 'not-modified' });
    expect(await f.svc.refreshDue()).toEqual([{ list: 'feed', result: 'unchanged' }]);
    expect(f.running()).toEqual(before);
    expect(f.commit).not.toHaveBeenCalled();
    expect(f.state()).toEqual({
      ...goodState,
      lastFetchAt: f.svc.now().toISOString(),
      lastResult: 'unchanged',
    });
    expect(f.record).toHaveBeenCalledTimes(1);
    expect(download.mock.calls[1]?.[1]).toMatchObject({
      etag: goodState.etag,
      lastModified: goodState.lastModified,
    });
  });

  it('failed system commit keeps last-good metadata and releases the refresh lease', async () => {
    const f = fixture();
    const before = f.running();
    f.commit.mockRejectedValueOnce(new Error('apply rejected'));
    download.mockResolvedValueOnce({ status: 'ok', text: '203.0.113.8\n', etag: '"uncommitted"' });
    expect(await f.svc.refreshDue()).toEqual([
      { list: 'feed', result: 'failed', detail: 'Error: apply rejected' },
    ]);
    expect(f.running()).toEqual(before);
    expect(f.state()).toMatchObject({
      etag: goodState.etag,
      entries: 2,
      lastResult: 'failed',
      lastError: expect.stringContaining('could not be committed'),
    });
    expect(f.store.has('gb:fetching:feed')).toBe(false);
    expect(f.record).toHaveBeenCalledTimes(1);
  });
});
