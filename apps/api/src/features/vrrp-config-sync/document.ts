import { parsePointer } from '@ngfw/schema';
import type { Doc } from '../../datastore/repo.js';

// Peer identity, credentials and transport settings must never overwrite this node's settings.
export const NODE_LOCAL = [
  '/ha/cluster',
  '/system/hostname',
  '/system/setup',
  '/management',
  '/host',
  '/dataplane',
] as const;
const FORBIDDEN = new Set(['__proto__', 'prototype', 'constructor']);
function parts(pointer: string): string[] {
  const p = parsePointer(pointer);
  if (p.length === 0 || p.some((x) => FORBIDDEN.has(x)))
    throw new Error('unsafe exclusion pointer');
  return p;
}
function get(doc: Doc, p: string[]): unknown {
  let value: unknown = doc;
  for (const k of p) {
    if (value === null || typeof value !== 'object' || !Object.hasOwn(value, k)) return undefined;
    value = (value as Record<string, unknown>)[k];
  }
  return value;
}
function remove(doc: Doc, p: string[]): void {
  const parent = get(doc, p.slice(0, -1));
  if (parent !== null && typeof parent === 'object') delete (parent as Doc)[p.at(-1)!];
}
function put(doc: Doc, p: string[], value: unknown): void {
  let node: Doc = doc;
  for (const k of p.slice(0, -1)) {
    const child = node[k];
    if (child === null || typeof child !== 'object') node[k] = {};
    node = node[k] as Doc;
  }
  node[p.at(-1)!] = structuredClone(value);
}
export function exportDocument(doc: Doc, exclude: readonly string[]): Doc {
  const next = structuredClone(doc);
  for (const pointer of [...NODE_LOCAL, ...exclude]) remove(next, parts(pointer));
  return next;
}
export function mergeDocument(incoming: Doc, local: Doc, exclude: readonly string[]): Doc {
  const next = exportDocument(incoming, exclude);
  for (const pointer of [...NODE_LOCAL, ...exclude]) {
    const p = parts(pointer),
      value = get(local, p);
    if (value !== undefined) put(next, p, value);
  }
  return next;
}
