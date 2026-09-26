import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import LinearProgress from '@mui/material/LinearProgress';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { problemFor } from '../../interfaces/InterfaceDrawer';
import { createMergePatch } from '../../interfaces/model';
import { useCandidateInterfaces } from '../../interfaces/queries';
import { NAT_POINTER } from '../nat44-ed-sessions/model';
import { useCandidateNat, usePatchNat } from '../nat44-ed-sessions/queries';
import { localizeDeep, NS, subtreeSchema, type Subtree } from './model';

/**
 * The schema-driven editor of one `nat.<subtree>` (nat64, nat66, nptv6): the candidate's value in a SchemaForm; Save
 * sends only the edits as a merge patch of `/config/nat` (`{ <subtree>: … }`, arrays replace whole) through the generic
 * pointer route. Server problems at `/nat/<subtree>/…` land on their fields.
 */
export function SubtreeForm({ subtree }: { subtree: Subtree }) {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const candidate = useCandidateNat();
  const patch = usePatchNat();
  const ifs = useCandidateInterfaces();
  const [saved, setSaved] = useState(false);
  const schema = useMemo(
    () => localizeDeep(subtreeSchema(subtree), (k, o) => t(k, o ?? {}), subtree),
    [t, subtree],
  );
  const interfaceOptions = useMemo(() => Object.keys(ifs.data ?? {}).sort(), [ifs.data]);
  // the form edits the value it opened with (P08 review N4): Save sends only the edits against it
  const [opened, setOpened] = useState<Record<string, unknown> | null>(null);
  if (candidate.isSuccess && opened === null) {
    const v = candidate.data[subtree];
    setOpened(
      v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {},
    );
  }

  const save = async (value: unknown) => {
    setSaved(false);
    const base = opened ?? {};
    const cleaned = value as Record<string, unknown>; // WEB-1: no dropPhantomOptionals (deprecated no-op)
    const inner = createMergePatch(base, cleaned) as Record<string, unknown>;
    if (Object.keys(inner).length === 0) {
      setSaved(true);
      return;
    }
    try {
      await patch.mutateAsync({ [subtree]: inner });
      setOpened(cleaned);
      setSaved(true);
    } catch {
      // shown from patch.error
    }
  };

  return (
    <Box>
      {candidate.isPending && <LinearProgress aria-label={t('loading')} />}
      {candidate.isError && <ProblemAlert error={candidate.error} sx={{ mb: 1 }} />}
      {patch.isError && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
      {saved && !patch.isError && (
        <Alert severity="success" sx={{ mb: 1 }}>
          {t('saved')}
        </Alert>
      )}
      {opened !== null && (
        <SchemaForm
          schema={schema}
          value={opened}
          readOnly={!perms.editConfig}
          interfaceOptions={interfaceOptions}
          submitLabel={t('save')}
          resetLabel={t('reset')}
          problem={problemFor(patch.error, `${NAT_POINTER}/${subtree}`)}
          onSubmit={save}
        />
      )}
    </Box>
  );
}
