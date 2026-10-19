import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useTopic } from '@ngfw/ui-kit/ws';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { BfdPage } from '../bfd/BfdPage';
import { PrefixListsTab, RouteMapsTab } from '../bgp/PolicyTab';
import { useCandidate, usePatch } from '../bgp/api';
import { ProtocolForm } from '../ospf/ProtocolForm';

const sessionColumns = ['engine', 'interface', 'peer', 'state', 'timers'] as const;
const policyKinds = ['routeMaps', 'prefixLists'] as const;
const policyFields = { routeMaps: 'entries', prefixLists: 'rules' } as const;
const policyTargets = ['bgp', 'ospf', 'isis', 'rip'];
const ROUTE_MAPS = 'routeMaps';
const ROUTING = 'routing';
export function BfdRedistributionPage() {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation('bfd-redistribution');
  const live = useQuery({
    queryKey: ['state', 'bfd'],
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/routing/bfd/sessions', { signal }))).data,
    refetchInterval: 30000,
  });
  const qc = useQueryClient();
  useTopic<{ attributes?: Record<string, string>; ts?: string }>('bfd-redistribution.events', {
    onBatch: (batch) => {
      qc.setQueryData<NonNullable<typeof live.data>>(['state', 'bfd'], (old) => {
        if (!old) return old;
        let sessions = old.sessions;
        for (const ev of batch) {
          const a = ev.data.attributes;
          if (!a || a['source'] !== 'bfd') continue;
          sessions = sessions.map((session) =>
            session.engine === a['engine'] &&
            session.interface === a['interface'] &&
            session.localAddress === a['localAddress'] &&
            session.peerAddress === a['peerAddress']
              ? {
                  ...session,
                  state: a['state'] ?? session.state,
                  lastFlap: ev.data.ts ?? session.lastFlap,
                }
              : session,
          );
        }
        return { ...old, sessions };
      });
    },
  });
  return (
    <Box>
      <BfdPage />
      <Typography variant="h6">{t('live')}</Typography>
      {live.isPending && <Typography>{t('loading')}</Typography>}
      {!live.isPending &&
        !live.isError &&
        !live.data?.agentError &&
        live.data?.sessions.length === 0 && <Typography>{t('empty')}</Typography>}
      {live.isError && <ProblemAlert error={live.error} />}
      {live.data?.agentError && <Alert severity="warning">{live.data.agentError}</Alert>}
      <Button onClick={() => void live.refetch()} disabled={live.isFetching}>
        {t('refresh')}
      </Button>
      {(() => {
        const gridItems = live.data?.sessions ?? [];
        type GridRow = (typeof gridItems)[number];
        return (
          <ReactGeereedMaterial
            hideFilterBuilderComponent
            noFormWrapper
            embedded
            memoEnable={false}
            hidePagination
            hideToolbar
            search={false}
            exportToCSV={false}
            sortable={false}
            footer={false}
            items={gridItems}
            columns={[
              {
                key: '0',
                sortable: false,
                filterable: false,
                headerRenderer: () => <>{t(sessionColumns[0])}</>,
                renderer: (_value: unknown, gridRow: GridRow) => {
                  const s = gridRow;
                  return <>{s.engine}</>;
                },
              },
              {
                key: '1',
                sortable: false,
                filterable: false,
                headerRenderer: () => <>{t(sessionColumns[1])}</>,
                renderer: (_value: unknown, gridRow: GridRow) => {
                  const s = gridRow;
                  return <>{interfaceLabel(s.interface)}</>;
                },
              },
              {
                key: '2',
                sortable: false,
                filterable: false,
                headerRenderer: () => <>{t(sessionColumns[2])}</>,
                renderer: (_value: unknown, gridRow: GridRow) => {
                  const s = gridRow;
                  return <>{s.peerAddress}</>;
                },
              },
              {
                key: '3',
                sortable: false,
                filterable: false,
                headerRenderer: () => <>{t(sessionColumns[3])}</>,
                renderer: (_value: unknown, gridRow: GridRow) => {
                  const s = gridRow;
                  return (
                    <>
                      {t(`states.${s.state}`, {
                        defaultValue: t('states.unknown'),
                      })}
                    </>
                  );
                },
              },
              {
                key: '4',
                sortable: false,
                filterable: false,
                headerRenderer: () => <>{t(sessionColumns[4])}</>,
                renderer: (_value: unknown, gridRow: GridRow) => {
                  const s = gridRow;
                  return (
                    <>
                      {t('timerValue', {
                        tx: s.desiredMinTxUs,
                        rx: s.requiredMinRxUs,
                        multiplier: s.detectMultiplier,
                      })}
                    </>
                  );
                },
              },
            ]}
            tableProps={{ size: 'small' }}
            rowKey={(nativeRow, _gridIndex) => {
              const gridRow = nativeRow as unknown as GridRow;
              const s = gridRow;
              return `${s.engine}/${s.interface}/${s.peerAddress}`;
            }}
            rowProps={(_nativeRow, _gridIndex) => {
              return {};
            }}
            cellProps={(_nativeRow, column, _gridIndex) => {
              switch (column.key) {
                case '0':
                  return {};
                case '1':
                  return {};
                case '2':
                  return { dir: 'ltr' };
                case '3':
                  return {};
                case '4':
                  return { dir: 'ltr' };
                default:
                  return {};
              }
            }}
            emptyContent={null}
          />
        );
      })()}
      <Button component={Link} to="/routing/redistribution">
        {t('matrix')}
      </Button>
      <Button component={Link} to="/routing/policy">
        {t('policy')}
      </Button>
    </Box>
  );
}
const sources = ['connected', 'static', 'bgp', 'ospf', 'isis', 'rip'] as const;
const targets = ['bgp', 'ospf', 'isis', 'rip', 'ospf6', 'ripng'] as const;
export function RedistributionPage() {
  const { t } = useTranslation('bfd-redistribution');
  const [edit, setEdit] = useState<'ospf' | 'isis' | 'rip' | 'ospf6' | 'ripng' | null>(null);
  const live = useQuery({
    queryKey: ['state', 'redistribution'],
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/routing/redistribution', { signal }))).data,
    refetchInterval: 30000,
  });
  return (
    <PageHeader title={t('matrix')}>
      {live.isPending && <Typography>{t('loadingMatrix')}</Typography>}
      {!live.isPending &&
        !live.isError &&
        !live.data?.agentError &&
        live.data?.edges.length === 0 && <Typography>{t('emptyMatrix')}</Typography>}
      {live.isError && <ProblemAlert error={live.error} />}
      {live.data?.agentError && <Alert severity="warning">{live.data.agentError}</Alert>}
      <ReactGeereedMaterial
        hideFilterBuilderComponent
        noFormWrapper
        embedded
        memoEnable={false}
        hidePagination
        hideToolbar
        search={false}
        exportToCSV={false}
        sortable={false}
        footer={false}
        items={targets.map((target) => ({ target }))}
        rowKey={(row) => row.target}
        columns={[
          {
            key: 'target',
            sortable: false,
            filterable: false,
            headerRenderer: () => <>{t('target')}</>,
            renderer: (_value: unknown, row: { target: (typeof targets)[number] }) => (
              <>{row.target}</>
            ),
          },
          ...sources.map((source) => ({
            key: source,
            sortable: false,
            filterable: false,
            headerRenderer: () => <>{source}</>,
            renderer: (_value: unknown, { target }: { target: (typeof targets)[number] }) => {
              const edge = live.data?.edges.find((e) => e.source === source && e.target === target);
              return (
                <>
                  {target === source ? (
                    '—'
                  ) : (
                    <Button
                      disabled={target === 'ospf6' || target === 'ripng'}
                      onClick={() => target !== 'bgp' && setEdit(target)}
                      component={target === 'bgp' ? Link : 'button'}
                      to={target === 'bgp' ? '/routing/bgp' : undefined}
                    >
                      {edge
                        ? `${edge.routeMap || t('enabled')} (${edge.routeCount ?? '?'}) · ${t('vrf')}: ${edge.vrf || t('defaultVrf')}`
                        : '—'}
                    </Button>
                  )}
                </>
              );
            },
          })),
        ]}
        emptyContent={null}
      />
      {edit && <ProtocolForm proto={edit} label={edit.toUpperCase()} />}
    </PageHeader>
  );
}
interface Entry {
  seq: number;
  [key: string]: unknown;
}
interface Policy {
  policy?: {
    routeMaps?: Record<string, { entries: Entry[] }>;
    prefixLists?: Record<string, { rules: Entry[] }>;
  };
  [key: string]: unknown;
}
export function PolicyUsagePage() {
  const { t } = useTranslation('bfd-redistribution');
  const doc = useCandidate<Policy>('routing');
  const patch = usePatch();
  const perms = usePermissions();
  return (
    <PageHeader title={t('policy')}>
      <PrefixListsTab />
      <RouteMapsTab />
      {patch.isError && <ProblemAlert error={patch.error} />}
      {policyKinds.map((kind) =>
        Object.entries(doc.data?.policy?.[kind] ?? {}).map(([name, value]) => {
          const field = policyFields[kind];
          const entries: Entry[] = ((value as { entries?: Entry[]; rules?: Entry[] })[field] ?? [])
            .slice()
            .sort((a, b) => a.seq - b.seq);
          return (
            <Box key={`${kind}/${name}`}>
              <Typography variant="h6">{name}</Typography>
              {entries.map((entry, i) => (
                <Box key={entry.seq}>
                  {entry.seq}
                  <Button
                    disabled={!perms.editConfig || i === 0 || patch.isPending}
                    onClick={() => {
                      const copy = entries.map((e) => ({ ...e }));
                      const before = copy[i - 1]!;
                      const current = copy[i]!;
                      const seq = before.seq;
                      before.seq = current.seq;
                      current.seq = seq;
                      patch.mutate({
                        key: ROUTING,
                        patch: {
                          policy: {
                            [kind]: {
                              [name]: {
                                [field]: copy.sort((a, b) => a.seq - b.seq),
                              },
                            },
                          },
                        },
                      });
                    }}
                  >
                    {t('moveUp')}
                  </Button>
                </Box>
              ))}
              {(kind === ROUTE_MAPS ? policyTargets : []).map((proto) => {
                const cfg = doc.data?.[proto] as
                  { redistribute?: Record<string, { routeMap?: string }> } | undefined;
                return Object.entries(cfg?.redistribute ?? {})
                  .filter(([, o]) => o.routeMap === name)
                  .map(([source]) => (
                    <Button
                      key={`${proto}/${source}`}
                      component={Link}
                      to={
                        proto === 'bgp'
                          ? '/routing/bgp'
                          : proto === 'ospf'
                            ? '/routing/ospf'
                            : '/routing/isis-rip'
                      }
                    >
                      {t('usage', { proto, source })}
                    </Button>
                  ));
              })}
            </Box>
          );
        }),
      )}
    </PageHeader>
  );
}
