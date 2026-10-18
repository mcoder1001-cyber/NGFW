import { PgBox as Box } from '@amnafzar/common-ui-lib/primitives';
import { PgTypography as Typography } from '@amnafzar/common-ui-lib/primitives';
import { useTranslation } from 'react-i18next';
import { NS, SUBTREES } from './model';
import { SubtreeForm } from './SubtreeForm';

/** MAP-E / MAP-T / lw4o6: the schema-driven `nat.map` form. */
export function MapTab() {
  const { t } = useTranslation(NS);
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('map.intro')}
      </Typography>
      <SubtreeForm subtree={SUBTREES.map} />
    </Box>
  );
}
