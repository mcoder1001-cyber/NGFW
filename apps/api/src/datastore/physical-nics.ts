import { deepEqual, isPlainObject, jsonPointer } from '@ngfw/schema';
import { getAt } from '../common/json.js';
import { ProblemError } from '../common/problem.js';
import type { Doc } from './repo.js';

// F-default-vpp-nics (D-164): the physical-NIC guards of every candidate edit (DatastoreService.edit(): PATCH, PUT,
// DELETE and import all pass through it). The first-boot seed and rollback are commits of whole documents, not edits:
// the seed is the only writer that creates the marker, and a rollback restores an earlier revision verbatim — every
// revision since the seed (revision 1) carries the same markers, so a rollback cannot drop one.

/**
 * F-default-vpp-nics (D-164): logical names of `interfaces` entries that have `physical` in `before` but are removed
 * — the whole entry gone, or its `physical` marker dropped — in `after`. Removing a physical NIC from the document is
 * refused (403); releasing it back to the host is `physical.owner = 'host'`, which keeps `physical` present. Covers
 * PATCH-null, PUT-without-it, DELETE and import-without-it, since all of them run through `DatastoreService.edit()`.
 */
export function physicalNicRemovals(before: Doc, after: Doc): string[] {
  const b = getAt(before, '/interfaces');
  const a = getAt(after, '/interfaces');
  const bIfs = isPlainObject(b) ? b : {};
  const aIfs = isPlainObject(a) ? a : {};
  const out: string[] = [];
  for (const [name, itf] of Object.entries(bIfs)) {
    if (!isPlainObject(itf) || itf['physical'] === undefined) continue;
    const next = aIfs[name];
    if (!isPlainObject(next) || next['physical'] === undefined) out.push(name);
  }
  return out;
}

/**
 * F-default-vpp-nics (review R2R4 #4): the `physical` marker is created only by the first-boot seed (a system commit,
 * not an edit). A user edit may change `physical.owner` (release / reclaim) but may not ADD the marker to an entry that
 * lacked it — that would make any interface (e.g. loop5) permanently undeletable — nor change its `pci` or `builtIn`.
 * Returns the offending pointers (`/interfaces/<name>/physical[/pci|/builtIn]`).
 */
export function physicalMarkerEdits(before: Doc, after: Doc): string[] {
  const b = getAt(before, '/interfaces');
  const a = getAt(after, '/interfaces');
  const bIfs = isPlainObject(b) ? b : {};
  const aIfs = isPlainObject(a) ? a : {};
  const out: string[] = [];
  for (const [name, itf] of Object.entries(aIfs)) {
    if (!isPlainObject(itf) || !isPlainObject(itf['physical'])) continue;
    const next = itf['physical'];
    const prevItf = bIfs[name];
    const prev = isPlainObject(prevItf) ? prevItf['physical'] : undefined;
    if (!isPlainObject(prev)) {
      out.push(jsonPointer('interfaces', name, 'physical'));
      continue;
    }
    for (const k of ['pci', 'builtIn'] as const) {
      if (!deepEqual(prev[k], next[k])) out.push(jsonPointer('interfaces', name, 'physical', k));
    }
  }
  return out;
}

/** Throws 403 problem+json when an edit removes a physical NIC or creates / alters its marker (pci, builtIn). */
export function assertPhysicalNicEdit(before: Doc, after: Doc): void {
  const removed = physicalNicRemovals(before, after);
  if (removed.length > 0) {
    throw new ProblemError(
      403,
      'interfaces.physical-nic-not-deletable',
      'Forbidden',
      "a built-in physical NIC cannot be removed from the configuration; release it to the host instead (set physical.owner to 'host')",
      removed.map((name) => ({
        pointer: jsonPointer('interfaces', name),
        message:
          "built-in physical NIC seeded from the host inventory; it can be edited, disabled or released (physical.owner = 'host') but not deleted",
        rule: 'interfaces.physical-nic-not-deletable',
      })),
    );
  }
  const edits = physicalMarkerEdits(before, after);
  if (edits.length > 0) {
    throw new ProblemError(
      403,
      'interfaces.physical-marker-readonly',
      'Forbidden',
      'the physical-NIC marker is created only by the first-boot seed; an edit may change physical.owner (release/reclaim) but not add the marker or change its pci/builtIn',
      edits.map((pointer) => ({
        pointer,
        message: 'read-only: set by the host NIC inventory seed; only physical.owner may be edited',
        rule: 'interfaces.physical-marker-readonly',
      })),
    );
  }
}
