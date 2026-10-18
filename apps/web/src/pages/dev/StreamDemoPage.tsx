import { PgAlert as Alert } from '@amnafzar/common-ui-lib/primitives';
import { PgBox as Box } from '@amnafzar/common-ui-lib/primitives';
import { PgChip as Chip } from '@amnafzar/common-ui-lib/primitives';
import { PgFormControlLabel as FormControlLabel } from '@amnafzar/common-ui-lib/primitives';
import { PgStack as Stack } from '@amnafzar/common-ui-lib/primitives';
import { PgSwitch as Switch } from '@amnafzar/common-ui-lib/primitives';
import { PgTypography as Typography } from '@amnafzar/common-ui-lib/primitives';
import { useTheme } from '@amnafzar/common-ui-lib/styles';
import { UI_KIT_NS, useFormatters } from '@ngfw/ui-kit';
import { useTopic, useWsStatus } from '@ngfw/ui-kit/ws';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '../../shell/PageHeader';

function TopicPanel() {
  const { t } = useTranslation(['dev']);
  const theme = useTheme();
  const fmt = useFormatters();
  const topic = useTopic<unknown>('iface.counters');
  return (
    <Stack gap={1}>
      <Typography>{t('dev:stream.batch', { count: topic.batchSize })}</Typography>
      <Typography component="h3" variant="subtitle2">
        {t('dev:stream.lastMessage')}
        {topic.updatedAt !== undefined && ` — ${fmt.time(topic.updatedAt)}`}
      </Typography>
      <Box component="pre" sx={{ m: 0, p: 2, border: 1, borderColor: 'divider', borderRadius: 1, fontFamily: theme.ngfw.monoFontFamily, fontSize: 12 }}>
        {topic.data === undefined ? t('dev:stream.noMessage') : JSON.stringify(topic.data, null, 2)}
      </Box>
    </Stack>
  );
}

export function StreamDemoPage() {
  const { t } = useTranslation(['dev', 'common', UI_KIT_NS]);
  const status = useWsStatus();
  const [subscribed, setSubscribed] = useState(false);
  return (
    <PageHeader title={t('dev:stream.title')}>
      <Stack gap={2} sx={{ maxInlineSize: 720 }}>
        <Alert severity="warning">{t('dev:banner')}</Alert>
        <Typography>{t('dev:stream.body')}</Typography>
        <Stack direction="row" gap={2} alignItems="center">
          <Typography>{t('dev:stream.status')}</Typography>
          <Chip label={t(`ws.${status}`, { ns: UI_KIT_NS })} color={status === 'open' ? 'success' : status === 'reconnecting' ? 'warning' : 'default'} />
        </Stack>
        <FormControlLabel control={<Switch checked={subscribed} onChange={(e) => setSubscribed(e.target.checked)} />} label={t('dev:stream.subscribe')} />
        {subscribed && <TopicPanel />}
      </Stack>
    </PageHeader>
  );
}
