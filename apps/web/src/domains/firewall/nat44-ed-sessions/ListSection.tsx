import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import { PgAddIcon as AddIcon, PgDeleteIcon as DeleteIcon } from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgDialog as Dialog,
  PgDialogContent as DialogContent,
  PgDialogTitle as DialogTitle,
  PgIconButton as IconButton,
  PgPaper as Paper,
  PgStack as Stack,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { problemFor } from '../../interfaces/InterfaceDrawer';
import { localizeSchema } from '../../interfaces/model';
import { useCandidateInterfaces } from '../../interfaces/queries';
import { itemSchema, type NatListKey } from './model';
import { useCandidateNat, useFreshCandidateNat, usePatchNat } from './queries';
import { NAT_NS } from './tabs';

type Item = Record<string, unknown>;

export interface ListColumn {
  label: string;
  value: (item: Item, index: number) => ReactNode;
  /** Addresses, ports, interface names: always left-to-right. */
  ltr?: boolean;
}

const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
const LTR = { dir: 'ltr' } as const;

/**
 * One `nat.<list>` array of the candidate as a table; each item is edited in a dialog with the item's schema
 * (SchemaForm). Every save replaces the whole array through a merge patch of `/config/nat` (RFC 7386: arrays replace),
 * computed against the candidate as it is now — an item another session changed meanwhile is not overwritten.
 */
export function ListSection({
  listKey,
  title,
  columns,
  addLabel,
  emptyText,
  itemLabel,
  status,
}: {
  listKey: NatListKey;
  title: string;
  columns: ListColumn[];
  addLabel: string;
  emptyText: string;
  itemLabel: (item: Item, index: number) => string;
  /** Extra live-status column (e.g. pool utilisation). */
  status?: { label: string; value: (item: Item) => ReactNode };
}) {
  const { t } = useTranslation(NAT_NS);
  const perms = usePermissions();
  const candidate = useCandidateNat();
  const fresh = useFreshCandidateNat();
  const patch = usePatchNat();
  const ifs = useCandidateInterfaces();
  const schema = useMemo(
    () => localizeSchema(itemSchema(listKey), (k, o) => t(k, o ?? {})),
    [t, listKey],
  );
  const interfaceOptions = useMemo(() => Object.keys(ifs.data ?? {}).sort(), [ifs.data]);
  const [editing, setEditing] = useState<{ index: number; value: Item | undefined } | null>(null);
  const [conflict, setConflict] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const items = (Array.isArray(candidate.data?.[listKey]) ? candidate.data[listKey] : []) as Item[];
  const readOnly = !perms.editConfig;

  /** Apply `change` to the list as the candidate holds it now; refuse when the edited item changed meanwhile. */
  const write = async (change: (list: Item[]) => Item[] | null) => {
    setError(null);
    setConflict(false);
    const now = (await fresh())[listKey];
    const list = (Array.isArray(now) ? now : []) as Item[];
    const next = change([...list]);
    if (next === null) {
      setConflict(true);
      return false;
    }
    try {
      await patch.mutateAsync({ [listKey]: next });
      return true;
    } catch (e) {
      setError(e);
      return false;
    }
  };

  const save = async (value: unknown) => {
    if (!editing) return;
    const { index, value: before } = editing;
    const cleaned = value as Item; // WEB-1: SchemaForm keeps absent optionals absent (no phantom to drop)
    const ok = await write((list) => {
      if (index < 0) return [...list, cleaned];
      if (!same(list[index], before)) return null;
      list[index] = cleaned;
      return list;
    });
    if (ok) setEditing(null);
  };

  const remove = async (index: number) => {
    const before = items[index];
    await write((list) => (same(list[index], before) ? list.filter((_, i) => i !== index) : null));
  };

  return (
    <Paper variant="outlined" sx={{ p: 2, mb: 2 }} component="section" aria-label={title}>
      <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
        <Typography component="h3" variant="subtitle1" sx={{ flex: 1 }}>
          {title}
        </Typography>
        <Button
          size="small"
          startIcon={<AddIcon />}
          disabled={readOnly || !candidate.isSuccess}
          onClick={() => setEditing({ index: -1, value: undefined })}
        >
          {addLabel}
        </Button>
      </Stack>
      {conflict && (
        <Alert severity="warning" sx={{ mb: 1 }}>
          {t('list.conflict')}
        </Alert>
      )}
      {error !== null && !editing && <ProblemAlert error={error} sx={{ mb: 1 }} />}
      <ReactGeereedMaterial
        items={items.map((item, index) => ({ item, index }))}
        columns={[
          ...columns.map((column, index) => ({
            key: String(index),
            title: column.label,
            filterable: false,
            renderer: (_value: unknown, row: { item: Item; index: number }) => (
              <>{column.value(row.item, row.index)}</>
            ),
          })),
          ...(status
            ? [
                {
                  key: 'status',
                  title: status.label,
                  filterable: false,
                  renderer: (_value: unknown, row: { item: Item }) => <>{status.value(row.item)}</>,
                },
              ]
            : []),
          {
            key: 'actions',
            title: t('list.actions'),
            filterable: false,
            headerCellProps: { sx: { textAlign: 'end' } },
            renderer: (_value, { item, index }: { item: Item; index: number }) => (
              <IconButton
                size="small"
                aria-label={t('list.remove', { name: itemLabel(item, index) })}
                disabled={readOnly || patch.isPending}
                onClick={(event) => {
                  event.stopPropagation();
                  void remove(index);
                }}
              >
                <DeleteIcon fontSize="small" />
              </IconButton>
            ),
          },
        ]}
        rowKey={({ item, index }) => String(index) + ':' + JSON.stringify(item)}
        rowProps={({ item, index }) => ({
          hover: true,
          sx: { cursor: readOnly ? 'default' : 'pointer' },
          onClick: () => !readOnly && setEditing({ index, value: item }),
        })}
        cellProps={(_row, column) =>
          column.key === 'actions'
            ? { sx: { textAlign: 'end' } }
            : columns[Number(column.key)]?.ltr
              ? {
                  ...LTR,
                  sx: {
                    textAlign: 'start',
                    fontFamily: (th) => th.ngfw.monoFontFamily,
                    fontSize: 12,
                  },
                }
              : {}
        }
        tableProps={{ size: 'small', 'aria-label': title }}
        emptyContent={<Typography color="text.secondary">{emptyText}</Typography>}
        noFormWrapper
        embedded
        hideToolbar
        hidePagination
        hideFilterBuilderComponent
        search={false}
        sortable={false}
        exportToCSV={false}
        footer={false}
        memoEnable={false}
      />
      <Dialog open={editing !== null} onClose={() => setEditing(null)} fullWidth maxWidth="sm">
        <DialogTitle>
          {editing && editing.index >= 0
            ? t('list.editTitle', { name: itemLabel(editing.value ?? {}, editing.index) })
            : addLabel}
        </DialogTitle>
        <DialogContent>
          {editing && (
            <Stack gap={1} sx={{ pt: 1 }}>
              {error !== null &&
                problemFor(
                  error,
                  `/nat/${listKey}/${editing.index < 0 ? items.length : editing.index}`,
                ) === null && <ProblemAlert error={error} />}
              <SchemaForm
                schema={schema}
                value={editing.value}
                readOnly={readOnly}
                interfaceOptions={interfaceOptions}
                problem={problemFor(
                  error,
                  `/nat/${listKey}/${editing.index < 0 ? items.length : editing.index}`,
                )}
                submitLabel={t('save')}
                resetLabel={t('reset')}
                onSubmit={save}
              >
                <Button onClick={() => setEditing(null)}>{t('cancel')}</Button>
              </SchemaForm>
            </Stack>
          )}
        </DialogContent>
      </Dialog>
    </Paper>
  );
}
