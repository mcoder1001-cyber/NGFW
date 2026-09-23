import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Typography from '@mui/material/Typography';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from './api';

export function HealthCard() {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['health'],
    queryFn: async () => {
      const { data, error } = await api.GET('/api/v1/health');
      if (error || !data) throw new Error('health failed');
      return data;
    },
    refetchInterval: 5000,
  });
  return (
    <Card sx={{ maxWidth: 420 }}>
      <CardContent>
        <Typography variant="subtitle2">{t('health.title')}</Typography>
        <Typography>
          {q.isPending && t('health.loading')}
          {q.isError && t('health.error')}
          {q.data && t('health.ok', { version: q.data.version })}
        </Typography>
      </CardContent>
    </Card>
  );
}
