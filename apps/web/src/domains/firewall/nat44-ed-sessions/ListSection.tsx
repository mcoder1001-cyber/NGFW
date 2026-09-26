import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
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
      <Table size="small" aria-label={title}>
        <TableHead>
          <TableRow>
            {columns.map((c) => (
              <TableCell key={c.label}>{c.label}</TableCell>
            ))}
            {status && <TableCell>{status.label}</TableCell>}
            <TableCell sx={{ textAlign: 'end' }}>{t('list.actions')}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {items.length === 0 && (
            <TableRow>
              <TableCell colSpan={columns.length + (status ? 2 : 1)}>
                <Typography color="text.secondary">{emptyText}</Typography>
              </TableCell>
            </TableRow>
          )}
          {items.map((item, i) => (
            <TableRow
              key={`${i}:${JSON.stringify(item)}`}
              hover
              sx={{ cursor: readOnly ? 'default' : 'pointer' }}
              onClick={() => !readOnly && setEditing({ index: i, value: item })}
            >
              {columns.map((c) => (
                <TableCell
                  key={c.label}
                  {...(c.ltr ? LTR : {})}
                  sx={
                    c.ltr
                      ? {
                          textAlign: 'start',
                          fontFamily: (th) => th.vrx.monoFontFamily,
                          fontSize: 12,
                        }
                      : {}
                  }
                >
                  {c.value(item, i)}
                </TableCell>
              ))}
              {status && <TableCell>{status.value(item)}</TableCell>}
              <TableCell sx={{ textAlign: 'end' }}>
                <IconButton
                  size="small"
                  aria-label={t('list.remove', { name: itemLabel(item, i) })}
                  disabled={readOnly || patch.isPending}
                  onClick={(e) => {
                    e.stopPropagation();
                    void remove(i);
                  }}
                >
                  <DeleteIcon fontSize="small" />
                </IconButton>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
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
