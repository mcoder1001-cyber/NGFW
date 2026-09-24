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
      // The OpenAPI document declares no body for /health (P07b-questions #2), so the shape is read defensively.
      const { data, response } = await api.GET('/api/v1/health', { parseAs: 'json' });
      if (!response.ok) throw new Error('health failed');
      const body = data as unknown as { version?: unknown } | undefined;
      return { version: typeof body?.version === 'string' ? body.version : '?' };
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
