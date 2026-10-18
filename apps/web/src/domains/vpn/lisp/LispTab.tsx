import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgCircularProgress as CircularProgress,
  PgStack as Stack,
  PgTab as Tab,
  PgTabs as Tabs,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { serviceText } from '../../../product-text';
import { StatusChip } from '@ngfw/ui-kit';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { serviceText } from '../../../product-text';
import { problemFor } from '../../interfaces/InterfaceDrawer';
import {
  LISP_POINTER,
  SECTIONS,
  sectionPatch,
  sectionSchema,
  sectionValue,
  statusRows,
  type SectionId,
} from './model';
import { useCandidateLisp, useLispState, usePatchTunnels } from './queries';

/**
 * LISP / LISP-GPE (F-lisp, "advanced"): one tab of the VPN page with sub-tabs Locators / EIDs / Mappings / Resolvers.
 * Each sub-tab edits its slice of `tunnels.lisp` with the SchemaForm (the one schema) and lists the configured objects
 * with their live status from `GET /api/v1/state/lisp`. Changes go to the candidate; commit from the pending bar.
 */
export function LispTab() {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation('lisp');
  const [section, setSection] = useState<SectionId>('locators');
  const perms = usePermissions();
  const candidate = useCandidateLisp();
  const state = useLispState();
  const patch = usePatchTunnels();
  const schema = useMemo(() => sectionSchema(section), [section]);
  const value = useMemo(() => sectionValue(section, candidate.data), [section, candidate.data]);
  const rows = useMemo(
    () => statusRows(section, candidate.data, state.data ?? null),
    [section, candidate.data, state.data],
  );

  return (
    <Stack gap={2}>
      <Alert severity="info">{t('advanced')}</Alert>
      <Tabs
        value={section}
        onChange={(_, v: SectionId) => setSection(v)}
        aria-label={t('sections')}
        variant="scrollable"
      >
        {SECTIONS.map((s) => (
          <Tab
            key={s.id}
            value={s.id}
            label={t(`section.${s.id}`)}
            id={`lisp-tab-${s.id}`}
            aria-controls={`lisp-panel-${s.id}`}
          />
        ))}
      </Tabs>
      <Box role="tabpanel" id={`lisp-panel-${section}`} aria-labelledby={`lisp-tab-${section}`}>
        {state.isError && <ProblemAlert error={state.error} sx={{ mb: 2 }} />}
        <Typography component="h3" variant="subtitle1" sx={{ mb: 1 }}>
          {t('status.title')}
        </Typography>
        <ReactGeereedMaterial
          hideFilterBuilderComponent
          items={rows}
          columns={[
            {
              key: 'kind',
              headerRenderer: () => t('col.kind'),
              sortable: false,
              filterable: false,
              renderer: (_value, r: (typeof rows)[number]) => {
                return <>{t(`kind.${r.kind}`)}</>;
              },
            },
            {
              key: 'id',
              headerRenderer: () => t('col.id'),
              sortable: false,
              filterable: false,
              renderer: (_value, r: (typeof rows)[number]) => {
                return <>{r.id}</>;
              },
            },
            {
              key: 'detail',
              headerRenderer: () => t('col.detail'),
              sortable: false,
              filterable: false,
              renderer: (_value, r: (typeof rows)[number]) => {
                return <>{serviceText(r.detail)}</>;
              },
            },
            {
              key: 'status',
              headerRenderer: () => t('col.status'),
              sortable: false,
              filterable: false,
              renderer: (_value, r: (typeof rows)[number]) => {
                return (
                  <>
                    <StatusChip size="small" status={r.status} label={t(r.labelKey)} />
                  </>
                );
              },
            },
          ]}
          rowKey={(r) => `${r.kind}:${r.id}`}

          cellProps={(_item, column) => {
            if (column.key === 'id') return { dir: 'ltr' };
            if (column.key === 'detail') return { dir: 'ltr' };
            return {};
          }}
          tableProps={{
            size: 'small',
            'aria-label': t('status.title'),
            sx: { mb: 3 },
          }}

          emptyContent={
            <>
              {' '}
              <Typography variant="body2" color="text.secondary">
                {t('empty')}
              </Typography>{' '}
            </>
          }
          noFormWrapper
          embedded
          memoEnable={false}
          hidePagination
          hideToolbar
          footer={false}
          search={false}
          sortable={false}
          exportToCSV={false}
        />
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
            onSubmit={(v) =>
              patch
                .mutateAsync(sectionPatch(section, v as Record<string, unknown>))
                .then(() => undefined)
            }
          />
        )}
      </Box>
    </Stack>
  );
}

export default LispTab;
