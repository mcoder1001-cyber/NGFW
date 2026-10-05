import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useTopic } from '@ngfw/ui-kit/ws';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import {
  Alert,
  Box,
  Button,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { BfdPage } from '../bfd/BfdPage';
import { ProtocolForm } from '../ospf/ProtocolForm';
import { PrefixListsTab, RouteMapsTab } from '../bgp/PolicyTab';
import { useCandidate, usePatch } from '../bgp/api';
import { usePermissions } from '../../../auth/AuthProvider';
import { PageHeader } from '../../../shell/PageHeader';

const sessionColumns = ['engine', 'interface', 'peer', 'state', 'timers'];
const policyKinds = ['routeMaps', 'prefixLists'] as const;
const policyFields = { routeMaps: 'entries', prefixLists: 'rules' } as const;
const policyTargets = ['bgp', 'ospf', 'isis', 'rip'];
const ROUTE_MAPS = 'routeMaps';
const ROUTING = 'routing';
export function BfdRedistributionPage() {
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
      <Table size="small">
        <TableHead>
          <TableRow>
            {sessionColumns.map((k) => (
              <TableCell key={k}>{t(k)}</TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {live.data?.sessions.map((s) => (
            <TableRow key={`${s.engine}/${s.interface}/${s.peerAddress}`}>
              <TableCell>{s.engine}</TableCell>
              <TableCell>{s.interface}</TableCell>
              <TableCell dir="ltr">{s.peerAddress}</TableCell>
              <TableCell>{t(`states.${s.state}`, { defaultValue: t('states.unknown') })}</TableCell>
              <TableCell dir="ltr">
                {t('timerValue', {
                  tx: s.desiredMinTxUs,
                  rx: s.requiredMinRxUs,
                  multiplier: s.detectMultiplier,
                })}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
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
      <Table>
        <TableHead>
          <TableRow>
            <TableCell>{t('target')}</TableCell>
            {sources.map((s) => (
              <TableCell key={s}>{s}</TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {targets.map((target) => (
            <TableRow key={target}>
              <TableCell>{target}</TableCell>
              {sources.map((source) => {
                const edge = live.data?.edges.find(
                  (e) => e.source === source && e.target === target,
                );
                return (
                  <TableCell key={source}>
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
                  </TableCell>
                );
              })}
            </TableRow>
          ))}
        </TableBody>
      </Table>
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
                            [kind]: { [name]: { [field]: copy.sort((a, b) => a.seq - b.seq) } },
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
