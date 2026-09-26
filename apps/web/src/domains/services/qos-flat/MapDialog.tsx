import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Stack from '@mui/material/Stack';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import type { Theme } from '@mui/material/styles';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import {
  cellsOf,
  esc,
  gridsFromMap,
  issuesUnder,
  parseCell,
  QOS_SOURCE_MAX,
  QOS_SOURCES,
  rowsFromGrids,
  type Grids,
  type MapCfg,
  type QosSource,
} from './model';
import { NAME_RE } from './SchemaDialog';

const MONO = { fontFamily: (th: Theme) => th.vrx.monoFontFamily, fontSize: 12 } as const;
const LTR = { dir: 'ltr' } as const;
const NUMERIC_LTR = { dir: 'ltr', inputMode: 'numeric' } as const;
const CELL_INPUT = { dir: 'ltr', inputMode: 'numeric', style: { textAlign: 'center' as const, padding: 2 } };

export interface MapTarget {
  name: string;
  cfg: MapCfg | undefined;
  editing: boolean;
}

/**
 * The marking map editor: name, optional fixed id, description and, per recorded source, a compact grid of every
 * recorded value (DSCP 0–63, PCP / EXP 0–7, ext 0–255) whose cell is the output value (empty = not listed: VPP writes
 * 0). Saved as the schema's `rows.<source>: [{from, to}]`, one entry per filled cell, sorted by `from`.
 */
export function MapDialog({
  target,
  existing,
  error,
  pending,
  onCancel,
  onSubmit,
}: {
  target: MapTarget | null;
  existing: readonly string[];
  error: unknown;
  pending: boolean;
  onCancel: () => void;
  onSubmit: (name: string, value: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation('qos-flat');
  return (
    <Dialog open={target !== null} onClose={onCancel} fullWidth maxWidth="lg">
      <DialogTitle>
        {target?.editing ? t('maps.editTitle', { name: target.name }) : t('maps.addTitle')}
      </DialogTitle>
      {target && (
        <Body
          key={target.name}
          target={target}
          existing={existing}
          error={error}
          pending={pending}
          onCancel={onCancel}
          onSubmit={onSubmit}
        />
      )}
    </Dialog>
  );
}

function Body({
  target,
  existing,
  error,
  pending,
  onCancel,
  onSubmit,
}: {
  target: MapTarget;
  existing: readonly string[];
  error: unknown;
  pending: boolean;
  onCancel: () => void;
  onSubmit: (name: string, value: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation('qos-flat');
  const [name, setName] = useState(target.name);
  const [id, setId] = useState(target.cfg?.id !== undefined ? String(target.cfg.id) : '');
  const [description, setDescription] = useState(target.cfg?.description ?? '');
  const [grids, setGrids] = useState<Grids>(() => gridsFromMap(target.cfg));
  const [texts, setTexts] = useState<Record<string, string>>({});
  const [source, setSource] = useState<QosSource>('ip');
  const taken = !target.editing && existing.includes(name);
  const nameOk = NAME_RE.test(name) && !taken;
  const idOk = id === '' || /^\d{1,10}$/.test(id);
  const invalid = Object.keys(texts).filter((k) => parseCell(texts[k] ?? '') === undefined);
  const rows = rowsFromGrids(grids);
  const listed = Object.values(rows).reduce((n, r) => n + (r?.length ?? 0), 0);
  const prefix = `/services/qos/maps/${esc(name)}`;
  const serverIssues = issuesUnder(error, prefix);

  const setCell = (s: QosSource, i: number, text: string) => {
    const key = `${s}:${i}`;
    setTexts((cur) => ({ ...cur, [key]: text }));
    const v = parseCell(text);
    if (v !== undefined) {
      setGrids((g) => ({ ...g, [s]: g[s].map((x, j) => (j === i ? v : x)) }));
    }
  };
  const cellText = (s: QosSource, i: number) => {
    const typed = texts[`${s}:${i}`];
    if (typed !== undefined) return typed;
    const v = grids[s][i];
    return v === null || v === undefined ? '' : String(v);
  };
  const submit = () => {
    const value: Record<string, unknown> = { rows };
    if (id !== '') value['id'] = Number(id);
    if (description.trim()) value['description'] = description.trim();
    onSubmit(name, value);
  };
  const cells = cellsOf(source);
  const cols = source === 'ip' ? 16 : source === 'ext' ? 16 : 8;
  return (
    <>
      <DialogContent>
        <Stack gap={2} sx={{ pt: 1 }}>
          <Stack direction="row" gap={2} flexWrap="wrap">
            <TextField
              label={t('dialog.name')}
              value={name}
              disabled={target.editing}
              onChange={(e) => setName(e.target.value)}
              error={name !== '' && !nameOk}
              helperText={taken ? t('dialog.nameTaken', { name }) : t('dialog.nameHelp')}
              slotProps={{ htmlInput: LTR }}
              sx={{ flex: 1, minInlineSize: 200 }}
            />
            <TextField
              label={t('maps.id')}
              value={id}
              onChange={(e) => setId(e.target.value.trim())}
              error={!idOk}
              helperText={t('maps.idHelp')}
              slotProps={{ htmlInput: NUMERIC_LTR }}
              sx={{ inlineSize: 220 }}
            />
            <TextField
              label={t('maps.description')}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              sx={{ flex: 2, minInlineSize: 240 }}
            />
          </Stack>
          <Typography variant="body2" color="text.secondary">
            {t('maps.gridHelp')}
          </Typography>
          <Tabs
            value={source}
            onChange={(_, s: QosSource) => setSource(s)}
            aria-label={t('maps.sources')}
            variant="scrollable"
          >
            {QOS_SOURCES.map((s) => (
              <Tab
                key={s}
                value={s}
                label={t('maps.sourceTab', {
                  source: t(`source.${s}`),
                  n: rows[s]?.length ?? 0,
                })}
              />
            ))}
          </Tabs>
          <Typography variant="caption" color="text.secondary">
            {t('maps.sourceRange', { source: t(`source.${source}`), max: QOS_SOURCE_MAX[source] })}
          </Typography>
          <Box
            role="grid"
            aria-label={t('maps.gridLabel', { source: t(`source.${source}`) })}
            dir="ltr"
            sx={{
              display: 'grid',
              gridTemplateColumns: `repeat(${cols}, minmax(44px, 1fr))`,
              gap: 0.5,
              maxBlockSize: 420,
              overflowY: 'auto',
            }}
          >
            {Array.from({ length: cells }, (_, i) => {
              const bad = invalid.includes(`${source}:${i}`);
              return (
                <Box key={i} role="gridcell" sx={{ textAlign: 'center' }}>
                  <Typography component="div" sx={{ ...MONO, color: 'text.secondary' }}>
                    {i}
                  </Typography>
                  <TextField
                    size="small"
                    value={cellText(source, i)}
                    error={bad}
                    onChange={(e) => setCell(source, i, e.target.value)}
                    slotProps={{
                      htmlInput: {
                        ...CELL_INPUT,
                        'aria-label': t('maps.cellLabel', { source: t(`source.${source}`), value: i }),
                      },
                    }}
                  />
                </Box>
              );
            })}
          </Box>
          {invalid.length > 0 && <Alert severity="warning">{t('maps.cellInvalid')}</Alert>}
          {serverIssues.length > 0 ? (
            <Alert severity="error">
              {serverIssues.map((e) => (
                <Typography key={`${e.pointer}:${e.message}`} variant="body2" dir="ltr">
                  {e.pointer.slice(prefix.length) || '/'} — {e.message}
                </Typography>
              ))}
            </Alert>
          ) : (
            error !== null && error !== undefined && <ProblemAlert error={error} />
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Typography variant="body2" color="text.secondary" sx={{ flex: 1, paddingInlineStart: 2 }}>
          {t('maps.listed', { n: listed })}
        </Typography>
        <Button onClick={onCancel}>{t('dialog.cancel')}</Button>
        <Button
          variant="contained"
          disabled={!nameOk || !idOk || invalid.length > 0 || pending}
          onClick={submit}
        >
          {t('dialog.save')}
        </Button>
      </DialogActions>
    </>
  );
}
