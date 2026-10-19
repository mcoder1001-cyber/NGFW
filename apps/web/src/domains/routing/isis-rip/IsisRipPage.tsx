import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTab as Tab,
  PgTabs as Tabs,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { LiveState } from './LiveState';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { NS, registerIgpLocale } from '../ospf/locale';
import { ProtocolForm } from '../ospf/ProtocolForm';
import { useCandidateRouting, useRunningRouting } from '../ospf/queries';

registerIgpLocale();

const ISIS = 'isis' as const;
const RIP = 'rip' as const;
const RIPNG = 'ripng' as const;

export type TabKey = 'isis' | 'rip' | 'ripng';

function Interfaces({ proto }: { proto: TabKey }) {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation(NS);
  const cand = useCandidateRouting();
  const cfg = cand.data?.[proto];
  const rows = Object.entries(cfg?.interfaces ?? {}) as [
    string,
    { passive?: boolean; metric?: number; circuitType?: string },
  ][];
  const networks = proto !== 'isis' ? (cand.data?.[proto]?.networks ?? []) : [];
  return (
    <Paper variant="outlined" sx={{ p: 2, flex: 1, width: '100%' }}>
      <Typography component="h3" variant="h6" gutterBottom>
        {t('isisRip.interfaces')}
      </Typography>
      {rows.length === 0 ? (
        <Typography color="text.secondary">{t('isisRip.empty')}</Typography>
      ) : (
        (() => {
          const gridItems = rows;
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
                  headerRenderer: () => <>{t('isisRip.interface')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const { 0: name } = gridRow;
                    return <>{interfaceLabel(name)}</>;
                  },
                },
                {
                  key: '1',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('isisRip.passive')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const { 1: i } = gridRow;
                    return <>{i.passive ? t('yes') : t('no')}</>;
                  },
                },
                ...(proto === 'isis'
                  ? [
                      {
                        key: '2',
                        sortable: false,
                        filterable: false,
                        headerRenderer: () => <>{t('isisRip.metric')}</>,
                        renderer: (_value: unknown, gridRow: GridRow) => {
                          const { 1: i } = gridRow;
                          return <>{i.metric ?? t('none')}</>;
                        },
                      },
                    ]
                  : []),
                ...(proto === 'isis'
                  ? [
                      {
                        key: '3',
                        sortable: false,
                        filterable: false,
                        headerRenderer: () => <>{t('isisRip.circuit')}</>,
                        renderer: (_value: unknown, gridRow: GridRow) => {
                          const { 1: i } = gridRow;
                          return <>{i.circuitType ?? t('none')}</>;
                        },
                      },
                    ]
                  : []),
              ]}
              tableProps={{
                size: 'small',
                'aria-label': t('isisRip.interfaces'),
              }}
              rowKey={(nativeRow, _gridIndex) => {
                const gridRow = nativeRow as unknown as GridRow;
                const { 0: name } = gridRow;
                return name;
              }}
              rowProps={(nativeRow, _gridIndex) => {
                const gridRow = nativeRow as unknown as GridRow;
                const { 0: name } = gridRow;
                return { role: 'row', 'data-testid': `${proto}-if-${name}` };
              }}
              cellProps={(_nativeRow, column, _gridIndex) => {
                switch (column.key) {
                  case '0':
                    return { dir: 'ltr' };
                  case '1':
                    return {};
                  case '2':
                    return { dir: 'ltr' };
                  case '3':
                    return { dir: 'ltr' };
                  default:
                    return {};
                }
              }}
              emptyContent={null}
            />
          );
        })()
      )}
      {proto !== 'isis' && (
        <>
          <Typography component="h3" variant="h6" gutterBottom sx={{ mt: 3 }}>
            {t('isisRip.networks')}
          </Typography>
          <Typography dir="ltr" data-testid="rip-networks">
            {networks.join(', ') || t('none')}
          </Typography>
        </>
      )}
    </Paper>
  );
}

/**
 * Routing › IS-IS and RIP (WEB-4a, D-123): schema-driven `routing.isis` / `routing.rip` forms with interface summaries.
 * Routed at /routing/isis-rip by F-isis-rip (bounded live adjacency and peer state).
 */
export function IsisRipPage() {
  const { t } = useTranslation([NS, 'config']);
  const cand = useCandidateRouting();
  const running = useRunningRouting();
  const [tab, setTab] = useState<TabKey>('isis');
  return (
    <PageHeader title={t('isisRip.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('isisRip.intro')}
      </Typography>
      {(cand.isPending || running.isPending) && (
        <LinearProgress aria-label={t('config:loading')} sx={{ mb: 2 }} />
      )}
      {cand.isError && <ProblemAlert error={cand.error} sx={{ mb: 2 }} />}
      {running.isError && <ProblemAlert error={running.error} sx={{ mb: 2 }} />}
      <Tabs value={tab} onChange={(_, v: TabKey) => setTab(v)} sx={{ mb: 2 }}>
        <Tab value={ISIS} label={t('isisRip.tabs.isis')} />
        <Tab value={RIP} label={t('isisRip.tabs.rip')} />
        <Tab value={RIPNG} label={t('isisRip.tabs.ripng')} />
      </Tabs>
      <Stack key={tab} direction={{ xs: 'column', lg: 'row' }} spacing={3} alignItems="flex-start">
        {tab === 'rip' && (
          <Typography color="text.secondary">{t('isisRip.authBoundary')}</Typography>
        )}
        <ProtocolForm proto={tab} label={t(`isisRip.tabs.${tab}`)} />
        <Interfaces proto={tab} />
      </Stack>
      <LiveState proto={tab} />
    </PageHeader>
  );
}
