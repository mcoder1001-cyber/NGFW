import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import { useTranslation } from 'react-i18next';
import { NS, SUBTREES } from './model';
import { SubtreeForm } from './SubtreeForm';

/** PNAT policy 1:1 NAT: the schema-driven `nat.pnat` form (IPv4 only). */
export function PnatTab() {
  const { t } = useTranslation(NS);
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('pnat.intro')}
      </Typography>
      <SubtreeForm subtree={SUBTREES.pnat} />
    </Box>
  );
}
