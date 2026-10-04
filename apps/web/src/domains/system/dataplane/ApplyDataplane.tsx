import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Typography from '@mui/material/Typography';
import { useMutation } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { usePreviewDataplane } from './queries';

/** Each opening obtains a fresh preview; approval exists only for the confirmed SHA and never enters storage. */
export function ApplyDataplane() {
  const { t } = useTranslation('dataplane');
  const perms = usePermissions();
  const [open, setOpen] = useState(false);
  const preview = usePreviewDataplane();
  const apply = useMutation({
    mutationFn: async () => {
      const sha256 = preview.data!.sha256;
      const approval = (
        await call(api.POST('/api/v1/actions/dataplane/approve', { body: { sha256 } }))
      ).data!;
      return (
        await call(
          api.POST('/api/v1/actions/dataplane/apply', {
            body: { sha256, token: approval.token },
          }),
        )
      ).data;
    },
  });
  return (
    <>
      <Button
        variant="contained"
        disabled={perms.role !== 'admin'}
        onClick={() => {
          apply.reset();
          preview.reset();
          setOpen(true);
          preview.mutate();
        }}
      >
        {t('apply.button')}
      </Button>
      <Dialog
        open={open}
        onClose={() => {
          if (!apply.isPending) setOpen(false);
        }}
        fullWidth
        maxWidth="md"
      >
        <DialogTitle>{t('apply.button')}</DialogTitle>
        <DialogContent>
          <Alert severity="warning">{t('apply.confirm')}</Alert>
          {preview.isError && <ProblemAlert error={preview.error} />}
          {apply.isError && <ProblemAlert error={apply.error} />}
          {preview.data && (
            <>
              <Typography component="pre" sx={{ whiteSpace: 'pre-wrap' }}>
                {preview.data.diff}
              </Typography>
              <Typography>{preview.data.sha256}</Typography>
            </>
          )}
          {apply.isSuccess && <Alert severity="success">{t('apply.accepted')}</Alert>}
        </DialogContent>
        <DialogActions>
          <Button disabled={apply.isPending} onClick={() => setOpen(false)}>
            {t('apply.close')}
          </Button>
          <Button
            disabled={
              perms.role !== 'admin' ||
              !preview.data?.changed ||
              preview.isPending ||
              apply.isPending ||
              apply.isSuccess
            }
            onClick={() => apply.mutate()}
          >
            {t('apply.confirmButton')}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
