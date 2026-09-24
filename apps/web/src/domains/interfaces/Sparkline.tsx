import { useTheme } from '@mui/material/styles';

/** Tiny inline SVG trend line (no chart dependency); flips with the reading direction. */
export function Sparkline({ values, width = 72, height = 18, label }: { values: readonly number[]; width?: number; height?: number; label: string }) {
  const theme = useTheme();
  if (values.length < 2) return <svg width={width} height={height} role="img" aria-label={label} />;
  const max = Math.max(...values, 1);
  const step = width / (values.length - 1);
  const pts = values.map((v, i) => {
    const x = theme.direction === 'rtl' ? width - i * step : i * step;
    const y = height - 1 - (v / max) * (height - 2);
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });
  return (
    <svg width={width} height={height} role="img" aria-label={label}>
      <polyline points={pts.join(' ')} fill="none" stroke={theme.palette.primary.main} strokeWidth={1.5} />
    </svg>
  );
}
