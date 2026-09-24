import Chip from '@mui/material/Chip';
import Box from '@mui/material/Box';
import Divider from '@mui/material/Divider';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useTranslation } from 'react-i18next';
import { DEFAULT_VRF } from '../nat44-ed-sessions/model';
import { useCandidateNat } from '../nat44-ed-sessions/queries';
import { driftUnder, NS } from './model';
import type { Subtree } from './model';
import { useDrift } from './queries';
import { SubtreeForm } from './SubtreeForm';

const SUBTREE: Subtree = 'nat66';

type Item = Record<string, unknown>;

/** Live status of the applied NAT66 configuration: the drift between running and what the agent retrieves. */
function DriftChip() {
  const { t } = useTranslation(NS);
  const drift = useDrift();
  if (drift.isPending) return <Chip size="small" label={t('loading')} />;
  if (drift.isError) return <Chip size="small" color="default" label={t('status.unknown')} />;
  const n = driftUnder(drift.data, '/nat/nat66').length;
  return n === 0 ? (
    <Chip size="small" color="success" label={t('status.applied')} />
  ) : (
    <Chip size="small" color="warning" label={t('status.drift', { count: n })} />
  );
}

/**
 * NAT66 (stateless 1:1 IPv6): the schema-driven `nat.nat66` form, then the static mappings of the candidate with the
 * live status column (running vs retrieved, `GET /state/drift`).
 */
export function Nat66Tab() {
  const { t } = useTranslation(NS);
  const candidate = useCandidateNat();
  const nat66 = (candidate.data?.['nat66'] ?? {}) as Item;
  const maps = (Array.isArray(nat66['staticMappings']) ? nat66['staticMappings'] : []) as Item[];
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('nat66.intro')}
      </Typography>
      <SubtreeForm subtree={SUBTREE} />
      <Divider sx={{ my: 3 }} />
      <Typography variant="h6" component="h3" sx={{ mb: 1 }}>
        {t('nat66.mappings')}
      </Typography>
      <Paper variant="outlined">
        <Table size="small" aria-label={t('nat66.mappings')}>
          <TableHead>
            <TableRow>
              <TableCell>{t('field.local.title')}</TableCell>
              <TableCell>{t('field.external.title')}</TableCell>
              <TableCell>{t('field.vrf.title')}</TableCell>
              <TableCell>{t('col.status')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {candidate.isSuccess && maps.length === 0 && (
              <TableRow>
                <TableCell colSpan={4}>{t('nat66.empty')}</TableCell>
              </TableRow>
            )}
            {maps.map((m, i) => (
              <TableRow key={`${String(m['local'])}|${i}`}>
                <TableCell dir="ltr">{String(m['local'] ?? '')}</TableCell>
                <TableCell dir="ltr">{String(m['external'] ?? '')}</TableCell>
                <TableCell>{typeof m['vrf'] === 'string' ? m['vrf'] : DEFAULT_VRF}</TableCell>
                <TableCell>
                  <DriftChip />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Paper>
    </Box>
  );
}
