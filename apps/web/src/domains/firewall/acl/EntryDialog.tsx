import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Stack from '@mui/material/Stack';
import { SchemaForm, type JsonSchema } from '@ngfw/ui-kit/schema-form';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { objectModelWidgets } from '../object-model';
import { problemFor, samePlain } from './model';
import { candidateAt, useAclEdit, type PointerPath } from './queries';

/** The candidate's array changed under the open form (another session): nothing was written. */
export class StaleEntryError extends Error {
  constructor() {
    super('stale');
    this.name = 'StaleEntryError';
  }
}

/**
 * Edits of an array member of `acl` (`attachments`, `macipAttachments`). A merge patch replaces arrays wholesale, so the
 * whole array is sent — built from the candidate as it is now, and refused when the edited entry moved or changed since
 * the table was read.
 */
export function useArrayEdit<T>(member: 'attachments' | 'macipAttachments') {
  const edit = useAclEdit();
  const path: PointerPath = ['acl', member];
  const write = async (change: (current: T[]) => T[] | null) => {
    const next = change(await candidateAt<T[]>(path, []));
    if (next === null) throw new StaleEntryError();
    await edit.mutateAsync({ method: 'PATCH', path: ['acl'], body: { [member]: next } });
  };
  return {
    busy: edit.isPending,
    /** Replace the entry at `index` (which must still equal `original`), or append when `index` is null. */
    save: (index: number | null, original: T | undefined, value: T) =>
      write((cur) =>
        index === null
          ? [...cur, value]
          : samePlain(cur[index], original)
            ? cur.map((x, i) => (i === index ? value : x))
            : null,
      ),
    remove: (index: number, original: T) =>
      write((cur) => (samePlain(cur[index], original) ? cur.filter((_x, i) => i !== index) : null)),
    /** Pointer prefix of the entry for server errors (`/acl/attachments/3`). */
    pointer: (index: number) => `/acl/${member}/${index}`,
  };
}

/** A schema-driven dialog for one array entry (attachment, MACIP attachment). */
export function EntryDialog<T>({
  open,
  title,
  schema,
  value,
  pointer,
  interfaceOptions,
  readOnly,
  validate,
  onSave,
  onClose,
}: {
  open: boolean;
  title: string;
  schema: JsonSchema;
  value: T | undefined;
  /** Pointer prefix of the entry (server errors are mapped onto the form below it). */
  pointer: string;
  interfaceOptions?: readonly string[];
  readOnly: boolean;
  /** Client-side check before anything is sent: an error message, or null. */
  validate?: (value: T) => string | null;
  onSave: (value: T) => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useTranslation('acl');
  const [error, setError] = useState<unknown>(null);
  const [message, setMessage] = useState<string | null>(null);
  const submit = async (v: unknown) => {
    setError(null);
    setMessage(null);
    const problem = validate?.(v as T) ?? null;
    if (problem) {
      setMessage(problem);
      return;
    }
    try {
      await onSave(v as T);
      onClose();
    } catch (e) {
      if (e instanceof StaleEntryError) setMessage(t('entry.stale'));
      else setError(e);
    }
  };
  const problem = problemFor(error, pointer);
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        {open && (
          <Stack gap={2} sx={{ pt: 1 }}>
            {message && <Alert severity="warning">{message}</Alert>}
            {error !== null && problem === null && <ProblemAlert error={error} />}
            <SchemaForm
              schema={schema}
              value={value}
              readOnly={readOnly}
              widgets={objectModelWidgets}
              interfaceOptions={interfaceOptions}
              problem={problem}
              submitLabel={t('save')}
              resetLabel={t('reset')}
              onSubmit={submit}
            >
              <Button onClick={onClose}>{t('cancel')}</Button>
            </SchemaForm>
          </Stack>
        )}
      </DialogContent>
    </Dialog>
  );
}
