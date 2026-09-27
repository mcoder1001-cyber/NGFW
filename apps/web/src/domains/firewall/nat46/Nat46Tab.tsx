import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Divider from '@mui/material/Divider';
import LinearProgress from '@mui/material/LinearProgress';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { problemFor } from '../../interfaces/InterfaceDrawer';
import { createMergePatch } from '../../interfaces/model';
import { useCandidateInterfaces } from '../../interfaces/queries';
import { NAT_POINTER } from '../nat44-ed-sessions/model';
import { useCandidateNat, usePatchNat } from '../nat44-ed-sessions/queries';
import { nat46Schema, NS } from './model';
import { useNat46Client } from './queries';

/**
 * NAT46 (stateless SIIT 1:1): the schema-driven `nat.nat46` form (Save sends only the edits as a merge patch of
 * `/config/nat`; server problems at `/nat/nat46/…` land on their fields), then the client-address helper
 * (`GET /state/nat/nat46/client`). No session table: NAT46 keeps no state in VPP.
 */
export function Nat46Tab() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const candidate = useCandidateNat();
  const patch = usePatchNat();
  const ifs = useCandidateInterfaces();
  const client = useNat46Client();
  const [saved, setSaved] = useState(false);
  const [ipv4, setIpv4] = useState('');
  const schema = useMemo(() => nat46Schema(), []);
  const interfaceOptions = useMemo(() => Object.keys(ifs.data ?? {}).sort(), [ifs.data]);
  const [opened, setOpened] = useState<Record<string, unknown> | null>(null);
  if (candidate.isSuccess && opened === null) {
    const v = candidate.data['nat46'];
    setOpened(
      v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {},
    );
  }

  const save = async (value: unknown) => {
    setSaved(false);
    const cleaned = value as Record<string, unknown>;
    const inner = createMergePatch(opened ?? {}, cleaned) as Record<string, unknown>;
    if (Object.keys(inner).length === 0) {
      setSaved(true);
      return;
    }
    try {
      await patch.mutateAsync({ nat46: inner });
      setOpened(cleaned);
      setSaved(true);
    } catch {
      // shown from patch.error
    }
  };

  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      {candidate.isPending && <LinearProgress aria-label={t('loading')} />}
      {candidate.isError && <ProblemAlert error={candidate.error} sx={{ mb: 1 }} />}
      {patch.isError && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
      {saved && !patch.isError && (
        <Alert severity="success" sx={{ mb: 1 }}>
          {t('saved')}
        </Alert>
      )}
      {opened !== null && (
        <SchemaForm
          schema={schema}
          value={opened}
          readOnly={!perms.editConfig}
          interfaceOptions={interfaceOptions}
          submitLabel={t('save')}
          resetLabel={t('reset')}
          problem={problemFor(patch.error, `${NAT_POINTER}/nat46`)}
          onSubmit={save}
        />
      )}
      <Divider sx={{ my: 3 }} />
      <Typography variant="h6" component="h3" sx={{ mb: 1 }}>
        {t('client.title')}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        {t('client.help')}
      </Typography>
      <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap">
        <TextField
          size="small"
          label={t('client.ipv4')}
          value={ipv4}
          onChange={(e) => setIpv4(e.target.value.trim())}
        />
        <Button
          variant="outlined"
          disabled={ipv4 === '' || client.isPending}
          onClick={() => client.mutate(ipv4)}
        >
          {t('client.show')}
        </Button>
      </Stack>
      {client.isError && <ProblemAlert error={client.error} sx={{ mt: 1 }} />}
      {client.data && (
        <Typography sx={{ mt: 1 }} data-testid="nat46-client" dir="ltr">
          {t('client.result', { ipv4: client.data.ipv4, ipv6: client.data.ipv6 })}
        </Typography>
      )}
    </Box>
  );
}
