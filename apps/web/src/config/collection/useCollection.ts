import type { QueryKey } from '@tanstack/react-query';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { jsonPointer } from '@ngfw/schema';
import type { ProblemDetails } from '@ngfw/ui-kit/schema-form';
import {
  collectionModel,
  collectionRows,
  createMergePatch,
  deepEqual,
  formValueOf,
  isObject,
  itemPointer,
  keyLabel,
  listKeyOf,
  mapKeyIssue,
  nest,
  nextList,
  problemAt,
  valueAt,
  type CollectionModel,
  type CollectionRow,
  type CollectionSpec,
  type Json,
} from './model';
import { useDomainNode, useFreshCandidate, usePatchDomain } from './queries';

export interface CollectionOptions {
  /** Query keys to refresh after an edit besides `['config']` (a screen's live state table). */
  alsoInvalidate?: readonly QueryKey[] | undefined;
  /** Poll period of the candidate/running node (default: the pending-change bar's 5 s; `false` = off). */
  refetchInterval?: number | false | undefined;
}

/** The candidate + running view of one collection (read side of the kit). */
export function useCollection<T = Json>(spec: CollectionSpec, options: CollectionOptions = {}) {
  const pathKey = (spec.path ?? []).join('/');
  const omitKey = (spec.omit ?? []).join(',');
  // the spec object may be a new literal on every render: the model follows its content
  const model = useMemo(() => collectionModel(spec), [spec.domain, pathKey, omitKey, spec.ns]);
  const candidate = useDomainNode(spec.domain, 'candidate', options.refetchInterval);
  const running = useDomainNode(spec.domain, 'running', options.refetchInterval);
  const rows = useMemo(
    () => collectionRows<T>(model, valueAt(candidate.data, model.path), valueAt(running.data, model.path)),
    [model, candidate.data, running.data],
  );
  return { model, candidate, running, rows, options };
}

export type Collection<T = Json> = ReturnType<typeof useCollection<T>>;

/** Row of the candidate by id (`undefined` when the candidate does not hold it). */
export function rowOf<T>(c: Collection<T>, id: string | null): CollectionRow<T> | undefined {
  return id === null ? undefined : c.rows.find((r) => r.id === id);
}

function keyTakenProblem(model: CollectionModel, detail: string): ProblemDetails {
  const members = model.shape.kind === 'list' ? model.shape.itemKey : [];
  return { status: 409, title: detail, errors: members.map((m) => ({ pointer: jsonPointer(m), detail })) };
}

/**
 * The write side for ONE item (the drawer form) — P08's reviewed editing semantics, for any collection:
 *   - the form edits the value it OPENED with; a candidate refetch never remounts it (unsaved edits stay, review N4);
 *   - Save sends only what changed against that value, so a member another session changed meanwhile is not written
 *     back with a stale value; "changed elsewhere" is detected against the candidate as it is NOW and offers Reload;
 *   - the form value is sent as-is, including an optional object the user just switched on that still holds only
 *     its defaults (WEB-1 H1: silently dropping it turned the feature back off; the presence toggle owns "on" vs
 *     "off" now, not a guess from the value);
 *   - server pointers are mapped onto the fields (`/interfaces/<name>/mtu` → `/mtu`; lists: the item's index);
 *   - a new item never silently merges over an existing one (review N5): the key is checked against the fresh candidate.
 * `id === null` edits a new item; after its first save the editor continues on the created id.
 */
export function useItemEditor<T = Json>(c: Collection<T>, id: string | null, onSaved?: (id: string, value: unknown) => void) {
  const { t } = useTranslation('config');
  const { model } = c;
  const patch = usePatchDomain(model.spec.domain, c.options.alsoInvalidate ?? []);
  const fresh = useFreshCandidate(model.spec.domain);
  const [currentId, setCurrentId] = useState(id);
  const [reload, setReload] = useState(0);
  const [opened, setOpened] = useState<{ reload: number; value: Json | undefined } | null>(null);
  const [changedElsewhere, setChangedElsewhere] = useState(false);
  const [saved, setSaved] = useState(false);
  const [local, setLocal] = useState<ProblemDetails | null>(null);
  const [failure, setFailure] = useState<unknown>(null);
  const [keyError, setKeyError] = useState<'empty' | 'invalid' | 'taken' | null>(null);
  const [pointer, setPointer] = useState<string | null>(null);

  const row = rowOf(c, currentId);
  if (c.candidate.isSuccess && (opened === null || opened.reload !== reload)) {
    setOpened({ reload, value: currentId === null ? undefined : formValueOf(model, row?.value) });
  }

  const freshNode = useCallback(async () => valueAt(await fresh(), model.path), [fresh, model.path]);

  /** Save the form value; `newKey` is the typed key of a new map item. Resolves with the item id, or null when refused. */
  const save = async (value: unknown, newKey?: string): Promise<string | null> => {
    setSaved(false);
    setLocal(null);
    setFailure(null);
    setKeyError(null);
    const base = opened?.value;
    let node: unknown;
    try {
      node = await freshNode();
    } catch (e) {
      setFailure(e);
      return null;
    }
    let body: unknown;
    let savedId: string;
    if (model.shape.kind === 'map') {
      savedId = currentId ?? newKey ?? '';
      if (currentId === null) {
        const issue = mapKeyIssue(model, savedId) ?? (isObject(node) && Object.hasOwn(node, savedId) ? 'taken' : undefined);
        if (issue) {
          setKeyError(issue);
          return null;
        }
      } else {
        setChangedElsewhere(!deepEqual(base ?? {}, formValueOf(model, isObject(node) && Object.hasOwn(node, savedId) ? node[savedId] : undefined) ?? {}));
      }
      body = nest([...model.path, savedId], base === undefined ? value : createMergePatch(base, value));
      setPointer(itemPointer(model, savedId));
    } else {
      const { itemKey } = model.shape;
      const list = Array.isArray(node) ? node : [];
      savedId = listKeyOf(value, itemKey);
      const at = currentId === null ? -1 : list.findIndex((i) => listKeyOf(i, itemKey) === currentId);
      const clash = list.findIndex((i) => listKeyOf(i, itemKey) === savedId);
      if (clash >= 0 && clash !== at) {
        setLocal(keyTakenProblem(model, t('kit.keyTaken', { key: keyLabel(model, savedId) })));
        return null;
      }
      if (at >= 0) setChangedElsewhere(!deepEqual(base ?? {}, formValueOf(model, list[at]) ?? {}));
      const next = nextList(list, itemKey, currentId, base, value);
      body = nest(model.path, next.list);
      setPointer(itemPointer(model, next.index));
    }
    try {
      await patch.mutateAsync(body);
    } catch {
      return null; // rendered from patch.error, pointers mapped onto the fields
    }
    setOpened({ reload, value: value as Json }); // the next save diffs against what is saved now
    setSaved(true);
    setCurrentId(savedId);
    onSaved?.(savedId, value);
    return savedId;
  };

  /** Remove the item from the candidate. Resolves true on success. */
  const remove = async (): Promise<boolean> => {
    if (currentId === null) return false;
    setSaved(false);
    setLocal(null);
    setFailure(null);
    let body: unknown;
    if (model.shape.kind === 'map') {
      body = nest([...model.path, currentId], null);
    } else {
      const { itemKey } = model.shape;
      let node: unknown;
      try {
        node = await freshNode();
      } catch (e) {
        setFailure(e);
        return false;
      }
      const list = Array.isArray(node) ? node : [];
      body = nest(model.path, list.filter((i) => listKeyOf(i, itemKey) !== currentId));
    }
    setPointer(null);
    try {
      await patch.mutateAsync(body);
      return true;
    } catch {
      return false;
    }
  };

  const doReload = () => {
    setChangedElsewhere(false);
    setSaved(false);
    setReload((r) => r + 1);
  };

  return {
    id: currentId,
    row,
    opened,
    formKey: `${currentId ?? '<new>'}:${reload}`,
    saved,
    changedElsewhere,
    keyError,
    /** Problem for `<SchemaForm problem>`: a local refusal, or the server's with pointers made relative to the item. */
    problem: local ?? (pointer === null ? null : problemAt(patch.error, pointer)),
    /** The server problem to show above the form (lock held by another user, validation, agent down …). */
    error: failure ?? (patch.isError ? patch.error : null),
    pending: patch.isPending,
    save,
    remove,
    reload: doReload,
  };
}

export type ItemEditorState = ReturnType<typeof useItemEditor>;
