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

const SECRET_REF = new RegExp(`^(?:${SECRET_KINDS.join('|')})/[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`);

/** Every secret reference in a document (`*Ref` members, D-051) with its pointer. */
export function secretRefs(doc: unknown): { pointer: string; ref: string }[] {
  const out: { pointer: string; ref: string }[] = [];
  const walk = (node: unknown, path: (string | number)[], refKey: boolean): void => {
    if (typeof node === 'string') {
      if (refKey && SECRET_REF.test(node)) out.push({ pointer: jsonPointer(...path), ref: node });
    } else if (Array.isArray(node)) {
      node.forEach((x, i) => walk(x, [...path, i], refKey));
    } else if (isPlainObject(node)) {
      for (const [k, v] of Object.entries(node)) walk(v, [...path, k], /Refs?$/.test(k));
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
