import { serviceText } from '../../../product-text';
import { PgCloseIcon as CloseIcon } from '@amnafzar/common-ui-lib/icons';
import { PgAlert as Alert } from '@amnafzar/common-ui-lib/primitives';
import { PgBox as Box } from '@amnafzar/common-ui-lib/primitives';
import { PgChip as Chip } from '@amnafzar/common-ui-lib/primitives';
import { PgDrawer as Drawer } from '@amnafzar/common-ui-lib/primitives';
import { PgIconButton as IconButton } from '@amnafzar/common-ui-lib/primitives';
import { PgLinearProgress as LinearProgress } from '@amnafzar/common-ui-lib/primitives';
import { PgList as List } from '@amnafzar/common-ui-lib/primitives';
import { PgListItem as ListItem } from '@amnafzar/common-ui-lib/primitives';
import { PgListItemText as ListItemText } from '@amnafzar/common-ui-lib/primitives';
import { PgStack as Stack } from '@amnafzar/common-ui-lib/primitives';
import { PgToggleButton as ToggleButton } from '@amnafzar/common-ui-lib/primitives';
import { PgToggleButtonGroup as ToggleButtonGroup } from '@amnafzar/common-ui-lib/primitives';
import { PgTooltip as Tooltip } from '@amnafzar/common-ui-lib/primitives';
import { PgTypography as Typography } from '@amnafzar/common-ui-lib/primitives';
import { useFormatters } from '@ngfw/ui-kit';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { contrastText, type FqdnItem, type ObjectsConfig } from './model';

const SOURCES = ['candidate', 'running'] as const;
import { useUsage } from './queries';

/** Tag chips in the tag's colour (text colour by contrast). */
export function TagChips({ tags, objects }: { tags: readonly string[]; objects: Partial<ObjectsConfig> | undefined }) {
  if (tags.length === 0) return null;
  return (
    <Stack direction="row" gap={0.5} flexWrap="wrap">
      {tags.map((tag) => {
        const color = objects?.tags?.[tag]?.color;
        return (
          <Chip
            key={tag}
            size="small"
            label={<bdi>{tag}</bdi>}
            variant={color ? 'filled' : 'outlined'}
            sx={color ? { bgcolor: color, color: contrastText(color) } : undefined}
          />
        );
      })}
    </Stack>
  );
}

/** FQDN resolution column: addresses in use, when they were resolved, the latest error (last-good kept). */
export function FqdnCell({ item, applied, unavailable }: { item: FqdnItem | undefined; applied: boolean; unavailable: boolean }) {
  const { t } = useTranslation('object-model');
  const fmt = useFormatters();
  if (unavailable) return <Typography variant="body2" color="text.secondary">{t('fqdn.unavailable')}</Typography>;
  if (!item) return <Typography variant="body2" color="text.secondary">{applied ? t('fqdn.notResolved') : t('fqdn.notApplied')}</Typography>;
  const when = item.lastResolved ? t('fqdn.resolvedAgo', { when: fmt.relative(item.lastResolved) }) : t('fqdn.notResolved');
  return (
    <Stack gap={0.25}>
      {item.addresses.length > 0 && (
        <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.ngfw.monoFontFamily, fontSize: 12, textAlign: 'start' }}>
          {item.addresses.join(' ')}
        </Box>
      )}
      <Stack direction="row" gap={0.5} alignItems="center" flexWrap="wrap">
        <Typography variant="caption" color="text.secondary">
          {when}
        </Typography>
        {item.error && (
          <Tooltip title={<bdi dir="ltr">{serviceText(item.error)}</bdi>}>
            <Chip
              size="small"
              color={item.addresses.length > 0 ? 'warning' : 'error'}
              variant="outlined"
              label={item.addresses.length > 0 ? t('fqdn.keptLastGood') : t('fqdn.failed')}
            />
          </Tooltip>
        )}
      </Stack>
    </Stack>
  );
}

/** Where-used drawer: `/api/v1/state/objects/usage` for one name, running or candidate. */
export function UsageDrawer({ name, onClose }: { name: string | null; onClose: () => void }) {
  return (
    <Drawer anchor="right" open={name !== null} onClose={onClose} sx={{ zIndex: (th) => th.zIndex.modal }} slotProps={{ paper: { sx: { inlineSize: { xs: '100%', md: 520 } } } }}>
      {name !== null && <UsageBody key={name} name={name} onClose={onClose} />}
    </Drawer>
  );
}

function UsageBody({ name, onClose }: { name: string; onClose: () => void }) {
  const { t } = useTranslation('object-model');
  const [source, setSource] = useState<'running' | 'candidate'>('candidate');
  const usage = useUsage(name, source);
  return (
    <Box sx={{ p: 2 }} role="region" aria-label={t('usage.title', { name })}>
      <Stack direction="row" alignItems="center" gap={1} sx={{ mb: 1 }}>
        <Typography component="h3" variant="h6" sx={{ flex: 1, textAlign: 'start' }}>
          {t('usage.title', { name })}
        </Typography>
        <IconButton aria-label={t('close')} onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </Stack>
      <ToggleButtonGroup size="small" exclusive value={source} onChange={(_e, v: 'running' | 'candidate' | null) => v && setSource(v)} sx={{ mb: 2 }} aria-label={t('usage.sourceLabel')}>
        {SOURCES.map((s) => (
          <ToggleButton key={s} value={s}>
            {t(`usage.source.${s}`)}
          </ToggleButton>
        ))}
      </ToggleButtonGroup>
      {usage.isPending && <LinearProgress aria-label={t('loading')} />}
      {usage.isError && <ProblemAlert error={usage.error} />}
      {usage.data && (
        <>
          <Typography variant="body2" sx={{ mb: 1 }}>
            {usage.data.definedAs.length > 0
              ? t('usage.definedAs', { kinds: usage.data.definedAs.map((k) => t(`kindOne.${k}`)).join(t('listSeparator')) })
              : t('usage.notDefined')}
          </Typography>
          {usage.data.usedBy.length === 0 ? (
            <Alert severity="info">{t('usage.none')}</Alert>
          ) : (
            <List dense aria-label={t('usage.listLabel')}>
              {usage.data.usedBy.map((r) => (
                <ListItem key={r.pointer} disableGutters>
                  <ListItemText
                    primary={t(`usage.kind.${r.kind}`)}
                    secondary={
                      <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.ngfw.monoFontFamily, fontSize: 12, display: 'block', textAlign: 'start' }}>
                        {r.pointer}
                      </Box>
                    }
                  />
                </ListItem>
              ))}
            </List>
          )}
        </>
      )}
    </Box>
  );
}
