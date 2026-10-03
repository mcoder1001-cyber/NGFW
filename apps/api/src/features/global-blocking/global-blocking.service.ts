import { Bus } from '../../infra/bus.js';
import {
  Inject,
  Injectable,
  Logger,
  Optional,
  type OnModuleDestroy,
  type OnModuleInit,
} from '@nestjs/common';
import { diffEntries, isPlainObject, MAX_BLOCK_ENTRIES, parseBlockList } from '@ngfw/schema';
import { eq } from 'drizzle-orm';
import { AgentClient } from '../../agent/agent.client.js';
import { SystemEventsService } from '../../audit/system-events.service.js';
import { CommitService } from '../../commit/commit.service.js';
import { problems, ProblemError } from '../../common/problem.js';
import type { Principal } from '../../common/principal.js';
import { ENV, type Env } from '../../config.js';
import { CONFIG_REPO, DatastoreService } from '../../datastore/datastore.service.js';
import type { ConfigRepo } from '../../datastore/repo.js';
import { DB, type Db } from '../../db/db.js';
import { secret } from '../../db/schema.js';
import { VALKEY, type Valkey } from '../../infra/valkey.js';
import { SecretsService } from '../../secrets/secrets.service.js';
import { authorizationHeader, FetchError, fetchList, type FetchResult } from './fetch.js';

type Json = Record<string, unknown>;
export type Source = 'running' | 'candidate';

/** A download with more invalid lines than this share (or no valid entry) is refused: the last good list stays. */
export const MAX_INVALID_SHARE = 0.1;
/** Invalid lines returned in a preview. */
const MAX_INVALID_SHOWN = 200;
/** Added/removed entries returned in a preview. */
const MAX_DIFF_SHOWN = 100;

/** What the last download of a list did (Valkey `gb:state:<list>`, shared by the API processes). */
export interface FetchState {
  lastFetchAt: string;
  /** ok = new entries applied or staged; unchanged = same file (304 or identical entries); failed = last good kept. */
  lastResult: 'ok' | 'unchanged' | 'failed' | 'deferred';
  lastError: string | null;
  etag?: string;
  lastModified?: string;
  /** Entries of the last good download. */
  entries?: number;
}

export interface Preview {
  list: string;
  dryRun: boolean;
  lines: number;
  entries: number;
  added: number;
  removed: number;
  unchanged: number;
  normalised: number;
  collapsed: number;
  invalidCount: number;
  invalid: { line: number; text: string; reason: string }[];
  addedSample: string[];
  removedSample: string[];
  staged: boolean;
}

function listsOf(doc: Json): Record<string, Json> {
  const acl = isPlainObject(doc['acl']) ? (doc['acl'] as Json) : {};
  const gb = isPlainObject(acl['globalBlocking']) ? (acl['globalBlocking'] as Json) : {};
  return isPlainObject(gb['lists']) ? (gb['lists'] as Record<string, Json>) : {};
}

function entriesOf(l: Json | undefined): string[] {
  return Array.isArray(l?.['entries']) ? (l['entries'] as string[]) : [];
}

function escapeToken(s: string): string {
  return s.replace(/~/g, '~0').replace(/\//g, '~1');
}

function lineCount(text: string): number {
  if (text === '') return 0;
  let n = 1;
  for (let i = text.indexOf('\n'); i !== -1; i = text.indexOf('\n', i + 1)) n++;
  return text.endsWith('\n') ? n - 1 : n;
}

/**
 * F-global-blocking API logic. One parser (`parseBlockList`, packages/schema) for uploads and downloads; an upload or a
 * "Fetch now" is previewed (added / removed / invalid lines) and, when confirmed, staged into the candidate like any
 * edit — the normal commit applies it. A list with a URL source and `refreshSec` is re-downloaded on schedule; a
 * changed file is applied as a system change (revision kind `system`) while nobody edits the candidate. Any failure —
 * network, HTTP, too many invalid lines, an empty file — keeps the last good list and raises BLOCKLIST_FETCH_FAILED.
 */
@Injectable()
export class GlobalBlockingService implements OnModuleInit, OnModuleDestroy {
  private readonly log = new Logger('global-blocking');
  private timer: NodeJS.Timeout | undefined;
  /** Clock (tests move it). */
  now: () => Date = () => new Date();
  /** Test seam: replaces the secret store lookup. */
  secretReader: ((ref: string) => Promise<string | null>) | null = null;

  constructor(
    @Inject(ENV) private readonly env: Env,
    private readonly ds: DatastoreService,
    private readonly commits: CommitService,
    private readonly agent: AgentClient,
    private readonly events: SystemEventsService,
    private readonly secrets: SecretsService,
    @Inject(DB) private readonly db: Db,
    @Inject(VALKEY) private readonly kv: Valkey,
    @Inject(CONFIG_REPO) private readonly repo: ConfigRepo,
    @Optional() private readonly notificationBus?: Bus,
  ) {}

  onModuleInit(): void {
    const sec = this.env.NGFW_GLOBAL_BLOCKING_CHECK_SEC;
    if (sec <= 0) return;
    this.timer = setInterval(
      () => void this.refreshDue().catch((e: unknown) => this.log.warn(`refresh: ${String(e)}`)),
      sec * 1000,
    );
    this.timer.unref();
  }

  onModuleDestroy(): void {
    if (this.timer) clearInterval(this.timer);
  }

  private async doc(source: Source): Promise<Json> {
    if (source === 'candidate') {
      // only the acl subtree (no secret leaf in it); no candidate = nobody edits = running
      const c = await this.repo.candidate();
      if (c.payload !== null) return { acl: c.payload['acl'] };
    }
    return (await this.ds.getRunning()).doc;
  }

  private async fetchState(name: string): Promise<FetchState | null> {
    const raw = await this.kv.get(`gb:state:${name}`);
    if (raw === null) return null;
    try {
      return JSON.parse(raw) as FetchState;
    } catch {
      return null;
    }
  }

  private async setFetchState(name: string, s: FetchState): Promise<void> {
    await this.kv.set(`gb:state:${name}`, JSON.stringify(s), 'EX', 90 * 86_400);
  }

  private async readSecret(ref: string): Promise<string | null> {
    if (this.secretReader) return this.secretReader(ref);
    const [row] = await this.db
      .select({ ciphertext: secret.ciphertext })
      .from(secret)
      .where(eq(secret.ref, ref))
      .limit(1);
    return row ? this.secrets.decrypt(row.ciphertext, ref) : null;
  }

  // ---- status -------------------------------------------------------------------------------------------------------

  async status() {
    const [running, candidate] = await Promise.all([this.doc('running'), this.doc('candidate')]);
    const rl = listsOf(running);
    const cl = listsOf(candidate);
    const names = [...new Set([...Object.keys(rl), ...Object.keys(cl)])].sort();
    const counters = await this.counters();
    const lists = await Promise.all(
      names.map(async (name) => {
        const l = cl[name] ?? rl[name] ?? {};
        const src = isPlainObject(l['source']) ? (l['source'] as Json) : { kind: 'upload' };
        const fs = await this.fetchState(name);
        const refreshSec = typeof src['refreshSec'] === 'number' ? src['refreshSec'] : null;
        const pending =
          rl[name] === undefined
            ? 'added'
            : cl[name] === undefined
              ? 'deleted'
              : JSON.stringify(rl[name]) === JSON.stringify(cl[name])
                ? null
                : 'changed';
        return {
          name,
          description: typeof l['description'] === 'string' ? l['description'] : null,
          enabled: l['enabled'] !== false,
          pending,
          source: {
            kind: src['kind'] === 'url' ? ('url' as const) : ('upload' as const),
            url: typeof src['url'] === 'string' ? src['url'] : null,
            refreshSec,
          },
          allInterfaces: l['allInterfaces'] === true,
          interfaces: Array.isArray(l['interfaces']) ? (l['interfaces'] as string[]) : [],
          direction: typeof l['direction'] === 'string' ? l['direction'] : 'both',
          protectHost: l['protectHost'] !== false,
          entries: entriesOf(l).length,
          runningEntries: entriesOf(rl[name]).length,
          fetch: fs,
          nextRefreshAt:
            src['kind'] === 'url' && refreshSec !== null && rl[name] !== undefined
              ? new Date(
                  (fs ? Date.parse(fs.lastFetchAt) : this.now().getTime()) + refreshSec * 1000,
                ).toISOString()
              : null,
          hits: counters.value?.get(name) ?? null,
        };
      }),
    );
    const total = Object.values(cl).reduce((n, l) => n + entriesOf(l).length, 0);
    return {
      maxEntries: MAX_BLOCK_ENTRIES,
      totalEntries: total,
      countersError: counters.error ?? null,
      lists,
    };
  }

  /** Per list: data-plane packets/bytes (sum of its `_gb.<list>.*` ACLs) and drops to the box (nftables). */
  private async counters(): Promise<{
    value?: Map<string, { dataplanePackets: number; dataplaneBytes: number; hostPackets: number }>;
    error?: string;
  }> {
    const out = new Map<
      string,
      { dataplanePackets: number; dataplaneBytes: number; hostPackets: number }
    >();
    const get = (n: string) => {
      let v = out.get(n);
      if (!v) out.set(n, (v = { dataplanePackets: 0, dataplaneBytes: 0, hostPackets: 0 }));
      return v;
    };
    try {
      const st = await this.agent.aclState({
        list: '',
        offset: 0,
        limit: 0,
        filter: undefined,
        includeInterfaces: false,
      });
      for (const l of st.lists) {
        const m = /^_gb\.(.+)\.[io]\d+$/.exec(l.name);
        if (!m) continue;
        const v = get(m[1]!);
        v.dataplanePackets += Number(l.packets);
        v.dataplaneBytes += Number(l.bytes);
      }
    } catch (e) {
      return {
        error: e instanceof ProblemError ? String(e.body().detail ?? e.body().title) : String(e),
      };
    }
    try {
      const h = await this.agent.hostAclState();
      for (const c of h.chains) {
        for (const r of c.rules)
          if (r.kind === 'global-blocking') get(r.list).hostPackets += Number(r.packets);
      }
    } catch {
      // no host firewall answer: data-plane counters only
    }
    return { value: out };
  }

  // ---- import / fetch / export ----------------------------------------------------------------------------------------

  /** Parses `text` against the candidate's list; stages the entries when `dryRun` is false. */
  async importText(
    user: Principal,
    name: string,
    text: string,
    dryRun: boolean,
    fromServer = false,
  ): Promise<Preview> {
    const candidate = await this.doc('candidate');
    const list = listsOf(candidate)[name];
    if (list === undefined)
      throw problems.notFound(
        `block list '${name}' does not exist in the candidate; create it first`,
      );
    const p = this.preview(name, text, entriesOf(list), dryRun);
    // a server's answer is not echoed (only line numbers and reasons): the preview must not become a way to read
    // arbitrary internal HTTP resources through the box
    if (fromServer) p.invalid = p.invalid.map((i) => ({ ...i, text: '' }));
    const others = Object.entries(listsOf(candidate))
      .filter(([n]) => n !== name)
      .reduce((n, [, l]) => n + entriesOf(l).length, 0);
    if (others + p.entries > MAX_BLOCK_ENTRIES)
      throw problems.badRequest(
        `${others + p.entries} entries over all block lists; at most ${MAX_BLOCK_ENTRIES}`,
        [{ pointer: `/acl/globalBlocking/lists/${name}/entries`, message: 'too many entries' }],
      );
    if (!dryRun && (p.added > 0 || p.removed > 0)) {
      await this.ds.putCandidate(
        user,
        `/acl/globalBlocking/lists/${escapeToken(name)}/entries`,
        parseBlockList(text).entries,
      );
      return { ...p, staged: true };
    }
    return p;
  }

  private preview(name: string, text: string, before: string[], dryRun: boolean): Preview {
    const parsed = parseBlockList(text);
    const d = diffEntries(before, parsed.entries);
    return {
      list: name,
      dryRun,
      lines: lineCount(text),
      entries: parsed.entries.length,
      added: d.added.length,
      removed: d.removed.length,
      unchanged: parsed.entries.length - d.added.length,
      normalised: parsed.normalised,
      collapsed: parsed.collapsed,
      invalidCount: parsed.invalid.length,
      invalid: parsed.invalid.slice(0, MAX_INVALID_SHOWN),
      addedSample: d.added.slice(0, MAX_DIFF_SHOWN),
      removedSample: d.removed.slice(0, MAX_DIFF_SHOWN),
      staged: false,
    };
  }

  /** The list's server file (candidate's source settings), downloaded now; conditional headers are not sent. */
  async fetchNow(user: Principal, name: string, dryRun: boolean): Promise<Preview> {
    const list = listsOf(await this.doc('candidate'))[name];
    if (list === undefined)
      throw problems.notFound(`block list '${name}' does not exist in the candidate`);
    let r: FetchResult;
    try {
      r = await this.download(list, false);
    } catch (e) {
      const why = e instanceof FetchError || e instanceof ProblemError ? e.message : String(e);
      await this.failed(name, why, 'manual');
      throw new ProblemError(
        502,
        'download-failed',
        'Download failed',
        `download failed: ${why}; the list is unchanged`,
      );
    }
    if (r.status !== 'ok')
      throw problems.badRequest('the server answered 304 to an unconditional request');
    const bad = this.refusal(r.text);
    if (bad !== null) {
      await this.failed(name, bad, 'manual');
      throw problems.badRequest(`the downloaded file is refused: ${bad}; the list is unchanged`);
    }
    const p = await this.importText(user, name, r.text, dryRun, true);
    if (!dryRun) {
      await this.setFetchState(name, {
        lastFetchAt: this.now().toISOString(),
        lastResult: p.staged ? 'ok' : 'unchanged',
        lastError: null,
        ...(r.etag ? { etag: r.etag } : {}),
        ...(r.lastModified ? { lastModified: r.lastModified } : {}),
        entries: p.entries,
      });
    }
    return p;
  }

  async exportText(name: string, source: Source): Promise<string> {
    const list = listsOf(await this.doc(source))[name];
    if (list === undefined)
      throw problems.notFound(`block list '${name}' does not exist in ${source}`);
    const e = entriesOf(list);
    return `# ngfw block list ${name} (${source}, ${e.length} entries)\n${e.join('\n')}${e.length ? '\n' : ''}`;
  }

  /** Why a downloaded file must not replace the list (null = acceptable). */
  private refusal(text: string): string | null {
    const p = parseBlockList(text);
    if (p.entries.length === 0)
      return p.invalid.length > 0
        ? `no valid entry (${p.invalid.length} invalid lines)`
        : 'the file is empty';
    const share = p.invalid.length / (p.entries.length + p.invalid.length);
    if (share > MAX_INVALID_SHARE)
      return `${p.invalid.length} invalid lines (${Math.round(share * 100)} %, at most ${MAX_INVALID_SHARE * 100} %), first on line ${p.invalid[0]!.line}`;
    if (p.entries.length > MAX_BLOCK_ENTRIES)
      return `${p.entries.length} entries, more than ${MAX_BLOCK_ENTRIES}`;
    return null;
  }

  private async download(
    list: Json,
    conditional: boolean,
    state?: FetchState | null,
  ): Promise<FetchResult> {
    const src = isPlainObject(list['source']) ? (list['source'] as Json) : {};
    if (src['kind'] !== 'url' || typeof src['url'] !== 'string')
      throw problems.badRequest('the list has no server URL (source.kind is not url)');
    let ca: string | undefined;
    if (typeof src['caRef'] === 'string') {
      ca = (await this.readSecret(src['caRef'])) ?? undefined;
      if (ca === undefined) throw new FetchError(`secret ${src['caRef']} does not exist`);
    }
    let authorization: string | undefined;
    if (typeof src['authRef'] === 'string') {
      const v = await this.readSecret(src['authRef']);
      if (v === null) throw new FetchError(`secret ${src['authRef']} does not exist`);
      authorization = authorizationHeader(src['authRef'], v);
    }
    return fetchList(src['url'], {
      verifyTls: src['verifyTls'] !== false,
      ...(ca ? { ca } : {}),
      ...(authorization ? { authorization } : {}),
      ...(conditional && state?.etag ? { etag: state.etag } : {}),
      ...(conditional && state?.lastModified ? { lastModified: state.lastModified } : {}),
    });
  }

  private async failed(
    name: string,
    why: string,
    via: 'manual' | 'scheduled',
    prev?: FetchState | null,
  ): Promise<void> {
    await this.setFetchState(name, {
      ...(prev ?? {}),
      lastFetchAt: this.now().toISOString(),
      lastResult: 'failed',
      lastError: why,
    });
    this.notificationBus?.publish('security.events', {
      type: 'global-blocking-fetch-failed',
      list: name,
    });
    await this.events.record(
      'warning',
      'global-blocking',
      'BLOCKLIST_FETCH_FAILED',
      `block list '${name}': ${why}; the last good list is kept`,
      { list: name, via, reason: why },
    );
  }

  // ---- scheduled refresh ----------------------------------------------------------------------------------------------

  /** One pass: every running list with a URL source and refreshSec that is due is downloaded; returns what happened. */
  async refreshDue(): Promise<
    { list: string; result: FetchState['lastResult']; detail?: string }[]
  > {
    const running = await this.doc('running');
    const done: { list: string; result: FetchState['lastResult']; detail?: string }[] = [];
    for (const [name, list] of Object.entries(listsOf(running))) {
      const src = isPlainObject(list['source']) ? (list['source'] as Json) : {};
      if (
        list['enabled'] === false ||
        src['kind'] !== 'url' ||
        typeof src['refreshSec'] !== 'number'
      )
        continue;
      const prev = await this.fetchState(name);
      const now = this.now();
      if (prev && now.getTime() - Date.parse(prev.lastFetchAt) < src['refreshSec'] * 1000) continue;
      // one API process refreshes a list at a time
      if ((await this.kv.set(`gb:fetching:${name}`, '1', 'EX', 120, 'NX')) !== 'OK') continue;
      try {
        done.push({ list: name, ...(await this.refreshOne(name, list, prev)) });
      } finally {
        await this.kv.del(`gb:fetching:${name}`);
      }
    }
    return done;
  }

  private async refreshOne(
    name: string,
    list: Json,
    prev: FetchState | null,
  ): Promise<{ result: FetchState['lastResult']; detail?: string }> {
    let r: FetchResult;
    try {
      r = await this.download(list, true, prev);
    } catch (e) {
      const why = e instanceof Error ? e.message : String(e);
      await this.failed(name, why, 'scheduled', prev);
      return { result: 'failed', detail: why };
    }
    const at = this.now().toISOString();
    if (r.status === 'not-modified') {
      await this.setFetchState(name, {
        ...(prev ?? {}),
        lastFetchAt: at,
        lastResult: 'unchanged',
        lastError: null,
      });
      return { result: 'unchanged' };
    }
    const bad = this.refusal(r.text);
    if (bad !== null) {
      await this.failed(name, bad, 'scheduled', prev);
      return { result: 'failed', detail: bad };
    }
    const entries = parseBlockList(r.text).entries;
    const meta = {
      ...(r.etag ? { etag: r.etag } : {}),
      ...(r.lastModified ? { lastModified: r.lastModified } : {}),
      entries: entries.length,
    };
    let diff = { added: 0, removed: 0 };
    let res: Awaited<ReturnType<CommitService['systemCommit']>>;
    try {
      res = await this.commits.systemCommit((doc) => {
        const l = listsOf(doc as Json)[name];
        if (l === undefined) return null; // deleted meanwhile
        const d = diffEntries(entriesOf(l), entries);
        diff = { added: d.added.length, removed: d.removed.length };
        if (d.added.length === 0 && d.removed.length === 0) return null;
        l['entries'] = entries;
        return doc;
      }, `global blocking: scheduled refresh of '${name}'`);
    } catch (e) {
      const why = e instanceof ProblemError ? String(e.body().detail ?? e.body().title) : String(e);
      await this.failed(
        name,
        `the refreshed list could not be committed: ${why}`,
        'scheduled',
        prev,
      );
      return { result: 'failed', detail: why };
    }
    if (res.status === 'deferred') {
      // not recorded as fetched: the next pass tries again (unconditionally, the file may have changed once more)
      await this.setFetchState(name, {
        ...(prev ?? { lastFetchAt: at }),
        lastResult: 'deferred',
        lastError: res.reason,
      });
      return { result: 'deferred', detail: res.reason };
    }
    if (res.status === 'unchanged') {
      await this.setFetchState(name, {
        lastFetchAt: at,
        lastResult: 'unchanged',
        lastError: null,
        ...meta,
      });
      return { result: 'unchanged' };
    }
    await this.setFetchState(name, { lastFetchAt: at, lastResult: 'ok', lastError: null, ...meta });
    await this.events.record(
      'info',
      'global-blocking',
      'BLOCKLIST_REFRESHED',
      `block list '${name}': ${diff.added} added, ${diff.removed} removed (${entries.length} entries)`,
      { list: name, ...diff, entries: entries.length },
    );
    return { result: 'ok', detail: `${diff.added} added, ${diff.removed} removed` };
  }
}
