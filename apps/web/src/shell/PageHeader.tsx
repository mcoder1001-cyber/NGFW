import { PgBox as Box } from '@amnafzar/common-ui-lib/primitives';
import { PgTypography as Typography } from '@amnafzar/common-ui-lib/primitives';
import type { ReactNode } from 'react';

export function PageHeader({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <Box>
      <Box component="header" className="ngfw-page-header">
        <Typography component="h2" variant="h5" sx={{ fontWeight: 700, letterSpacing: '-0.025em' }}>
          {title}
        </Typography>
      </Box>
      {children && <Box sx={{ mt: 1.5 }}>{children}</Box>}
    </Box>
  );
}
