import {
  deepEqual,
  isPlainObject,
  jsonPointer,
  parsePointer,
  pointerIssues,
  redactSecrets,
  RootConfig,
  SECRET_KINDS,
  secretPointers,
} from '@ngfw/schema';
import { z } from 'zod';
import { problems, type ProblemIssue } from '../common/problem.js';
import { getAt } from '../common/json.js';
import type { Doc } from './repo.js';

/** Running configuration before the first commit: the empty document with every default filled (D-048). */
export function emptyDocument(): Doc {
  return RootConfig.parse({}) as Doc;
}

/**
 * Tier (a) on the write path: parse with the root schema (defaults filled, unknown keys rejected). Candidates are
 * always stored parsed, so every stored document is complete. Throws 400 problem+json with pointers.
 */
export function parseDocument(doc: unknown): Doc {
  const r = RootConfig.safeParse(doc);
  if (!r.success)
    throw problems.validation(pointerIssues(r.error), 'the document does not match the schema');
  return r.data as Doc;
}

/** Top-level key of a pointer (`/interfaces/loop0` → `interfaces`), '' for the whole document. */
export function rootKeyOf(pointer: string): string {
  return parsePointer(pointer)[0] ?? '';
}

/** `redactSecrets` for any document (D-046/D-070): what GET, diff, revisions, export and audit show. */
export function redact<T>(doc: T): T {
  return redactSecrets(doc);
}

/** The redacted node at `pointer` of a whole document. */
export function redactedAt(doc: Doc, pointer: string): unknown {
  return getAt(redact(doc), pointer);
}

const USERS_POINTER = '/management/users';

/**
 * Revisions carry no password hashes (D-046); app_user does. Before validating or applying a stored document, users
 * without `passwordHash` get their stored hash back so `management.admin-exists` sees the real state. The result
 * never leaves the process except through redactSecrets.
 */
export function hydrateHashes(doc: Doc, hashes: ReadonlyMap<string, string>): Doc {
  const users = getAt(doc, USERS_POINTER);
  if (!Array.isArray(users) || users.length === 0) return doc;
  const copy = structuredClone(doc);
  for (const u of getAt(copy, USERS_POINTER) as unknown[]) {
    if (isPlainObject(u) && u['passwordHash'] === undefined && typeof u['username'] === 'string') {
      const h = hashes.get(u['username']);
      if (h !== undefined) u['passwordHash'] = h;
    }
  }
  return copy;
}

/**
 * TD-2 #1: a password set through the API replaces the hash a stored document (candidate, pending commit) already
 * carries for that user, so a later commit/confirm does not write the old hash back into app_user. Returns null when
 * the document has no hash for the user (it is hydrated from app_user then). Never used for revisions (redacted).
 */
export function replaceUserHash(doc: Doc, username: string, hash: string): Doc | null {
  const users = getAt(doc, USERS_POINTER);
  if (!Array.isArray(users)) return null;
  const i = users.findIndex(
    (u) => isPlainObject(u) && u['username'] === username && u['passwordHash'] !== undefined,
  );
  if (i < 0) return null;
  const copy = structuredClone(doc);
  (getAt(copy, USERS_POINTER) as Record<string, unknown>[])[i]!['passwordHash'] = hash;
  return copy;
}

/**
 * D-097 / TD-2 review H1: password hashes are owned by app_user. The document a commit stores (pending row, in-flight
 * reconcile copy, the one promote() syncs into app_user) keeps only the hashes the RAW candidate explicitly staged
 * (an admin setting one through the config API) — never the ones hydration copied from app_user, so a delayed
 * promote/confirm/reconcile cannot write an older hash back. `raw` is the unhydrated document that was committed.
 */
export function stagedHashesOnly(config: Doc, raw: Doc): Doc {
  const users = getAt(config, USERS_POINTER);
  if (!Array.isArray(users)) return config;
  const staged = new Map<string, unknown>();
  const rawUsers = getAt(raw, USERS_POINTER);
  if (Array.isArray(rawUsers))
    for (const u of rawUsers)
      if (isPlainObject(u) && typeof u['username'] === 'string' && u['passwordHash'] !== undefined)
        staged.set(u['username'], u['passwordHash']);
  const copy = structuredClone(config);
  for (const u of getAt(copy, USERS_POINTER) as unknown[]) {
    if (!isPlainObject(u) || typeof u['username'] !== 'string') continue;
    if (staged.has(u['username'])) u['passwordHash'] = staged.get(u['username']);
    else delete u['passwordHash'];
  }
  return copy;
}

/** Import (D-097): a snapshot never brings password hashes; returns the document without them and their pointers. */
export function withoutPasswordHashes(doc: unknown): { doc: unknown; removed: string[] } {
  const users = getAt(doc, USERS_POINTER);
  if (!Array.isArray(users)) return { doc, removed: [] };
  const copy = structuredClone(doc);
  const removed: string[] = [];
  (getAt(copy, USERS_POINTER) as unknown[]).forEach((u, i) => {
    if (isPlainObject(u) && u['passwordHash'] !== undefined) {
      delete u['passwordHash'];
      removed.push(`${USERS_POINTER}/${i}/passwordHash`);
    }
  });
  return { doc: copy, removed };
}

/** Natural key of an array item for secret preservation: users by `username`, other lists by `name`. */
function itemKey(item: unknown): string | undefined {
  if (!isPlainObject(item)) return undefined;
  for (const k of ['username', 'name']) if (typeof item[k] === 'string') return `${k}=${item[k]}`;
  return undefined;
}

/**
 * Round-trip rule (D-046): a secret leaf that the previous candidate had and the new document omits keeps its value
 * — GET (redacted) → edit → PUT never wipes a password hash. Arrays are matched by natural key, not by index.
 */
export function preserveSecrets(prev: Doc, next: Doc): Doc {
  const pointers = secretPointers(prev);
  if (pointers.length === 0) return next;
  const out = structuredClone(next);
  for (const pointer of pointers) {
    const segs = parsePointer(pointer);
    let from: unknown = prev;
    let to: unknown = out;
    for (const seg of segs.slice(0, -1)) {
      if (Array.isArray(from) && Array.isArray(to)) {
        const item = from[Number(seg)];
        const key = itemKey(item);
        from = item;
        to = key === undefined ? to[Number(seg)] : to.find((x) => itemKey(x) === key);
      } else if (isPlainObject(from) && isPlainObject(to)) {
        from = from[seg];
        to = to[seg];
      } else {
        to = undefined;
        break;
      }
    }
    const leaf = segs.at(-1) as string;
    if (
      isPlainObject(from) &&
      isPlainObject(to) &&
      to[leaf] === undefined &&
      from[leaf] !== undefined
    ) {
      to[leaf] = from[leaf];
    }
  }
  return out;
}

/**
 * A change of a secret (write-only) leaf, reported without any value (TD-2 #6, P07b review H1): diffs of redacted
 * documents cannot see a password-hash-only edit, so it would stay invisible in the pending changes, the commit
 * dialog and the revision history while still being committed.
 */
export interface SecretChange {
  op: 'add' | 'remove' | 'replace';
  pointer: string;
  redacted: true;
}

/** Secret leaves of `doc` by natural path (`/management/users/username=alice/passwordHash`) → {pointer, value}. */
function secretLeaves(doc: Doc): Map<string, { pointer: string; value: unknown }> {
  const out = new Map<string, { pointer: string; value: unknown }>();
  for (const pointer of secretPointers(doc)) {
    let node: unknown = doc;
    const natural: string[] = [];
    for (const seg of parsePointer(pointer)) {
      if (Array.isArray(node)) {
        const item = node[Number(seg)];
        natural.push(itemKey(item) ?? seg);
        node = item;
      } else {
        natural.push(seg);
        node = isPlainObject(node) ? node[seg] : undefined;
      }
    }
    out.set(natural.map((x) => '/' + x).join(''), { pointer, value: node });
  }
  return out;
}

/**
 * Secret leaves that differ between two documents in the SAME hydration state (both hydrated, or both raw), matched
 * by natural key (users by username), as value-free `SecretChange`s sorted by pointer. The pointer is the leaf's
 * position in `after` (in `before` for a removal).
 */
export function secretChanges(before: Doc, after: Doc): SecretChange[] {
  const a = secretLeaves(before);
  const b = secretLeaves(after);
  const out: SecretChange[] = [];
  for (const [k, x] of a) {
    const y = b.get(k);
    if (y === undefined) out.push({ op: 'remove', pointer: x.pointer, redacted: true });
    else if (!deepEqual(x.value, y.value))
      out.push({ op: 'replace', pointer: y.pointer, redacted: true });
  }
  for (const [k, y] of b) {
    if (!a.has(k)) out.push({ op: 'add', pointer: y.pointer, redacted: true });
  }
  return out.sort((p, q) => (p.pointer < q.pointer ? -1 : p.pointer > q.pointer ? 1 : 0));
}

/**
 * For the audit log: the redacted before/after subtrees of an edit at `base`, with every changed secret leaf marked
 * `"<redacted>"` (before) / `"<redacted:changed>"` (after), so the row shows THAT a credential changed — never what.
 */
export function markSecretChanges(
  base: string,
  before: unknown,
  after: unknown,
  changes: readonly SecretChange[],
): { before: unknown; after: unknown } {
  const mark = (node: unknown, pointer: string, value: string): unknown => {
    const rel = parsePointer(pointer).slice(parsePointer(base).length);
    if (rel.length === 0) return value;
    const copy = structuredClone(node);
    let cur: unknown = copy;
    for (const seg of rel.slice(0, -1)) {
      cur = Array.isArray(cur) ? cur[Number(seg)] : isPlainObject(cur) ? cur[seg] : undefined;
    }
    const leaf = rel.at(-1) as string;
    if (Array.isArray(cur)) cur[Number(leaf)] = value;
    else if (isPlainObject(cur)) cur[leaf] = value;
    return copy;
  };
  let b = before;
  let a = after;
  for (const c of changes) {
    if (!(c.pointer === base || c.pointer.startsWith(base + '/'))) continue;
    if (c.op !== 'add' && b !== undefined) b = mark(b, c.pointer, '<redacted>');
    if (c.op !== 'remove' && a !== undefined) a = mark(a, c.pointer, '<redacted:changed>');
  }
  return { before: b, after: a };
}

const SECRET_REF = new RegExp(`^(?:${SECRET_KINDS.join('|')})/[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`);

type SchemaNode = Record<string, unknown>;

/**
 * ARCH-05 (TD-15): the member names that hold secret references, read from the root schema — every property whose
 * schema (through `$ref`, `anyOf`/`oneOf`/`allOf`, `.optional()`, array `items`) is a `secretRefOf(…)` field
 * (`x-vrx-ui.widget: 'secret-ref'`, D-051). No naming convention involved: a ref field named anything is found, and
 * a `…Ref`-named field that is not a secret reference is not.
 *
 * Pinned: a secret-ref schema reachable only as a record value (`additionalProperties`) or a bare array item has no
 * member name to key on — that throws here on first use, so such a schema change fails the unit tests instead of
 * silently hiding the reference from the existence check and the admin-only rule.
 */
export function secretRefMembers(): ReadonlySet<string> {
  if (refMembers !== undefined) return refMembers;
  const root = z.toJSONSchema(RootConfig, { io: 'output', unrepresentable: 'any' }) as SchemaNode;
  const defs: SchemaNode = isPlainObject(root['$defs']) ? root['$defs'] : {};
  const resolve = (n: unknown): unknown => {
    if (!isPlainObject(n) || typeof n['$ref'] !== 'string') return n;
    return n['$ref'] === '#' ? root : defs[n['$ref'].replace('#/$defs/', '')];
  };
  // does `n` (or a branch / array item of it) describe a secret reference?
  const isRef = (n: unknown, seen: Set<unknown>): boolean => {
    const x = resolve(n);
    if (!isPlainObject(x) || seen.has(x)) return false;
    seen.add(x);
    if ((x['x-vrx-ui'] as { widget?: unknown } | undefined)?.widget === 'secret-ref') return true;
    for (const k of ['anyOf', 'oneOf', 'allOf'] as const) {
      const list = x[k];
      if (Array.isArray(list) && list.some((b) => isRef(b, seen))) return true;
    }
    return isRef(x['items'], seen);
  };
  const names = new Set<string>();
  const visited = new Set<unknown>();
  const walk = (n: unknown, where: string): void => {
    if (Array.isArray(n)) {
      n.forEach((x) => walk(x, where));
      return;
    }
    if (!isPlainObject(n) || visited.has(n)) return;
    visited.add(n);
    for (const [k, v] of Object.entries(n)) {
      if (k === 'properties' && isPlainObject(v)) {
        for (const [name, sub] of Object.entries(v)) {
          if (isRef(sub, new Set())) names.add(name);
          else walk(sub, `${where}/${name}`);
        }
      } else if ((k === 'additionalProperties' || k === 'items') && isRef(v, new Set())) {
        throw new Error(
          `secret reference at ${where || '/'} (${k}) has no member name; secretRefs() cannot find it (ARCH-05)`,
        );
      } else walk(v, where);
    }
  };
  walk(root, '');
  refMembers = names;
  return names;
}
let refMembers: ReadonlySet<string> | undefined;

/** Every secret reference in a document (members typed `secretRefOf(…)` in the schema, D-051) with its pointer. */
export function secretRefs(doc: unknown): { pointer: string; ref: string }[] {
  const members = secretRefMembers();
  const out: { pointer: string; ref: string }[] = [];
  const walk = (node: unknown, path: (string | number)[], refKey: boolean): void => {
    if (typeof node === 'string') {
      if (refKey && SECRET_REF.test(node)) out.push({ pointer: jsonPointer(...path), ref: node });
    } else if (Array.isArray(node)) {
      node.forEach((x, i) => walk(x, [...path, i], refKey));
    } else if (isPlainObject(node)) {
      for (const [k, v] of Object.entries(node)) walk(v, [...path, k], members.has(k));
    }
  };
  walk(doc, [], false);
  return out;
}

/** Tier-2 addition of the API: referenced secrets must exist in the store (D-051). */
export function missingSecretIssues(
  refs: readonly { pointer: string; ref: string }[],
  existing: ReadonlySet<string>,
): ProblemIssue[] {
  return refs
    .filter((r) => !existing.has(r.ref))
    .map((r) => ({
      pointer: r.pointer,
      message: `secret '${r.ref}' does not exist (create it with POST /api/v1/secrets)`,
      rule: 'secrets.ref-exists',
    }));
}

/** Pointers an operator may not change (P06 §6: operator — no user/AAA changes). */
export const ADMIN_ONLY_POINTERS = ['/management/users', '/management/aaa'] as const;

/**
 * The admin-only subtrees that differ between two documents. Compared unredacted (a new password hash is a user
 * change), so both sides must be in the same hydration state.
 */
export function adminOnlyChanges(before: Doc, after: Doc): string[] {
  return ADMIN_ONLY_POINTERS.filter((p) => !deepEqual(getAt(before, p), getAt(after, p)));
}

/**
 * Everything only an admin may change (P06 §6 + review M1): users, AAA, and any secret reference anywhere in the
 * document (added, removed or re-pointed) — a reference decides which credential a tunnel/server uses. Returns the
 * pointers that differ; both documents must be in the same hydration state.
 */
export function privilegedChanges(before: Doc, after: Doc): string[] {
  const out = adminOnlyChanges(before, after);
  const a = new Map(secretRefs(before).map((r) => [r.pointer, r.ref]));
  const b = new Map(secretRefs(after).map((r) => [r.pointer, r.ref]));
  const refPointers = new Set<string>();
  for (const [p, ref] of a) if (b.get(p) !== ref) refPointers.add(p);
  for (const [p, ref] of b) if (a.get(p) !== ref) refPointers.add(p);
  for (const p of [...refPointers].sort()) {
    if (!out.some((o) => p === o || p.startsWith(o + '/'))) out.push(p);
  }
  return out;
}
