import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { SchemaForm, type JsonSchema, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { domainSchemas } from '../../../schema/registry';
import { configPathTo } from '../../advanced/schemaPath';
import { PATH, useCandidateSecurity, usePatchAutoBlock, type AutoBlockConfig } from './queries';

export const NS = 'security';
/** JSON pointer of the edited node in the whole document; server problems under it map onto the form. */
const BASE = `/${PATH}/autoBlock`;

/** `security.autoBlock` sub-schema — the one schema, never a hand-written form. */
export function autoBlockSchema(): JsonSchema {
  const security = domainSchemas.security as { properties?: { autoBlock?: JsonSchema } };
  const autoBlock = security.properties?.autoBlock;
  if (!autoBlock) throw new Error('security.autoBlock schema not found');
  return autoBlock;
}

/**
 * The node as the schema defaults it (`enabled: false`, empty rules/allow-list, the default cap) — what a candidate
 * without `security.autoBlock` yet means. Read from the schema, never written by hand (R1: the form left a number
 * field empty when seeded with `undefined`).
 */
export function autoBlockDefaults(schema: JsonSchema): AutoBlockConfig {
  const out: Record<string, unknown> = {};
  for (const [key, prop] of Object.entries(schema.properties ?? {})) {
    if (prop.default !== undefined) out[key] = prop.default;
  }
  return out as AutoBlockConfig;
}

/** Server problem with pointers made relative to the form (`/security/autoBlock/rules/0/maxBlockSec` → `/rules/0/maxBlockSec`). */
export function problemUnder(error: unknown): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const problem = error.toFormProblem();
  const relative = (pointer: string) =>
    pointer === BASE ? '/' : pointer.startsWith(`${BASE}/`) ? pointer.slice(BASE.length) : pointer;
  return {
    ...problem,
    ...(typeof problem.pointer === 'string' ? { pointer: relative(problem.pointer) } : {}),
    errors: (problem.errors ?? []).map((e) => ({ ...e, pointer: relative(e.pointer) })),
  };
}

/**
 * Security › Auto-block policy (F-security-page): the committed `security.autoBlock` policy — master switch, one
 * detector rule per source, the allow-list and the cap on the live set — edited as a schema form and saved to the
 * candidate through the same domain PATCH the Management tabs use. Validation problems from the API (duplicate
 * detector, cap below the first-offence block, …) land on their field; the rest is listed on top. The blocked
 * sources themselves are runtime state, shown on the Blocked sources tab.
 */
export default function PolicyTab() {
  const { t } = useTranslation([NS, 'advanced']);
  const perms = usePermissions();
  const readOnly = !perms.editConfig;
  const candidate = useCandidateSecurity();
  const patch = usePatchAutoBlock();
  const schema = useMemo(() => autoBlockSchema(), []);
  const [saved, setSaved] = useState(false);

  const save = async (value: unknown) => {
    setSaved(false);
    try {
      await patch.mutateAsync({ autoBlock: value as AutoBlockConfig, loaded: candidate.data?.autoBlock });
      setSaved(true);
    } catch {
      // shown through `patch.error` (ProblemAlert + field pointers); nothing else to do here
    }
  };

  return (
    <Stack gap={2} alignItems="flex-start">
      <Typography variant="body2" color="text.secondary" sx={{ maxInlineSize: 900 }}>
        {t('policy.intro')}
      </Typography>
      <Stack direction="row" gap={1} flexWrap="wrap" alignItems="center">
        <Button component={RouterLink} to={`/${configPathTo(PATH, ['autoBlock'])}`} size="small" variant="text">
          {t('advanced:openInAdvanced')}
        </Button>
        {readOnly && <Chip size="small" label={t('policy.readOnly')} />}
      </Stack>
      <Paper variant="outlined" sx={{ p: 2, width: '100%', maxWidth: 960 }}>
        {candidate.isPending && <LinearProgress aria-label={t('policy.loading')} />}
        {candidate.isError && <ProblemAlert error={candidate.error} />}
        {patch.isError && <ProblemAlert error={patch.error} sx={{ mb: 2 }} />}
        {saved && !patch.isError && (
          <Alert severity="success" sx={{ mb: 2 }} data-testid="security-policy-saved">
            {t('policy.saved')}
          </Alert>
        )}
        {candidate.isSuccess && (
          <SchemaForm
            id="security-policy-form"
            schema={schema}
            value={candidate.data.autoBlock ?? autoBlockDefaults(schema)}
            readOnly={readOnly}
            i18nPrefix={`${NS}:field.autoBlock`}
            problem={problemUnder(patch.error)}
            submitLabel={t('policy.save')}
            resetLabel={t('policy.reset')}
            onSubmit={save}
          />
        )}
      </Paper>
    </Stack>
  );
}
