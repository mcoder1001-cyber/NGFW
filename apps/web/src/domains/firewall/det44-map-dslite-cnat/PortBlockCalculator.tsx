import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { LTR_INPUT, NS, det44Plan } from './model';

/** DET44 port-block calculator: what an inside/outside prefix pair gives each subscriber (VPP's formulas). */
export function PortBlockCalculator() {
  const { t } = useTranslation(NS);
  const [inside, setInside] = useState('100.64.0.0/24');
  const [outside, setOutside] = useState('192.0.2.0/30');
  const [host, setHost] = useState('0');
  const plan = det44Plan(inside, outside);
  const idx = Math.max(0, Math.floor(Number(host) || 0));
  return (
    <Paper variant="outlined" sx={{ p: 2, mb: 3 }}>
      <Typography variant="h6" component="h3" sx={{ mb: 1 }}>
        {t('calc.title')}
      </Typography>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('calc.intro')}
      </Typography>
      <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap', mb: 2 }}>
        <TextField
          label={t('calc.inside')}
          value={inside}
          onChange={(e) => setInside(e.target.value)}
          size="small"
          slotProps={LTR_INPUT}
        />
        <TextField
          label={t('calc.outside')}
          value={outside}
          onChange={(e) => setOutside(e.target.value)}
          size="small"
          slotProps={LTR_INPUT}
        />
        <TextField
          label={t('calc.host')}
          value={host}
          onChange={(e) => setHost(e.target.value)}
          size="small"
          type="number"
        />
      </Box>
      {typeof plan === 'string' ? (
        <Typography color="error" role="alert">
          {t(`calc.error.${plan}`)}
        </Typography>
      ) : (
        <Box component="dl" data-testid="det44-plan" sx={{ m: 0 }}>
          <Typography>{t('calc.ratio', { ratio: plan.ratio })}</Typography>
          <Typography>{t('calc.ports', { ports: plan.portsPerHost })}</Typography>
          <Typography>
            {t('calc.hosts', { hosts: plan.insideHosts, addresses: plan.outsideAddresses })}
          </Typography>
          {idx < plan.insideHosts && (
            <Typography>
              {t('calc.block', {
                host: idx,
                offset: plan.blockOf(idx).outsideOffset,
                lo: plan.blockOf(idx).lo,
                hi: plan.blockOf(idx).hi,
              })}
            </Typography>
          )}
        </Box>
      )}
    </Paper>
  );
}
