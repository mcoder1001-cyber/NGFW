import ReportProblemOutlined from '@mui/icons-material/ReportProblemOutlined';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Chip from '@mui/material/Chip';
import Link from '@mui/material/Link';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import { useDashboardSummary } from './queries';

const SEVERITIES = ['critical', 'warning', 'info'] as const;
const COLOR: Record<(typeof SEVERITIES)[number], 'error' | 'warning' | 'info'> = {
  critical: 'error',
  warning: 'warning',
  info: 'info',
};

/** F-dashboard-prom-alarms: the dashboard's active-alarms card (WEB-dashboard owns the page; this is one card). */
export function AlarmsCard() {
  const { t } = useTranslation('dashboard-prom-alarms');
  const q = useDashboardSummary();
  const s = q.data?.alarms;
  return (
    <Card variant="outlined">
      <CardContent>
        <Stack direction="row" alignItems="center" gap={1} sx={{ mb: 1 }}>
          <ReportProblemOutlined fontSize="small" />
          <Typography component="h3" variant="subtitle1" sx={{ flex: 1 }}>
            {t('card.title')}
          </Typography>
          <Link component={RouterLink} to="/system/alarms" variant="body2">
            {t('card.viewAll')}
          </Link>
        </Stack>
        {q.isError && <Typography color="text.secondary">{t('card.unavailable')}</Typography>}
        {s && s.active === 0 && <Typography color="text.secondary">{t('card.none')}</Typography>}
        {s && s.active > 0 && (
          <Stack direction="row" gap={1} flexWrap="wrap">
            {SEVERITIES.filter((sev) => (s.bySeverity[sev] ?? 0) > 0).map((sev) => (
              <Chip
                key={sev}
                color={COLOR[sev]}
                variant="outlined"
                label={`${t(`severity.${sev}`)}: ${s.bySeverity[sev]}`}
              />
            ))}
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}
