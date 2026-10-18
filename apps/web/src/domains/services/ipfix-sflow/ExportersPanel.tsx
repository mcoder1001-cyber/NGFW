import { PgButton as Button, PgText as TextField } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAddIcon as AddIcon,
  PgDeleteIcon as DeleteIcon,
  PgEditIcon as EditIcon,
} from '@amnafzar/common-ui-lib/icons';
import {
  PgChip as Chip,
  PgDialog as Dialog,
  PgDialogActions as DialogActions,
  PgDialogContent as DialogContent,
  PgDialogTitle as DialogTitle,
  PgFormControlLabel as FormControlLabel,
  PgIconButton as IconButton,
  PgStack as Stack,
  PgSwitch as Switch,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { NS, type PanelProps } from './FlowExportTab';
import type { ExporterConfig } from './queries';

interface Draft {
  name: string;
  address: string;
  port: string;
  source: string;
  vrf: string;
  pathMtu: string;
  template: string;
  enabled: boolean;
  description: string;
}

const DEFAULT_VRF = 'default';
const DRAFT_KEYS: (keyof Draft)[] = [
  'name',
  'address',
  'port',
  'source',
  'vrf',
  'pathMtu',
  'template',
  'enabled',
  'description',
];

const draftOf = (name: string, e?: ExporterConfig): Draft => ({
  name,
  address: e?.collector.address ?? '',
  port: String(e?.collector.port ?? 4739),
  source: e?.sourceAddress ?? '',
  vrf: e?.vrf ?? 'default',
  pathMtu: String(e?.pathMtu ?? 512),
  template: String(e?.templateIntervalSec ?? 20),
  enabled: e?.enabled ?? true,
  description: e?.description ?? '',
});

/** IPFIX exporters: the first enabled IPv4 one (by name) becomes exporter 0, the one flowprobe records use. */
export function ExportersPanel({ cfg, state, readOnly, pending, save }: PanelProps) {
  const { t } = useTranslation(NS);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [isNew, setIsNew] = useState(false);
  const exporters = cfg.exporters ?? {};
  const live = new Map((state?.exporters ?? []).map((e) => [e.name, e]));
  const submit = async () => {
    if (draft === null) return;
    const body: ExporterConfig = {
      enabled: draft.enabled,
      collector: { address: draft.address.trim(), port: Number(draft.port) },
      sourceAddress: draft.source.trim(),
      vrf: draft.vrf.trim() || 'default',
      pathMtu: Number(draft.pathMtu),
      templateIntervalSec: Number(draft.template),
      ...(draft.description.trim() ? { description: draft.description.trim() } : {}),
    };
    await save({ exporters: { [draft.name.trim()]: body } });
    setDraft(null);
  };
  const set = (k: keyof Draft) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setDraft((d) => (d ? { ...d, [k]: k === 'enabled' ? e.target.checked : e.target.value } : d));
  const on = Object.fromEntries(DRAFT_KEYS.map((k) => [k, set(k)])) as Record<
    keyof Draft,
    ReturnType<typeof set>
  >;
  return (
    <Stack spacing={2}>
      <Typography variant="body2" color="text.secondary">
        {t('exporters.help')}
      </Typography>
      {(() => {
        const nativeRows = Object.entries(exporters)
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([name, e]) => {
            const l = live.get(name);
            return { name, e, l };
          });
        return (
          <ReactGeereedMaterial
            footer={false}
            hideFilterBuilderComponent
            items={nativeRows}
            columns={[
              {
                key: 'name',
                headerRenderer: () => <>{t('exporters.name')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { name }: (typeof nativeRows)[number]) => <>{name}</>,
              },
              {
                key: 'collector',
                headerRenderer: () => <>{t('exporters.collector')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { e }: (typeof nativeRows)[number]) => (
                  <>
                    {e.collector.address}:{e.collector.port ?? 4739}
                  </>
                ),
              },
              {
                key: 'source',
                headerRenderer: () => <>{t('exporters.source')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { e }: (typeof nativeRows)[number]) => (
                  <>{e.sourceAddress}</>
                ),
              },
              {
                key: 'vrf',
                headerRenderer: () => <>{t('exporters.vrf')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { e }: (typeof nativeRows)[number]) => (
                  <>{e.vrf ?? DEFAULT_VRF}</>
                ),
              },
              {
                key: 'status',
                headerRenderer: () => <>{t('exporters.status')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { e, l }: (typeof nativeRows)[number]) => (
                  <>
                    {e.enabled === false ? (
                      <Chip size="small" label={t('exporters.disabled')} />
                    ) : l === undefined ? (
                      <Chip size="small" color="warning" label={t('exporters.notApplied')} />
                    ) : (
                      <Stack direction="row" spacing={1}>
                        <Chip size="small" color="success" label={t('exporters.active')} />
                        {l.defaultExporter && (
                          <Chip size="small" color="primary" label={t('exporters.default')} />
                        )}
                        {l.statIndex !== null && (
                          <Chip
                            size="small"
                            variant="outlined"
                            label={t('exporters.statIndex', { index: l.statIndex })}
                          />
                        )}
                      </Stack>
                    )}
                  </>
                ),
              },
              {
                key: 'actions',
                headerRenderer: () => <></>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { name, e }: (typeof nativeRows)[number]) => (
                  <>
                    <IconButton
                      aria-label={t('edit', { name })}
                      disabled={readOnly || pending}
                      onClick={() => {
                        setIsNew(false);
                        setDraft(draftOf(name, e));
                      }}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      aria-label={t('remove', { name })}
                      disabled={readOnly || pending}
                      onClick={() => void save({ exporters: { [name]: null } })}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </>
                ),
              },
            ]}
            rowKey={(item) => {
              const { name } = item as (typeof nativeRows)[number];
              return name;
            }}
            cellProps={(_item, column) => {
              switch (column.key) {
                case 'collector':
                  return { dir: 'ltr' };
                case 'source':
                  return { dir: 'ltr' };
                default:
                  return {};
              }
            }}
            tableProps={{ size: 'small', 'aria-label': t('section.exporters') }}
            emptyContent={<>{t('exporters.empty')}</>}
            noFormWrapper
            embedded
            memoEnable={false}
            hidePagination
            hideToolbar
            search={false}
            exportToCSV={false}
            sortable={false}
          />
        );
      })()}
      <div>
        <Button
          startIcon={<AddIcon />}
          variant="outlined"
          disabled={readOnly || pending}
          onClick={() => {
            setIsNew(true);
            setDraft(draftOf(''));
          }}
        >
          {t('exporters.add')}
        </Button>
      </div>
      <Dialog open={draft !== null} onClose={() => setDraft(null)} fullWidth maxWidth="sm">
        <DialogTitle>
          {isNew ? t('exporters.add') : t('edit', { name: draft?.name ?? '' })}
        </DialogTitle>
        <DialogContent>
          {draft !== null && (
            <Stack spacing={2} sx={{ mt: 1 }}>
              <TextField
                label={t('exporters.name')}
                value={draft.name}
                onChange={on.name}
                disabled={!isNew}
                required
              />
              <Stack direction="row" spacing={2}>
                <TextField
                  label={t('exporters.collectorAddress')}
                  value={draft.address}
                  onChange={on.address}
                  required
                  fullWidth
                />
                <TextField
                  label={t('exporters.port')}
                  value={draft.port}
                  onChange={on.port}
                  type="number"
                  sx={{ width: 140 }}
                />
              </Stack>
              <TextField
                label={t('exporters.source')}
                value={draft.source}
                onChange={on.source}
                required
              />
              <TextField label={t('exporters.vrf')} value={draft.vrf} onChange={on.vrf} />
              <Stack direction="row" spacing={2}>
                <TextField
                  label={t('exporters.pathMtu')}
                  value={draft.pathMtu}
                  onChange={on.pathMtu}
                  type="number"
                  fullWidth
                />
                <TextField
                  label={t('exporters.template')}
                  value={draft.template}
                  onChange={on.template}
                  type="number"
                  fullWidth
                />
              </Stack>
              <TextField
                label={t('exporters.description')}
                value={draft.description}
                onChange={on.description}
              />
              <FormControlLabel
                control={<Switch checked={draft.enabled} onChange={on.enabled} />}
                label={t('enabled')}
              />
            </Stack>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDraft(null)}>{t('cancel')}</Button>
          <Button
            variant="contained"
            disabled={pending || draft?.name.trim() === ''}
            onClick={() => void submit()}
          >
            {t('save')}
          </Button>
        </DialogActions>
      </Dialog>
    </Stack>
  );
}
