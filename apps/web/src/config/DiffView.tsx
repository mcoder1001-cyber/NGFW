import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import RemoveIcon from '@mui/icons-material/Remove';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { parsePointer } from '@ngfw/schema';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { refineChanges } from './refine';

export interface DiffChange {
  op: 'add' | 'remove' | 'replace';
  pointer: string;
  from?: unknown;
  to?: unknown;
}

const OP_META = {
  add: { color: 'success', Icon: AddIcon, key: 'add' },
  remove: { color: 'error', Icon: RemoveIcon, key: 'remove' },
  replace: { color: 'warning', Icon: EditIcon, key: 'replace' },
} as const;

const OP_ORDER = ['add', 'replace', 'remove'] as const;

/** Top-level configuration domain of a pointer (`/interfaces/loop101/mtu` → `interfaces`). */
export function domainOf(pointer: string): string {
  try {
    return parsePointer(pointer)[0] ?? '';
  } catch {
    return '';
  }
}

export function countOps(changes: readonly DiffChange[]): Record<DiffChange['op'], number> {
  const out = { add: 0, remove: 0, replace: 0 };
  for (const c of changes) out[c.op] += 1;
  return out;
}

function Value({ value, label }: { value: unknown; label: string }) {
  const text = typeof value === 'string' ? JSON.stringify(value) : JSON.stringify(value, null, 2);
  const multiline = text !== undefined && text.includes('\n');
  return (
    <Box
      component={multiline ? 'pre' : 'code'}
      aria-label={label}
      dir="ltr"
      sx={{
        fontFamily: (t) => t.vrx.monoFontFamily,
        fontSize: '0.8125rem',
        m: 0,
        p: multiline ? 1 : 0,
        bgcolor: multiline ? 'action.hover' : 'transparent',
        borderRadius: 1,
        overflow: 'auto',
        maxBlockSize: 240,
        whiteSpace: 'pre',
        textAlign: 'start',
      }}
    >
      {text ?? '—'}
    </Box>
  );
}

/**
 * Structured diff (added / removed / changed, each with its RFC 6901 pointer) grouped by configuration domain. Whole-list
 * replacements are refined into per-item changes on the schema's item key (`refineChanges`). Values
 * come from the API already redacted (D-070): write-only members such as password hashes never appear.
 */
export function DiffView({ changes: raw, dense = false }: { changes: readonly DiffChange[]; dense?: boolean }) {
  const { t } = useTranslation(['config', 'nav']);
  const changes = useMemo(() => refineChanges(raw), [raw]);
  const groups = useMemo(() => {
    const m = new Map<string, DiffChange[]>();
    for (const c of changes) {
      const d = domainOf(c.pointer);
      m.set(d, [...(m.get(d) ?? []), c]);
    }
    return [...m.entries()];
  }, [changes]);
  const counts = countOps(changes);

  if (changes.length === 0) return <Typography color="text.secondary">{t('diff.empty')}</Typography>;

  return (
    <Stack gap={2}>
      <Stack direction="row" gap={1} flexWrap="wrap" aria-label={t('diff.summary')}>
        {OP_ORDER.map((op) => {
          const { color, Icon, key } = OP_META[op];
          return <Chip key={op} size="small" color={color} variant="outlined" icon={<Icon />} label={t(`diff.count.${key}`, { count: counts[op] })} />;
        })}
      </Stack>
      {groups.map(([domain, list]) => (
        <Box key={domain} component="section" aria-label={t(`nav:domains.${domain}`, { defaultValue: domain })}>
          <Typography component="h3" variant="subtitle2" sx={{ mb: 1 }}>
            {t(`nav:domains.${domain}`, { defaultValue: domain })}
          </Typography>
          <Stack component="ul" gap={1} sx={{ listStyle: 'none', m: 0, p: 0 }}>
            {list.map((c) => {
              const { color, Icon, key } = OP_META[c.op];
              return (
                <Box
                  component="li"
                  key={`${c.op}:${c.pointer}`}
                  sx={{
                    borderInlineStart: 4,
                    borderColor: `${color}.main`,
                    paddingInlineStart: 1.5,
                    py: dense ? 0.25 : 0.5,
                  }}
                >
                  <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap">
                    <Chip size="small" color={color} icon={<Icon />} label={t(`diff.op.${key}`)} />
                    <Box component="code" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily, fontSize: '0.8125rem', wordBreak: 'break-all' }}>
                      {c.pointer}
                    </Box>
                  </Stack>
                  {c.op !== 'add' && (
                    <Stack direction="row" gap={1} alignItems="flex-start" sx={{ mt: 0.5 }}>
                      <Typography variant="caption" color="text.secondary" sx={{ minInlineSize: 48 }}>
                        {t('diff.from')}
                      </Typography>
                      <Value value={c.from} label={t('diff.from')} />
                    </Stack>
                  )}
                  {c.op !== 'remove' && (
                    <Stack direction="row" gap={1} alignItems="flex-start" sx={{ mt: 0.5 }}>
                      <Typography variant="caption" color="text.secondary" sx={{ minInlineSize: 48 }}>
                        {t('diff.to')}
                      </Typography>
                      <Value value={c.to} label={t('diff.to')} />
                    </Stack>
                  )}
                </Box>
              );
            })}
          </Stack>
        </Box>
      ))}
    </Stack>
  );
}
