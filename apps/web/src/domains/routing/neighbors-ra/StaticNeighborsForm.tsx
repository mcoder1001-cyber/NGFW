import Alert from '@mui/material/Alert';
import LinearProgress from '@mui/material/LinearProgress';
import Typography from '@mui/material/Typography';
import { SchemaForm, type JsonSchema, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { domainSchemas } from '../../../schema/registry';
import { createMergePatch, dropPhantomOptionals } from '../../interfaces/model';
import { useCandidateNeighbors, useInterfaceNames, usePatchNeighbors } from './queries';

const PREFIX = '/routing/neighbors';

/** `routing.neighbors` as JSON Schema — the one schema (00-CONTEXT rule 5), from the generated `routing` domain. */
export function neighborsSchema(): JsonSchema {
  const props = (domainSchemas.routing.properties ?? {}) as Record<string, JsonSchema>;
  const s = props['neighbors'];
  if (!s) throw new Error('routing.neighbors schema not found');
  return s;
}

/** Server pointers `/routing/neighbors/…` → pointers relative to this form; the rest stay (listed on top). */
export function neighborsProblem(error: unknown): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({
      ...e,
      pointer: e.pointer.startsWith(PREFIX) ? e.pointer.slice(PREFIX.length) : e.pointer,
    })),
  };
}

/**
 * Static neighbours, neighbour-table limits and DAD (`routing.neighbors`): a schema-driven form over the candidate. Save
 * writes the candidate (a merge patch of `routing` on the generic pointer route); the pending-change bar shows the diff and commits it. An
 * optional object the form filled with its defaults (limits, DAD — which would turn DAD on) is dropped again unless the
 * user changed it (P08's dropPhantomOptionals).
 */
export function StaticNeighborsForm() {
  const { t } = useTranslation('neighbors-ra');
  const perms = usePermissions();
  const candidate = useCandidateNeighbors();
  const names = useInterfaceNames();
  const put = usePatchNeighbors();
  const [saved, setSaved] = useState(false);
  const schema = useMemo(neighborsSchema, []);
  const translateLabel = useMemo(
    () => (path: string, fallback: string) => t(`labels.${path}`, { defaultValue: fallback }),
    [t],
  );

  if (candidate.isPending) return <LinearProgress aria-label={t('static.loading')} />;
  const before = candidate.data ?? undefined;

  const save = async (value: unknown) => {
    setSaved(false);
    try {
      const cleaned = dropPhantomOptionals(schema, before, value);
      await put.mutateAsync(before === undefined ? cleaned : createMergePatch(before, cleaned));
      setSaved(true);
    } catch {
      // rendered through `problem` (pointers mapped onto the fields)
    }
  };

  return (
    <>
      <Typography color="text.secondary" sx={{ mb: 1 }}>
        {t('static.intro')}
      </Typography>
      <Alert severity="info" sx={{ mb: 2 }}>
        {t('static.globals')}
      </Alert>
      {saved && (
        <Alert severity="success" sx={{ mb: 2 }} onClose={() => setSaved(false)}>
          {t('static.saved')}
        </Alert>
      )}
      <SchemaForm
        schema={schema}
        value={before}
        readOnly={!perms.editConfig}
        submitLabel={t('static.save')}
        resetLabel={t('static.reset')}
        problem={neighborsProblem(put.error)}
        interfaceOptions={names.data ?? []}
        translateLabel={translateLabel}
        onSubmit={save}
      />
    </>
  );
}
