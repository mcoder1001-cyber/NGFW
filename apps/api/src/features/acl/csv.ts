import { AclRuleSchema } from '@ngfw/schema';
import type { Json } from './rules.js';

/**
 * F-acl CSV format of an L3/L4 list's rules (import and export; docs/user/firewall/acl.md):
 *
 *   sequence,action,enabled,ipVersion,source,destination,service,schedule,log,description
 *
 * - source / destination: `any`, a CIDR prefix (`10.0.0.0/8`; a bare address is a host prefix) or `object:<name>`;
 * - service: `any`, `object:<name>`, or inline `<protocol>[:<ports>][;src=<ports>][;flags=<value>/<mask>]` with protocol
 *   tcp, udp, tcp-udp, sctp (ports `80|443|8000-8080`), `icmp[:<type>[/<code>]]`, `icmp6[:<type>[/<code>]]`, or
 *   `proto:<number>`;
 * - enabled / log: true|false (empty = the default: enabled true, log false); ipVersion: ipv4|ipv6|any (empty = any);
 * - description: free text; RFC 4180 quoting (commas, quotes, line breaks). A text cell that starts with = + - @ is
 *   written with a leading `'` (spreadsheet formula injection) and read back without it.
 */
export const CSV_COLUMNS = [
  'sequence',
  'action',
  'enabled',
  'ipVersion',
  'source',
  'destination',
  'service',
  'schedule',
  'log',
  'description',
] as const;

export const MAX_IMPORT_ROWS = 100_000;
export const MAX_IMPORT_BYTES = 64 * 1024 * 1024;

export class CsvError extends Error {
  constructor(
    message: string,
    readonly line: number,
  ) {
    super(message);
  }
}

// ---- export ------------------------------------------------------------------------------------------------------

const FORMULA = /^[=+\-@\t\r]/;

function cell(v: string): string {
  const s = FORMULA.test(v) ? `'${v}` : v;
  return /[",\r\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

function addressCell(m: unknown): string {
  const o = (m ?? {}) as Json;
  switch (o['kind']) {
    case 'prefix':
      return String(o['prefix']);
    case 'object':
      return `object:${String(o['name'])}`;
    default:
      return 'any';
  }
}

function ports(list: unknown): string {
  return Array.isArray(list) ? list.map(String).join('|') : '';
}

function serviceCell(m: unknown): string {
  const o = (m ?? {}) as Json;
  if (o['kind'] === 'object') return `object:${String(o['name'])}`;
  if (o['kind'] !== 'inline') return 'any';
  const spec = (o['spec'] ?? {}) as Json;
  const p = String(spec['protocol'] ?? 'any');
  switch (p) {
    case 'any':
      return 'any';
    case 'icmp':
    case 'icmp6': {
      if (typeof spec['type'] !== 'number') return p;
      return typeof spec['code'] === 'number'
        ? `${p}:${spec['type']}/${spec['code']}`
        : `${p}:${spec['type']}`;
    }
    case 'other':
      return `proto:${String(spec['number'])}`;
    default: {
      let s = p;
      const dst = ports(spec['destinationPorts']);
      if (dst) s += `:${dst}`;
      const src = ports(spec['sourcePorts']);
      if (src) s += `;src=${src}`;
      const f = spec['tcpFlags'] as Json | undefined;
      if (f && typeof f['mask'] === 'number') s += `;flags=${String(f['value'] ?? 0)}/${f['mask']}`;
      return s;
    }
  }
}

export function csvHeader(): string {
  return `${CSV_COLUMNS.join(',')}\r\n`;
}

export function ruleToCsv(rule: Json): string {
  return (
    [
      String(rule['sequence'] ?? ''),
      String(rule['action'] ?? ''),
      rule['enabled'] === false ? 'false' : 'true',
      String(rule['ipVersion'] ?? 'any'),
      addressCell(rule['source']),
      addressCell(rule['destination']),
      serviceCell(rule['service']),
      typeof rule['schedule'] === 'string' ? rule['schedule'] : '',
      rule['log'] === true ? 'true' : 'false',
      typeof rule['description'] === 'string' ? rule['description'] : '',
    ]
      .map(cell)
      .join(',') + '\r\n'
  );
}

// ---- parse -------------------------------------------------------------------------------------------------------

export interface CsvRecord {
  /** 1-based line of the record's first character. */
  line: number;
  fields: string[];
}

/**
 * Streaming RFC 4180 reader: yields one record per row as chunks arrive (quoted fields may span chunks and lines).
 * Stops with a CsvError past maxBytes.
 */
export async function* csvRecords(
  chunks: AsyncIterable<Buffer | string>,
  maxBytes = MAX_IMPORT_BYTES,
): AsyncGenerator<CsvRecord> {
  let field = '';
  let fields: string[] = [];
  let quoted = false;
  let afterQuote = false;
  let line = 1;
  let start = 1;
  let bytes = 0;
  let pendingCR = false;
  let any = false;
  const decoder = new TextDecoder('utf-8', { fatal: false });
  for await (const chunk of chunks) {
    const text = typeof chunk === 'string' ? chunk : decoder.decode(chunk, { stream: true });
    bytes += typeof chunk === 'string' ? Buffer.byteLength(chunk) : chunk.length;
    if (bytes > maxBytes) throw new CsvError(`the file is larger than ${maxBytes} bytes`, line);
    for (let i = 0; i < text.length; i++) {
      const c = text[i]!;
      if (pendingCR) {
        pendingCR = false;
        if (c === '\n') continue;
      }
      if (quoted) {
        if (c === '"') {
          if (text[i + 1] === '"') {
            field += '"';
            i++;
          } else {
            // closing quote; when it ends the chunk and the next chunk starts with '"', it was an escaped quote
            quoted = false;
            afterQuote = true;
          }
        } else {
          if (c === '\n') line++;
          field += c;
        }
        continue;
      }
      if (afterQuote && c === '"') {
        // "" split across two chunks: an escaped quote
        field += '"';
        quoted = true;
        afterQuote = false;
        continue;
      }
      afterQuote = false;
      if (c === '"' && field.length === 0) {
        quoted = true;
        any = true;
      } else if (c === ',') {
        fields.push(field);
        field = '';
        any = true;
      } else if (c === '\r' || c === '\n') {
        if (any || field.length > 0 || fields.length > 0) {
          fields.push(field);
          yield { line: start, fields };
        }
        fields = [];
        field = '';
        any = false;
        line++;
        start = line;
        pendingCR = c === '\r';
      } else {
        field += c;
        any = true;
      }
    }
  }
  if (quoted) throw new CsvError('unterminated quoted field', start);
  if (any || field.length > 0 || fields.length > 0) {
    fields.push(field);
    yield { line: start, fields };
  }
}

function unformula(v: string): string {
  return v.length > 1 && v[0] === "'" && FORMULA.test(v.slice(1)) ? v.slice(1) : v;
}

function bool(v: string, column: string): boolean | undefined {
  const s = v.trim().toLowerCase();
  if (s === '') return undefined;
  if (['true', 'yes', '1'].includes(s)) return true;
  if (['false', 'no', '0'].includes(s)) return false;
  throw new Error(`${column}: expected true or false, got '${v}'`);
}

function addressFromCell(v: string, column: string): Json {
  const s = v.trim();
  if (s === '' || s.toLowerCase() === 'any') return { kind: 'any' };
  if (s.startsWith('object:')) return { kind: 'object', name: s.slice(7) };
  if (s.includes('/')) return { kind: 'prefix', prefix: s };
  if (/^[0-9.]+$/.test(s)) return { kind: 'prefix', prefix: `${s}/32` };
  if (s.includes(':')) return { kind: 'prefix', prefix: `${s}/128` };
  throw new Error(`${column}: expected any, a prefix or object:<name>, got '${v}'`);
}

function portList(s: string): string[] {
  return s
    .split('|')
    .map((p) => p.trim())
    .filter((p) => p.length > 0);
}

function serviceFromCell(v: string): Json {
  const s = v.trim();
  if (s === '' || s.toLowerCase() === 'any') return { kind: 'any' };
  if (s.startsWith('object:')) return { kind: 'object', name: s.slice(7) };
  const [head = '', ...opts] = s.split(';');
  const [proto = '', arg] = head.split(':', 2) as [string, string | undefined];
  const p = proto.toLowerCase();
  const num = (x: string, what: string): number => {
    if (!/^\d+$/.test(x)) throw new Error(`service: ${what} '${x}' is not a number`);
    return Number(x);
  };
  if (p === 'proto') {
    if (arg === undefined) throw new Error('service: proto:<number> needs a number');
    return { kind: 'inline', spec: { protocol: 'other', number: num(arg, 'protocol') } };
  }
  if (p === 'icmp' || p === 'icmp6') {
    const spec: Json = { protocol: p };
    if (arg !== undefined && arg !== '') {
      const [t = '', c] = arg.split('/');
      spec['type'] = num(t, 'ICMP type');
      if (c !== undefined) spec['code'] = num(c, 'ICMP code');
    }
    return { kind: 'inline', spec };
  }
  if (!['tcp', 'udp', 'tcp-udp', 'sctp'].includes(p)) {
    throw new Error(
      `service: unknown protocol '${proto}' (any, object:<name>, tcp, udp, tcp-udp, sctp, icmp, icmp6, proto:<n>)`,
    );
  }
  const spec: Json = { protocol: p };
  if (arg !== undefined && arg !== '') spec['destinationPorts'] = portList(arg);
  for (const o of opts) {
    const [k = '', val = ''] = o.split('=', 2);
    if (k.trim() === 'src') spec['sourcePorts'] = portList(val);
    else if (k.trim() === 'flags') {
      const [value = '', mask = ''] = val.split('/');
      spec['tcpFlags'] = {
        value: num(value, 'TCP flags value'),
        mask: num(mask, 'TCP flags mask'),
      };
    } else throw new Error(`service: unknown option '${o}' (src=…, flags=<value>/<mask>)`);
  }
  return { kind: 'inline', spec };
}

export interface RowIssue {
  line: number;
  column?: string;
  message: string;
}

/** Header check: the columns must be CSV_COLUMNS (in any order; description and schedule may be omitted). */
export function headerIndex(fields: string[]): Map<string, number> {
  const idx = new Map<string, number>();
  fields.forEach((f, i) => idx.set(f.trim().replace(/^\uFEFF/, ''), i));
  for (const c of ['sequence', 'action']) {
    if (!idx.has(c))
      throw new CsvError(`the header has no '${c}' column (expected ${CSV_COLUMNS.join(',')})`, 1);
  }
  for (const k of idx.keys()) {
    if (!(CSV_COLUMNS as readonly string[]).includes(k)) {
      throw new CsvError(`unknown column '${k}' (expected ${CSV_COLUMNS.join(',')})`, 1);
    }
  }
  return idx;
}

/** One CSV row → a schema-valid ACL rule (defaults applied), or the row's issues. */
export function rowToRule(
  rec: CsvRecord,
  header: Map<string, number>,
): { rule?: Json; issues: RowIssue[] } {
  const get = (c: string): string => {
    const i = header.get(c);
    return i === undefined ? '' : unformula(rec.fields[i] ?? '');
  };
  const raw: Json = {};
  const issues: RowIssue[] = [];
  const step = (column: string, f: () => void) => {
    try {
      f();
    } catch (e) {
      issues.push({ line: rec.line, column, message: e instanceof Error ? e.message : String(e) });
    }
  };
  step('sequence', () => {
    const s = get('sequence').trim();
    if (!/^\d+$/.test(s)) throw new Error(`sequence: '${s}' is not a positive integer`);
    raw['sequence'] = Number(s);
  });
  raw['action'] = get('action').trim().toLowerCase();
  step('enabled', () => {
    const b = bool(get('enabled'), 'enabled');
    if (b !== undefined) raw['enabled'] = b;
  });
  const ver = get('ipVersion').trim().toLowerCase();
  if (ver !== '') raw['ipVersion'] = ver;
  step('source', () => (raw['source'] = addressFromCell(get('source'), 'source')));
  step(
    'destination',
    () => (raw['destination'] = addressFromCell(get('destination'), 'destination')),
  );
  step('service', () => (raw['service'] = serviceFromCell(get('service'))));
  const sched = get('schedule').trim();
  if (sched !== '') raw['schedule'] = sched;
  step('log', () => {
    const b = bool(get('log'), 'log');
    if (b !== undefined) raw['log'] = b;
  });
  const desc = get('description');
  if (desc !== '') raw['description'] = desc;
  if (issues.length > 0) return { issues };
  const r = AclRuleSchema.safeParse(raw);
  if (!r.success) {
    for (const i of r.error.issues) {
      issues.push({
        line: rec.line,
        column: String(i.path[0] ?? ''),
        message: `${i.path.join('.') || 'rule'}: ${i.message}`,
      });
    }
    return { issues };
  }
  return { rule: r.data as Json, issues };
}
