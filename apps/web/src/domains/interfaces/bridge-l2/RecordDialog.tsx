import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import MenuItem from '@mui/material/MenuItem';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import { SchemaForm, type JsonSchema } from '@ngfw/ui-kit/schema-form';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { problemFor } from '../InterfaceDrawer';
import { dropPhantomOptionals, localizeSchema } from '../model';

const LTR = { dir: 'ltr' } as const;

/**
 * One keyed record of the L2 model edited with the schema-driven form: the key is typed (validated by `keyRe`) or picked
 * from `keyOptions`; the value is the record's SchemaForm. Server problems are mapped onto the form by `pointerOf(key)`.
 */
export function RecordDialog({
  open,
  title,
  help,
  keyLabel,
  keyHelp,
  keyRe,
  keyOptions,
  fixedKey,
  takenKeys = [],
  schema,
  value,
  error,
  pending,
  readOnly,
  pointerOf,
  onClose,
  onSubmit,
}: {
  open: boolean;
  title: string;
  help?: string;
  keyLabel: string;
  keyHelp?: string;
  keyRe?: RegExp;
  keyOptions?: string[];
  fixedKey?: string;
  takenKeys?: string[];
  schema: JsonSchema;
  value: unknown;
  error: unknown;
  pending: boolean;
  readOnly: boolean;
  pointerOf: (key: string) => string;
  onClose: () => void;
  onSubmit: (key: string, value: unknown) => void;
}) {
  const { t } = useTranslation('bridge-l2');
  const [key, setKey] = useState(fixedKey ?? '');
  const localized = localizeSchema(schema, (k, o) => t(k, o ?? {}));
  const taken = fixedKey === undefined && takenKeys.includes(key);
  const keyOk = key !== '' && !taken && (keyRe === undefined || keyRe.test(key));
  const problem = problemFor(error, pointerOf(key));
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        <Stack gap={2} sx={{ pt: 1 }}>
          {help && <DialogContentText>{help}</DialogContentText>}
          {keyOptions ? (
            <TextField
              select
              label={keyLabel}
              value={key}
              disabled={fixedKey !== undefined}
              onChange={(e) => setKey(e.target.value)}
              helperText={taken ? t('nameExists') : (keyHelp ?? ' ')}
              error={taken}
              slotProps={{ htmlInput: LTR }}
            >
              {(fixedKey !== undefined ? [fixedKey] : keyOptions).map((o) => (
                <MenuItem key={o} value={o} dir="ltr">
                  {o}
                </MenuItem>
              ))}
            </TextField>
          ) : (
            <TextField
              autoFocus
              label={keyLabel}
              value={key}
              disabled={fixedKey !== undefined}
              onChange={(e) => setKey(e.target.value)}
              error={key !== '' && !keyOk}
              helperText={
                taken ? t('nameExists') : key !== '' && !keyOk ? t('badName') : (keyHelp ?? ' ')
              }
              slotProps={{ htmlInput: LTR }}
            />
          )}
          {error !== null && error !== undefined && problem === null && (
            <ProblemAlert error={error} />
          )}
          <SchemaForm
            schema={localized}
            value={value}
            readOnly={readOnly}
            problem={problem}
            submitLabel={t('save')}
            resetLabel={t('reset')}
            onSubmit={(v) => {
              if (keyOk && !pending) onSubmit(key, dropPhantomOptionals(schema, value, v));
            }}
          >
            <Button onClick={onClose}>{t('cancel')}</Button>
          </SchemaForm>
        </Stack>
      </DialogContent>
    </Dialog>
  );
}
