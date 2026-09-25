import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import MenuItem from '@mui/material/MenuItem';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import { SchemaForm, type JsonSchema } from '@ngfw/ui-kit/schema-form';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { problemFor } from './model';

/** Record keys and parents of the edited node: a free name, or a choice among existing names (server, subnet). */
export interface KeyField {
  id: string;
  label: string;
  /** Choices; undefined = free text (a new record name). */
  options?: (keys: Record<string, string>) => string[];
}

/** objectName in packages/schema: the record key rule of servers, subnets, reservations and relays. */
const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/;
const LTR = { dir: 'ltr' } as const;

export interface EditTarget {
  /** Initial key values; all present = editing an existing node (keys are then read-only). */
  keys: Record<string, string>;
  value: unknown;
  editing: boolean;
}

/**
 * One schema-driven editor for every DHCP node (server, subnet, reservation, relay): the key fields, then a plain
 * SchemaForm on the node's JSON Schema. Server problems are mapped onto the form through `pointerPrefix`.
 */
export function EditDialog({
  open,
  title,
  target,
  keyFields,
  existing,
  schema,
  pointerPrefix,
  error,
  pending,
  interfaceOptions,
  onCancel,
  onSubmit,
}: {
  open: boolean;
  title: string;
  target: EditTarget | null;
  keyFields: KeyField[];
  /** Names already taken for the given parent keys (an add never merges over an existing node). */
  existing: (keys: Record<string, string>) => string[];
  schema: JsonSchema;
  pointerPrefix: (keys: Record<string, string>) => string;
  error: unknown;
  pending: boolean;
  interfaceOptions: readonly string[];
  onCancel: () => void;
  onSubmit: (keys: Record<string, string>, value: unknown) => void;
}) {
  return (
    <Dialog open={open} onClose={onCancel} fullWidth maxWidth="md">
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        {target && (
          <Body
            key={JSON.stringify(target.keys)}
            target={target}
            keyFields={keyFields}
            existing={existing}
            schema={schema}
            pointerPrefix={pointerPrefix}
            error={error}
            pending={pending}
            interfaceOptions={interfaceOptions}
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
  keyFields,
  existing,
  schema,
  pointerPrefix,
  error,
  pending,
  interfaceOptions,
  onCancel,
  onSubmit,
}: {
  target: EditTarget;
  keyFields: KeyField[];
  existing: (keys: Record<string, string>) => string[];
  schema: JsonSchema;
  pointerPrefix: (keys: Record<string, string>) => string;
  error: unknown;
  pending: boolean;
  interfaceOptions: readonly string[];
  onCancel: () => void;
  onSubmit: (keys: Record<string, string>, value: unknown) => void;
}) {
  const { t } = useTranslation('kea-dhcp-relay');
  const [keys, setKeys] = useState<Record<string, string>>(target.keys);
  const nameField = keyFields.find((f) => f.options === undefined);
  const name = nameField ? (keys[nameField.id] ?? '') : '';
  const taken = !target.editing && nameField !== undefined && existing(keys).includes(name);
  const keysOk =
    keyFields.every(
      (f) =>
        (keys[f.id] ?? '') !== '' &&
        (f.options ? f.options(keys).includes(keys[f.id] ?? '') : NAME_RE.test(keys[f.id] ?? '')),
    ) && !taken;
  const problem = problemFor(error, pointerPrefix(keys));
  return (
    <Stack gap={2} sx={{ pt: 1 }}>
      {keyFields.map((f) =>
        f.options ? (
          <TextField
            key={f.id}
            select
            label={f.label}
            value={keys[f.id] ?? ''}
            disabled={target.editing}
            onChange={(e) => setKeys((k) => ({ ...k, [f.id]: e.target.value }))}
          >
            {f.options(keys).map((o) => (
              <MenuItem key={o} value={o} dir="ltr">
                {o}
              </MenuItem>
            ))}
          </TextField>
        ) : (
          <TextField
            key={f.id}
            label={f.label}
            value={keys[f.id] ?? ''}
            disabled={target.editing}
            onChange={(e) => setKeys((k) => ({ ...k, [f.id]: e.target.value }))}
            error={(keys[f.id] ?? '') !== '' && (!NAME_RE.test(keys[f.id] ?? '') || taken)}
            helperText={taken ? t('dialog.nameTaken', { name }) : t('dialog.nameHelp')}
            slotProps={{ htmlInput: LTR }}
          />
        ),
      )}
      {error !== null && error !== undefined && problem === null && <ProblemAlert error={error} />}
      <SchemaForm
        schema={schema}
        value={target.value}
        problem={problem}
        interfaceOptions={interfaceOptions}
        submitLabel={t('dialog.save')}
        resetLabel={t('dialog.reset')}
        onSubmit={(v) => {
          if (keysOk && !pending) onSubmit(keys, v);
        }}
      >
        <Button onClick={onCancel}>{t('dialog.cancel')}</Button>
      </SchemaForm>
    </Stack>
  );
}
