import { ROOT_KEYS, type RootKey } from '@ngfw/schema';
import { problems } from '../../common/problem.js';

/**
 * F-restconf-yang: translate a RESTCONF data-resource path (RFC 8040 §3.5.3) to the document's JSON pointer, and wrap
 * results with module-qualified names (RFC 8040 §3.5.3.1 / §4.8.7). The module for root key `k` is `ngfw-<k>` and its
 * one top data node is `<k>`, so `ngfw-interfaces:interfaces=eth0/mtu` -> pointer `/interfaces/eth0/mtu`.
 *
 * List keys use the RFC 8040 `=` form (`interfaces=eth0`); a plain segment is a container/leaf name. Our lists are all
 * single-key (`name`, or one `itemKey`), so a comma-joined multi-key is passed through verbatim as one pointer segment
 * — documented as a deviation for the (currently none) multi-key lists.
 */

const MODULE_PREFIX = 'ngfw-';

/** `ngfw-interfaces` -> `interfaces`, validated against the real root keys. */
function rootKeyOfModule(module: string): RootKey {
  if (!module.startsWith(MODULE_PREFIX)) {
    throw problems.badRequest(`unknown YANG module '${module}'`);
  }
  const key = module.slice(MODULE_PREFIX.length);
  if (!(ROOT_KEYS as readonly string[]).includes(key)) {
    throw problems.badRequest(`unknown YANG module '${module}'`);
  }
  return key as RootKey;
}

/** One JSON-pointer segment, with `~1`/`~0` escaping (RFC 6901). */
function escapeSegment(s: string): string {
  return s.replace(/~/g, '~0').replace(/\//g, '~1');
}

export interface RestconfTarget {
  /** The root domain the path addresses. */
  rootKey: RootKey;
  /** JSON pointer into the document (empty string = whole document). */
  pointer: string;
  /** The module-qualified name of the addressed top node, for wrapping a response. */
  qualified: string;
}

/**
 * Parse the part of the URL after `/restconf/data/` (already URL-decoded per segment by the caller). An empty string
 * addresses the whole datastore.
 */
export function parseDataPath(rest: string): RestconfTarget | null {
  const trimmed = rest.replace(/^\/+|\/+$/g, '');
  if (trimmed === '') return null; // whole datastore

  const segments = trimmed.split('/');
  const first = segments[0]!;
  const colon = first.indexOf(':');
  if (colon < 0) {
    throw problems.badRequest(
      `the first RESTCONF path segment must be module-qualified, e.g. ngfw-interfaces:interfaces`,
    );
  }
  const module = first.slice(0, colon);
  const rootKey = rootKeyOfModule(module);
  const firstNode = first.slice(colon + 1);
  const eq = firstNode.indexOf('=');
  const nodeName = eq < 0 ? firstNode : firstNode.slice(0, eq);
  if (nodeName !== rootKey) {
    throw problems.badRequest(`'${nodeName}' is not the top node of module '${module}' (expected '${rootKey}')`);
  }

  const parts: string[] = [rootKey];
  if (eq >= 0) parts.push(firstNode.slice(eq + 1));
  for (const seg of segments.slice(1)) {
    const e = seg.indexOf('=');
    if (e < 0) {
      parts.push(seg);
    } else {
      // container/list name plus its key(s)
      parts.push(seg.slice(0, e));
      parts.push(seg.slice(e + 1));
    }
  }
  const pointer = '/' + parts.map(escapeSegment).join('/');
  return { rootKey, pointer, qualified: `${module}:${rootKey}` };
}

/** Wrap a whole-domain (or top-node) value as RFC 8040 module-qualified JSON. */
export function qualify(target: RestconfTarget, value: unknown): Record<string, unknown> {
  return { [target.qualified]: value };
}

/** The whole datastore, every domain module-qualified. */
export function qualifyAll(doc: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const key of ROOT_KEYS) {
    if (key in doc) out[`${MODULE_PREFIX}${key}:${key}`] = doc[key];
  }
  return out;
}

/**
 * Unwrap a yang-data+json request body: it must be a single member `{"<module>:<node>": value}` whose key matches the
 * addressed target. Returns the inner value.
 */
export function unwrapBody(target: RestconfTarget, body: unknown): unknown {
  if (typeof body !== 'object' || body === null || Array.isArray(body)) {
    throw problems.badRequest('a RESTCONF data body must be a JSON object with one module-qualified member');
  }
  const keys = Object.keys(body);
  if (keys.length !== 1) {
    throw problems.badRequest('a RESTCONF data body must have exactly one top-level member');
  }
  const key = keys[0]!;
  // accept the fully qualified name or the bare node name (RFC 8040 allows a bare name for a nested node)
  const bare = key.includes(':') ? key.slice(key.indexOf(':') + 1) : key;
  const wantBare = target.qualified.slice(target.qualified.indexOf(':') + 1);
  const lastSeg = target.pointer.split('/').pop() ?? wantBare;
  if (key !== target.qualified && bare !== wantBare && bare !== lastSeg) {
    throw problems.badRequest(`the body member '${key}' does not match the target node`);
  }
  return (body as Record<string, unknown>)[key];
}
