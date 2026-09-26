import Box from '@mui/material/Box';
import type { GridColDef } from '@ngfw/ui-kit/data-grid';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { useInterfaceNames } from './api';
import { Mono } from './common';
import { ListEditor, type EditorRow } from './ListEditor';
import { PathsView } from './PathsView';
import {
  itemSchema,
  labelRouteRows,
  localizeSchema,
  NS,
  type LabelRouteRow,
  type MplsConfig,
} from './model';
import { useMpls } from './useMpls';

type BindingRow = EditorRow & { index: number; label: number; vrf: string; prefix: string };
type Binding = MplsConfig['ipBindings'][number];
type Route = MplsConfig['labelRoutes'][number];

/**
 * Static label routes (`routing.mpls.labelRoutes`: local label → paths with out labels — swap, push, pop, or pop and
 * look up in a VRF) and label ↔ IP-prefix bindings (`routing.mpls.ipBindings`), each a grid with a schema-driven
 * editor. Lists are written back whole, in the agent's canonical order (table, label, end of stack first).
 */
export function LabelRoutesTab() {
  const { t } = useTranslation(NS);
  const { routing, mpls, update, write } = useMpls();
  const ifNames = useInterfaceNames();
  const tunnels = Object.keys(mpls?.tunnels ?? {});
  const tr = useCallback((k: string, o?: Record<string, unknown>) => t(k, o ?? {}), [t]);
  const routeSchema = useMemo(() => localizeSchema(itemSchema('labelRoutes'), tr), [tr]);
  const bindSchema = useMemo(() => localizeSchema(itemSchema('ipBindings'), tr, 'binding.'), [tr]);
  const rows = useMemo(() => labelRouteRows(mpls, tr), [mpls, tr]);
  const bindRows: BindingRow[] = (mpls?.ipBindings ?? []).map((b, index) => ({
    id: `${index}`,
    index,
    ...b,
  }));

  const routeCols = useMemo<GridColDef<LabelRouteRow>[]>(
    () => [
      {
        field: 'table',
        headerName: t('routes.col.table'),
        type: 'number',
        width: 110,
        renderCell: (p) => <Mono>{p.row.table}</Mono>,
      },
      {
        field: 'label',
        headerName: t('routes.col.label'),
        type: 'number',
        width: 110,
        renderCell: (p) => <Mono>{p.row.label}</Mono>,
      },
      { field: 'eos', headerName: t('routes.col.eos'), width: 150 },
      { field: 'payload', headerName: t('routes.col.payload'), width: 110 },
      {
        field: 'paths',
        headerName: t('routes.col.paths'),
        minWidth: 300,
        flex: 1,
        renderCell: (p) => <PathsView paths={p.row.pathList} />,
      },
    ],
    [t],
  );
  const bindCols = useMemo<GridColDef<BindingRow>[]>(
    () => [
      {
        field: 'label',
        headerName: t('routes.col.label'),
        type: 'number',
        width: 120,
        renderCell: (p) => <Mono>{p.row.label}</Mono>,
      },
      { field: 'vrf', headerName: t('bindings.col.vrf'), width: 140 },
      {
        field: 'prefix',
        headerName: t('bindings.col.prefix'),
        minWidth: 200,
        flex: 1,
        renderCell: (p) => <Mono>{p.row.prefix}</Mono>,
      },
    ],
    [t],
  );

  const sortRoutes = (list: Route[]) =>
    [...list].sort(
      (a, b) => a.table - b.table || a.label - b.label || Number(b.eos) - Number(a.eos),
    );

  return (
    <Box>
      {routing.isError && <ProblemAlert error={routing.error} sx={{ mb: 1 }} />}
      <ListEditor<LabelRouteRow>
        title={t('routes.title')}
        intro={t('routes.intro')}
        columns={routeCols}
        rows={rows}
        version={routing.dataUpdatedAt}
        schema={routeSchema}
        valueOf={(r) => (r ? mpls?.labelRoutes[r.index] : undefined)}
        interfaceOptions={[...ifNames, ...tunnels]}
        error={write.error}
        pending={write.isPending}
        pointerOf={(r) =>
          `/routing/mpls/labelRoutes/${r ? r.index : (mpls?.labelRoutes.length ?? 0)}`
        }
        addLabel={t('routes.add')}
        addTitle={t('routes.addTitle')}
        editTitle={(r) => t('routes.editTitle', { label: r.label, table: r.table })}
        onSave={(v, r) =>
          update((m) => {
            const list = [...m.labelRoutes];
            if (r) list[r.index] = v as Route;
            else list.push(v as Route);
            return { ...m, labelRoutes: r ? list : sortRoutes(list) };
          })
        }
        onRemove={(r) =>
          update((m) => ({ ...m, labelRoutes: m.labelRoutes.filter((_, i) => i !== r.index) }))
        }
      />
      <ListEditor<BindingRow>
        title={t('bindings.title')}
        intro={t('bindings.intro')}
        columns={bindCols}
        rows={bindRows}
        version={routing.dataUpdatedAt}
        schema={bindSchema}
        valueOf={(r) => (r ? mpls?.ipBindings[r.index] : undefined)}
        error={write.error}
        pending={write.isPending}
        pointerOf={(r) =>
          `/routing/mpls/ipBindings/${r ? r.index : (mpls?.ipBindings.length ?? 0)}`
        }
        addLabel={t('bindings.add')}
        addTitle={t('bindings.addTitle')}
        editTitle={(r) => t('bindings.editTitle', { label: r.label, prefix: r.prefix })}
        height={240}
        onSave={(v, r) =>
          update((m) => {
            const list = [...m.ipBindings];
            if (r) list[r.index] = v as Binding;
            else list.push(v as Binding);
            return { ...m, ipBindings: list };
          })
        }
        onRemove={(r) =>
          update((m) => ({ ...m, ipBindings: m.ipBindings.filter((_, i) => i !== r.index) }))
        }
      />
    </Box>
  );
}
