import {
  PgAutocomplete as Autocomplete,
  PgButton as Button,
  PgText as TextField,
} from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgFormControlLabel as FormControlLabel,
  PgMenuItem as MenuItem,
  PgStack as Stack,
  PgSwitch as Switch,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { NS, type PanelProps } from './FlowExportTab';
import type { SflowConfig } from './queries';

/** VPP rounds other sizes silently (DF-8 sflow.md): offer only 64..256 in steps of 32. */
export const HEADER_SIZES = [64, 96, 128, 160, 192, 224, 256] as const;

const DEFAULT_VRF = 'default';
const blank: SflowConfig = {
  enabled: false,
  samplingN: 10000,
  pollingIntervalSec: 20,
  headerBytes: 128,
  collectors: [],
  interfaces: [],
};

/** sFlow sampling (VPP plugin): the sampling itself; export to collectors is hsflowd's (banner above). */
export function SflowPanel({ cfg, state, interfaces, readOnly, pending, save }: PanelProps) {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation(NS);
  const [sf, setSf] = useState<SflowConfig>(cfg.sflow ?? blank);
  const [collectors, setCollectors] = useState(() =>
    (cfg.sflow?.collectors ?? []).map((c) => `${c.address}:${c.port ?? 6343}`).join(', '),
  );
  useEffect(() => {
    setSf(cfg.sflow ?? blank);
    setCollectors(
      (cfg.sflow?.collectors ?? []).map((c) => `${c.address}:${c.port ?? 6343}`).join(', '),
    );
  }, [cfg.sflow]);
  const num =
    (k: 'samplingN' | 'pollingIntervalSec' | 'headerBytes') =>
    (e: { target: { value: string | number } }) =>
      setSf({ ...sf, [k]: Number(e.target.value) });
  const onRate = num('samplingN');
  const onPolling = num('pollingIntervalSec');
  const onHeader = num('headerBytes');
  const parseCollectors = () =>
    collectors
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
      .map((s) => {
        const m = /^\[?([^\]]+?)\]?(?::(\d+))?$/.exec(
          s.includes('.') || s.startsWith('[') ? s : `[${s}]`,
        );
        return { address: m?.[1] ?? s, port: m?.[2] !== undefined ? Number(m[2]) : 6343 };
      });
  return (
    <Stack spacing={2}>
      <Typography variant="body2" color="text.secondary">
        {t('sflow.help')}
      </Typography>
      <FormControlLabel
        control={
          <Switch
            checked={sf.enabled ?? false}
            onChange={(e) => setSf({ ...sf, enabled: e.target.checked })}
            disabled={readOnly}
          />
        }
        label={t('enabled')}
      />
      <Stack direction="row" spacing={2} useFlexGap flexWrap="wrap">
        <TextField
          label={t('sflow.samplingN')}
          type="number"
          value={sf.samplingN ?? 10000}
          onChange={onRate}
          disabled={readOnly}
        />
        <TextField
          label={t('sflow.polling')}
          type="number"
          value={sf.pollingIntervalSec ?? 20}
          onChange={onPolling}
          disabled={readOnly}
        />
        <TextField
          select
          label={t('sflow.headerBytes')}
          value={sf.headerBytes ?? 128}
          onChange={onHeader}
          disabled={readOnly}
          sx={{ minWidth: 160 }}
        >
          {HEADER_SIZES.map((h) => (
            <MenuItem key={h} value={h}>
              {h}
            </MenuItem>
          ))}
        </TextField>
      </Stack>
      <TextField
        label={t('sflow.collectors')}
        helperText={t('sflow.collectorsHelp')}
        value={collectors}
        onChange={(e) => setCollectors(e.target.value)}
        disabled={readOnly}
      />
      <Stack direction="row" spacing={2}>
        <TextField
          label={t('sflow.agentAddress')}
          value={sf.agentAddress ?? ''}
          onChange={(e) => setSf({ ...sf, agentAddress: e.target.value })}
          disabled={readOnly}
        />
        <TextField
          label={t('exporters.vrf')}
          value={sf.vrf ?? DEFAULT_VRF}
          onChange={(e) => setSf({ ...sf, vrf: e.target.value })}
          disabled={readOnly}
        />
      </Stack>
      <Autocomplete
        multiple
        options={interfaces}
        getOptionLabel={interfaceLabel}
        value={sf.interfaces ?? []}
        onChange={(_, v) => setSf({ ...sf, interfaces: v })}
        disabled={readOnly}
        renderInput={(p) => <TextField {...p} label={t('sflow.interfaces')} />}
      />
      <div>
        <Button
          variant="contained"
          disabled={readOnly || pending}
          onClick={() => {
            const { agentAddress, ...rest } = sf;
            void save({
              sflow: {
                ...rest,
                ...(agentAddress ? { agentAddress } : {}),
                collectors: parseCollectors(),
              },
            });
          }}
        >
          {t('save')}
        </Button>
      </div>
      <Typography variant="subtitle2">{t('sflow.live')}</Typography>
      {(() => {
        const rows = [
          ...(state?.sflow.interfaces ?? []).map((item) => ({
            key: `interface-${item.interface}`,
            name: item.interface,
            value: item.hwIfIndex,
            counter: false,
          })),
          ...(state?.sflow.counters ?? []).map((item) => ({
            key: `counter-${item.name}`,
            name: item.name,
            value: item.value,
            counter: true,
          })),
        ];
        return (
          <ReactGeereedMaterial
            footer={false}
            hideFilterBuilderComponent
            items={rows}
            columns={[
              {
                key: 'name',
                headerRenderer: () => <>{t('interface')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, row: (typeof rows)[number]) => <>{row.name}</>,
              },
              {
                key: 'value',
                headerRenderer: () => <>{t('sflow.hwIfIndex')}</>,
                sortable: false,
                filterable: false,
                renderer: (_value: unknown, row: (typeof rows)[number]) => <>{row.value}</>,
              },
            ]}
            rowKey={(item) => (item as (typeof rows)[number]).key}
            cellProps={(item, column) =>
              (item as (typeof rows)[number]).counter && column.key === 'name' ? { dir: 'ltr' } : {}
            }
            tableProps={{ size: 'small', 'aria-label': t('sflow.live') }}
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
    </Stack>
  );
}
