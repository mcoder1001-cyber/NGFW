import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgBox as Box,
  PgChip as Chip,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTab as Tab,
  PgTabs as Tabs,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { NS, registerIgpLocale } from '../ospf/locale';
import { ProtocolForm } from '../ospf/ProtocolForm';
import {
  REDISTRIBUTE_SOURCES,
  useCandidateRouting,
  useRunningRouting,
  type RoutingIgp,
} from '../ospf/queries';

registerIgpLocale();

const SESSIONS = 'sessions' as const;
const REDIST = 'redistribution' as const;
const BFD = 'bfd' as const;
const CHECK = '✓';
const TARGETS = ['bgp', 'ospf', 'isis', 'rip'] as const;
type Target = (typeof TARGETS)[number];

/** Redistribution matrix: for each target protocol, which sources it redistributes (null when it is not configured). */
export function redistributionMatrix(r: RoutingIgp): Record<Target, string[] | null> {
  const out = {} as Record<Target, string[] | null>;
  for (const p of TARGETS) {
    const cfg = r[p];
    out[p] = cfg ? REDISTRIBUTE_SOURCES.filter((s) => cfg.redistribute?.[s] !== undefined) : null;
  }
  return out;
}

function Sessions() {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation(NS);
  const cand = useCandidateRouting();
  const sessions = cand.data?.bfd?.sessions ?? [];
  return (
    <Paper variant="outlined" sx={{ p: 2, flex: 1, width: '100%' }}>
      <Typography component="h3" variant="h6" gutterBottom>
        {t('bfd.sessions')}
      </Typography>
      {sessions.length === 0 ? (
        <Typography color="text.secondary">{t('bfd.empty')}</Typography>
      ) : (
        <Box sx={{ overflowX: 'auto' }}>
          {(() => {
            const gridItems = sessions;
            type GridRow = (typeof gridItems)[number];
            return (
              <ReactGeereedMaterial
                hideFilterBuilderComponent
                noFormWrapper
                embedded
                memoEnable={false}
                hidePagination
                hideToolbar
                search={false}
                exportToCSV={false}
                sortable={false}
                footer={false}
                items={gridItems}
                columns={[
                  {
                    key: '0',
                    sortable: false,
                    filterable: false,
                    headerRenderer: () => <>{t('bfd.interface')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const s = gridRow;
                      return <>{interfaceLabel(s.interface)}</>;
                    },
                  },
                  {
                    key: '1',
                    sortable: false,
                    filterable: false,
                    headerRenderer: () => <>{t('bfd.local')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const s = gridRow;
                      return <>{s.localAddress}</>;
                    },
                  },
                  {
                    key: '2',
                    sortable: false,
                    filterable: false,
                    headerRenderer: () => <>{t('bfd.peer')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const s = gridRow;
                      return <>{s.peerAddress}</>;
                    },
                  },
                  {
                    key: '3',
                    sortable: false,
                    filterable: false,
                    headerRenderer: () => <>{t('bfd.timers')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const s = gridRow;
                      return (
                        <>{`${s.desiredMinTxUs ?? 300000} / ${s.requiredMinRxUs ?? 300000} × ${s.detectMultiplier ?? 3}`}</>
                      );
                    },
                  },
                  {
                    key: '4',
                    sortable: false,
                    filterable: false,
                    headerRenderer: () => <>{t('bfd.enabled')}</>,
                    renderer: (_value: unknown, gridRow: GridRow) => {
                      const s = gridRow;
                      return <>{s.enabled === false ? t('no') : t('yes')}</>;
                    },
                  },
                ]}
                tableProps={{ size: 'small', 'aria-label': t('bfd.sessions') }}
                rowKey={(nativeRow, _gridIndex) => {
                  const gridRow = nativeRow as unknown as GridRow;
                  const s = gridRow;
                  return `${s.interface}|${s.peerAddress}`;
                }}
                rowProps={(nativeRow, _gridIndex) => {
                  const gridRow = nativeRow as unknown as GridRow;
                  const s = gridRow;
                  return { role: 'row', 'data-testid': `bfd-${s.peerAddress}` };
                }}
                cellProps={(_nativeRow, column, _gridIndex) => {
                  switch (column.key) {
                    case '0':
                      return { dir: 'ltr' };
                    case '1':
                      return { dir: 'ltr' };
                    case '2':
                      return { dir: 'ltr' };
                    case '3':
                      return { dir: 'ltr' };
                    case '4':
                      return {};
                    default:
                      return {};
                  }
                }}
                emptyContent={null}
              />
            );
          })()}
        </Box>
      )}
    </Paper>
  );
}

function Redistribution() {
  const { t } = useTranslation(NS);
  const cand = useCandidateRouting();
  const m = redistributionMatrix(cand.data ?? {});
  return (
    <Paper variant="outlined" sx={{ p: 2, width: '100%' }}>
      <Typography component="h3" variant="h6" gutterBottom>
        {t('bfd.matrix')}
      </Typography>
      <Box sx={{ overflowX: 'auto' }}>
        <ReactGeereedMaterial
          hideFilterBuilderComponent
          noFormWrapper
          embedded
          memoEnable={false}
          hidePagination
          hideToolbar
          search={false}
          exportToCSV={false}
          sortable={false}
          footer={false}
          items={TARGETS.map((protocol) => ({ protocol }))}
          tableProps={{ size: 'small', 'aria-label': t('bfd.matrix') }}
          rowKey={(row) => row.protocol}
          rowProps={(row) => ({ role: 'row', 'data-testid': `redist-${row.protocol}` })}
          columns={[
            {
              key: 'target',
              sortable: false,
              filterable: false,
              headerRenderer: () => <>{t('bfd.into')}</>,
              renderer: (_value: unknown, row: { protocol: (typeof TARGETS)[number] }) => (
                <>{row.protocol}</>
              ),
            },
            ...REDISTRIBUTE_SOURCES.map((source, index) => ({
              key: source,
              sortable: false,
              filterable: false,
              headerRenderer: () => <bdi dir="ltr">{source}</bdi>,
              renderer: (_value: unknown, row: { protocol: (typeof TARGETS)[number] }) => (
                <>
                  {m[row.protocol] === null ? (
                    index === 0 ? (
                      <Chip size="small" label={t('bfd.notRunning')} />
                    ) : null
                  ) : source === row.protocol ? (
                    t('none')
                  ) : m[row.protocol]?.includes(source) ? (
                    CHECK
                  ) : null}
                </>
              ),
            })),
          ]}
          cellProps={(row, column) =>
            column.key === 'target'
              ? { dir: 'ltr' }
              : m[row.protocol as (typeof TARGETS)[number]] === null
                ? column.key === REDISTRIBUTE_SOURCES[0]
                  ? { colSpan: REDISTRIBUTE_SOURCES.length }
                  : null
                : {}
          }
          emptyContent={null}
        />
      </Box>
    </Paper>
  );
}

/**
 * Routing › BFD and redistribution (WEB-4a, D-123): schema-driven `routing.bfd` sessions and a read-only
 * redistribution matrix (edited in each protocol's form). Merged UNROUTED — F-bfd-redistribution adds the route/nav
 * entry and live session state.
 */
export function BfdPage() {
  const { t } = useTranslation([NS, 'config']);
  const cand = useCandidateRouting();
  const running = useRunningRouting();
  const [tab, setTab] = useState<'sessions' | 'redistribution'>('sessions');
  return (
    <PageHeader title={t('bfd.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('bfd.intro')}
      </Typography>
      {(cand.isPending || running.isPending) && (
        <LinearProgress aria-label={t('config:loading')} sx={{ mb: 2 }} />
      )}
      {cand.isError && <ProblemAlert error={cand.error} sx={{ mb: 2 }} />}
      {running.isError && <ProblemAlert error={running.error} sx={{ mb: 2 }} />}
      <Tabs
        value={tab}
        onChange={(_, v: 'sessions' | 'redistribution') => setTab(v)}
        sx={{ mb: 2 }}
      >
        <Tab value={SESSIONS} label={t('bfd.tabs.sessions')} />
        <Tab value={REDIST} label={t('bfd.tabs.redistribution')} />
      </Tabs>
      {tab === 'sessions' ? (
        <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3} alignItems="flex-start">
          <ProtocolForm proto={BFD} label={BFD.toUpperCase()} />
          <Sessions />
        </Stack>
      ) : (
        <Redistribution />
      )}
    </PageHeader>
  );
}
