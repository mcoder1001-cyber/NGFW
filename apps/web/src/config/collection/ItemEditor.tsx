import DeleteIcon from '@mui/icons-material/Delete';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import TextField from '@mui/material/TextField';
import { SchemaForm, type JsonSchema } from '@ngfw/ui-kit/schema-form';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../ProblemAlert';
import { mapKeyIssue, type CollectionModel } from './model';
import type { ItemEditorState } from './useCollection';

const KEY_INPUT = { dir: 'ltr', spellCheck: false, autoComplete: 'off' } as const;

export interface ItemEditorProps {
  model: CollectionModel;
  editor: ItemEditorState;
  /** The localized form schema (`localizeSchema(model.formSchema, t)`). */
  schema: JsonSchema;
  readOnly: boolean;
  /** Label of the key field of a new map item (`Name`, `Interface`). */
  keyLabel: string;
  /** A new item was saved under `id` (the drawer switches to it). */
  onCreated?: ((id: string) => void) | undefined;
  /** The item was removed from the candidate (the drawer closes). */
  onRemoved: () => void;
  /** Extra buttons next to Save/Reset/Remove. */
  children?: ReactNode;
}

/**
 * The item form of a config drawer: SchemaForm over the item schema with P08's saving semantics (see `useItemEditor`),
 * the server problem above it, "saved", "changed elsewhere → Reload", and Remove with a confirmation.
 */
export function ItemEditor({ model, editor, schema, readOnly, keyLabel, onCreated, onRemoved, children }: ItemEditorProps) {
  const { t } = useTranslation('config');
  const [newKey, setNewKey] = useState('');
  const [confirming, setConfirming] = useState(false);
  const isNew = editor.id === null;
  const askKey = isNew && model.shape.kind === 'map';
  const typedIssue = newKey === '' ? undefined : mapKeyIssue(model, newKey);
  const keyIssue = editor.keyError ?? typedIssue ?? null;

  return (
    <>
      {editor.error !== null && <ProblemAlert error={editor.error} sx={{ mb: 1 }} />}
      {editor.saved && editor.error === null && (
        <Alert severity="success" sx={{ mb: 1 }}>
          {t('kit.saved')}
        </Alert>
      )}
      {editor.changedElsewhere && (
        <Alert
          severity="warning"
          sx={{ mb: 1 }}
          action={
            <Button color="inherit" size="small" onClick={editor.reload}>
              {t('kit.reload')}
            </Button>
          }
        >
          {t('kit.changedElsewhere')}
        </Alert>
      )}
      {askKey && (
        <TextField
          fullWidth
          required
          label={keyLabel}
          value={newKey}
          disabled={readOnly}
          onChange={(e) => setNewKey(e.target.value.trim())}
          error={keyIssue !== null}
          helperText={keyIssue ? t(`kit.key.${keyIssue}`, { key: newKey }) : t('kit.key.help')}
          slotProps={{ htmlInput: KEY_INPUT }}
          sx={{ mb: 2 }}
        />
      )}
      {editor.opened !== null && (
        <SchemaForm
          key={editor.formKey}
          schema={schema}
          value={editor.opened.value}
          readOnly={readOnly}
          submitLabel={t('kit.save')}
          resetLabel={t('kit.reset')}
          problem={editor.problem}
          onSubmit={async (value) => {
            if (editor.pending) return;
            const wasNew = editor.id === null;
            const id = await editor.save(value, newKey);
            if (id !== null && wasNew) onCreated?.(id);
          }}
        >
          {!isNew && editor.row?.value !== undefined && (
            <Button color="error" variant="outlined" startIcon={<DeleteIcon />} disabled={readOnly || editor.pending} onClick={() => setConfirming(true)}>
              {t('kit.remove')}
            </Button>
          )}
          {children}
        </SchemaForm>
      )}
      <Dialog open={confirming} onClose={() => setConfirming(false)} aria-labelledby="kit-remove-title">
        <DialogTitle id="kit-remove-title">{t('kit.removeTitle', { key: editor.row?.label ?? '' })}</DialogTitle>
        <DialogContent>
          <DialogContentText>{t('kit.removeBody')}</DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirming(false)}>{t('cancel')}</Button>
          <Button
            color="error"
            variant="contained"
            disabled={editor.pending}
            onClick={() => {
              setConfirming(false);
              void editor.remove().then((ok) => ok && onRemoved());
            }}
          >
            {t('kit.removeConfirm')}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
