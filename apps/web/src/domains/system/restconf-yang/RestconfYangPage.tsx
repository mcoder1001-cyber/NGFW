import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import {
  ReactGeereedMaterial,
  type GeereedTablePassthrough,
} from '@amnafzar/common-ui-lib/geereed';
import { PgDownloadIcon as DownloadIcon } from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTableContainer as TableContainer,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { fetchYangModule, useYangModules } from './queries';

const RESTCONF_ROOT = '/restconf';

function saveText(name: string, text: string): void {
  const blob = new Blob([text], { type: 'application/yang' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${name}.yang`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/** F-restconf-yang: `/system/restconf` — a link/download card for the generated YANG modules and the RESTCONF root. */
export function RestconfYangPage() {
  const { t } = useTranslation('restconf-yang');
  const q = useYangModules();
  const modules = q.data?.modules ?? [];

  const download = async (name: string) => {
    const m = await fetchYangModule(name);
    saveText(m.name, m.yang);
  };
  const downloadAll = async () => {
    for (const m of modules) await download(m.name);
  };

  return (
    <Box>
      <PageHeader title={t('title')}>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2, maxInlineSize: 900 }}>
          {t('intro')}
        </Typography>
      </PageHeader>

      <Paper variant="outlined" sx={{ p: 2, mb: 3, maxInlineSize: 900 }}>
        <Typography component="h3" variant="subtitle2" sx={{ mb: 0.5 }}>
          {t('endpointTitle')}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {t('endpointHelp')}
        </Typography>
        <Typography variant="body2" component="code" dir="ltr" sx={{ display: 'block', mt: 0.5 }}>
          {RESTCONF_ROOT}
        </Typography>
      </Paper>

      <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
        <Typography component="h3" variant="subtitle1" sx={{ flex: 1 }}>
          {t('modulesTitle')}
        </Typography>
        <Button
          startIcon={<DownloadIcon />}
          onClick={() => void downloadAll()}
          disabled={modules.length === 0}
        >
          {t('downloadAll')}
        </Button>
      </Stack>

      {q.isPending && <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />}
      {q.isError && <ProblemAlert error={q.error} sx={{ mb: 1 }} />}
      {q.data && modules.length === 0 && <Alert severity="info">{t('empty')}</Alert>}

      {modules.length > 0 && (
        <TableContainer component={Paper} variant="outlined" sx={{ maxInlineSize: 900 }}>
          <ReactGeereedMaterial
            tableProps={{ size: 'small', 'aria-label': t('modulesTitle') }}
            columns={[
              {
                key: 'name',
                headerRenderer: () => t('col.name'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'namespace',
                headerRenderer: () => t('col.namespace'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'revision',
                headerRenderer: () => t('col.revision'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'column3',
                headerRenderer: () => null,
                headerCellProps: { sx: { textAlign: 'end' } },
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
            ]}
            items={[
              ...(modules.map((m) => ({
                id: m.name,
                name: m.name,
                namespace: m.namespace,
                revision: m.revision,
                column3: (
                  <>
                    <Button
                      size="small"
                      startIcon={<DownloadIcon />}
                      onClick={() => void download(m.name)}
                    >
                      {t('download')}
                    </Button>
                  </>
                ),
                cellProps: {
                  name: { dir: 'ltr' },
                  namespace: { dir: 'ltr' },
                  revision: { dir: 'ltr' },
                  column3: { sx: { textAlign: 'end' } },
                } satisfies Record<
                  string,
                  NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                >,
              })) ?? []),
            ]}
            rowKey={(item) => item.id}
            rowProps={(item) => item.rowProps ?? {}}
            cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
            emptyContent={null}
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
        </TableContainer>
      )}
    </Box>
  );
}
