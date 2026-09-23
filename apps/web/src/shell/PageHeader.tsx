import Typography from '@mui/material/Typography';
import type { ReactNode } from 'react';

export function PageHeader({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <>
      <Typography component="h2" variant="h5" gutterBottom>
        {title}
      </Typography>
      {children}
    </>
  );
}
