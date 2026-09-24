import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import type { Theme } from '@mui/material/styles';
import { StatusChip } from '@ngfw/ui-kit';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { adminStatus, linkStatus, type InterfaceItem, type SubinterfaceConfig } from '../model';
import { formatTagStack, liveTagStack, sameTagStack, tagStack } from './tagStack';

export interface SubinterfaceTableProps {
  /** Logical name of the parent interface (rows are `<parent>.<id>`). */
  parent: string;
  /** The parent's sub-interfaces in the candidate, sorted by id. */
  subs: readonly (readonly [string, SubinterfaceConfig])[];
  /** `/api/v1/state/interfaces` rows (live state, retrieved config, running). */
  items: readonly InterfaceItem[] | undefined;
  readOnly: boolean;
  /** The parent is in the candidate (a sub-interface can be added). */
  canAdd: boolean;
  /** A save is in flight. */
  busy: boolean;
  /** Error of the last add/edit/remove, shown above the table (`null`: none). */
  error: unknown;
  onAdd: () => void;
  onEdit: (id: string, value: SubinterfaceConfig) => void;
  onRemove: (id: string) => void;
}

const CELLS = { '& .MuiTableCell-root': { px: 0.75 } } as const;
const MONO = { fontFamily: (th: Theme) => th.vrx.monoFontFamily, fontSize: 12 } as const;

/**
 * The sub-interfaces of one parent in the interface drawer (F-vlan-qinq; extracted from P08's drawer). One row per
 * sub-interface of the candidate: its tag stack (Encapsulation `dot1q 100` / `dot1ad 200 · dot1q 100`, Inner VLAN), admin
 * and link state from the live table, and its addresses. When VPP holds another stack than the candidate (a pending tag
 * change), the live stack is shown under the configured one.
 */
export function SubinterfaceTable({
  parent,
  subs,
  items,
  readOnly,
  canAdd,
  busy,
  error,
  onAdd,
  onEdit,
  onRemove,
}: SubinterfaceTableProps) {
  const { t } = useTranslation(['vlan-qinq', 'interfaces']);
  const protoTitle = (stack: ReturnType<typeof tagStack>) =>
    t('encapTitle', { stack: stack.map((tag) => t(`proto.${tag.proto}`)).join(' · ') });
  return (
    <Box>
      <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
        <Typography component="h4" variant="subtitle1" sx={{ flex: 1 }}>
          {t('interfaces:sub.title')}
        </Typography>
        <Button size="small" startIcon={<AddIcon />} disabled={readOnly || !canAdd} onClick={onAdd}>
          {t('interfaces:sub.add')}
        </Button>
      </Stack>
      {error !== null && error !== undefined && <ProblemAlert error={error} sx={{ mb: 1 }} />}
      {/* seven columns in the 640 px drawer: tight cells, and a horizontal scroll rather than clipping */}
      <Box sx={{ overflowX: 'auto' }}>
        <Table size="small" aria-label={t('tableLabel', { parent })} sx={CELLS}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.name')}</TableCell>
              <TableCell>{t('col.encapsulation')}</TableCell>
              <TableCell>{t('col.innerVlan')}</TableCell>
              <TableCell>{t('col.admin')}</TableCell>
              <TableCell>{t('col.link')}</TableCell>
              <TableCell>{t('col.addresses')}</TableCell>
              <TableCell sx={{ textAlign: 'end' }}>{t('interfaces:sub.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {subs.length === 0 && (
              <TableRow>
                <TableCell colSpan={7}>
                  <Typography color="text.secondary">{t('interfaces:sub.none')}</Typography>
                </TableCell>
              </TableRow>
            )}
            {subs.map(([id, sub]) => {
              const name = `${parent}.${id}`;
              const row = items?.find((i) => i.name === name);
              const live = row?.state ?? null;
              const configured = tagStack(sub);
              const inVpp = liveTagStack(row);
              const drift = inVpp !== undefined && !sameTagStack(configured, inVpp);
              return (
                <TableRow
                  key={id}
                  hover
                  sx={{ cursor: readOnly ? 'default' : 'pointer' }}
                  onClick={() => !readOnly && onEdit(id, sub)}
                >
                  <TableCell dir="ltr" sx={{ textAlign: 'start', whiteSpace: 'nowrap' }}>
                    {name}
                  </TableCell>
                  <TableCell sx={{ whiteSpace: 'nowrap' }}>
                    <Box component="span" dir="ltr" title={protoTitle(configured)} sx={MONO}>
                      {formatTagStack(configured)}
                    </Box>
                    {drift && (
                      <Typography variant="caption" color="warning.main" sx={{ display: 'block' }}>
                        {t('inVpp')} <bdi dir="ltr">{formatTagStack(inVpp)}</bdi>
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell
                    title={sub.innerVlanId === undefined ? t('noInner') : t('proto.dot1q')}
                  >
                    {sub.innerVlanId ?? '—'}
                  </TableCell>
                  <TableCell>
                    {live ? (
                      <StatusChip size="small" status={adminStatus(live)!} />
                    ) : (
                      t('interfaces:notInVpp')
                    )}
                  </TableCell>
                  <TableCell>
                    {live ? <StatusChip size="small" status={linkStatus(live)!} /> : '—'}
                  </TableCell>
                  <TableCell dir="ltr" sx={{ ...MONO, textAlign: 'start' }}>
                    {[...(sub.ipv4 ?? []), ...(sub.ipv6 ?? [])].join(' ')}
                  </TableCell>
                  <TableCell sx={{ textAlign: 'end' }}>
                    <IconButton
                      size="small"
                      aria-label={t('interfaces:sub.remove', { id })}
                      disabled={readOnly || busy}
                      onClick={(e) => {
                        e.stopPropagation();
                        onRemove(id);
                      }}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </Box>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
        {t('exactMatch')}
      </Typography>
    </Box>
  );
}
