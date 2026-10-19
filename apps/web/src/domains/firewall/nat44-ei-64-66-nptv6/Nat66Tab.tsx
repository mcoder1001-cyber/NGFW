import {
  ReactGeereedMaterial,
  type GeereedTablePassthrough,
} from '@amnafzar/common-ui-lib/geereed';
import {
  PgBox as Box,
  PgChip as Chip,
  PgDivider as Divider,
  PgPaper as Paper,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useTranslation } from 'react-i18next';
import { DEFAULT_VRF } from '../nat44-ed-sessions/model';
import { useCandidateNat } from '../nat44-ed-sessions/queries';
import type { Subtree } from './model';
import { driftUnder, NS } from './model';
import { useDrift } from './queries';
import { SubtreeForm } from './SubtreeForm';

const SUBTREE: Subtree = 'nat66';

type Item = Record<string, unknown>;

/** Live status of the applied NAT66 configuration: the drift between running and what the agent retrieves. */
function DriftChip() {
  const { t } = useTranslation(NS);
  const drift = useDrift();
  if (drift.isPending) return <Chip size="small" label={t('loading')} />;
  if (drift.isError) return <Chip size="small" color="default" label={t('status.unknown')} />;
  const n = driftUnder(drift.data, '/nat/nat66').length;
  return n === 0 ? (
    <Chip size="small" color="success" label={t('status.applied')} />
  ) : (
    <Chip size="small" color="warning" label={t('status.drift', { count: n })} />
  );
}

/**
 * NAT66 (stateless 1:1 IPv6): the schema-driven `nat.nat66` form, then the static mappings of the candidate with the
 * live status column (running vs retrieved, `GET /state/drift`).
 */
export function Nat66Tab() {
  const { t } = useTranslation(NS);
  const candidate = useCandidateNat();
  const nat66 = (candidate.data?.['nat66'] ?? {}) as Item;
  const maps = (Array.isArray(nat66['staticMappings']) ? nat66['staticMappings'] : []) as Item[];
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('nat66.intro')}
      </Typography>
      <SubtreeForm subtree={SUBTREE} />
      <Divider sx={{ my: 3 }} />
      <Typography variant="h6" component="h3" sx={{ mb: 1 }}>
        {t('nat66.mappings')}
      </Typography>
      <Paper variant="outlined">
        <ReactGeereedMaterial
          tableProps={{ size: 'small', 'aria-label': t('nat66.mappings') }}
          columns={[
            {
              key: 'local',
              headerRenderer: () => t('field.local.title'),
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
              key: 'vrf',
              headerRenderer: () => t('field.vrf.title'),
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
            ...(maps.map((m, i) => ({
              id: `${String(m['local'])}|${i}`,
              local: String(m['local'] ?? ''),
              external: String(m['external'] ?? ''),
              vrf: typeof m['vrf'] === 'string' ? m['vrf'] : DEFAULT_VRF,
              status: (
                <>
                  <DriftChip />
                </>
              ),
              cellProps: { local: { dir: 'ltr' }, external: { dir: 'ltr' } } satisfies Record<
                string,
                NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
              >,
            })) ?? []),
          ]}
          rowKey={(item) => item.id}
          rowProps={(item) => item.rowProps ?? {}}
          cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
          emptyContent={candidate.isSuccess && maps.length === 0 ? t('nat66.empty') : null}
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
    </Box>
  );
}
