import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import { PgUploadFileIcon as UploadFileIcon } from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgDialog as Dialog,
  PgDialogActions as DialogActions,
  PgDialogContent as DialogContent,
  PgDialogContentText as DialogContentText,
  PgDialogTitle as DialogTitle,
  PgFormControlLabel as FormControlLabel,
  PgLinearProgress as LinearProgress,
  PgRadio as Radio,
  PgRadioGroup as RadioGroup,
  PgStack as Stack,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import type { AddressMatch, ServiceMatch } from '@ngfw/schema';
import { useFormatters } from '@ngfw/ui-kit';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { serviceText as productText } from '../../../product-text';
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
          <ReactGeereedMaterial
            tableProps={{ size: 'small', 'aria-label': t('import.errors') }}
            columns={[
              {
                key: 'column0',
                headerRenderer: () => t('import.line'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'column1',
                headerRenderer: () => t('import.column'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
              {
                key: 'column2',
                headerRenderer: () => t('import.message'),
                filterable: false,
                renderer: (value) => <>{value}</>,
              },
            ]}
            items={[
              ...(result.errors.map((e, i) => ({
                id: i,
                column0: e.line !== undefined ? n(e.line) : '',
                column1: e.column ? <Mono>{e.column}</Mono> : null,
                column2: (
                  <>
                    <bdi>{productText(e.message)}</bdi>
                  </>
                ),
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
          <bdi>{productText(w.message)}</bdi>
        </Alert>
      ))}
      {result.preview.length > 0 && (
        <>
          <Typography variant="subtitle2">
            {t('import.preview', { count: result.preview.length, n: n(result.preview.length) })}
          </Typography>
          <Box sx={{ maxBlockSize: 260, overflow: 'auto' }}>
            <ReactGeereedMaterial
              tableProps={{ size: 'small', 'aria-label': t('import.previewLabel') }}
              columns={[
                {
                  key: 'sequence',
                  headerRenderer: () => t('col.sequence'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'action',
                  headerRenderer: () => t('col.action'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'source',
                  headerRenderer: () => t('col.source'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'destination',
                  headerRenderer: () => t('col.destination'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'service',
                  headerRenderer: () => t('col.service'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'description',
                  headerRenderer: () => t('col.description'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
              ]}
              items={[
                ...(result.preview.map((r, i) => ({
                  id: i,
                  sequence:
                    typeof r['sequence'] === 'number'
                      ? fmt.number(r['sequence'], { useGrouping: false })
                      : '',
                  action: t(`enum.action.${String(r['action'])}`, {
                    defaultValue: String(r['action'] ?? ''),
                  }),
                  source: (
                    <>
                      <Mono>
                        {addressText(r['source'] as AddressMatch | undefined) ?? t('match.any')}
                      </Mono>
                    </>
                  ),
                  destination: (
                    <>
                      <Mono>
                        {addressText(r['destination'] as AddressMatch | undefined) ??
                          t('match.any')}
                      </Mono>
                    </>
                  ),
                  service: (
                    <>
                      <Mono>
                        {serviceText(r['service'] as ServiceMatch | undefined) ?? t('match.any')}
                      </Mono>
                    </>
                  ),
                  description: typeof r['description'] === 'string' ? r['description'] : '',
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
          </Box>
        </>
      )}
    </Stack>
  );
}
