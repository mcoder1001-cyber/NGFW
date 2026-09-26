import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { useTheme } from '@mui/material/styles';
import { useId, useState, type MouseEvent, type ReactNode } from 'react';
import { niceTicks } from './model';

/**
 * Inline-SVG charts of the dashboard (no chart library in the bundle). Mark specs: 2 px lines with round joins,
 * a light area wash, hairline solid gridlines, ≥ 8 px hover markers with a 2 px surface ring, one y axis, 2 px surface
 * gaps between donut segments. Plots keep left-to-right time in RTL too; labels stay in text colours.
 */

/** Screen-reader-only (the table view of a chart). */
const visuallyHidden = {
  border: 0,
  clip: 'rect(0 0 0 0)',
  height: 1,
  margin: -1,
  overflow: 'hidden',
  padding: 0,
  position: 'absolute',
  whiteSpace: 'nowrap',
  width: 1,
} as const;

export interface Series {
  key: string;
  label: string;
  color: string;
}

/** One sample of a time chart: `values[series.key]`, null = no value at that instant (the line breaks). */
export interface TimePoint {
  at: number;
  values: Record<string, number | null>;
}

/** Legend: a short line key beside each label (identity is never colour alone). */
export function Legend({ series }: { series: readonly Series[] }) {
  return (
    <Stack
      direction="row"
      gap={2}
      component="ul"
      sx={{ listStyle: 'none', m: 0, p: 0 }}
      flexWrap="wrap"
    >
      {series.map((s) => (
        <Stack key={s.key} direction="row" gap={0.75} alignItems="center" component="li">
          <Box
            aria-hidden
            sx={{ inlineSize: 14, blockSize: 3, borderRadius: 2, bgcolor: s.color }}
          />
          <Typography variant="caption" color="text.secondary">
            {s.label}
          </Typography>
        </Stack>
      ))}
    </Stack>
  );
}

const W = 640;
const PAD = { top: 12, right: 12, bottom: 24, left: 64 }; // logical-css-ignore: SVG plot coordinates, time runs left to right in RTL too

export interface TimeChartProps {
  points: readonly TimePoint[];
  series: readonly Series[];
  /** Accessible name of the chart (the card title). */
  title: string;
  formatValue: (v: number) => string;
  formatTime: (at: number) => string;
  /** Text shown in the plot while there are fewer than two points. */
  emptyText: string;
  /** Header of the time column of the table view. */
  timeLabel: string;
  /** Fixed top of the y axis (e.g. 100 for percentages); default: rounded up from the data. */
  yMax?: number;
  height?: number;
}

/** Values over time, with a crosshair + tooltip on hover and a hidden table view. */
export function TimeChart({
  points,
  series,
  title,
  formatValue,
  formatTime,
  emptyText,
  timeLabel,
  yMax: fixedMax,
  height = 220,
}: TimeChartProps) {
  const theme = useTheme();
  const clip = useId();
  const [hover, setHover] = useState<number | null>(null);
  const H = height;
  const grid = theme.palette.divider;
  const ink = theme.palette.text.secondary;
  const surface = theme.palette.background.paper;

  if (points.length < 2) {
    return (
      <Box
        sx={{
          blockSize: H,
          display: 'grid',
          placeItems: 'center',
          border: 1,
          borderColor: 'divider',
          borderRadius: 2,
          borderStyle: 'dashed',
        }}
      >
        <Typography variant="body2" color="text.secondary" sx={{ px: 2, textAlign: 'center' }}>
          {emptyText}
        </Typography>
      </Box>
    );
  }

  const t0 = points[0]!.at;
  const t1 = points[points.length - 1]!.at;
  const span = Math.max(t1 - t0, 1);
  const peak = Math.max(1e-9, ...points.flatMap((p) => series.map((s) => p.values[s.key] ?? 0)));
  const ticks = niceTicks(fixedMax ?? peak);
  const yMax = ticks[ticks.length - 1]!;
  const x = (at: number) => PAD.left + ((at - t0) / span) * (W - PAD.left - PAD.right);
  const y = (v: number) => H - PAD.bottom - (Math.min(v, yMax) / yMax) * (H - PAD.top - PAD.bottom);
  const base = y(0);
  /** Path segments of one series; a null value starts a new segment. */
  const segments = (k: string) => {
    const segs: { d: string; first: number; last: number }[] = [];
    let cur: { d: string; first: number; last: number } | undefined;
    for (const p of points) {
      const v = p.values[k];
      if (v === null || v === undefined) {
        cur = undefined;
        continue;
      }
      const cmd = `${x(p.at).toFixed(1)},${y(v).toFixed(1)}`;
      if (!cur) segs.push((cur = { d: `M${cmd}`, first: p.at, last: p.at }));
      else {
        cur.d += `L${cmd}`;
        cur.last = p.at;
      }
    }
    return segs;
  };

  const onMove = (e: MouseEvent<SVGRectElement>) => {
    const box = e.currentTarget.getBoundingClientRect();
    const at = t0 + ((e.clientX - box.left) / box.width) * span;
    let best = 0;
    for (let i = 1; i < points.length; i += 1)
      if (Math.abs(points[i]!.at - at) < Math.abs(points[best]!.at - at)) best = i;
    setHover(best);
  };
  const hp = hover === null ? undefined : points[hover];
  const tipLeftPct = hp ? (x(hp.at) / W) * 100 : 0;

  return (
    <Box sx={{ position: 'relative' }} dir="ltr">
      <svg
        viewBox={`0 0 ${W} ${H}`}
        width="100%"
        role="img"
        aria-label={title}
        style={{ display: 'block', overflow: 'visible' }}
      >
        <defs>
          <clipPath id={clip}>
            <rect
              x={PAD.left}
              y={PAD.top - 6}
              width={W - PAD.left - PAD.right}
              height={H - PAD.top - PAD.bottom + 6}
            />
          </clipPath>
          {series.map((s) => (
            <linearGradient key={s.key} id={`${clip}-${s.key}`} x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stopColor={s.color} stopOpacity={0.22} />
              <stop offset="100%" stopColor={s.color} stopOpacity={0.02} />
            </linearGradient>
          ))}
        </defs>
        {ticks.map((v) => (
          <g key={v}>
            <line
              x1={PAD.left}
              x2={W - PAD.right}
              y1={y(v)}
              y2={y(v)}
              stroke={grid}
              strokeWidth={1}
            />
            <text
              x={PAD.left - 8}
              y={y(v)}
              fill={ink}
              fontSize={11}
              textAnchor="end"
              dominantBaseline="middle"
              style={{ fontVariantNumeric: 'tabular-nums' }}
            >
              {formatValue(v)}
            </text>
          </g>
        ))}
        <text x={PAD.left} y={H - 6} fill={ink} fontSize={11} textAnchor="start">
          {formatTime(t0)}
        </text>
        <text x={W - PAD.right} y={H - 6} fill={ink} fontSize={11} textAnchor="end">
          {formatTime(t1)}
        </text>
        <g clipPath={`url(#${clip})`}>
          {series.flatMap((s) =>
            segments(s.key).map((seg, i) => (
              <path
                key={`a-${s.key}-${i}`}
                d={`${seg.d}L${x(seg.last).toFixed(1)},${base}L${x(seg.first).toFixed(1)},${base}Z`}
                fill={`url(#${clip}-${s.key})`}
              />
            )),
          )}
          {series.flatMap((s) =>
            segments(s.key).map((seg, i) => (
              <path
                key={`l-${s.key}-${i}`}
                d={seg.d}
                fill="none"
                stroke={s.color}
                strokeWidth={2}
                strokeLinejoin="round"
                strokeLinecap="round"
              />
            )),
          )}
        </g>
        {hp && (
          <g pointerEvents="none">
            <line x1={x(hp.at)} x2={x(hp.at)} y1={PAD.top} y2={base} stroke={ink} strokeWidth={1} />
            {series.map((s) => {
              const v = hp.values[s.key];
              return v === null || v === undefined ? null : (
                <circle
                  key={`h-${s.key}`}
                  cx={x(hp.at)}
                  cy={y(v)}
                  r={4.5}
                  fill={s.color}
                  stroke={surface}
                  strokeWidth={2}
                />
              );
            })}
          </g>
        )}
        <rect
          x={PAD.left}
          y={PAD.top}
          width={W - PAD.left - PAD.right}
          height={H - PAD.top - PAD.bottom}
          fill="transparent"
          onMouseMove={onMove}
          onMouseLeave={() => setHover(null)}
          data-testid="chart-hit"
        />
      </svg>
      {hp && (
        <Paper
          role="tooltip"
          elevation={4}
          // plot coordinates are always left-to-right: an inline style is not mirrored by the RTL style plugin (sx would be)
          style={{
            position: 'absolute',
            top: 4,
            left: `${tipLeftPct}%`, // logical-css-ignore: positioned in the LTR plot's own coordinates
            transform: tipLeftPct > 60 ? 'translateX(calc(-100% - 12px))' : 'translateX(12px)',
          }}
          sx={{ px: 1.25, py: 0.75, pointerEvents: 'none', minInlineSize: 150, borderRadius: 2 }}
        >
          <Typography variant="caption" color="text.secondary" component="div">
            {formatTime(hp.at)}
          </Typography>
          {series.map((s) => {
            const v = hp.values[s.key];
            return (
              <Stack
                key={s.key}
                direction="row"
                gap={1}
                alignItems="center"
                justifyContent="space-between"
              >
                <Stack direction="row" gap={0.75} alignItems="center">
                  <Box
                    aria-hidden
                    sx={{ inlineSize: 8, blockSize: 8, borderRadius: '50%', bgcolor: s.color }}
                  />
                  <Typography variant="caption">{s.label}</Typography>
                </Stack>
                <Typography
                  variant="caption"
                  fontWeight={600}
                  sx={{ fontVariantNumeric: 'tabular-nums' }}
                >
                  {v === null || v === undefined ? '—' : formatValue(v)}
                </Typography>
              </Stack>
            );
          })}
        </Paper>
      )}
      <table style={visuallyHidden}>
        <caption>{title}</caption>
        <thead>
          <tr>
            <th scope="col">{timeLabel}</th>
            {series.map((s) => (
              <th key={s.key} scope="col">
                {s.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {points.slice(-10).map((p) => (
            <tr key={p.at}>
              <th scope="row">{formatTime(p.at)}</th>
              {series.map((s) => {
                const v = p.values[s.key];
                return <td key={s.key}>{v === null || v === undefined ? '—' : formatValue(v)}</td>;
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </Box>
  );
}

/** A small trend line (decorative: the tile states the value in text). */
export function Sparkline({
  values,
  color,
  width = 120,
  height = 32,
}: {
  values: readonly number[];
  color: string;
  width?: number;
  height?: number;
}) {
  const id = useId();
  if (values.length < 2) return <Box sx={{ blockSize: height }} />;
  const max = Math.max(...values, 1e-9);
  const pts = values.map(
    (v, i) =>
      [(i / (values.length - 1)) * (width - 4) + 2, height - 3 - (v / max) * (height - 6)] as const,
  );
  const d = pts
    .map(([px, py], i) => `${i === 0 ? 'M' : 'L'}${px.toFixed(1)},${py.toFixed(1)}`)
    .join('');
  const [ex] = pts[pts.length - 1]!;
  return (
    <svg
      width="100%"
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      aria-hidden
      style={{ display: 'block' }}
    >
      <defs>
        <linearGradient id={id} x1="0" x2="0" y1="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.28} />
          <stop offset="100%" stopColor={color} stopOpacity={0} />
        </linearGradient>
      </defs>
      <path d={`${d}L${ex},${height}L2,${height}Z`} fill={`url(#${id})`} />
      <path
        d={d}
        fill="none"
        stroke={color}
        strokeWidth={2}
        strokeLinejoin="round"
        strokeLinecap="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}

/** Ring gauge: `value` 0..1 as an arc; the centre shows the figure as text (colour is never the only channel). */
export function RingGauge({
  value,
  color,
  label,
  sublabel,
  ariaLabel,
  size = 112,
  thickness = 10,
}: {
  value: number | null;
  color: string;
  label: ReactNode;
  sublabel?: ReactNode;
  ariaLabel: string;
  size?: number;
  thickness?: number;
}) {
  const theme = useTheme();
  const r = (size - thickness) / 2;
  const c = 2 * Math.PI * r;
  const v = value === null ? 0 : Math.min(1, Math.max(0, value));
  return (
    <Box
      role="img"
      aria-label={ariaLabel}
      sx={{ position: 'relative', inlineSize: size, blockSize: size, flexShrink: 0 }}
    >
      <svg
        width={size}
        height={size}
        viewBox={`0 0 ${size} ${size}`}
        aria-hidden
        style={{ display: 'block', transform: 'rotate(-90deg)' }}
      >
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={theme.palette.action.hover}
          strokeWidth={thickness}
        />
        {value !== null && (
          <circle
            cx={size / 2}
            cy={size / 2}
            r={r}
            fill="none"
            stroke={color}
            strokeWidth={thickness}
            strokeLinecap="round"
            strokeDasharray={`${Math.max(v * c, 0.001)} ${c}`}
            style={{ transition: 'stroke-dasharray 600ms ease' }}
          />
        )}
      </svg>
      <Stack sx={{ position: 'absolute', inset: 0 }} alignItems="center" justifyContent="center">
        <Typography variant="h6" component="span" fontWeight={700} lineHeight={1.1}>
          {label}
        </Typography>
        {sublabel && (
          <Typography variant="caption" color="text.secondary" lineHeight={1.2}>
            {sublabel}
          </Typography>
        )}
      </Stack>
    </Box>
  );
}

export interface DonutSegment {
  key: string;
  value: number;
  color: string;
}

/** Donut of parts of a whole, 2 px surface gaps between segments; the centre carries the headline figure. */
export function Donut({
  segments,
  ariaLabel,
  center,
  size = 112,
  thickness = 14,
}: {
  segments: readonly DonutSegment[];
  ariaLabel: string;
  center: ReactNode;
  size?: number;
  thickness?: number;
}) {
  const theme = useTheme();
  const r = (size - thickness) / 2;
  const c = 2 * Math.PI * r;
  const total = segments.reduce((a, s) => a + s.value, 0);
  const visible = segments.filter((s) => s.value > 0);
  const gap = visible.length > 1 ? 2 : 0;
  let offset = 0;
  return (
    <Box
      role="img"
      aria-label={ariaLabel}
      sx={{ position: 'relative', inlineSize: size, blockSize: size, flexShrink: 0 }}
    >
      <svg
        width={size}
        height={size}
        viewBox={`0 0 ${size} ${size}`}
        aria-hidden
        style={{ display: 'block', transform: 'rotate(-90deg)' }}
      >
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={theme.palette.action.hover}
          strokeWidth={thickness}
        />
        {total > 0 &&
          visible.map((s) => {
            const len = (s.value / total) * c;
            const el = (
              <circle
                key={s.key}
                cx={size / 2}
                cy={size / 2}
                r={r}
                fill="none"
                stroke={s.color}
                strokeWidth={thickness}
                strokeDasharray={`${Math.max(len - gap, 0.001)} ${c}`}
                strokeDashoffset={-offset}
              />
            );
            offset += len;
            return el;
          })}
      </svg>
      <Stack sx={{ position: 'absolute', inset: 0 }} alignItems="center" justifyContent="center">
        {center}
      </Stack>
    </Box>
  );
}
