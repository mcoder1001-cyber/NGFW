import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAlert as Alert,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { NS } from '../ospf/locale';
import type { TabKey } from './IsisRipPage';
interface Observed {
  unavailable: string | null;
  scope: 'all-vrfs' | 'default-vrf';
  total: number;
  rows: Record<string, string | number | boolean>[];
}
const ISIS_COLUMNS = ['vrf', 'systemId', 'interface', 'level', 'state'] as const;
const RIP_COLUMNS = ['address', 'badPackets', 'badRoutes', 'distance', 'lastUpdate'] as const;
/** Observed daemon state has a separate query and never populates candidate configuration. */
export function LiveState({ proto }: { proto: TabKey }) {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation(NS);
  const query = useQuery({
    queryKey: ['state', 'isis-rip', proto],
    refetchInterval: 5000,
    queryFn: async ({ signal }) => {
      // The integration generator produces these new fixed endpoint types.
      const request =
        proto === 'isis'
          ? api.GET(
              '/api/v1/state/routing/isis/adjacencies' as never,
              { signal, params: { query: { limit: 100 } } } as never,
            )
          : api.GET(
              '/api/v1/state/routing/rip/peers' as never,
              {
                signal,
                params: {
                  query: {
                    version: proto === 'ripng' ? 'ng' : '2',
                    limit: 100,
                  },
                },
              } as never,
            );
      return (await call(request)).data as unknown as Observed;
    },
  });
  const columns = proto === 'isis' ? ISIS_COLUMNS : RIP_COLUMNS;
  return (
    <Paper variant="outlined" sx={{ p: 2, mt: 3 }}>
      <Typography component="h3" variant="h6">
        {t('isisRip.live.title')}
      </Typography>
      <Typography color="text.secondary">{t('isisRip.live.observed')}</Typography>
      {query.isPending && <LinearProgress aria-label={t('isisRip.live.loading')} />}
      {query.isError && <ProblemAlert error={query.error} />}
      {query.data?.scope === 'default-vrf' && (
        <Alert severity="info">{t('isisRip.live.defaultVrf')}</Alert>
      )}
      {query.data?.unavailable && (
        <Alert severity="warning">{t(`isisRip.live.${query.data.unavailable}`)}</Alert>
      )}
      {query.isSuccess && !query.data.unavailable && (
        <>
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
            items={query.data.rows}
            tableProps={{
              size: 'small',
              'aria-label': t('isisRip.live.title'),
            }}
            rowKey={(row, i) => `${String(row['systemId'] ?? row['address'])}-${i}`}
            columns={columns.map((c) => ({
              key: c,
              sortable: false,
              filterable: false,
              headerRenderer: () => <>{t(`isisRip.live.${c}`)}</>,
              renderer: (_value: unknown, row: Observed['rows'][number]) => (
                <>{c === 'interface' ? interfaceLabel(String(row[c] ?? t('none'))) : String(row[c] ?? t('none'))}</>
              ),
            }))}
            cellProps={() => ({ dir: 'ltr' })}
            emptyContent={null}
          />
          {query.data.rows.length === 0 && <Typography>{t('isisRip.live.empty')}</Typography>}
          {query.data.total > query.data.rows.length && (
            <Alert severity="info">
              {t('isisRip.live.truncated', { count: query.data.total })}
            </Alert>
          )}
        </>
      )}
    </Paper>
  );
}
