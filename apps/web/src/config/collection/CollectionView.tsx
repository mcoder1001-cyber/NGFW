import AddIcon from '@mui/icons-material/Add';
import Box from '@mui/material/Box';
import LinearProgress from '@mui/material/LinearProgress';
import Button from '@mui/material/Button';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import type { GridColDef } from '@ngfw/ui-kit/data-grid';
import { useCallback, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../auth/AuthProvider';
import { ProblemAlert } from '../ProblemAlert';
import { IdText, RowStateChip, Why } from '../widgets/cells';
import { KeyButton } from '../widgets/KeyButton';
import { LocalDataGrid } from '../widgets/LocalDataGrid';
import type { CellValue } from '../widgets/paging';
import { CollectionDrawer } from './CollectionDrawer';
import { ItemEditor } from './ItemEditor';
import { isObject, keyLabel, localizeSchema, type CollectionRow, type CollectionSpec, type Json } from './model';
import { rowOf, useCollection, useItemEditor, type Collection, type CollectionOptions, type ItemEditorState } from './useCollection';

export interface CollectionViewProps<T = Json> {
  spec: CollectionSpec;
  /** Accessible name of the list (usually the page title). */
  label: string;
  /** Accessible name of the drawer of one item (`VRF red`). */
  itemLabel: (key: string) => string;
  /** Header of the key column. */
  keyHeader: string;
  /** Label of the "add" button. */
  addLabel: string;
  /**
   * Further columns. A column without `renderCell`/`valueGetter` shows the item member named by its `field` (candidate,
   * else running); sorting and filtering use the same value unless `valueOf` says otherwise.
   */
  columns?: readonly GridColDef<CollectionRow<T>>[] | undefined;
  /** Live-status slot of the list: a column rendered from the row (e.g. a StatusChip from `/state/...`). */
  status?: { header: string; render: (row: CollectionRow<T>) => ReactNode; width?: number } | undefined;
  /** Live-status slot of the drawer: chips next to the title. */
  drawerStatus?: ((row: CollectionRow<T> | undefined, id: string) => ReactNode) | undefined;
  /** Live panel above the form (live state table, counters). */
  drawerLive?: ((row: CollectionRow<T> | undefined, id: string) => ReactNode) | undefined;
  /** Below the form (a nested table, e.g. P08's sub-interfaces). */
  drawerExtra?: ((row: CollectionRow<T> | undefined, editor: ItemEditorState) => ReactNode) | undefined;
  /** Right end of the toolbar (a LiveChip). */
  toolbarEnd?: ReactNode;
  /** Quick-filter text of a row (default: key + primitive members of the item). */
  searchText?: ((row: CollectionRow<T>) => string) | undefined;
  valueOf?: CellValue<CollectionRow<T>> | undefined;
  options?: CollectionOptions | undefined;
  /** After every successful save (Users: remember a password edit — never its value). */
  onSaved?: ((id: string, value: unknown) => void) | undefined;
  /** Default: the role cannot edit the configuration. */
  readOnly?: boolean | undefined;
  /** Grid height in px (default 520, as P08). */
  height?: number | undefined;
}

const NO_COLUMNS: readonly never[] = [];
const REMOVED = 'kit-removed';
const GRID_SX = { '& .MuiDataGrid-row': { cursor: 'pointer' }, [`& .${REMOVED}`]: { cursor: 'default', textDecoration: 'line-through', color: 'text.disabled' } };
const removedClass = (p: { row: CollectionRow<unknown> }) => (p.row.state === 'removed' ? REMOVED : '');

function memberOf(row: CollectionRow<unknown>, field: string): unknown {
  const item = row.value ?? row.running;
  return isObject(item) ? item[field] : undefined;
}

function defaultValueOf(row: CollectionRow<unknown>, field: string): unknown {
  if (field === 'id') return row.label;
  if (field === 'state') return row.state ?? '';
  return memberOf(row, field);
}

function defaultSearch(row: CollectionRow<unknown>): string {
  const item = row.value ?? row.running;
  const members = isObject(item) ? Object.values(item).filter((v) => typeof v !== 'object' || v === null) : [];
  return [row.label, ...members.map((v) => String(v ?? ''))].join(' ');
}

/**
 * The generic config screen body: a list of one collection of the candidate (key column as a real button, pending
 * state, caller columns and a live-status slot), an "add" action, and a drawer with the schema form. The page header,
 * intro text and any live data are the caller's; the pending-change bar and commit flow are the shell's.
 */
export function CollectionView<T = Json>(props: CollectionViewProps<T>) {
  const { spec, label, keyHeader, addLabel, columns = NO_COLUMNS, status, toolbarEnd, searchText, valueOf, options, readOnly: ro, height = 520 } = props;
  const { t } = useTranslation('config');
  const perms = usePermissions();
  const readOnly = ro ?? !perms.editConfig;
  const collection = useCollection<T>(spec, options ?? {});
  // `session` remounts the drawer body when another item is opened (fresh form state), not when a new item is created
  const [open, setOpen] = useState<{ session: number; id: string | null } | null>(null);
  const sessions = useRef(0);
  const openItem = useCallback((id: string | null) => {
    sessions.current += 1;
    setOpen({ session: sessions.current, id });
  }, []);

  const gridColumns = useMemo<GridColDef<CollectionRow<T>>[]>(() => {
    const keyCol: GridColDef<CollectionRow<T>> = {
      field: 'id',
      headerName: keyHeader,
      minWidth: 170,
      flex: 1,
      renderCell: (p) =>
        p.row.state === 'removed' ? (
          <IdText>{p.row.label}</IdText>
        ) : (
          <KeyButton tabIndex={p.tabIndex} hasFocus={p.hasFocus} aria-label={t('kit.open', { key: p.row.label })} onClick={() => openItem(p.row.id)}>
            {p.row.label}
          </KeyButton>
        ),
    };
    const extra = columns.map((c) =>
      c.renderCell || c.valueGetter ? c : { ...c, valueGetter: (_v: unknown, row: CollectionRow<T>) => memberOf(row as CollectionRow<unknown>, c.field) as never },
    );
    const statusCol: GridColDef<CollectionRow<T>>[] = status
      ? [{ field: 'liveStatus', headerName: status.header, width: status.width ?? 130, sortable: false, filterable: false, renderCell: (p) => status.render(p.row) }]
      : [];
    const stateCol: GridColDef<CollectionRow<T>> = {
      field: 'state',
      headerName: t('kit.col.pending'),
      width: 140,
      renderCell: (p) => <RowStateChip state={p.row.state} />,
    };
    return [keyCol, ...extra, ...statusCol, stateCol];
  }, [keyHeader, columns, status, t, openItem]);

  const gridKey = useMemo(() => ['kit', 'collection', spec.domain, ...(spec.path ?? [])], [spec.domain, spec.path]);
  const vo = (valueOf ?? defaultValueOf) as CellValue<CollectionRow<T>>;
  const search = (searchText ?? defaultSearch) as (row: CollectionRow<T>) => string;

  return (
    <>
      <Stack direction="row" gap={1} sx={{ mb: 1 }} alignItems="center">
        <Why reason={readOnly ? t('perm.edit') : undefined}>
          <Button variant="contained" startIcon={<AddIcon />} disabled={readOnly || !collection.candidate.isSuccess} onClick={() => openItem(null)}>
            {addLabel}
          </Button>
        </Why>
        <Box sx={{ flex: 1 }} />
        {toolbarEnd}
      </Stack>
      {(collection.candidate.isPending || collection.running.isPending) && <LinearProgress aria-label={t('loading')} />}
      {collection.candidate.isError && <ProblemAlert error={collection.candidate.error} sx={{ mb: 1 }} />}
      <Paper variant="outlined" sx={{ blockSize: height }}>
        <LocalDataGrid<CollectionRow<T>>
          aria-label={label}
          gridKey={gridKey}
          rows={collection.rows}
          columns={gridColumns}
          searchText={search}
          valueOf={vo}
          getRowId={(r) => r.id}
          getRowClassName={removedClass}
          onRowClick={(p) => p.row.state !== 'removed' && openItem(p.row.id)}
          sx={GRID_SX}
        />
      </Paper>
      <CollectionDrawer
        open={open !== null}
        onClose={() => setOpen(null)}
        title={open?.id != null ? (rowOf(collection, open.id)?.label ?? keyLabel(collection.model, open.id)) : t('kit.newTitle')}
        titleIsKey={open?.id !== null}
        label={open?.id ? props.itemLabel(rowOf(collection, open.id)?.label ?? open.id) : t('kit.newTitle')}
        status={open?.id ? props.drawerStatus?.(rowOf(collection, open.id), open.id) : null}
      >
        {open && (
          <DrawerBody
            key={open.session}
            {...props}
            collection={collection}
            id={open.id}
            readOnly={readOnly}
            onCreated={(id) => setOpen({ session: open.session, id })}
            onClose={() => setOpen(null)}
          />
        )}
      </CollectionDrawer>
    </>
  );
}

function DrawerBody<T>({
  spec,
  keyHeader,
  collection,
  id,
  readOnly,
  onCreated,
  onClose,
  onSaved,
  drawerLive,
  drawerExtra,
}: CollectionViewProps<T> & { collection: Collection<T>; id: string | null; readOnly: boolean; onCreated: (id: string) => void; onClose: () => void }) {
  const { t } = useTranslation(spec.ns);
  // the editor keeps its own id: after "create" it continues on the new item without remounting the form
  const editor = useItemEditor(collection, id, onSaved);
  const schema = useMemo(() => localizeSchema(collection.model.formSchema, (k, o) => t(k, o ?? {})), [collection.model, t]);
  const row = rowOf(collection, editor.id);
  return (
    <>
      {editor.id !== null && drawerLive?.(row, editor.id)}
      <ItemEditor model={collection.model} editor={editor} schema={schema} readOnly={readOnly} keyLabel={keyHeader} onCreated={onCreated} onRemoved={onClose} />
      {drawerExtra?.(row, editor)}
    </>
  );
}
