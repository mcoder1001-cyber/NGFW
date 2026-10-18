import {
  ReactGeereedMaterial,
  type GeereedTablePassthrough,
} from '@amnafzar/common-ui-lib/geereed';
import {
  PgBox as Box,
  PgChip as Chip,
  PgDivider as Divider,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgTooltip as Tooltip,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import type { Subtree } from './model';
import { NS } from './model';
import { useNptv6State } from './queries';
import { SubtreeForm } from './SubtreeForm';

const SUBTREE: Subtree = 'nptv6';

/**
 * NPTv6 (RFC 6296): the schema-driven `nat.nptv6` form, then the running bindings from `GET /state/nat/nptv6` with
 * their status: write-only (VPP 26.06 has no npt66 dump; the agent re-applies every binding on each resync).
 */
export function Nptv6Tab() {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation(NS);
  const state = useNptv6State();
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('nptv6.intro')}
      </Typography>
      <SubtreeForm subtree={SUBTREE} />
      <Divider sx={{ my: 3 }} />
      <Typography variant="h6" component="h3" sx={{ mb: 1 }}>
        {t('nptv6.running')}
      </Typography>
      {state.isPending && <LinearProgress aria-label={t('loading')} />}
      {state.isError && <ProblemAlert error={state.error} sx={{ mb: 1 }} />}
      {state.isSuccess && (
        <Paper variant="outlined">
          <ReactGeereedMaterial
            tableProps={{ size: 'small', 'aria-label': t('nptv6.running') }}
            columns={[
              {
                key: 'interface',
                headerRenderer: () => t('field.interface.title'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'internal',
                headerRenderer: () => t('field.internal.title'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'external',
                headerRenderer: () => t('field.external.title'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'description',
                headerRenderer: () => t('field.description.title'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'status',
                headerRenderer: () => t('col.status'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
            ]}
            items={[
              ...(state.data.bindings.map((b) => ({
                id: `${b.interface}|${b.internal}`,
                interface: b.interface,
                internal: b.internal,
                external: b.external,
                description: b.description ?? '',
                status: (
                  <>
                    <Tooltip title={t('nptv6.writeOnlyHelp')}>
                      <Chip
                        size="small"
                        variant="outlined"
                        color="info"
                        label={t('status.writeOnly')}
                      />
                    </Tooltip>
                  </>
                ),
                cellProps: {
                  interface: { dir: 'ltr' },
                  internal: { dir: 'ltr' },
                  external: { dir: 'ltr' },
                } satisfies Record<
                  string,
                  NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                >,
              })) ?? []),
            ]}
            rowKey={(item) => item.id}
            rowProps={(item) => item.rowProps ?? {}}
            cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
            emptyContent={state.data.bindings.length === 0 ? t('nptv6.empty') : null}
            noFormWrapper
            embedded
            hideToolbar
            hidePagination
            hideFilterBuilderComponent
            search={false}
            sortable={false}
            exportToCSV={false}
            footer={false}
            memoEnable={false}
          />
        </Paper>
      )}
    </Box>
  );
}
