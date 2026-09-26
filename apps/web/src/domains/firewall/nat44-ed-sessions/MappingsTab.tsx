import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { useTranslation } from 'react-i18next';
import { ListSection } from './ListSection';
import { DEFAULT_VRF, type NatListKey } from './model';
import { NAT_NS } from './tabs';

type Item = Record<string, unknown>;

const STATIC: NatListKey = 'staticMappings';
const IDENTITY: NatListKey = 'identityMappings';
const LB: NatListKey = 'loadBalancedMappings';
const MAPPING_FLAGS = ['twiceNat', 'selfTwiceNat', 'out2inOnly'];

const str = (v: unknown): string =>
  typeof v === 'string' || typeof v === 'number' ? String(v) : '';

/** `ip:port`, `ip`, the interface or `pool <name>` of a mapping endpoint. */
export function endpoint(e: unknown): string {
  const o = (e ?? {}) as Item;
  const where =
    str(o['ip']) ||
    (o['interface'] ? str(o['interface']) : o['pool'] ? `pool ${str(o['pool'])}` : '');
  return o['port'] !== undefined ? `${where}:${str(o['port'])}` : where;
}

function Flags({ item, keys }: { item: Item; keys: string[] }) {
  const { t } = useTranslation(NAT_NS);
  const on = keys.filter((k) => item[k] === true);
  return (
    <Stack direction="row" gap={0.5}>
      {on.map((k) => (
        <Chip key={k} size="small" variant="outlined" label={t(`field.${k}.title`)} />
      ))}
    </Stack>
  );
}

/** Static & port forwards: 1:1 and port-forward static mappings, identity mappings, load-balanced mappings. */
export function MappingsTab() {
  const { t } = useTranslation(NAT_NS);
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('static.intro')}
      </Typography>
      <ListSection
        listKey={STATIC}
        title={t('static.title')}
        addLabel={t('static.add')}
        emptyText={t('static.none')}
        itemLabel={(m) => str(m['name'])}
        columns={[
          { label: t('col.name'), value: (m) => str(m['name']), ltr: true },
          {
            label: t('col.kind'),
            value: (m) =>
              (m['local'] as Item | undefined)?.['port'] !== undefined
                ? t('static.portForward')
                : t('static.oneToOne'),
          },
          { label: t('col.protocol'), value: (m) => str(m['protocol']) || t('col.any') },
          { label: t('col.local'), value: (m) => endpoint(m['local']), ltr: true },
          { label: t('col.external'), value: (m) => endpoint(m['external']), ltr: true },
          { label: t('col.vrf'), value: (m) => str(m['vrf']) || DEFAULT_VRF },
          { label: t('col.flags'), value: (m) => <Flags item={m} keys={MAPPING_FLAGS} /> },
        ]}
      />
      <ListSection
        listKey={IDENTITY}
        title={t('identity.title')}
        addLabel={t('identity.add')}
        emptyText={t('identity.none')}
        itemLabel={(m, i) => str(m['ip']) || str(m['interface']) || String(i + 1)}
        columns={[
          { label: t('col.address'), value: (m) => str(m['ip']) || str(m['interface']), ltr: true },
          { label: t('col.protocol'), value: (m) => str(m['protocol']) || t('col.any') },
          { label: t('col.port'), value: (m) => str(m['port']) || t('col.any') },
          { label: t('col.vrf'), value: (m) => str(m['vrf']) || DEFAULT_VRF },
        ]}
      />
      <ListSection
        listKey={LB}
        title={t('lb.title')}
        addLabel={t('lb.add')}
        emptyText={t('lb.none')}
        itemLabel={(m) => str(m['name'])}
        columns={[
          { label: t('col.name'), value: (m) => str(m['name']), ltr: true },
          { label: t('col.protocol'), value: (m) => str(m['protocol']) },
          { label: t('col.external'), value: (m) => endpoint(m['external']), ltr: true },
          {
            label: t('col.locals'),
            value: (m) =>
              ((m['locals'] as Item[] | undefined) ?? [])
                .map((l) => `${endpoint(l)} (${str(l['probability'])})`)
                .join(' '),
            ltr: true,
          },
          { label: t('col.flags'), value: (m) => <Flags item={m} keys={MAPPING_FLAGS} /> },
        ]}
      />
    </Box>
  );
}
