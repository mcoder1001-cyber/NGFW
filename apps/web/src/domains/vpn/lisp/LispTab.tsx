import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import Stack from '@mui/material/Stack';
import Tab from '@mui/material/Tab';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tabs from '@mui/material/Tabs';
import Typography from '@mui/material/Typography';
import { StatusChip } from '@ngfw/ui-kit';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { problemFor } from '../../interfaces/InterfaceDrawer';
import { LISP_POINTER, SECTIONS, sectionPatch, sectionSchema, sectionValue, statusRows, type SectionId } from './model';
import { useCandidateLisp, useLispState, usePatchTunnels } from './queries';

/**
 * LISP / LISP-GPE (F-lisp, "advanced"): one tab of the VPN page with sub-tabs Locators / EIDs / Mappings / Resolvers.
 * Each sub-tab edits its slice of `tunnels.lisp` with the SchemaForm (the one schema) and lists the configured objects
 * with their live status from `GET /api/v1/state/lisp`. Changes go to the candidate; commit from the pending bar.
 */
export function LispTab() {
  const { t } = useTranslation('lisp');
  const [section, setSection] = useState<SectionId>('locators');
  const perms = usePermissions();
  const candidate = useCandidateLisp();
  const state = useLispState();
  const patch = usePatchTunnels();
  const schema = useMemo(() => sectionSchema(section), [section]);
  const value = useMemo(() => sectionValue(section, candidate.data), [section, candidate.data]);
  const rows = useMemo(() => statusRows(section, candidate.data, state.data ?? null), [section, candidate.data, state.data]);

  return (
    <Stack gap={2}>
      <Alert severity="info">{t('advanced')}</Alert>
      <Tabs value={section} onChange={(_, v: SectionId) => setSection(v)} aria-label={t('sections')} variant="scrollable">
        {SECTIONS.map((s) => (
          <Tab key={s.id} value={s.id} label={t(`section.${s.id}`)} id={`lisp-tab-${s.id}`} aria-controls={`lisp-panel-${s.id}`} />
        ))}
      </Tabs>
      <Box role="tabpanel" id={`lisp-panel-${section}`} aria-labelledby={`lisp-tab-${section}`}>
        {state.isError && <ProblemAlert error={state.error} sx={{ mb: 2 }} />}
        <Typography component="h3" variant="subtitle1" sx={{ mb: 1 }}>
          {t('status.title')}
        </Typography>
        <Table size="small" aria-label={t('status.title')} sx={{ mb: 3 }}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.kind')}</TableCell>
              <TableCell>{t('col.id')}</TableCell>
              <TableCell>{t('col.detail')}</TableCell>
              <TableCell>{t('col.status')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.length === 0 && (
              <TableRow>
                <TableCell colSpan={4}>
                  <Typography variant="body2" color="text.secondary">
                    {t('empty')}
                  </Typography>
                </TableCell>
              </TableRow>
            )}
            {rows.map((r) => (
              <TableRow key={`${r.kind}:${r.id}`}>
                <TableCell>{t(`kind.${r.kind}`)}</TableCell>
                <TableCell dir="ltr">{r.id}</TableCell>
                <TableCell dir="ltr">{r.detail}</TableCell>
                <TableCell>
                  <StatusChip size="small" status={r.status} label={t(r.labelKey)} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {candidate.isPending && <CircularProgress aria-label={t('loading')} />}
        {candidate.isError && <ProblemAlert error={candidate.error} />}
        {candidate.isSuccess && (
          <SchemaForm
            key={section}
            schema={schema}
            value={value}
            readOnly={!perms.editConfig}
            submitLabel={t('save')}
            resetLabel={t('reset')}
            problem={problemFor(patch.error, LISP_POINTER)}
            onSubmit={(v) => patch.mutateAsync(sectionPatch(section, v as Record<string, unknown>)).then(() => undefined)}
          />
        )}
      </Box>
    </Stack>
  );
}

export default LispTab;
