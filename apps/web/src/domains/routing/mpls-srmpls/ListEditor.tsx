import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { SchemaForm, type JsonSchema } from '@ngfw/ui-kit/schema-form';
import { useCallback, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { pageOf, problemFor } from './common';
import { NS } from './model';

/** One row of a ListEditor: `id` is the grid key, `ref` whatever the owner needs to find the entry again. */
export interface EditorRow {
  id: string;
}

export interface ListEditorProps<R extends EditorRow> {
  /** Section title and the grid's accessible name. */
  title: string;
  intro?: string | undefined;
  columns: GridColDef<R>[];
  rows: R[];
  /** Grid refresh key (e.g. the candidate's dataUpdatedAt). */
  version: number;
  /** Schema of one entry (localized). */
  schema: JsonSchema;
  /** Form value of an existing entry (`null` = a new one: schema defaults). */
  valueOf: (row: R | null) => unknown;
  onSave: (value: unknown, row: R | null) => Promise<void>;
  onRemove: (row: R) => Promise<void>;
  /** The last write's error (mapped onto the form by pointer). */
  error: unknown;
  pending: boolean;
  /** Pointer of the edited entry in the document (server problems under it land on the form). */
  pointerOf: (row: R | null) => string;
  /** Form member that holds a record key (`name`), for problems at the entry's own pointer. */
  keyField?: string | undefined;
  interfaceOptions?: readonly string[] | undefined;
  addLabel: string;
  editTitle: (row: R) => string;
  addTitle: string;
  /** Extra content above the grid (live status, refresh). */
  toolbar?: ReactNode;
  height?: number | undefined;
}

/**
 * A grid of configuration entries plus a schema-driven editor dialog (SchemaForm over the entry's JSON Schema — the one
 * schema, 00-CONTEXT rule 5). Saving writes the whole `routing.mpls` back through the owner's `onSave`; problems from
 * the API are mapped onto the form by pointer. Read-only users see the grid and a disabled form.
 */
export function ListEditor<R extends EditorRow>(p: ListEditorProps<R>) {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const [edit, setEdit] = useState<{ row: R | null } | null>(null);
  const fetchPage = useCallback(
    async (req: ServerPageRequest) => pageOf(p.rows, req, (r) => JSON.stringify(r)),
    [p.rows],
  );
  const save = async (value: unknown) => {
    if (!edit) return;
    try {
      await p.onSave(value, edit.row);
      setEdit(null);
    } catch {
      // shown in the dialog
    }
  };
  const remove = async (row: R) => {
    try {
      await p.onRemove(row);
      setEdit(null);
    } catch {
      // shown in the dialog
    }
  };
  return (
    <Box sx={{ mb: 3 }}>
      <Typography variant="h6" component="h2" sx={{ mb: 1 }}>
        {p.title}
      </Typography>
      {p.intro && (
        <Typography color="text.secondary" sx={{ mb: 1 }}>
          {p.intro}
        </Typography>
      )}
      <Stack direction="row" gap={1} alignItems="center" sx={{ mb: 1 }}>
        <Tooltip title={perms.editConfig ? '' : t('readonly')}>
          <span>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              disabled={!perms.editConfig}
              onClick={() => setEdit({ row: null })}
            >
              {p.addLabel}
            </Button>
          </span>
        </Tooltip>
        {p.toolbar}
      </Stack>
      <Paper variant="outlined" sx={{ blockSize: p.height ?? 320 }}>
        <ServerDataGrid<R>
          aria-label={p.title}
          columns={p.columns}
          queryKey={['config', 'candidate', 'routing', 'mpls', p.title, p.version, p.rows.length]}
          fetchPage={fetchPage}
          initialPageSize={25}
          onRowClick={(e) => setEdit({ row: e.row })}
          sx={{ '& .MuiDataGrid-row': { cursor: 'pointer' } }}
        />
      </Paper>
      <Dialog open={edit !== null} onClose={() => setEdit(null)} fullWidth maxWidth="md">
        <DialogTitle>{edit?.row ? p.editTitle(edit.row) : p.addTitle}</DialogTitle>
        <DialogContent>
          {edit && (
            <Box sx={{ pt: 1 }}>
              <SchemaForm
                key={edit.row?.id ?? 'new'}
                schema={p.schema}
                value={p.valueOf(edit.row)}
                readOnly={!perms.editConfig}
                interfaceOptions={p.interfaceOptions}
                submitLabel={t('save')}
                resetLabel={t('reset')}
                problem={problemFor(p.error, p.pointerOf(edit.row), p.keyField)}
                onSubmit={save}
              >
                {edit.row && (
                  <Button
                    color="error"
                    variant="outlined"
                    startIcon={<DeleteIcon />}
                    disabled={!perms.editConfig || p.pending}
                    onClick={() => void remove(edit.row as R)}
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
