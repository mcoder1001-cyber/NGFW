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
import { NS, subtreeSchema, type Subtree } from './model';

/**
 * The schema-driven editor of one `nat.<subtree>` (det44, dslite, map, cnat, pnat): Save sends only the edits as a merge
 * patch of `/config/nat` through the generic pointer route; server problems at `/nat/<subtree>/…` land on their fields.
 * Field titles come from the schema (English); the tab texts around it are localized.
 */
export function SubtreeForm({ subtree }: { subtree: Subtree }) {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const candidate = useCandidateNat();
  const patch = usePatchNat();
  const ifs = useCandidateInterfaces();
  const [saved, setSaved] = useState(false);
  const schema = useMemo(() => subtreeSchema(subtree), [subtree]);
  const interfaceOptions = useMemo(() => Object.keys(ifs.data ?? {}).sort(), [ifs.data]);
  const [opened, setOpened] = useState<Record<string, unknown> | null>(null);
  if (candidate.isSuccess && opened === null) {
    const v = candidate.data[subtree];
    setOpened(
      v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {},
    );
  }

  const save = async (value: unknown) => {
    setSaved(false);
    const cleaned = value as Record<string, unknown>;
    const inner = createMergePatch(opened ?? {}, cleaned) as Record<string, unknown>;
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
