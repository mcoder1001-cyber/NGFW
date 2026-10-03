import { serviceText } from '../../../product-text';
import CheckCircleOutlined from '@mui/icons-material/CheckCircleOutlined';
import ErrorOutlineOutlined from '@mui/icons-material/ErrorOutlineOutlined';
import InfoOutlined from '@mui/icons-material/InfoOutlined';
import ReportProblemOutlined from '@mui/icons-material/ReportProblemOutlined';
import Box from '@mui/material/Box';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Chip from '@mui/material/Chip';
import Link from '@mui/material/Link';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { useTheme, type Theme } from '@mui/material/styles';
import type { paths } from '@ngfw/api-client';
import { StatusChip, UI_KIT_NS, useFormatters } from '@ngfw/ui-kit';
import { useTopic } from '@ngfw/ui-kit/ws';
import { useQuery } from '@tanstack/react-query';
import { Suspense, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { linkStatus } from '../../interfaces/model';
import { useIfaceRates } from '../../interfaces/rates';
import { Donut, Legend, RingGauge, Sparkline, TimeChart, type Series } from './charts';
import { dashboardCards } from './cards';
import {
  byteParts,
  engineThread,
  engineWording,
  EVENT_CODES,
  HEALTH,
  type Health,
  HOST_SERIES,
  hostSeries,
  levelOf,
  LINK_STATES,
  pushTraffic,
  SERIES,
  severityColor,
  summarizeInterfaces,
  topInterfaces,
  totals,
  trafficSeries,
  uptimeParts,
  type TrafficPoint,
} from './model';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;
type SystemState = Ok<NonNullable<paths['/api/v1/state/system']['get']>>;
type InterfacesState = Ok<NonNullable<paths['/api/v1/state/interfaces']['get']>>;
type EventsState = Ok<NonNullable<paths['/api/v1/state/events']['get']>>;
type HostState = Ok<NonNullable<paths['/api/v1/state/host']['get']>>;

const NS = 'dashboard';
const POLL_MS = 5_000;

/** `worker.cpu` topic payload (StatsBatch.workerCpu as the API relays it). */
interface WorkerCpuData {
  workers?: { worker: number; name: string; utilizationPct: number; vectorsPerCall: number }[];
}

const CARD_SX = {
  blockSize: '100%',
  borderRadius: 3,
  transition: 'box-shadow 200ms ease',
  '&:hover': { boxShadow: 4 },
} as const;

function Panel({
  title,
  action,
  children,
}: {
  title: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Card variant="outlined" component="section" aria-label={title} sx={CARD_SX}>
      <CardContent
        sx={{
          blockSize: '100%',
          boxSizing: 'border-box',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
          gap={1}
          sx={{ mb: 1.5 }}
          flexWrap="wrap"
        >
          <Typography variant="subtitle1" fontWeight={700} component="h3">
            {title}
          </Typography>
          {action}
        </Stack>
        <Box sx={{ flex: 1 }}>{children}</Box>
      </CardContent>
    </Card>
  );
}

/** Colour of a load level: the chart hue when normal, the status tokens when busy or overloaded. */
function levelColour(
  theme: Theme,
  level: 'normal' | 'warning' | 'critical',
  normal: string,
): string {
  return level === 'critical'
    ? theme.ngfw.status.down
    : level === 'warning'
      ? theme.ngfw.status.degraded
      : normal;
}

/** A health pill on the banner: icon + text on the banner's own dark surface. */
function Pill({ state, label }: { state: Health; label: string }) {
  const Icon =
    state === HEALTH.ok
      ? CheckCircleOutlined
      : state === HEALTH.warn
        ? ReportProblemOutlined
        : ErrorOutlineOutlined;
  return (
    <Stack
      direction="row"
      gap={0.75}
      alignItems="center"
      sx={{
        px: 1.25,
        py: 0.5,
        borderRadius: 999,
        bgcolor: 'rgba(255,255,255,0.14)',
        border: '1px solid rgba(255,255,255,0.22)',
      }}
    >
      <Icon
        fontSize="small"
        sx={{
          color: state === HEALTH.ok ? '#9ff0bf' : state === HEALTH.warn ? '#ffd68a' : '#ffb3b3',
        }}
        aria-hidden
      />
      <Typography variant="body2" fontWeight={600} sx={{ color: '#fff' }}>
        {label}
      </Typography>
    </Stack>
  );
}

/** WEB-dashboard: the landing screen — status banner, device resources, traffic, packet engine, interfaces and events. */
export function DashboardOverview() {
  const { t } = useTranslation([NS, UI_KIT_NS]);
  const theme = useTheme();
  const fmt = useFormatters();
  const mode = theme.palette.mode;
  const colours = SERIES[mode];
  const hostColours = HOST_SERIES[mode];
  const trafficSeriesDef: Series[] = [
    { key: 'rx', label: t('traffic.rx'), color: colours.rx },
    { key: 'tx', label: t('traffic.tx'), color: colours.tx },
  ];
  const hostSeriesDef: Series[] = [
    { key: 'cpu', label: t('resources.cpu'), color: hostColours.cpu },
    { key: 'mem', label: t('resources.mem'), color: hostColours.mem },
  ];

  const system = useQuery({
    queryKey: ['state', 'system'],
    queryFn: async (): Promise<SystemState> => (await call(api.GET('/api/v1/state/system'))).data,
    refetchInterval: POLL_MS,
  });
  const host = useQuery({
    queryKey: ['state', 'host'],
    queryFn: async (): Promise<HostState> => (await call(api.GET('/api/v1/state/host'))).data,
    refetchInterval: POLL_MS,
  });
  const ifaces = useQuery({
    queryKey: ['state', 'interfaces'],
    queryFn: async (): Promise<InterfacesState> =>
      (await call(api.GET('/api/v1/state/interfaces'))).data,
    refetchInterval: POLL_MS * 2,
  });
  const events = useQuery({
    queryKey: ['state', 'events', 'dashboard'],
    queryFn: async (): Promise<EventsState> =>
      (await call(api.GET('/api/v1/state/events', { params: { query: { limit: 6 } } }))).data,
    refetchInterval: POLL_MS * 2,
  });
  const { rates, status: wsStatus } = useIfaceRates();
  const cpu = useTopic<WorkerCpuData>('worker.cpu');

  const [traffic, setTraffic] = useState<TrafficPoint[]>([]);
  useEffect(() => {
    setTraffic((prev) => pushTraffic(prev, rates, Date.now()));
  }, [rates]);

  const sum = totals(rates);
  const items = ifaces.data?.items ?? [];
  const links = summarizeInterfaces(items);
  const logicalName = useMemo(
    () => new Map(items.map((i) => [i.state?.vppName ?? i.name, i.name])),
    [items],
  );
  const top = topInterfaces(rates, 5);
  const topMax = Math.max(...top.map((r) => r.bps), 1);

  const sys = system.data;
  const agent = (sys?.agent ?? {}) as {
    reachable?: boolean;
    vppConnected?: boolean;
    degraded?: boolean;
    vppVersion?: string;
  };
  const apiUp = system.isSuccess;
  const agentState: Health = agent.reachable
    ? agent.degraded
      ? HEALTH.warn
      : HEALTH.ok
    : HEALTH.down;
  const engineState: Health = agent.vppConnected ? HEALTH.ok : HEALTH.down;
  const allOk = apiUp && agentState === HEALTH.ok && engineState === HEALTH.ok && links.down === 0;
  const pending = sys?.pendingCommit != null;

  const bps = (v: number) => fmt.rate(v, 'bps');
  const bytes = (n: number) => {
    const p = byteParts(n);
    return t('units.bytes', {
      value: fmt.number(p.value, { maximumFractionDigits: p.value < 10 ? 1 : 0 }),
      unit: p.unit,
    });
  };
  const pct = (v: number) => fmt.percent(v / 100, 0);
  const h = host.data;
  const up = h ? uptimeParts(h.uptimeSec) : undefined;
  const cpuPct = h?.cpu.usagePct ?? null;
  const memPct = h ? (h.memory.usedBytes / Math.max(h.memory.totalBytes, 1)) * 100 : null;
  const root = h?.disks[0];
  const rootPct = root ? (root.usedBytes / Math.max(root.totalBytes, 1)) * 100 : null;
  const hp = h?.hugepages ?? null;
  const hpUsedPct = hp ? ((hp.total - hp.free) / Math.max(hp.total, 1)) * 100 : null;

  const bannerBg =
    mode === 'light'
      ? 'linear-gradient(120deg, #1f3864 0%, #25508f 55%, #2a78d6 100%)'
      : 'linear-gradient(120deg, #111b2e 0%, #17305a 60%, #1f4a8a 100%)';
  const bannerLine = [
    up
      ? up.d > 0
        ? t('banner.uptimeDays', { d: fmt.integer(up.d), h: fmt.integer(up.h) })
        : t('banner.uptime', { h: fmt.integer(up.h), m: fmt.integer(up.m) })
      : null,
    sys?.runningRevision != null
      ? t('banner.revision', { id: fmt.integer(sys.runningRevision) })
      : null,
    agent.vppVersion ? t('banner.engine', { version: serviceText(agent.vppVersion) }) : null,
  ]
    .filter(Boolean)
    .join(t('banner.separator'));

  return (
    <Stack gap={2.5}>
      {/* banner: overall state, identity, health pills */}
      <Box
        component="section"
        aria-label={t('banner.label')}
        sx={{
          position: 'relative',
          overflow: 'hidden',
          borderRadius: 3,
          px: { xs: 2.5, md: 3.5 },
          py: 3,
          color: '#fff',
          background: bannerBg,
        }}
      >
        <Box
          aria-hidden
          sx={{
            position: 'absolute',
            insetInlineEnd: -60,
            top: -80,
            inlineSize: 280,
            blockSize: 280,
            borderRadius: '50%',
            bgcolor: 'rgba(255,255,255,0.06)',
          }}
        />
        <Box
          aria-hidden
          sx={{
            position: 'absolute',
            insetInlineEnd: 140,
            bottom: -120,
            inlineSize: 220,
            blockSize: 220,
            borderRadius: '50%',
            bgcolor: 'rgba(255,255,255,0.04)',
          }}
        />
        <Stack
          direction={{ xs: 'column', md: 'row' }}
          justifyContent="space-between"
          gap={2}
          sx={{ position: 'relative' }}
        >
          <Box>
            <Typography variant="overline" sx={{ opacity: 0.8, letterSpacing: 1.2 }}>
              {h?.hostname ?? t('banner.appliance')}
            </Typography>
            <Typography variant="h5" component="p" fontWeight={700} data-testid="banner-state">
              {system.isPending
                ? t('health.checking')
                : !apiUp
                  ? t('banner.apiDown')
                  : allOk
                    ? t('banner.ok')
                    : t('banner.attention')}
            </Typography>
            <Typography variant="body2" sx={{ opacity: 0.85, mt: 0.5 }}>
              {bannerLine}
            </Typography>
          </Box>
          <Stack
            direction="row"
            gap={1}
            flexWrap="wrap"
            alignItems="center"
            sx={{ alignSelf: { md: 'center' } }}
          >
            <Pill
              state={apiUp ? HEALTH.ok : HEALTH.down}
              label={
                apiUp
                  ? t('health.apiUp', { version: sys?.api.version ?? '' })
                  : system.isPending
                    ? t('health.checking')
                    : t('health.apiDown')
              }
            />
            <Pill
              state={agentState}
              label={
                agent.reachable
                  ? agent.degraded
                    ? t('health.agentDegraded')
                    : t('health.agentUp')
                  : t('health.agentDown')
              }
            />
            <Pill
              state={engineState}
              label={agent.vppConnected ? t('health.engineUp') : t('health.engineDown')}
            />
            {sys && sys.sync.state !== 'in-sync' && (
              <Pill state={HEALTH.warn} label={t('config.sync', { state: sys.sync.state })} />
            )}
            {pending && <Pill state={HEALTH.warn} label={t('config.pending')} />}
          </Stack>
        </Stack>
      </Box>

      {/* row 1: device CPU, memory, storage, throughput */}
      <Box
        sx={{
          display: 'grid',
          gap: 2,
          gridTemplateColumns: { xs: '1fr', sm: 'repeat(2, 1fr)', lg: 'repeat(4, 1fr)' },
        }}
      >
        <Panel title={t('tiles.cpu')}>
          <Stack direction="row" gap={2} alignItems="center">
            <RingGauge
              value={cpuPct === null ? null : cpuPct / 100}
              color={levelColour(theme, levelOf(cpuPct), hostColours.cpu)}
              label={cpuPct === null ? '—' : pct(cpuPct)}
              sublabel={levelOf(cpuPct) === 'normal' ? undefined : t(`level.${levelOf(cpuPct)}`)}
              ariaLabel={t('tiles.cpu')}
              size={96}
            />
            <Box sx={{ minInlineSize: 0, flex: 1 }}>
              <Typography variant="body2" color="text.secondary">
                {h
                  ? t('cpu.cores', { cores: fmt.integer(h.cpu.cores) })
                  : host.isError
                    ? t('host.unavailable')
                    : t('health.checking')}
              </Typography>
              {h && (
                <Typography variant="body2" color="text.secondary">
                  {t('cpu.load', {
                    l1: fmt.number(h.cpu.load[0], { maximumFractionDigits: 2 }),
                    l5: fmt.number(h.cpu.load[1], { maximumFractionDigits: 2 }),
                  })}
                </Typography>
              )}
              <Box sx={{ mt: 1 }}>
                <Sparkline
                  values={(h?.history ?? []).map((x) => x.cpuPct ?? 0)}
                  color={hostColours.cpu}
                  height={28}
                />
              </Box>
            </Box>
          </Stack>
        </Panel>

        <Panel title={t('tiles.memory')}>
          <Stack direction="row" gap={2} alignItems="center">
            <RingGauge
              value={memPct === null ? null : memPct / 100}
              color={levelColour(theme, levelOf(memPct, 80, 92), hostColours.mem)}
              label={memPct === null ? '—' : pct(memPct)}
              sublabel={memPct === null ? undefined : t('memory.used')}
              ariaLabel={t('tiles.memory')}
              size={96}
            />
            <Box sx={{ minInlineSize: 0, flex: 1 }}>
              <Typography variant="body2" color="text.secondary">
                {h
                  ? t('memory.detail', {
                      used: bytes(h.memory.usedBytes),
                      total: bytes(h.memory.totalBytes),
                    })
                  : host.isError
                    ? t('host.unavailable')
                    : t('health.checking')}
              </Typography>
              {h && (
                <Typography variant="body2" color="text.secondary">
                  {t('memory.available', { available: bytes(h.memory.availableBytes) })}
                </Typography>
              )}
              <Box sx={{ mt: 1 }}>
                <Sparkline
                  values={(h?.history ?? []).map((x) => x.memUsedPct)}
                  color={hostColours.mem}
                  height={28}
                />
              </Box>
            </Box>
          </Stack>
        </Panel>

        <Panel title={t('tiles.storage')}>
          <Stack direction="row" gap={2} alignItems="center">
            <RingGauge
              value={rootPct === null ? null : rootPct / 100}
              color={levelColour(theme, levelOf(rootPct, 80, 92), hostColours.disk)}
              label={rootPct === null ? '—' : pct(rootPct)}
              sublabel={root ? t('memory.used') : undefined}
              ariaLabel={t('tiles.storage')}
              size={96}
            />
            <Stack gap={1} sx={{ minInlineSize: 0, flex: 1 }}>
              {(h?.disks ?? []).map((d) => {
                const p = (d.usedBytes / Math.max(d.totalBytes, 1)) * 100;
                return (
                  <Box key={d.mount}>
                    <Typography
                      variant="body2"
                      fontWeight={600}
                      noWrap
                      sx={{ fontFamily: theme.ngfw.monoFontFamily }}
                    >
                      {d.mount}
                    </Typography>
                    <Box sx={{ blockSize: 6, borderRadius: 3, bgcolor: 'action.hover', my: 0.5 }}>
                      <Box
                        sx={{
                          blockSize: '100%',
                          inlineSize: `${Math.min(100, p)}%`,
                          borderRadius: 3,
                          bgcolor: levelColour(theme, levelOf(p, 80, 92), hostColours.disk),
                        }}
                      />
                    </Box>
                    <Typography variant="caption" color="text.secondary" noWrap component="div">
                      {t('storage.detail', {
                        used: bytes(d.usedBytes),
                        total: bytes(d.totalBytes),
                      })}
                    </Typography>
                  </Box>
                );
              })}
              {!h && (
                <Typography variant="body2" color="text.secondary">
                  {host.isError ? t('host.unavailable') : t('health.checking')}
                </Typography>
              )}
            </Stack>
          </Stack>
        </Panel>

        <Panel title={t('tiles.throughput')}>
          <Typography variant="h4" component="p" fontWeight={700} data-testid="throughput-total">
            {rates.size > 0 ? bps(sum.rxBps + sum.txBps) : '—'}
          </Typography>
          <Stack direction="row" gap={2} sx={{ mt: 0.5 }} flexWrap="wrap">
            {rates.size > 0 ? (
              <>
                <Stack direction="row" gap={0.75} alignItems="center">
                  <Box
                    aria-hidden
                    sx={{ inlineSize: 8, blockSize: 8, borderRadius: '50%', bgcolor: colours.rx }}
                  />
                  <Typography variant="body2" color="text.secondary">
                    {t('throughput.in', { value: bps(sum.rxBps) })}
                  </Typography>
                </Stack>
                <Stack direction="row" gap={0.75} alignItems="center">
                  <Box
                    aria-hidden
                    sx={{ inlineSize: 8, blockSize: 8, borderRadius: '50%', bgcolor: colours.tx }}
                  />
                  <Typography variant="body2" color="text.secondary">
                    {t('throughput.out', { value: bps(sum.txBps) })}
                  </Typography>
                </Stack>
              </>
            ) : (
              <Typography variant="body2" color="text.secondary">
                {t('throughput.waiting')}
              </Typography>
            )}
          </Stack>
          <Box sx={{ mt: 1.5 }}>
            <Sparkline
              values={traffic.slice(-40).map((p) => p.rx + p.tx)}
              color={colours.rx}
              height={40}
            />
          </Box>
        </Panel>
      </Box>

      {/* row 2: traffic + packet engine */}
      <Box sx={{ display: 'grid', gap: 2, gridTemplateColumns: { xs: '1fr', lg: '2fr 1fr' } }}>
        <Panel title={t('traffic.title')} action={<Legend series={trafficSeriesDef} />}>
          <TimeChart
            points={trafficSeries(traffic)}
            series={trafficSeriesDef}
            title={t('traffic.title')}
            formatValue={bps}
            formatTime={(at) => fmt.time(at)}
            timeLabel={t('traffic.time')}
            emptyText={
              wsStatus === 'open'
                ? t('traffic.collecting')
                : t('traffic.noStream', { status: t(`ws.${wsStatus}`, { ns: UI_KIT_NS }) })
            }
          />
        </Panel>
        <Panel
          title={t('engine.title')}
          action={
            <Chip
              size="small"
              variant="outlined"
              color={agent.vppConnected ? 'success' : 'error'}
              label={agent.vppConnected ? t('engine.connected') : t('engine.disconnected')}
            />
          }
        >
          <Stack gap={2}>
            <Stack direction="row" gap={2} alignItems="center">
              <RingGauge
                size={88}
                thickness={9}
                value={hpUsedPct === null ? null : hpUsedPct / 100}
                color={levelColour(theme, levelOf(hpUsedPct, 85, 95), hostColours.cpu)}
                label={hpUsedPct === null ? '—' : pct(hpUsedPct)}
                ariaLabel={t('engine.hugepages')}
              />
              <Box>
                <Typography variant="body2" fontWeight={600}>
                  {t('engine.hugepages')}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  {hp
                    ? t('engine.hugepagesDetail', {
                        used: bytes((hp.total - hp.free) * hp.sizeBytes),
                        total: bytes(hp.total * hp.sizeBytes),
                      })
                    : t('engine.noHugepages')}
                </Typography>
              </Box>
            </Stack>
            <Box>
              <Typography variant="body2" fontWeight={600} sx={{ mb: 1 }}>
                {t('engine.threads')}
              </Typography>
              {cpu.data?.workers && cpu.data.workers.length > 0 ? (
                <Stack gap={1.25} component="ul" sx={{ listStyle: 'none', m: 0, p: 0 }}>
                  {cpu.data.workers.map((w) => {
                    const level = levelOf(w.utilizationPct);
                    const id = engineThread(w.name, w.worker);
                    const name =
                      id.kind === 'main'
                        ? t('engine.main')
                        : id.kind === 'worker'
                          ? t('engine.worker', { n: fmt.integer(id.n) })
                          : serviceText(id.name);
                    return (
                      <Box component="li" key={w.worker}>
                        <Stack direction="row" justifyContent="space-between">
                          <Typography variant="body2">{name}</Typography>
                          <Typography variant="body2" fontWeight={600}>
                            {level === 'normal'
                              ? pct(w.utilizationPct)
                              : t('engine.threadLevel', {
                                  value: pct(w.utilizationPct),
                                  level: t(`level.${level}`),
                                })}
                          </Typography>
                        </Stack>
                        <Box
                          sx={{ blockSize: 6, borderRadius: 3, bgcolor: 'action.hover', mt: 0.5 }}
                        >
                          <Box
                            sx={{
                              blockSize: '100%',
                              inlineSize: `${Math.min(100, Math.max(0, w.utilizationPct))}%`,
                              borderRadius: 3,
                              bgcolor: levelColour(theme, level, colours.rx),
                            }}
                          />
                        </Box>
                      </Box>
                    );
                  })}
                </Stack>
              ) : (
                <Typography variant="body2" color="text.secondary">
                  {t('engine.noThreads')}
                </Typography>
              )}
            </Box>
          </Stack>
        </Panel>
      </Box>

      {/* row 3: device resource history + interface status */}
      <Box sx={{ display: 'grid', gap: 2, gridTemplateColumns: { xs: '1fr', lg: '2fr 1fr' } }}>
        <Panel title={t('resources.title')} action={<Legend series={hostSeriesDef} />}>
          <TimeChart
            points={hostSeries(h?.history ?? [])}
            series={hostSeriesDef}
            title={t('resources.title')}
            yMax={100}
            height={200}
            formatValue={pct}
            formatTime={(at) => fmt.time(at)}
            timeLabel={t('traffic.time')}
            emptyText={host.isError ? t('host.unavailable') : t('resources.collecting')}
          />
        </Panel>
        <Panel
          title={t('interfaces.title')}
          action={
            <Link component={RouterLink} to="/interfaces" variant="body2">
              {t('interfaces.open')}
            </Link>
          }
        >
          <Stack direction="row" gap={2.5} alignItems="center">
            <Donut
              segments={LINK_STATES.map((k) => ({
                key: k,
                value: links[k],
                color: theme.ngfw.status[k],
              }))}
              ariaLabel={t('interfaces.breakdown', {
                up: links.up,
                down: links.down,
                adminDown: links.adminDown,
                missing: links.missing,
              })}
              center={
                <>
                  <Typography variant="h5" component="span" fontWeight={700} lineHeight={1}>
                    {ifaces.isSuccess ? fmt.integer(links.up) : '—'}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {ifaces.isSuccess
                      ? t('interfaces.ofTotal', { total: fmt.integer(links.total) })
                      : ''}
                  </Typography>
                </>
              }
            />
            <Stack gap={1} component="ul" sx={{ listStyle: 'none', m: 0, p: 0 }}>
              {LINK_STATES.map((k) => (
                <Stack key={k} component="li" direction="row" gap={1} alignItems="center">
                  <Box
                    aria-hidden
                    sx={{
                      inlineSize: 10,
                      blockSize: 10,
                      borderRadius: '50%',
                      bgcolor: theme.ngfw.status[k],
                    }}
                  />
                  <Typography variant="body2">{t(`status.${k}`, { ns: UI_KIT_NS })}</Typography>
                  <Typography variant="body2" fontWeight={700}>
                    {fmt.integer(links[k])}
                  </Typography>
                </Stack>
              ))}
              {links.missing > 0 && (
                <Typography component="li" variant="caption" color="text.secondary">
                  {t('interfaces.missing', { n: fmt.integer(links.missing) })}
                </Typography>
              )}
            </Stack>
          </Stack>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
            {ifaces.isSuccess
              ? t('interfaces.upOfTotal', {
                  up: fmt.integer(links.up),
                  total: fmt.integer(links.total),
                })
              : ''}
          </Typography>
        </Panel>
      </Box>

      {/* row 4: busiest interfaces + recent events */}
      <Box sx={{ display: 'grid', gap: 2, gridTemplateColumns: { xs: '1fr', lg: '1fr 1fr' } }}>
        <Panel title={t('top.title')}>
          {top.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              {t('top.none')}
            </Typography>
          ) : (
            <Stack gap={1.5} component="ol" sx={{ listStyle: 'none', m: 0, p: 0 }}>
              {top.map(({ name, rate, bps: total }) => {
                const st = items.find((i) => (i.state?.vppName ?? i.name) === name)?.state;
                return (
                  <Box component="li" key={name}>
                    <Stack
                      direction="row"
                      justifyContent="space-between"
                      alignItems="center"
                      gap={1}
                    >
                      <Stack direction="row" gap={1} alignItems="center" sx={{ minInlineSize: 0 }}>
                        <Typography
                          variant="body2"
                          noWrap
                          sx={{ fontFamily: theme.ngfw.monoFontFamily }}
                        >
                          {logicalName.get(name) ?? name}
                        </Typography>
                        {st && <StatusChip status={linkStatus(st)!} />}
                      </Stack>
                      <Typography variant="body2" fontWeight={700} sx={{ whiteSpace: 'nowrap' }}>
                        {bps(total)}
                      </Typography>
                    </Stack>
                    <Box
                      sx={{
                        display: 'flex',
                        gap: '2px',
                        blockSize: 8,
                        mt: 0.5,
                        inlineSize: `${Math.max(2, (total / topMax) * 100)}%`,
                      }}
                      dir="ltr"
                    >
                      <Box
                        sx={{
                          flexGrow: rate.rxBps || 0.0001,
                          bgcolor: colours.rx,
                          borderRadius: '4px 0 0 4px',
                        }}
                      />
                      <Box
                        sx={{
                          flexGrow: rate.txBps || 0.0001,
                          bgcolor: colours.tx,
                          borderRadius: '0 4px 4px 0',
                        }}
                      />
                    </Box>
                  </Box>
                );
              })}
            </Stack>
          )}
        </Panel>
        <Panel title={t('events.title')}>
          {(events.data?.items ?? []).length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              {events.isError
                ? t('events.error')
                : events.isPending
                  ? t('health.checking')
                  : t('events.none')}
            </Typography>
          ) : (
            <Stack component="ul" sx={{ listStyle: 'none', m: 0, p: 0 }}>
              {events.data!.items.map((e, i, all) => {
                const c = severityColor(e.severity);
                const Icon =
                  c === 'error'
                    ? ErrorOutlineOutlined
                    : c === 'warning'
                      ? ReportProblemOutlined
                      : c === 'success'
                        ? CheckCircleOutlined
                        : InfoOutlined;
                return (
                  <Stack
                    component="li"
                    key={e.id}
                    direction="row"
                    gap={1.5}
                    alignItems="flex-start"
                    sx={{ pb: i === all.length - 1 ? 0 : 1.75 }}
                  >
                    <Box
                      sx={{
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        alignSelf: 'stretch',
                      }}
                    >
                      <Icon fontSize="small" color={c} aria-label={e.severity} />
                      {i < all.length - 1 && (
                        <Box
                          aria-hidden
                          sx={{ flex: 1, inlineSize: '1px', bgcolor: 'divider', mt: 0.5 }}
                        />
                      )}
                    </Box>
                    <Box sx={{ minInlineSize: 0 }}>
                      <Typography variant="body2">
                        {EVENT_CODES.has(e.code)
                          ? t(`events.codes.${e.code}`)
                          : serviceText(engineWording(e.message, t('engine.word')))}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {t('events.meta', {
                          when: fmt.relative(e.ts),
                          subsystem: serviceText(e.subsystem),
                        })}
                      </Typography>
                    </Box>
                  </Stack>
                );
              })}
            </Stack>
          )}
        </Panel>
      </Box>

      {/* row 5: cards contributed by features (alarms, tunnels…) — see ./cards.ts */}
      {dashboardCards.length > 0 && (
        <Box sx={{ display: 'grid', gap: 2, gridTemplateColumns: { xs: '1fr', lg: '1fr 1fr' } }}>
          {dashboardCards.map(({ id, Component }) => (
            <Suspense key={id} fallback={null}>
              <Component />
            </Suspense>
          ))}
        </Box>
      )}
    </Stack>
  );
}
