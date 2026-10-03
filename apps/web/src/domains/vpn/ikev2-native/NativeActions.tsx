import Button from '@mui/material/Button';
import Stack from '@mui/material/Stack';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { ipsecKeys } from '../ipsec/queries';

const operation = { initiate: 'initiate', rekey: 'rekey', deleteSa: 'delete-sa' } as const;

export function NativeActions({
  tunnel,
  ikeSpi,
  childSpi,
  initiator = true,
}: {
  tunnel: string;
  ikeSpi?: string;
  childSpi?: string;
  initiator?: boolean;
}) {
  const { t } = useTranslation('ipsec');
  const perms = usePermissions();
  const qc = useQueryClient();
  const action = useMutation({
    mutationFn: async (operation: 'initiate' | 'rekey' | 'delete-sa') =>
      call(
        api.POST('/api/v1/actions/ipsec/ikev2/{tunnel}/{operation}', {
          params: { path: { tunnel, operation } },
          body: {
            ikeSpi: ikeSpi === undefined ? '0' : BigInt(`0x${ikeSpi}`).toString(),
            childSpi: childSpi ? Number.parseInt(childSpi, 16) : 0,
          },
        }),
      ),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ipsecKeys.sas });
      await qc.invalidateQueries({ queryKey: ipsecKeys.tunnels });
    },
  });
  if (perms.role !== 'admin') return null;
  return (
    <>
      <Stack direction="row" spacing={1} sx={{ my: 1 }}>
        {ikeSpi === undefined && (
          <Button disabled={action.isPending} onClick={() => action.mutate(operation.initiate)}>
            {t('nativeActions.initiate')}
          </Button>
        )}
        {childSpi !== undefined && (
          <Button
            disabled={action.isPending || !initiator}
            title={!initiator ? t('nativeActions.peerRekey') : undefined}
            onClick={() => action.mutate(operation.rekey)}
          >
            {t('nativeActions.rekey')}
          </Button>
        )}
        {childSpi === undefined && ikeSpi !== undefined && (
          <Button
            color="warning"
            disabled={action.isPending}
            onClick={() => action.mutate(operation.deleteSa)}
          >
            {t('nativeActions.deleteSA')}
          </Button>
        )}
      </Stack>
      {action.error !== null && <ProblemAlert error={action.error} />}
    </>
  );
}
