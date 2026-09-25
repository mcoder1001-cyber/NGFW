import RefreshIcon from '@mui/icons-material/Refresh';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import { StatusChip } from '@ngfw/ui-kit';
import type { GridColDef } from '@ngfw/ui-kit/data-grid';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { useInterfaceNames, useMplsTunnels, useRefreshMpls } from './api';
import { Mono } from './common';
import { ListEditor, type EditorRow } from './ListEditor';
import {
  keyedSchema,
  localizeSchema,
  mplsSchema,
  NS,
  pathText,
  type MplsTunnelConfig,
} from './model';
import { useMpls } from './useMpls';

type TunnelRow = EditorRow & {
  name: string;
  paths: string;
  l2Only: string;
  status: 'up' | 'down' | 'degraded';
  vpp: string;
};

/** Form member that carries the record key (tunnel name / binding SID). */
const KEY_FIELD = 'name';
/**
 * MPLS tunnels (`routing.mpls.tunnels`): head-end LSPs as tunnel interfaces, with a live column from
 * `GET /state/routing/mpls/tunnels` (read on demand and once a minute, D-132). The name is the tunnel interface's
 * logical name, so label routes can send packets into it.
 */
export function TunnelsTab() {
  const { t } = useTranslation(NS);
  const { routing, mpls, update, write } = useMpls();
  const live = useMplsTunnels();
  const refresh = useRefreshMpls();
  const ifNames = useInterfaceNames();
  const tr = useCallback((k: string, o?: Record<string, unknown>) => t(k, o ?? {}), [t]);
  const schema = useMemo(() => {
    const tunnels = (mplsSchema().properties as Record<string, JsonSchema>)[
      'tunnels'
    ] as JsonSchema;
    const keyed = keyedSchema(
      tunnels.additionalProperties as JsonSchema,
      {
        ...(tunnels.propertyNames as JsonSchema),
        title: 'Name',
      } as JsonSchema,
    );
    return localizeSchema(keyed, tr, 'tunnel.');
  }, [tr]);
  const byName = new Map((live.data?.items ?? []).filter((x) => x.owned).map((x) => [x.name, x]));
  const rows: TunnelRow[] = Object.entries(mpls?.tunnels ?? {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([name, tn]) => {
      const l = byName.get(name);
      return {
        id: name,
        name,
        paths: tn.paths.map((p) => pathText(p, tr)).join(' · '),
        l2Only: tn.l2Only ? t('yes') : t('no'),
        status: live.data === undefined ? 'degraded' : l ? 'up' : 'down',
        vpp: l ? `${l.interface} (#${l.swIfIndex})` : '',
      };
    });
  const cols = useMemo<GridColDef<TunnelRow>[]>(
    () => [
      {
        field: 'name',
        headerName: t('tunnels.col.name'),
        width: 140,
        renderCell: (p) => <Mono>{p.row.name}</Mono>,
      },
      {
        field: 'paths',
        headerName: t('tunnels.col.paths'),
        minWidth: 260,
        flex: 1,
        renderCell: (p) => <Mono>{p.row.paths}</Mono>,
      },
      { field: 'l2Only', headerName: t('tunnels.col.l2Only'), width: 100 },
      {
        field: 'status',
        headerName: t('tunnels.col.status'),
        width: 150,
        sortable: false,
        renderCell: (p) => (
          <StatusChip
            size="small"
            status={p.row.status}
            label={t(`tunnels.status.${p.row.status}`)}
          />
        ),
      },
      {
        field: 'vpp',
        headerName: t('tunnels.col.vpp'),
        width: 190,
        renderCell: (p) => <Mono>{p.row.vpp}</Mono>,
      },
    ],
    [t],
  );
  return (
    <Box>
      {routing.isError && <ProblemAlert error={routing.error} sx={{ mb: 1 }} />}
      {live.isError && <ProblemAlert error={live.error} sx={{ mb: 1 }} />}
      <ListEditor<TunnelRow>
        title={t('tunnels.title')}
        intro={t('tunnels.intro')}
        columns={cols}
        rows={rows}
        version={routing.dataUpdatedAt + (live.dataUpdatedAt ?? 0)}
        schema={schema}
        keyField={KEY_FIELD}
        valueOf={(r) => (r ? { name: r.name, ...mpls?.tunnels[r.name] } : undefined)}
        interfaceOptions={[...ifNames, ...Object.keys(mpls?.tunnels ?? {})]}
        error={write.error}
        pending={write.isPending}
        pointerOf={(r) => `/routing/mpls/tunnels/${r?.name ?? ''}`}
        addLabel={t('tunnels.add')}
        addTitle={t('tunnels.addTitle')}
        editTitle={(r) => t('tunnels.editTitle', { name: r.name })}
        toolbar={
          <Button startIcon={<RefreshIcon />} onClick={refresh}>
            {t('refresh')}
          </Button>
        }
        onSave={(v, r) =>
          update((m) => {
            const { name, ...value } = v as MplsTunnelConfig & { name: string };
            const tunnels = { ...m.tunnels };
            if (r && r.name !== name) delete tunnels[r.name];
            tunnels[name] = value;
            return { ...m, tunnels };
          })
        }
        onRemove={(r) =>
          update((m) => {
            const tunnels = { ...m.tunnels };
            delete tunnels[r.name];
            return { ...m, tunnels };
          })
        }
      />
    </Box>
  );
}
