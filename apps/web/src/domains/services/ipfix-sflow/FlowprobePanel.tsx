import {
  PgAutocomplete as Autocomplete,
  PgButton as Button,
  PgText as TextField,
} from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import { PgAddIcon as AddIcon, PgDeleteIcon as DeleteIcon } from '@amnafzar/common-ui-lib/icons';
import {
  PgChip as Chip,
  PgFormControlLabel as FormControlLabel,
  PgIconButton as IconButton,
  PgMenuItem as MenuItem,
  PgStack as Stack,
  PgSwitch as Switch,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { NS, type PanelProps } from './FlowExportTab';
import type { FlowprobeConfig, FlowprobeInterfaceConfig } from './queries';

type Variant = 'ip4' | 'ip6' | 'l2';
const VARIANTS: readonly Variant[] = ['ip4', 'ip6', 'l2'];
const BOTH = 'both' as const;
const DIRECTIONS = ['rx', 'tx', BOTH] as const;
const newInterface = (name: string): FlowprobeInterfaceConfig => ({
  interface: name,
  direction: BOTH,
  l2: false,
  ip4: true,
  ip6: false,
});

/** The one variant VPP records on an interface (schema default ip4; ip4+ip6 is a semantic error, D-150). */
export function variantOf(f: FlowprobeInterfaceConfig): Variant {
  if (f.ip4 !== false) return 'ip4';
  if (f.ip6 === true) return 'ip6';
  return 'l2';
}

const withVariant = (f: FlowprobeInterfaceConfig, v: Variant): FlowprobeInterfaceConfig => ({
  ...f,
  l2: v === 'l2',
  ip4: v === 'ip4',
  ip6: v === 'ip6',
});

/** flowprobe: global timers/record flags and the monitored interfaces (one variant each). */
export function FlowprobePanel({ cfg, state, interfaces, readOnly, pending, save }: PanelProps) {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation(NS);
  const [fp, setFp] = useState<FlowprobeConfig>(cfg.flowprobe ?? {});
  const [pick, setPick] = useState<string | null>(null);
  useEffect(() => setFp(cfg.flowprobe ?? {}), [cfg.flowprobe]);
  const list = fp.interfaces ?? [];
  const live = new Map((state?.flowprobe.interfaces ?? []).map((i) => [i.interface, i]));
  const setList = (next: FlowprobeInterfaceConfig[]) => setFp({ ...fp, interfaces: next });
  const num =
    (k: 'activeTimerSec' | 'passiveTimerSec') => (e: React.ChangeEvent<HTMLInputElement>) =>
      setFp({ ...fp, [k]: Number(e.target.value) });
  const flag =
    (k: 'recordL2' | 'recordL3' | 'recordL4') => (e: React.ChangeEvent<HTMLInputElement>) =>
      setFp({ ...fp, [k]: e.target.checked });
  const onActive = num('activeTimerSec');
  const onPassive = num('passiveTimerSec');
  const flags = {
    recordL2: flag('recordL2'),
    recordL3: flag('recordL3'),
    recordL4: flag('recordL4'),
  };
  const hasV4Exporter = Object.values(cfg.exporters ?? {}).some(
    (e) => e.enabled !== false && !e.collector.address.includes(':'),
  );
  return (
    <Stack spacing={2}>
      <Typography variant="body2" color="text.secondary">
        {t('flowprobe.help')}
      </Typography>
      {!hasV4Exporter && list.length > 0 && (
        <Typography color="error">{t('flowprobe.needsExporter')}</Typography>
      )}
      <Stack direction="row" spacing={2} useFlexGap flexWrap="wrap">
        <TextField
          label={t('flowprobe.active')}
          type="number"
          value={fp.activeTimerSec ?? 15}
          onChange={onActive}
          disabled={readOnly}
        />
        <TextField
          label={t('flowprobe.passive')}
          type="number"
          value={fp.passiveTimerSec ?? 120}
          onChange={onPassive}
          disabled={readOnly}
        />
        <FormControlLabel
          control={
            <Switch checked={fp.recordL2 ?? false} onChange={flags.recordL2} disabled={readOnly} />
          }
          label={t('flowprobe.recordL2')}
        />
        <FormControlLabel
          control={
            <Switch checked={fp.recordL3 ?? true} onChange={flags.recordL3} disabled={readOnly} />
          }
          label={t('flowprobe.recordL3')}
        />
        <FormControlLabel
          control={
            <Switch checked={fp.recordL4 ?? true} onChange={flags.recordL4} disabled={readOnly} />
          }
          label={t('flowprobe.recordL4')}
        />
      </Stack>
      {(() => {
        const nativeRows = list.map((f, i) => {
          return { f, i };
        });
        return (
          <ReactGeereedMaterial
            footer={false}
            hideFilterBuilderComponent
            items={nativeRows}
            columns={[
              {
                key: 'interface',
                headerRenderer: () => <>{t('interface')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { f }: (typeof nativeRows)[number]) => (
                  <>{f.interface}</>
                ),
              },
              {
                key: 'variant',
                headerRenderer: () => <>{t('flowprobe.variant')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { f, i }: (typeof nativeRows)[number]) => (
                  <>
                    <TextField
                      select
                      size="small"
                      value={variantOf(f)}
                      disabled={readOnly}
                      aria-label={t('flowprobe.variant')}
                      onChange={(e) =>
                        setList(
                          list.map((x, k) =>
                            k === i ? withVariant(x, e.target.value as Variant) : x,
                          ),
                        )
                      }
                    >
                      {VARIANTS.map((v) => (
                        <MenuItem key={v} value={v}>
                          {t(`flowprobe.which.${v}`)}
                        </MenuItem>
                      ))}
                    </TextField>
                  </>
                ),
              },
              {
                key: 'direction',
                headerRenderer: () => <>{t('flowprobe.direction')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { f, i }: (typeof nativeRows)[number]) => (
                  <>
                    <TextField
                      select
                      size="small"
                      value={f.direction ?? BOTH}
                      disabled={readOnly}
                      aria-label={t('flowprobe.direction')}
                      onChange={(e) =>
                        setList(
                          list.map((x, k) =>
                            k === i
                              ? { ...x, direction: e.target.value as 'rx' | 'tx' | 'both' }
                              : x,
                          ),
                        )
                      }
                    >
                      {DIRECTIONS.map((d) => (
                        <MenuItem key={d} value={d}>
                          {t(`direction.${d}`)}
                        </MenuItem>
                      ))}
                    </TextField>
                  </>
                ),
              },
              {
                key: 'status',
                headerRenderer: () => <>{t('exporters.status')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { f }: (typeof nativeRows)[number]) => (
                  <>
                    {live.has(f.interface) ? (
                      <Chip size="small" color="success" label={t('exporters.active')} />
                    ) : (
                      <Chip size="small" color="warning" label={t('exporters.notApplied')} />
                    )}
                  </>
                ),
              },
              {
                key: 'actions',
                headerRenderer: () => <></>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, { f, i }: (typeof nativeRows)[number]) => (
                  <>
                    <IconButton
                      aria-label={t('remove', { name: f.interface })}
                      disabled={readOnly}
                      onClick={() => setList(list.filter((_, k) => k !== i))}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </>
                ),
              },
            ]}
            rowKey={(item) => {
              const { f } = item as (typeof nativeRows)[number];
              return f.interface;
            }}
            tableProps={{ size: 'small', 'aria-label': t('flowprobe.interfaces') }}
            emptyContent={null}
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
      <Stack direction="row" spacing={2}>
        <Autocomplete
          options={interfaces.filter((n) => !list.some((f) => f.interface === n))}
          getOptionLabel={interfaceLabel}
          value={pick}
          onChange={(_, v) => setPick(v)}
          sx={{ minWidth: 260 }}
          disabled={readOnly}
          renderInput={(p) => <TextField {...p} size="small" label={t('interfacePicker')} />}
        />
        <Button
          startIcon={<AddIcon />}
          disabled={readOnly || pick === null}
          onClick={() => {
            if (pick !== null) setList([...list, newInterface(pick)]);
            setPick(null);
          }}
        >
          {t('add')}
        </Button>
      </Stack>
      <div>
        <Button
          variant="contained"
          disabled={readOnly || pending}
          onClick={() => void save({ flowprobe: fp })}
        >
          {t('save')}
        </Button>
      </div>
    </Stack>
  );
}
