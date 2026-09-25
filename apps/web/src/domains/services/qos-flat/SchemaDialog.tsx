import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import { SchemaForm, type JsonSchema } from '@ngfw/ui-kit/schema-form';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { esc, problemFor } from './model';

/** objectName in packages/schema: the record key rule of policers, shapers and maps. */
export const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/;
const LTR = { dir: 'ltr' } as const;

export interface EditTarget {
  /** '' = a new node (the name is typed in the dialog). */
  name: string;
  value: unknown;
  editing: boolean;
}

/**
 * One schema-driven editor for a named `services.qos` node (a policer or a rate limit): the record key, then a plain
 * SchemaForm on the node's JSON Schema (the one schema). Server problems are mapped onto the form through the node's
 * pointer prefix. `extra` renders below the form (the policer's rate/burst helper).
 */
export function SchemaDialog({
  open,
  title,
  target,
  existing,
  schema,
  collection,
  error,
  pending,
  extra,
  onCancel,
  onSubmit,
}: {
  open: boolean;
  title: string;
  target: EditTarget | null;
  existing: readonly string[];
  schema: JsonSchema;
  /** `policers` or `shapers`: the pointer prefix is /services/qos/<collection>/<name>. */
  collection: string;
  error: unknown;
  pending: boolean;
  extra?: ((apply: (patch: Record<string, unknown>) => void, value: unknown) => ReactNode) | undefined;
  onCancel: () => void;
  onSubmit: (name: string, value: unknown) => void;
}) {
  return (
    <Dialog open={open} onClose={onCancel} fullWidth maxWidth="md">
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        {target && (
          <Body
            key={`${target.name}:${JSON.stringify(target.value)}`}
            target={target}
            existing={existing}
            schema={schema}
            collection={collection}
            error={error}
            pending={pending}
            extra={extra}
            onCancel={onCancel}
            onSubmit={onSubmit}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function Body({
  target,
  existing,
  schema,
  collection,
  error,
  pending,
  extra,
  onCancel,
  onSubmit,
}: {
  target: EditTarget;
  existing: readonly string[];
  schema: JsonSchema;
  collection: string;
  error: unknown;
  pending: boolean;
  extra?: ((apply: (patch: Record<string, unknown>) => void, value: unknown) => ReactNode) | undefined;
  onCancel: () => void;
  onSubmit: (name: string, value: unknown) => void;
}) {
  const { t } = useTranslation('qos-flat');
  const [name, setName] = useState(target.name);
  // the helper below the form writes into the value the form starts from (a new form instance, same schema)
  const [value, setValue] = useState<unknown>(target.value);
  const [generation, setGeneration] = useState(0);
  const taken = !target.editing && existing.includes(name);
  const nameOk = NAME_RE.test(name) && !taken;
  const problem = problemFor(error, `/services/qos/${collection}/${esc(name)}`);
  // the helper restarts the form from its last starting value plus the patch (unsaved edits in the form are reset)
  const apply = (patch: Record<string, unknown>) => {
    setValue((cur: unknown) => ({ ...(typeof cur === 'object' && cur !== null ? cur : {}), ...patch }));
    setGeneration((g) => g + 1);
  };
  return (
    <Stack gap={2} sx={{ pt: 1 }}>
      <TextField
        label={t('dialog.name')}
        value={name}
        disabled={target.editing}
        onChange={(e) => setName(e.target.value)}
        error={name !== '' && !nameOk}
        helperText={taken ? t('dialog.nameTaken', { name }) : t('dialog.nameHelp')}
        slotProps={{ htmlInput: LTR }}
      />
      {error !== null && error !== undefined && problem === null && <ProblemAlert error={error} />}
      {extra?.(apply, value)}
      <SchemaForm
        key={generation}
        schema={schema}
        value={value}
        problem={problem}
        submitLabel={t('dialog.save')}
        resetLabel={t('dialog.reset')}
        onSubmit={(v) => {
          if (nameOk && !pending) onSubmit(name, v);
        }}
      >
        <Button onClick={onCancel}>{t('dialog.cancel')}</Button>
      </SchemaForm>
    </Stack>
  );
}
