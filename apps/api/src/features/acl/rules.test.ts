import { describe, expect, it } from 'vitest';
import { csvRecords, headerIndex, rowToRule, ruleToCsv, csvHeader, CsvError } from './csv.js';
import {
  applyBulk,
  BulkError,
  filterTerms,
  matchesFilter,
  pendingBySequence,
  sortedRules,
  type Json,
} from './rules.js';

const r = (sequence: number, extra: Json = {}): Json => ({
  sequence,
  action: 'permit',
  enabled: true,
  ...extra,
});

describe('acl rules helpers', () => {
  it('sorts by sequence with the document index, filters by every term', () => {
    const list = {
      rules: [
        r(30, { description: 'web servers' }),
        r(10),
        r(20, { source: { kind: 'object', name: 'lan' } }),
      ],
    };
    const s = sortedRules(list);
    expect(s.map((x) => [x.sequence, x.index])).toEqual([
      [10, 1],
      [20, 2],
      [30, 0],
    ]);
    expect(
      s.filter((x) => matchesFilter(x.rule, filterTerms('WEB permit'))).map((x) => x.sequence),
    ).toEqual([30]);
    expect(
      s.filter((x) => matchesFilter(x.rule, filterTerms('lan'))).map((x) => x.sequence),
    ).toEqual([20]);
  });

  it('marks pending rules against running by sequence', () => {
    const pend = pendingBySequence({ rules: [r(10), r(20)] });
    expect(pend(r(10))).toBeNull();
    expect(pend(r(20, { action: 'deny' }))).toBe('changed');
    expect(pend(r(30))).toBe('added');
  });

  it('bulk: disable, delete, move to sequence with shifting, renumber', () => {
    const rules = [r(10), r(20), r(30), r(40)];
    expect(applyBulk(rules, { op: 'disable', sequences: [20, 30] }).changed).toBe(2);
    expect(
      applyBulk(rules, { op: 'delete', sequences: [10] }).rules.map((x) => x['sequence']),
    ).toEqual([20, 30, 40]);
    // move 40 and 10 (in that evaluation order: 10 first) to 20: 20 is taken → 20,30,40 shift by 2
    const m = applyBulk(rules, { op: 'move', sequences: [40, 10], to: 20 });
    expect(m.rules.map((x) => x['sequence'])).toEqual([20, 21, 22, 32]);
    expect(m.rules.map((x) => (x === m.rules[0] ? 'first' : ''))[0]).toBe('first');
    // move into a free gap: nothing else moves
    const g = applyBulk(rules, { op: 'move', sequences: [40], to: 15 });
    expect(g.rules.map((x) => x['sequence'])).toEqual([10, 15, 20, 30]);
    expect(g.changed).toBe(1);
    expect(
      applyBulk(rules, { op: 'renumber', start: 100, step: 5 }).rules.map((x) => x['sequence']),
    ).toEqual([100, 105, 110, 115]);
    expect(() => applyBulk(rules, { op: 'enable', sequences: [99] })).toThrow(BulkError);
  });
});

async function* chunks(...parts: string[]) {
  for (const p of parts) yield Buffer.from(p);
}

async function records(...parts: string[]) {
  const out = [];
  for await (const rec of csvRecords(chunks(...parts))) out.push(rec);
  return out;
}

describe('acl CSV', () => {
  it('round-trips rules through export and import (quotes, line breaks, formula guard)', async () => {
    const rules: Json[] = [
      {
        sequence: 10,
        action: 'permit',
        enabled: true,
        ipVersion: 'any',
        source: { kind: 'prefix', prefix: '10.3.1.0/24' },
        destination: { kind: 'object', name: 'web-servers' },
        service: {
          kind: 'inline',
          spec: {
            protocol: 'tcp',
            destinationPorts: ['80', '8000-8080'],
            sourcePorts: ['1024-65535'],
          },
        },
        schedule: 'office-hours',
        log: false,
        description: 'web, "public"\nsecond line',
      },
      {
        sequence: 20,
        action: 'deny',
        enabled: false,
        ipVersion: 'ipv4',
        source: { kind: 'any' },
        destination: { kind: 'any' },
        service: { kind: 'inline', spec: { protocol: 'icmp', type: 3, code: 4 } },
        log: true,
        description: '=cmd()',
      },
      {
        sequence: 30,
        action: 'reflect',
        enabled: true,
        ipVersion: 'ipv6',
        source: { kind: 'any' },
        destination: { kind: 'any' },
        service: { kind: 'object', name: 'dns' },
        log: false,
      },
    ];
    const text = csvHeader() + rules.map(ruleToCsv).join('');
    expect(text).toContain("'=cmd()");
    // split into awkward chunks, inside quoted fields and between "" pairs
    const parts = [text.slice(0, 7), text.slice(7, 190), text.slice(190, 191), text.slice(191)];
    const recs = await records(...parts);
    const header = headerIndex(recs[0]!.fields);
    const back = recs.slice(1).map((rec) => rowToRule(rec, header));
    expect(back.map((b) => b.issues)).toEqual([[], [], []]);
    expect(back.map((b) => b.rule)).toEqual(rules);
    expect(recs.map((x) => x.line)).toEqual([1, 2, 4, 5]);
  });

  it('reports row issues with line and column; rejects unknown columns and oversize files', async () => {
    const recs = await records(
      'sequence,action,source,service\r\n',
      'x,permit,any,any\r\n5,allow,10.0.0.1,tcp:99999\r\n',
    );
    const h = headerIndex(recs[0]!.fields);
    expect(rowToRule(recs[1]!, h).issues[0]).toMatchObject({ line: 2, column: 'sequence' });
    const bad = rowToRule(recs[2]!, h);
    expect(bad.rule).toBeUndefined();
    expect(bad.issues.map((i) => i.column)).toContain('action');
    expect(() => headerIndex(['sequence', 'action', 'bogus'])).toThrow(CsvError);
    await expect(records('a'.repeat(10))).resolves.toHaveLength(1);
    const big = csvRecords(chunks('x'.repeat(100)), 50);
    await expect(big.next()).rejects.toThrow(/larger than 50 bytes/);
  });
});

describe('acl candidate reads', () => {
  it('the acl and objects schemas have no secret leaf (the candidate subtrees are read without redaction)', async () => {
    const { z } = await import('zod');
    const { AclSchema, ObjectsSchema } = await import('@ngfw/schema');
    for (const s of [AclSchema, ObjectsSchema]) {
      const js = JSON.stringify(z.toJSONSchema(s, { io: 'input', unrepresentable: 'any' }));
      expect(js).not.toContain('"writeOnly":true');
      expect(js).not.toContain('"secret":true');
    }
  });
});
