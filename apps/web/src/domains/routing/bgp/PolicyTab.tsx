import Box from '@mui/material/Box';
import type { GridColDef } from '@ngfw/ui-kit/data-grid';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { localizeSchema } from '../vrf-static-ecmp/model';
import { useCandidate } from './api';
import { NS, OBJECT_NAME, policyItemSchema, type RoutingDoc } from './model';
import { RecordEditor } from './RecordEditor';

const PREFIX_LISTS = 'prefixLists' as const;
const ROUTE_MAPS = 'routeMaps' as const;
const policyPath = (kind: 'prefixLists' | 'routeMaps') => ['policy', kind] as const;

interface PolicyRow {
  id: string;
  name: string;
  family: string;
  count: number;
  description: string;
  usedBy: string;
}

/** Where a prefix list / route map is referenced (BGP neighbours, peer groups, networks, redistribution, route maps). */
function usages(
  doc: RoutingDoc | undefined,
  kind: 'prefixLists' | 'routeMaps',
  name: string,
): string[] {
  const out: string[] = [];
  const bgp = doc?.bgp;
  const peers = [...Object.entries(bgp?.peerGroups ?? {}), ...Object.entries(bgp?.neighbors ?? {})];
  for (const [id, p] of peers) {
    for (const f of Object.values(p.afi ?? {})) {
      if (!f) continue;
      const refs =
        kind === 'prefixLists' ? [f.prefixListIn, f.prefixListOut] : [f.routeMapIn, f.routeMapOut];
      if (refs.includes(name)) out.push(id);
    }
  }
  if (kind === 'routeMaps') {
    if ((bgp?.networks ?? []).some((n) => n.routeMap === name)) out.push('networks');
    if (
      Object.values(bgp?.redistribute ?? {}).some(
        (r) => (r as { routeMap?: string } | undefined)?.routeMap === name,
      )
    )
      out.push('redistribute');
  } else {
    for (const [rm, m] of Object.entries(doc?.policy?.routeMaps ?? {})) {
      if (m.entries.some((e) => e.match.prefixList === name || e.match.nextHopPrefixList === name))
        out.push(rm);
    }
  }
  return [...new Set(out)];
}

/**
 * Routing policy: prefix lists and route maps (`routing.policy`, D-045/D-070), each an object edited with SchemaForm —
 * the rules / entries are array widgets. BGP (and later OSPF, IS-IS …) refer to them by name; the grid shows where.
 */
export function PolicyTab({ kind }: { kind: 'prefixLists' | 'routeMaps' }) {
  const { t } = useTranslation(NS);
  const routing = useCandidate<RoutingDoc>('routing');
  const tr = (k: string, o?: Record<string, unknown>) => t(k, o ?? {});
  const schema = useMemo(
    () => localizeSchema(policyItemSchema(kind), tr, kind === 'prefixLists' ? 'pl.' : 'rm.'),
    [t, kind],
  );
  const record = routing.data?.policy?.[kind] as Record<string, unknown> | undefined;
  const rows = useMemo<PolicyRow[]>(
    () =>
      Object.entries(record ?? {}).map(([name, v]) => {
        const o = v as {
          family?: string;
          rules?: unknown[];
          entries?: unknown[];
          description?: string;
        };
        return {
          id: name,
          name,
          family: o.family ?? '',
          count: (o.rules ?? o.entries ?? []).length,
          description: o.description ?? '',
          usedBy: usages(routing.data, kind, name).join(', '),
        };
      }),
    [record, routing.data, kind],
  );
  const columns = useMemo<GridColDef<PolicyRow>[]>(
    () => [
      { field: 'name', headerName: t('policy.col.name'), flex: 1 },
      ...(kind === 'prefixLists'
        ? [
            {
              field: 'family',
              headerName: t('policy.col.family'),
              width: 100,
            } as GridColDef<PolicyRow>,
          ]
        : []),
      {
        field: 'count',
        headerName: t(kind === 'prefixLists' ? 'policy.col.rules' : 'policy.col.entries'),
        type: 'number',
        width: 100,
      },
      { field: 'usedBy', headerName: t('policy.col.usedBy'), flex: 1 },
      { field: 'description', headerName: t('policy.col.description'), flex: 1 },
    ],
    [t, kind],
  );
  return (
    <Box>
      <RecordEditor<PolicyRow>
        k={kind}
        record={record}
        path={policyPath(kind)}
        schema={schema}
        rows={rows}
        columns={columns}
        checkKey={(k) =>
          OBJECT_NAME.test(k) ? (record?.[k] ? t(`${kind}.exists`) : '') : t('badName')
        }
        version={routing.dataUpdatedAt}
      />
    </Box>
  );
}

export function PrefixListsTab() {
  return <PolicyTab kind={PREFIX_LISTS} />;
}

export function RouteMapsTab() {
  return <PolicyTab kind={ROUTE_MAPS} />;
}
