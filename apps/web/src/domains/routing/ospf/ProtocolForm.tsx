import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { NS } from './locale';
import {
  mergePatch,
  problemUnder,
  routingSubSchema,
  same,
  useCandidateRouting,
  usePatchRouting,
  useRunningRouting,
  type Protocol,
} from './queries';

/**
 * WEB-4a: the schema-driven form of one `routing.<protocol>` object. Saving sends a merge patch against the candidate
 * (removed record entries become null); "Remove" sends `{ <protocol>: null }`.
 */
export function ProtocolForm({ proto, label }: { proto: Protocol; label: string }) {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const cand = useCandidateRouting();
  const running = useRunningRouting();
  const patch = usePatchRouting();
  if (!cand.isSuccess) return null;
  const value = cand.data[proto];
  const committed = same(value, running.data?.[proto]);
  return (
    <Paper variant="outlined" sx={{ p: 2, flex: 1, maxWidth: 720, width: '100%' }}>
      <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 1 }}>
        <Typography component="h3" variant="h6" sx={{ flex: 1 }}>
          {label}
        </Typography>
        {running.isSuccess && (
          <Chip
            size="small"
            data-testid={`${proto}-status`}
            color={committed ? 'success' : 'warning'}
            label={committed ? t('committed') : t('pending')}
          />
        )}
        {value !== undefined && perms.editConfig && (
          <Button
            size="small"
            color="error"
            onClick={() => void patch.mutateAsync({ [proto]: null }).catch(() => undefined)}
          >
            {t('remove')}
          </Button>
        )}
      </Stack>
      {value === undefined && (
        <Typography color="text.secondary" sx={{ mb: 1 }}>
          {t('notConfigured', { proto: label })}
        </Typography>
      )}
      <SchemaForm
        id={`routing-${proto}-form`}
        schema={routingSubSchema(proto)}
        value={value ?? {}}
        readOnly={!perms.editConfig}
        onSubmit={async (v) => {
          await patch
            .mutateAsync({ [proto]: mergePatch(value ?? {}, v ?? {}) })
            .catch(() => undefined);
        }}
        problem={patch.error ? problemUnder(patch.error, `/routing/${proto}`) : null}
        submitLabel={t('save')}
      />
    </Paper>
  );
}
