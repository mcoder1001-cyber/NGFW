import { Injectable, type OnModuleInit } from '@nestjs/common';
import { HttpAdapterHost } from '@nestjs/core';
import { AclRuleStatus, type AclBoundAcl, type AclStateResponse } from '@ngfw/proto';
import { isPlainObject } from '@ngfw/schema';
import type { FastifyInstance } from 'fastify';
import { Readable } from 'node:stream';
import { AgentClient } from '../../agent/agent.client.js';
import { ProblemError, problems } from '../../common/problem.js';
import type { Principal } from '../../common/principal.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import {
  CsvError,
  csvHeader,
  csvRecords,
  headerIndex,
  MAX_IMPORT_ROWS,
  rowToRule,
  ruleToCsv,
  type RowIssue,
} from './csv.js';
import {
  aclOf,
  applyBulk,
  BulkError,
  filterTerms,
  listsOf,
  matchesFilter,
  pendingBySequence,
  rulesOf,
  sameJson,
  sortedRules,
  type BulkOp,
  type Json,
  type Pending,
} from './rules.js';

export type Source = 'running' | 'candidate';

/** Live (agent) part of a list or rule; null when the agent cannot say. */
export interface LiveList {
  aclIndex: number;
  vppRules: number;
  mappingKnown: boolean;
  configRules: number;
  packets: number;
  bytes: number;
}

export interface LiveRule {
  status: 'applied' | 'disabled' | 'schedule-inactive' | 'empty' | 'unknown';
  vppRules: number;
  packets: number;
  bytes: number;
}

const STATUS: Record<number, LiveRule['status']> = {
  [AclRuleStatus.ACL_RULE_STATUS_APPLIED]: 'applied',
  [AclRuleStatus.ACL_RULE_STATUS_DISABLED]: 'disabled',
  [AclRuleStatus.ACL_RULE_STATUS_SCHEDULE_INACTIVE]: 'schedule-inactive',
  [AclRuleStatus.ACL_RULE_STATUS_EMPTY]: 'empty',
};

export interface AgentView<T> {
  value?: T;
  /** Why the live part is missing (agent unavailable, older agent, list not applied). */
  error?: string;
  status?: number;
}

/** Counter page size the rule view asks the agent for at most (AclState's limit). */
const AGENT_PAGE = 1000;
/** hits-only scans the agent's pages up to this many rules. */
const HITS_SCAN = 100_000;

function num(v: string | number | undefined): number {
  return v === undefined ? 0 : Number(v);
}

function bound(b: AclBoundAcl): {
  aclIndex: number;
  name: string | null;
  tag: string;
  foreign: boolean;
} {
  return { aclIndex: b.aclIndex, name: b.name || null, tag: b.tag, foreign: b.foreign };
}

/**
 * F-acl API logic: the ACL state routes merge the configuration document (running or candidate) with the agent's
 * AclState (counters per configuration rule, VPP indexes, bindings incl. other owners' ACLs); CSV import/export and
 * bulk edits change the candidate through the datastore (one edit each, compact audit rows).
 */
@Injectable()
export class AclService implements OnModuleInit {
  constructor(
    private readonly ds: DatastoreService,
    private readonly agent: AgentClient,
    private readonly host: HttpAdapterHost,
  ) {}

  /** `text/csv` bodies reach the import route as the raw request stream (streamed parse, own size cap). */
  onModuleInit(): void {
    const f = this.host.httpAdapter?.getInstance?.() as FastifyInstance | undefined;
    if (f && !f.hasContentTypeParser('text/csv')) {
      f.addContentTypeParser('text/csv', (_req, payload, done) => done(null, payload));
    }
  }

  /** The running document, reused while the latest revision id is unchanged (revisions are immutable). */
  private runningCache: { id: number | null; doc: Json } | undefined;

  async doc(source: Source): Promise<Json> {
    if (source === 'candidate') return await this.ds.getCandidate();
    const latest = (await this.ds.listRevisions(1, 0)).items[0]?.id ?? null;
    if (this.runningCache !== undefined && this.runningCache.id === latest) return this.runningCache.doc;
    const r = await this.ds.getRunning();
    this.runningCache = { id: r.revision?.id ?? null, doc: r.doc };
    return r.doc;
  }

  private async live<T>(f: () => Promise<T>): Promise<AgentView<T>> {
    try {
      return { value: await f() };
    } catch (e) {
      if (e instanceof ProblemError) {
        const b = e.body();
        return { error: String(b.detail ?? b.title), status: b.status };
      }
      return { error: e instanceof Error ? e.message : String(e) };
    }
  }

  // ---- state: lists -----------------------------------------------------------------------------------------------

  async lists() {
    const [running, candidate] = await Promise.all([this.doc('running'), this.doc('candidate')]);
    const live = await this.live(() =>
      this.agent.aclState({
        list: '',
        offset: 0,
        limit: 0,
        filter: undefined,
        includeInterfaces: false,
      }),
    );
    const liveLists = new Map((live.value?.lists ?? []).map((l) => [l.name, l]));
    const liveMacip = new Map((live.value?.macipLists ?? []).map((l) => [l.name, l]));
    const attachments = (aclOf(candidate)['attachments'] as Json[] | undefined) ?? [];
    const macipAttachments = (aclOf(candidate)['macipAttachments'] as Json[] | undefined) ?? [];
    const pending = (name: string, kind: 'lists' | 'macip'): Pending | 'deleted' => {
      const r = listsOf(running, kind)[name];
      const c = listsOf(candidate, kind)[name];
      if (r === undefined) return 'added';
      if (c === undefined) return 'deleted';
      return sameJson(r, c) ? null : 'changed';
    };
    const names = (kind: 'lists' | 'macip') =>
      [
        ...new Set([
          ...Object.keys(listsOf(candidate, kind)),
          ...Object.keys(listsOf(running, kind)),
        ]),
      ].sort();
    return {
      countersAvailable: live.value?.countersAvailable ?? false,
      countersReason: live.value?.countersReason ?? '',
      agentError: live.error ?? null,
      retrievedAt: live.value?.retrievedAt?.toISOString() ?? null,
      lists: names('lists').map((name) => {
        const list = listsOf(candidate)[name] ?? listsOf(running)[name] ?? {};
        const l = liveLists.get(name);
        return {
          name,
          description: typeof list['description'] === 'string' ? list['description'] : null,
          tags: Array.isArray(list['tags']) ? (list['tags'] as string[]) : [],
          rules: rulesOf(list).length,
          pending: pending(name, 'lists'),
          attachments: attachments
            .filter((a) => a['list'] === name)
            .map((a) => ({
              target: a['target'] as Json,
              direction: (a['direction'] as string | undefined) ?? 'in',
              sequence: a['sequence'] as number,
              enabled: a['enabled'] !== false,
            })),
          live:
            l === undefined
              ? null
              : {
                  aclIndex: l.aclIndex,
                  vppRules: l.vppRules,
                  mappingKnown: l.mappingKnown,
                  configRules: l.configRules,
                  packets: num(l.packets),
                  bytes: num(l.bytes),
                },
        };
      }),
      macip: names('macip').map((name) => {
        const list = listsOf(candidate, 'macip')[name] ?? listsOf(running, 'macip')[name] ?? {};
        const l = liveMacip.get(name);
        return {
          name,
          description: typeof list['description'] === 'string' ? list['description'] : null,
          rules: rulesOf(list).length,
          pending: pending(name, 'macip'),
          interfaces: macipAttachments
            .filter((a) => a['list'] === name && a['enabled'] !== false)
            .map((a) => String(a['interface'])),
          live: l === undefined ? null : { aclIndex: l.aclIndex, vppRules: l.vppRules },
        };
      }),
    };
  }

  // ---- state: one list's rules --------------------------------------------------------------------------------------

  async rules(
    name: string,
    q: {
      page: number;
      pageSize: number;
      filter?: string | undefined;
      source: Source;
      hitsOnly: boolean;
    },
  ) {
    const [doc, running] = await Promise.all([
      this.doc(q.source),
      q.source === 'running' ? Promise.resolve(undefined) : this.doc('running'),
    ]);
    const list = listsOf(doc)[name];
    if (list === undefined)
      throw problems.notFound(
        `access list '${name}' does not exist in the ${q.source} configuration`,
      );
    const terms = filterTerms(q.filter);
    let rows = sortedRules(list).filter((r) => matchesFilter(r.rule, terms));
    let hitsError: string | undefined;
    if (q.hitsOnly) {
      const hits = await this.live(() => this.hitSequences(name));
      if (hits.value) rows = rows.filter((r) => hits.value!.has(r.sequence));
      else {
        rows = [];
        hitsError = hits.error;
      }
    }
    const total = rows.length;
    const start = (q.page - 1) * q.pageSize;
    const page = rows.slice(start, start + q.pageSize);
    const pend =
      q.source === 'candidate' ? pendingBySequence(listsOf(running ?? {})[name]) : () => null;
    let live: AgentView<AclStateResponse> = { error: 'no rows' };
    if (page.length > 0) {
      live = await this.live(() =>
        this.agent.aclState({
          list: name,
          offset: 0,
          limit: Math.min(AGENT_PAGE, page.length),
          filter: { sequences: page.map((r) => r.sequence), hitsOnly: false },
          includeInterfaces: false,
        }),
      );
    }
    const bySeq = new Map((live.value?.rules ?? []).map((r) => [r.sequence, r]));
    const summary = live.value?.lists[0];
    return {
      list: name,
      source: q.source,
      page: q.page,
      pageSize: q.pageSize,
      total,
      size: rulesOf(list).length,
      applied: live.value !== undefined,
      mappingKnown: summary?.mappingKnown ?? false,
      countersAvailable: live.value?.countersAvailable ?? false,
      countersReason: live.value?.countersReason ?? '',
      agentError:
        hitsError ??
        (live.status === 404
          ? 'the list is not applied (commit it first)'
          : live.value
            ? null
            : page.length > 0
              ? (live.error ?? null)
              : null),
      items: page.map((r) => {
        const l = bySeq.get(r.sequence);
        return {
          index: r.index,
          sequence: r.sequence,
          rule: r.rule,
          pending: pend(r.rule),
          live:
            l === undefined
              ? null
              : {
                  status: STATUS[l.status] ?? 'unknown',
                  vppRules: l.vppRules,
                  packets: num(l.packets),
                  bytes: num(l.bytes),
                },
        };
      }),
    };
  }

  /** Sequences of the list's rules that have hits (the agent pages ≤ 1000 at a time). */
  private async hitSequences(name: string): Promise<Set<number>> {
    const out = new Set<number>();
    for (let offset = 0; offset < HITS_SCAN; offset += AGENT_PAGE) {
      const r = await this.agent.aclState({
        list: name,
        offset,
        limit: AGENT_PAGE,
        filter: { sequences: [], hitsOnly: true },
        includeInterfaces: false,
      });
      for (const x of r.rules) out.add(x.sequence);
      if (offset + AGENT_PAGE >= r.total) break;
    }
    return out;
  }

  // ---- state: attachments -------------------------------------------------------------------------------------------

  async attachments() {
    const running = await this.doc('running');
    const expected = expectedBindings(running);
    const live = await this.live(() =>
      this.agent.aclState({
        list: '',
        offset: 0,
        limit: 0,
        filter: undefined,
        includeInterfaces: true,
      }),
    );
    const byName = new Map<string, ReturnType<typeof liveIface>>();
    for (const i of live.value?.interfaces ?? []) byName.set(i.interface, liveIface(i));
    const names = [...new Set([...byName.keys(), ...expected.keys()])].sort();
    return {
      agentError: live.error ?? null,
      retrievedAt: live.value?.retrievedAt?.toISOString() ?? null,
      interfaces: names.map((name) => {
        const l = byName.get(name);
        const e = expected.get(name) ?? { input: [], output: [], macip: null };
        const ours = (xs: { name: string | null; foreign: boolean }[]) =>
          xs.filter((x) => !x.foreign).map((x) => x.name);
        const inSync =
          live.value !== undefined &&
          sameJson(ours(l?.input ?? []), e.input) &&
          sameJson(ours(l?.output ?? []), e.output) &&
          (l?.macip && !l.macip.foreign ? l.macip.name : null) === e.macip;
        return {
          interface: name,
          swIfIndex: l?.swIfIndex ?? null,
          input: l?.input ?? [],
          output: l?.output ?? [],
          macip: l?.macip ?? null,
          expected: e,
          inSync: live.value === undefined ? null : inSync,
        };
      }),
    };
  }

  // ---- CSV ----------------------------------------------------------------------------------------------------------

  async *exportCsv(name: string, source: Source): AsyncGenerator<string> {
    const list = listsOf(await this.doc(source))[name];
    if (list === undefined)
      throw problems.notFound(
        `access list '${name}' does not exist in the ${source} configuration`,
      );
    yield csvHeader();
    let chunk = '';
    for (const r of sortedRules(list)) {
      chunk += ruleToCsv(r.rule);
      if (chunk.length > 64 * 1024) {
        yield chunk;
        chunk = '';
      }
    }
    if (chunk) yield chunk;
  }

  async exportStream(name: string, source: Source): Promise<Readable> {
    const it = this.exportCsv(name, source);
    const first = await it.next(); // 404 before the headers are sent
    return Readable.from(
      (async function* () {
        if (!first.done) yield first.value;
        yield* it;
      })(),
    );
  }

  async importCsv(
    user: Principal,
    name: string,
    body: unknown,
    opts: { mode: 'replace' | 'append'; dryRun: boolean },
  ) {
    const chunks = toChunks(body);
    const issues: RowIssue[] = [];
    let errorCount = 0;
    const rules: Json[] = [];
    let rows = 0;
    let header: Map<string, number> | undefined;
    const seen = new Map<number, number>();
    try {
      for await (const rec of csvRecords(chunks)) {
        if (header === undefined) {
          header = headerIndex(rec.fields);
          continue;
        }
        if (rec.fields.every((f) => f.trim() === '')) continue;
        rows++;
        if (rows > MAX_IMPORT_ROWS)
          throw new CsvError(`more than ${MAX_IMPORT_ROWS} rows`, rec.line);
        const r = rowToRule(rec, header);
        for (const i of r.issues) {
          errorCount++;
          if (issues.length < 200) issues.push(i);
        }
        if (r.rule) {
          const seq = r.rule['sequence'] as number;
          const first = seen.get(seq);
          if (first !== undefined) {
            errorCount++;
            if (issues.length < 200)
              issues.push({
                line: rec.line,
                column: 'sequence',
                message: `sequence ${seq} is already used on line ${first}`,
              });
            continue;
          }
          seen.set(seq, rec.line);
          rules.push(r.rule);
        }
      }
    } catch (e) {
      if (e instanceof CsvError) {
        throw problems.badRequest(`CSV: ${e.message}`, [
          { pointer: `/csv/${e.line}`, message: e.message },
        ]);
      }
      throw e;
    }
    if (header === undefined)
      throw problems.badRequest('CSV: the file is empty (no header row)', [
        { pointer: '/csv/1', message: 'no header row' },
      ]);

    const candidate = await this.ds.getCandidate();
    const existing = listsOf(candidate)[name];
    const existingRules = rulesOf(existing);
    if (opts.mode === 'append') {
      const taken = new Set(existingRules.map((r) => r['sequence'] as number));
      for (const r of rules) {
        if (taken.has(r['sequence'] as number)) {
          errorCount++;
          if (issues.length < 200) {
            issues.push({
              line: seen.get(r['sequence'] as number) ?? 0,
              column: 'sequence',
              message: `sequence ${String(r['sequence'])} is already used in list '${name}'`,
            });
          }
        }
      }
    }
    issues.sort((a, b) => a.line - b.line);
    const warnings = unknownReferences(candidate, rules).slice(0, 200);
    const result = {
      list: name,
      mode: opts.mode,
      dryRun: opts.dryRun,
      rows,
      valid: rules.length,
      errorCount,
      errors: issues,
      warnings,
      existingRules: existingRules.length,
      preview: rules.slice(0, 20),
    };
    if (opts.dryRun) return { ...result, imported: 0, total: existingRules.length };
    if (errorCount > 0) {
      throw problems.validation(
        issues.map((i) => ({
          pointer: `/csv/${i.line}${i.column ? `/${i.column}` : ''}`,
          message: i.message,
        })),
        `the CSV has ${errorCount} error(s); nothing was imported`,
      );
    }
    const next = [...(opts.mode === 'append' ? existingRules : []), ...rules].sort(
      (a, b) => (a['sequence'] as number) - (b['sequence'] as number),
    );
    if (existing === undefined) {
      await this.ds.putCandidate(user, `/acl/lists/${escapeToken(name)}`, { rules: next });
    } else {
      await this.ds.putCandidate(user, `/acl/lists/${escapeToken(name)}/rules`, next);
    }
    return { ...result, preview: [], imported: rules.length, total: next.length };
  }

  // ---- bulk ---------------------------------------------------------------------------------------------------------

  async bulk(user: Principal, name: string, op: BulkOp) {
    const candidate = await this.ds.getCandidate();
    const list = listsOf(candidate)[name];
    if (list === undefined)
      throw problems.notFound(`access list '${name}' does not exist in the candidate`);
    const before = rulesOf(list);
    let out: { rules: Json[]; changed: number };
    try {
      out = applyBulk(before, op);
    } catch (e) {
      if (e instanceof BulkError)
        throw problems.badRequest(e.message, [{ pointer: '/sequences', message: e.message }]);
      throw e;
    }
    if (out.changed > 0)
      await this.ds.putCandidate(user, `/acl/lists/${escapeToken(name)}/rules`, out.rules);
    return {
      list: name,
      op: op.op,
      changed: out.changed,
      total: out.rules.length,
      before: before.length,
    };
  }
}

function escapeToken(s: string): string {
  return s.replace(/~/g, '~0').replace(/\//g, '~1');
}

function toChunks(body: unknown): AsyncIterable<Buffer | string> {
  if (typeof body === 'string') return Readable.from([body]);
  if (Buffer.isBuffer(body)) return Readable.from([body]);
  if (body && typeof (body as AsyncIterable<unknown>)[Symbol.asyncIterator] === 'function') {
    return body as AsyncIterable<Buffer | string>;
  }
  throw problems.badRequest('send the CSV as the request body with content-type text/csv');
}

/** Object names the imported rules use that the candidate's objects do not define (warnings: commit rejects them). */
function unknownReferences(doc: Json, rules: Json[]): { line?: number; message: string }[] {
  const objs = isPlainObject(doc['objects']) ? (doc['objects'] as Json) : {};
  const has = (kinds: string[], n: string) =>
    kinds.some((k) => isPlainObject(objs[k]) && Object.hasOwn(objs[k] as Json, n));
  const out: { message: string }[] = [];
  const seen = new Set<string>();
  const note = (msg: string) => {
    if (!seen.has(msg)) {
      seen.add(msg);
      out.push({ message: msg });
    }
  };
  for (const r of rules) {
    for (const side of ['source', 'destination'] as const) {
      const m = r[side] as Json | undefined;
      if (m?.['kind'] === 'object' && !has(['addresses', 'addressGroups'], String(m['name']))) {
        note(
          `address object '${String(m['name'])}' is not defined in objects (the commit will reject it)`,
        );
      }
    }
    const s = r['service'] as Json | undefined;
    if (s?.['kind'] === 'object' && !has(['services', 'serviceGroups'], String(s['name']))) {
      note(
        `service object '${String(s['name'])}' is not defined in objects (the commit will reject it)`,
      );
    }
    if (typeof r['schedule'] === 'string' && !has(['schedules'], r['schedule'])) {
      note(`schedule '${r['schedule']}' is not defined in objects (the commit will reject it)`);
    }
  }
  return out;
}

function liveIface(i: AclStateResponse['interfaces'][number]) {
  return {
    swIfIndex: i.swIfIndex,
    input: i.input.map(bound),
    output: i.output.map(bound),
    macip: i.macip ? bound(i.macip) : null,
  };
}

/** Bindings the running configuration asks for, per interface (zones expanded, ordered by attachment sequence). */
export function expectedBindings(
  doc: Json,
): Map<string, { input: string[]; output: string[]; macip: string | null }> {
  const acl = aclOf(doc);
  const objs = isPlainObject(doc['objects']) ? (doc['objects'] as Json) : {};
  const zones = isPlainObject(objs['zones']) ? (objs['zones'] as Record<string, Json>) : {};
  const per = new Map<
    string,
    {
      in: { seq: number; i: number; list: string }[];
      out: { seq: number; i: number; list: string }[];
    }
  >();
  ((acl['attachments'] as Json[] | undefined) ?? []).forEach((a, i) => {
    if (a['enabled'] === false) return;
    const t = (a['target'] ?? {}) as Json;
    const ifs =
      t['kind'] === 'zone'
        ? (((zones[String(t['zone'])] ?? {})['interfaces'] as string[] | undefined) ?? [])
        : [String(t['interface'])];
    for (const n of ifs) {
      const e = per.get(n) ?? { in: [], out: [] };
      (a['direction'] === 'out' ? e.out : e.in).push({
        seq: Number(a['sequence'] ?? 0),
        i,
        list: String(a['list']),
      });
      per.set(n, e);
    }
  });
  const ord = (xs: { seq: number; i: number; list: string }[]) =>
    xs.sort((a, b) => a.seq - b.seq || a.i - b.i).map((x) => x.list);
  const out = new Map<string, { input: string[]; output: string[]; macip: string | null }>();
  for (const [n, e] of per) out.set(n, { input: ord(e.in), output: ord(e.out), macip: null });
  for (const a of (acl['macipAttachments'] as Json[] | undefined) ?? []) {
    if (a['enabled'] === false) continue;
    const n = String(a['interface']);
    const e = out.get(n) ?? { input: [], output: [], macip: null };
    e.macip = String(a['list']);
    out.set(n, e);
  }
  return out;
}
