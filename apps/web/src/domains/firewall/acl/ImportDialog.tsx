import UploadFileIcon from '@mui/icons-material/UploadFile';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import FormControlLabel from '@mui/material/FormControlLabel';
import LinearProgress from '@mui/material/LinearProgress';
import Radio from '@mui/material/Radio';
import RadioGroup from '@mui/material/RadioGroup';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import type { AddressMatch, ServiceMatch } from '@ngfw/schema';
import { useFormatters } from '@ngfw/ui-kit';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { addressText, serviceText, type ImportResult } from './model';
import { Mono } from './parts';
import { importCsv, invalidateAcl } from './queries';

const MODES = ['replace', 'append'] as const;
type Mode = (typeof MODES)[number];
const CSV_ACCEPT = '.csv,text/csv';

/** FileReader works in every browser (and in jsdom, which has no Blob.text()). */
export function readText(file: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(typeof r.result === 'string' ? r.result : '');
    r.onerror = () => reject(r.error ?? new Error('read failed'));
    r.readAsText(file);
  });
}

/**
 * CSV import into a list of the candidate: choose a file and a mode, a dry run validates and previews it (counts,
 * errors with line/column, the first rules), then "Import" posts the same file with `dryRun=false`.
 */
export function ImportDialog({
  list,
  open,
  onClose,
}: {
  list: string;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation('acl');
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="md">
      <DialogTitle>{t('import.title', { list })}</DialogTitle>
      {open && <ImportBody list={list} onClose={onClose} />}
    </Dialog>
  );
}

function ImportBody({ list, onClose }: { list: string; onClose: () => void }) {
  const { t } = useTranslation('acl');
  const fmt = useFormatters();
  const qc = useQueryClient();
  const [file, setFile] = useState<{ name: string; text: string } | null>(null);
  const [mode, setMode] = useState<Mode>('replace');
  const [check, setCheck] = useState<ImportResult | null>(null);
  const [done, setDone] = useState<ImportResult | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const reset = () => {
    setCheck(null);
    setDone(null);
    setError(null);
  };
  const pick = async (f: File | undefined) => {
    reset();
    setFile(f ? { name: f.name, text: await readText(f) } : null);
  };
  const run = async (dryRun: boolean) => {
    if (!file) return;
    setBusy(true);
    setError(null);
    try {
      const r = await importCsv(list, file.text, mode, dryRun);
      if (dryRun) setCheck(r);
      else {
        setDone(r);
        await invalidateAcl(qc);
      }
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };

  const n = (v: number) => fmt.integer(v);
  return (
    <>
      <DialogContent>
        <DialogContentText sx={{ mb: 2 }}>{t('import.help')}</DialogContentText>
        <Stack direction="row" gap={2} alignItems="center" flexWrap="wrap" sx={{ mb: 2 }}>
          <Button
            component="label"
            variant="outlined"
            startIcon={<UploadFileIcon />}
            disabled={busy}
          >
            {t('import.choose')}
            <input
              hidden
              type="file"
              accept={CSV_ACCEPT}
              onChange={(e) => void pick(e.target.files?.[0])}
            />
          </Button>
          <Typography variant="body2" color="text.secondary">
            {file ? <bdi>{file.name}</bdi> : t('import.noFile')}
          </Typography>
        </Stack>
        <RadioGroup
          row
          value={mode}
          onChange={(e) => {
            setMode(e.target.value as Mode);
            reset();
          }}
          aria-label={t('import.mode')}
          sx={{ mb: 1 }}
        >
          {MODES.map((m) => (
            <FormControlLabel
              key={m}
              value={m}
              control={<Radio size="small" />}
              label={t(`import.modes.${m}`)}
            />
          ))}
        </RadioGroup>
        {busy && <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />}
        {error !== null && <ProblemAlert error={error} sx={{ mb: 1 }} />}
        {done && (
          <Alert severity="success" sx={{ mb: 1 }}>
            {t('import.done', { imported: n(done.imported), total: n(done.total) })}
          </Alert>
        )}
        {check && !done && <CheckResult result={check} />}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{done ? t('close') : t('cancel')}</Button>
        {!done && (
          <Button variant="outlined" disabled={!file || busy} onClick={() => void run(true)}>
            {t('import.check')}
          </Button>
        )}
        {!done && (
          <Button
            variant="contained"
            disabled={!file || busy || !check || check.errorCount > 0 || check.valid === 0}
            onClick={() => void run(false)}
          >
            {t('import.run')}
          </Button>
        )}
      </DialogActions>
    </>
  );
}

function CheckResult({ result }: { result: ImportResult }) {
  const { t } = useTranslation('acl');
  const fmt = useFormatters();
  const n = (v: number) => fmt.integer(v);
  return (
    <Stack gap={1} role="region" aria-label={t('import.resultLabel')}>
      <Alert severity={result.errorCount > 0 ? 'error' : 'success'}>
        {t('import.summary', {
          rows: n(result.rows),
          valid: n(result.valid),
          errors: n(result.errorCount),
        })}{' '}
        {result.mode === 'replace'
          ? t('import.afterReplace', { existing: n(result.existingRules), total: n(result.total) })
          : t('import.afterAppend', { existing: n(result.existingRules), total: n(result.total) })}
      </Alert>
      {result.errors.length > 0 && (
        <Box sx={{ maxBlockSize: 220, overflow: 'auto' }}>
          <Table size="small" aria-label={t('import.errors')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('import.line')}</TableCell>
                <TableCell>{t('import.column')}</TableCell>
                <TableCell>{t('import.message')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {result.errors.map((e, i) => (
                <TableRow key={i}>
                  <TableCell>{e.line !== undefined ? n(e.line) : ''}</TableCell>
                  <TableCell>{e.column ? <Mono>{e.column}</Mono> : null}</TableCell>
                  <TableCell>
                    <bdi>{e.message}</bdi>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {result.errorCount > result.errors.length && (
            <Typography variant="caption" color="text.secondary">
              {t('import.moreErrors', {
                shown: n(result.errors.length),
                total: n(result.errorCount),
              })}
            </Typography>
          )}
        </Box>
      )}
      {result.warnings.map((w, i) => (
        <Alert key={i} severity="warning">
          {w.line !== undefined ? `${t('import.lineNo', { line: n(w.line) })} ` : ''}
          <bdi>{w.message}</bdi>
        </Alert>
      ))}
      {result.preview.length > 0 && (
        <>
          <Typography variant="subtitle2">
            {t('import.preview', { count: result.preview.length, n: n(result.preview.length) })}
          </Typography>
          <Box sx={{ maxBlockSize: 260, overflow: 'auto' }}>
            <Table size="small" aria-label={t('import.previewLabel')}>
              <TableHead>
                <TableRow>
                  <TableCell>{t('col.sequence')}</TableCell>
                  <TableCell>{t('col.action')}</TableCell>
                  <TableCell>{t('col.source')}</TableCell>
                  <TableCell>{t('col.destination')}</TableCell>
                  <TableCell>{t('col.service')}</TableCell>
                  <TableCell>{t('col.description')}</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {result.preview.map((r, i) => (
                  <TableRow key={i}>
                    <TableCell>
                      {typeof r['sequence'] === 'number'
                        ? fmt.number(r['sequence'], { useGrouping: false })
                        : ''}
                    </TableCell>
                    <TableCell>
                      {t(`enum.action.${String(r['action'])}`, {
                        defaultValue: String(r['action'] ?? ''),
                      })}
                    </TableCell>
                    <TableCell>
                      <Mono>
                        {addressText(r['source'] as AddressMatch | undefined) ?? t('match.any')}
                      </Mono>
                    </TableCell>
                    <TableCell>
                      <Mono>
                        {addressText(r['destination'] as AddressMatch | undefined) ??
                          t('match.any')}
                      </Mono>
                    </TableCell>
                    <TableCell>
                      <Mono>
                        {serviceText(r['service'] as ServiceMatch | undefined) ?? t('match.any')}
                      </Mono>
                    </TableCell>
                    <TableCell>
                      {typeof r['description'] === 'string' ? r['description'] : ''}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Box>
        </>
      )}
    </Stack>
  );
}
