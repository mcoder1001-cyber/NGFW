import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Stack from '@mui/material/Stack';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { objectModelWidgets } from '../object-model';
import {
  esc,
  localizeSchema,
  problemFor,
  ruleFormSchema,
  type AclRule,
  type RuleRow,
} from './model';
import { useAclT } from './parts';
import { candidateAt, fetchRules, useAclEdit } from './queries';

export interface RuleEdit {
  /** The rule being edited; `null` = a new rule (appended to the list). */
  row: RuleRow | null;
  /** Sequence offered for a new rule. */
  suggested?: number | undefined;
}

/**
 * Add or edit one rule with the schema-driven form (AclRuleSchema; source, destination, service and schedule use the
 * inline object picker). Edit → PUT `/config/acl/lists/<list>/rules/<index>` after checking that the candidate still has
 * this rule at that position; add → PUT `…/rules/<size>` (appends) with the size read right before.
 */
export function RuleDialog({
  list,
  edit,
  readOnly,
  onClose,
}: {
  list: string;
  edit: RuleEdit | null;
  readOnly: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation('acl');
  const title = edit?.row
    ? t('rule.editTitle', { sequence: edit.row.sequence })
    : t('rule.addTitle');
  return (
    <Dialog open={edit !== null} onClose={onClose} fullWidth maxWidth="md">
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        {edit && (
          <RuleForm
            key={edit.row?.index ?? 'new'}
            list={list}
            edit={edit}
            readOnly={readOnly}
            onDone={onClose}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function RuleForm({
  list,
  edit,
  readOnly,
  onDone,
}: {
  list: string;
  edit: RuleEdit;
  readOnly: boolean;
  onDone: () => void;
}) {
  const { t } = useTranslation('acl');
  const tr = useAclT();
  const mutation = useAclEdit();
  const [error, setError] = useState<unknown>(null);
  const [stale, setStale] = useState(false);
  const [index, setIndex] = useState<number | null>(edit.row?.index ?? null);
  const schema = useMemo(() => localizeSchema(ruleFormSchema(), tr), [tr]);
  const value = useMemo<Partial<AclRule>>(
    () => edit.row?.rule ?? { sequence: edit.suggested ?? 10, action: 'permit' },
    [edit],
  );

  const save = async (v: unknown) => {
    setError(null);
    setStale(false);
    try {
      let at: number;
      if (edit.row) {
        at = edit.row.index;
        // another session may have moved or deleted rules since the page was read: never overwrite a different rule
        const current = await candidateAt<Partial<AclRule> | null>(
          ['acl', 'lists', list, 'rules', at],
          null,
        );
        if (!current || current.sequence !== edit.row.sequence) {
          setStale(true);
          return;
        }
      } else {
        at = (await fetchRules(list, { page: 1, pageSize: 1, source: 'candidate' })).size;
      }
      setIndex(at);
      await mutation.mutateAsync({
        method: 'PUT',
        path: ['acl', 'lists', list, 'rules', at],
        body: v,
      });
      onDone();
    } catch (e) {
      setError(e);
    }
  };
  const problem =
    index === null ? null : problemFor(error, `/acl/lists/${esc(list)}/rules/${index}`);
  return (
    <Stack gap={2} sx={{ pt: 1 }}>
      {stale && <Alert severity="warning">{t('rule.stale')}</Alert>}
      {error !== null && problem === null && <ProblemAlert error={error} />}
      <SchemaForm
        schema={schema}
        value={value}
        readOnly={readOnly}
        widgets={objectModelWidgets}
        problem={problem}
        submitLabel={t('save')}
        resetLabel={t('reset')}
        onSubmit={save}
      >
        <Button onClick={onDone}>{t('cancel')}</Button>
      </SchemaForm>
    </Stack>
  );
}
