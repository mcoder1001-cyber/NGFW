import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { SchemaForm, type JsonSchema } from '@ngfw/ui-kit/schema-form';
import { useCallback, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { esc, pageOf, problemFor } from '../vrf-static-ecmp/common';
import { usePatch } from './api';
import { NS } from './model';

export interface RecordEditorProps<R extends { id: string }> {
  /** i18n key prefix of the texts (`<k>.title`, `.add`, `.addTitle`, `.editTitle`, `.key`, `.badKey`, `.intro`). */
  k: string;
  /** The candidate record being edited (key → value). */
  record: Record<string, unknown> | undefined;
  /** Pointer segments of the record inside `routing` / `interfaces`, e.g. ['bgp', 'neighbors']. */
  path: readonly string[];
  /** Root node patched ('routing' | 'interfaces'). */
  root?: 'routing' | 'interfaces';
  /** The edited value is this leaf of the record entry (`interfaces.<n>.lcp`), not the entry itself. */
  leaf?: string;
  /** Item schema (the one schema, localized). */
  schema: JsonSchema;
  /** Grid rows and columns (row id = record key). */
  rows: R[];
  columns: GridColDef<R>[];
  /** Validates a new key; returns an error text or ''. */
  checkKey: (key: string) => string;
  interfaceOptions?: readonly string[];
  /** Grid query key suffix (changes when rows change). */
  version: number;
  children?: ReactNode;
}

/** Addresses and names stay left-to-right inside RTL text. */
const LTR_INPUT = { htmlInput: { dir: 'ltr' } } as const;

/** Builds `{a: {b: {key: value}}}` from path segments (a merge patch; null deletes). */
function nest(path: readonly string[], key: string, value: unknown): Record<string, unknown> {
  let out: Record<string, unknown> = { [key]: value };
  for (const seg of [...path].reverse()) out = { [seg]: out };
  return out;
}

/**
 * A record of named configuration objects (BGP neighbours and peer groups, prefix lists, route maps): a grid, an editor
 * dialog rendering the item schema with SchemaForm (array widgets for rules / entries), add with a key, delete. Every
 * change is a merge patch of that one record entry into the candidate (the generic pointer route); commit happens from
 * the pending-change bar.
 */
export function RecordEditor<R extends { id: string }>(p: RecordEditorProps<R>) {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const patch = usePatch();
  const root = p.root ?? 'routing';
  const [edit, setEdit] = useState<{ key: string; isNew: boolean } | null>(null);
  const [newKey, setNewKey] = useState('');
  const fetchPage = useCallback(
    async (req: ServerPageRequest) => pageOf(p.rows, req, (r) => JSON.stringify(r)),
    [p.rows],
  );

  const write = async (key: string, value: unknown) => {
    try {
      await patch.mutateAsync({
        key: root,
        patch: nest(p.path, key, p.leaf !== undefined ? { [p.leaf]: value } : value),
      });
      setEdit(null);
    } catch {
      // shown in the dialog
    }
  };
  const keyError = edit?.isNew ? p.checkKey(newKey) : '';
  const current = edit && !edit.isNew ? p.record?.[edit.key] : undefined;
  const segs = [
    ...p.path,
    edit?.isNew ? newKey : (edit?.key ?? ''),
    ...(p.leaf !== undefined ? [p.leaf] : []),
  ];
  const prefix = `/${root}/${segs.map(esc).join('/')}`;

  return (
    <Box sx={{ mb: 3 }}>
      <Typography variant="h6" sx={{ mb: 1 }}>
        {t(`${p.k}.title`)}
      </Typography>
      <Typography color="text.secondary" sx={{ mb: 1 }}>
        {t(`${p.k}.intro`)}
      </Typography>
      <Stack direction="row" gap={1} sx={{ mb: 1 }}>
        <Tooltip title={perms.editConfig ? '' : t('readonly')}>
          <span>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              disabled={!perms.editConfig}
              onClick={() => {
                setNewKey('');
                setEdit({ key: '', isNew: true });
              }}
            >
              {t(`${p.k}.add`)}
            </Button>
          </span>
        </Tooltip>
        {p.children}
      </Stack>
      <Paper variant="outlined" sx={{ blockSize: 340 }}>
        <ServerDataGrid<R>
          aria-label={t(`${p.k}.title`)}
          columns={p.columns}
          queryKey={['config', 'candidate', root, ...p.path, 'grid', p.version]}
          fetchPage={fetchPage}
          initialPageSize={25}
          onRowClick={(r) => setEdit({ key: r.row.id, isNew: false })}
          sx={{ '& .MuiDataGrid-row': { cursor: 'pointer' } }}
        />
      </Paper>
      <Dialog open={edit !== null} onClose={() => setEdit(null)} fullWidth maxWidth="md">
        <DialogTitle>
          {edit?.isNew ? t(`${p.k}.addTitle`) : t(`${p.k}.editTitle`, { key: edit?.key ?? '' })}
        </DialogTitle>
        <DialogContent>
          {edit && (
            <Box sx={{ pt: 1 }}>
              {edit.isNew && (
                <TextField
                  label={t(`${p.k}.key`)}
                  value={newKey}
                  onChange={(e) => setNewKey(e.target.value.trim())}
                  error={newKey !== '' && keyError !== ''}
                  helperText={newKey !== '' ? keyError : ''}
                  fullWidth
                  sx={{ mb: 2 }}
                  slotProps={LTR_INPUT}
                />
              )}
              {patch.isError && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
              <SchemaForm
                key={edit.key}
                schema={p.schema}
                value={current}
                readOnly={!perms.editConfig || (edit.isNew && (newKey === '' || keyError !== ''))}
                interfaceOptions={p.interfaceOptions ?? []}
                submitLabel={t('save')}
                resetLabel={t('reset')}
                problem={problemFor(patch.error, prefix)}
                onSubmit={(v) => write(edit.isNew ? newKey : edit.key, v)}
              >
                {!edit.isNew && (
                  <Button
                    color="error"
                    variant="outlined"
                    startIcon={<DeleteIcon />}
                    disabled={!perms.editConfig || patch.isPending}
                    onClick={() => void write(edit.key, null)}
                  >
                    {t('remove')}
                  </Button>
                )}
              </SchemaForm>
            </Box>
          )}
        </DialogContent>
      </Dialog>
    </Box>
  );
}
