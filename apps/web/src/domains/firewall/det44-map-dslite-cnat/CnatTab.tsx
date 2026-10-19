import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import {
  ReactGeereedMaterial,
  type GeereedTablePassthrough,
} from '@amnafzar/common-ui-lib/geereed';
import {
  PgBox as Box,
  PgDivider as Divider,
  PgPaper as Paper,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { NS, SUBTREES } from './model';
import { useCnatPurge, useCnatSessions } from './queries';
import { SubtreeForm } from './SubtreeForm';

const PAGE_SIZE = 100;

/** CNAT: the schema-driven `nat.cnat` form, the paged session table and the (admin) purge. */
export function CnatTab() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const [page, setPage] = useState(0);
  const q = useCnatSessions(page, PAGE_SIZE);
  const purge = useCnatPurge();
  const pages = q.data ? Math.max(1, Math.ceil(q.data.total / PAGE_SIZE)) : 1;
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('cnat.intro')}
      </Typography>
      <SubtreeForm subtree={SUBTREES.cnat} />
      <Divider sx={{ my: 3 }} />
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
        <Typography variant="h6" component="h3" sx={{ flexGrow: 1 }}>
          {t('cnat.sessions', { total: q.data?.total ?? 0 })}
        </Typography>
        <Button disabled={page === 0} onClick={() => setPage(page - 1)}>
          {t('prev')}
        </Button>
        <Typography>{t('page', { page: page + 1, pages })}</Typography>
        <Button disabled={page + 1 >= pages} onClick={() => setPage(page + 1)}>
          {t('next')}
        </Button>
        {perms.manageUsers && (
          <Button
            color="warning"
            variant="outlined"
            disabled={purge.isPending}
            onClick={() => purge.mutate()}
          >
            {t('cnat.purge')}
          </Button>
        )}
      </Box>
      {q.isError && <ProblemAlert error={q.error} sx={{ mb: 1 }} />}
      {purge.isError && <ProblemAlert error={purge.error} sx={{ mb: 1 }} />}
      <Paper variant="outlined">
        <ReactGeereedMaterial
          tableProps={{
            size: 'small',
            'aria-label': t('cnat.sessions', { total: q.data?.total ?? 0 }),
          }}
          columns={[
            {
              key: 'dst',
              headerRenderer: () => t('col.dst'),
              filterable: false,
              renderer: (value) => <>{value}</>,
            },
            {
              key: 'src',
              headerRenderer: () => t('col.src'),
              filterable: false,
              renderer: (value) => <>{value}</>,
            },
            {
              key: 'protocol',
              headerRenderer: () => t('col.protocol'),
              filterable: false,
              renderer: (value) => <>{value}</>,
            },
          ]}
          items={[
            ...(q.data?.items.map((s, i) => ({
              id: `${s.dstAddress}:${s.dstPort}|${s.srcAddress}:${s.srcPort}|${i}`,
              dst: `${s.dstAddress}:${s.dstPort}`,
              src: `${s.srcAddress}:${s.srcPort}`,
              protocol: s.protocol,
              cellProps: { dst: { dir: 'ltr' }, src: { dir: 'ltr' } } satisfies Record<
                string,
                NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
              >,
            })) ?? []),
          ]}
          rowKey={(item) => item.id}
          rowProps={(item) => item.rowProps ?? {}}
          cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
          emptyContent={q.data?.items.length === 0 ? t('empty') : null}
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
