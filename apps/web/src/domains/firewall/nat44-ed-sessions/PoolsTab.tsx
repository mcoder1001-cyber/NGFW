import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import LinearProgress from '@mui/material/LinearProgress';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { useTranslation } from 'react-i18next';
import { ListSection } from './ListSection';
import { DEFAULT_VRF, usageFor, type NatListKey, type PoolUsage } from './model';
import { NatStatus } from './OutboundTab';
import { useNatSummary } from './queries';
import { NAT_NS } from './tabs';

type Item = Record<string, unknown>;

const POOLS: NatListKey = 'pools';

const str = (v: unknown): string =>
  typeof v === 'string' || typeof v === 'number' ? String(v) : '';

/** Utilisation bar of one pool: sessions per address (an estimate, ED reuses ports per destination) + applied state. */
export function PoolUsageCell({ name, usage }: { name: string; usage: PoolUsage | undefined }) {
  const { t } = useTranslation(NAT_NS);
  const fmt = useFormatters();
  if (!usage)
    return <Chip size="small" color="warning" variant="outlined" label={t('pools.notApplied')} />;
  const pct = Math.round(usage.utilisation * 1000) / 10;
  return (
    <Stack gap={0.5} sx={{ minInlineSize: 160 }}>
      <LinearProgress
        variant="determinate"
        value={Math.min(100, usage.utilisation * 100)}
        aria-label={t('pools.utilisationOf', { name })}
        aria-valuetext={t('pools.utilisationText', { pct })}
        color={usage.utilisation > 0.8 ? 'warning' : 'primary'}
        data-testid={`pool-utilisation-${name}`}
      />
      <Typography variant="caption" color="text.secondary">
        {t('pools.usage', {
          sessions: fmt.integer(usage.sessions),
          addresses: fmt.integer(usage.addresses),
          pct,
        })}
        {usage.applied ? '' : ` · ${t('pools.notApplied')}`}
      </Typography>
    </Stack>
  );
}

/** Address pools (range or interface address) with their live utilisation from the agent (NatSummary). */
export function PoolsTab() {
  const { t } = useTranslation(NAT_NS);
  const summary = useNatSummary();
  const usage = summary.data?.pools ?? [];
  const orphans = usage.filter((u) => !u.configured);
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('pools.intro')}
      </Typography>
      <NatStatus />
      <ListSection
        listKey={POOLS}
        title={t('pools.title')}
        addLabel={t('pools.add')}
        emptyText={t('pools.none')}
        itemLabel={(p) => str(p['name'])}
        columns={[
          { label: t('col.name'), value: (p) => str(p['name']), ltr: true },
          {
            label: t('col.kind'),
            value: (p) => (p['interface'] ? t('pools.kindInterface') : t('pools.kindRange')),
          },
          {
            label: t('col.addresses'),
            value: (p) => str(p['range']) || str(p['interface']),
            ltr: true,
          },
          {
            label: t('col.vrf'),
            value: (p) => (p['interface'] ? '—' : str(p['vrf']) || DEFAULT_VRF),
          },
          { label: t('col.twiceNat'), value: (p) => (p['twiceNat'] === true ? t('yes') : t('no')) },
        ]}
        status={{
          label: t('col.utilisation'),
          value: (p: Item) => <PoolUsageCell name={str(p['name'])} usage={usageFor(p, usage)} />,
        }}
      />
      {orphans.length > 0 && (
        <Stack
          direction="row"
          gap={1}
          flexWrap="wrap"
          role="status"
          aria-label={t('pools.orphans')}
        >
          <Typography variant="body2">{t('pools.orphans')}</Typography>
          {orphans.map((o) => (
            <Chip
              key={`${o.range ?? ''}${o.interface ?? ''}${String(o.twiceNat)}`}
              size="small"
              color="warning"
              variant="outlined"
              label={o.range ?? o.interface ?? ''}
            />
          ))}
        </Stack>
      )}
    </Box>
  );
}
