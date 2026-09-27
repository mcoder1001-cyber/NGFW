import DownloadIcon from '@mui/icons-material/Download';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
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
        <Button startIcon={<DownloadIcon />} onClick={() => void downloadAll()} disabled={modules.length === 0}>
          {t('downloadAll')}
        </Button>
      </Stack>

      {q.isPending && <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />}
      {q.isError && <ProblemAlert error={q.error} sx={{ mb: 1 }} />}
      {q.data && modules.length === 0 && <Alert severity="info">{t('empty')}</Alert>}

      {modules.length > 0 && (
        <TableContainer component={Paper} variant="outlined" sx={{ maxInlineSize: 900 }}>
          <Table size="small" aria-label={t('modulesTitle')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('col.name')}</TableCell>
                <TableCell>{t('col.namespace')}</TableCell>
                <TableCell>{t('col.revision')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }} />
              </TableRow>
            </TableHead>
            <TableBody>
              {modules.map((m) => (
                <TableRow key={m.name}>
                  <TableCell dir="ltr">{m.name}</TableCell>
                  <TableCell dir="ltr">{m.namespace}</TableCell>
                  <TableCell dir="ltr">{m.revision}</TableCell>
                  <TableCell sx={{ textAlign: 'end' }}>
                    <Button size="small" startIcon={<DownloadIcon />} onClick={() => void download(m.name)}>
                      {t('download')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );
}
