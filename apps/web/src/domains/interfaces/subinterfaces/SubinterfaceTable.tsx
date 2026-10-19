import { PgButton as Button } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import { PgAddIcon as AddIcon, PgDeleteIcon as DeleteIcon } from '@amnafzar/common-ui-lib/icons';
import {
  PgBox as Box,
  PgIconButton as IconButton,
  PgStack as Stack,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import type { Theme } from '@amnafzar/common-ui-lib/styles';
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
const MONO = {
  fontFamily: (th: Theme) => th.ngfw.monoFontFamily,
  fontSize: 12,
} as const;

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
    t('encapTitle', {
      stack: stack.map((tag) => t(`proto.${tag.proto}`)).join(' · '),
    });
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
        {(() => {
          const gridItems = subs;
          type GridRow = (typeof gridItems)[number];
          return (
            <ReactGeereedMaterial
              hideFilterBuilderComponent
              noFormWrapper
              embedded
              memoEnable={false}
              hidePagination
              hideToolbar
              search={false}
              exportToCSV={false}
              sortable={false}
              footer={false}
              items={[...gridItems]}
              columns={[
                {
                  key: '0',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.name')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const { 0: id } = gridRow;
                    const name = `${parent}.${id}`;
                    return <>{name}</>;
                  },
                },
                {
                  key: '1',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.encapsulation')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const { 0: id, 1: sub } = gridRow;
                    const name = `${parent}.${id}`;
                    const row = items?.find((i) => i.name === name);
                    const configured = tagStack(sub);
                    const inVpp = liveTagStack(row);
                    const drift = inVpp !== undefined && !sameTagStack(configured, inVpp);
                    return (
                      <>
                        <Box component="span" dir="ltr" title={protoTitle(configured)} sx={MONO}>
                          {formatTagStack(configured)}
                        </Box>
                        {drift && (
                          <Typography
                            variant="caption"
                            color="warning.main"
                            sx={{ display: 'block' }}
                          >
                            {t('inVpp')} <bdi dir="ltr">{formatTagStack(inVpp)}</bdi>
                          </Typography>
                        )}
                      </>
                    );
                  },
                },
                {
                  key: '2',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.innerVlan')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const { 1: sub } = gridRow;
                    return <>{sub.innerVlanId ?? '—'}</>;
                  },
                },
                {
                  key: '3',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.admin')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const { 0: id } = gridRow;
                    const name = `${parent}.${id}`;
                    const row = items?.find((i) => i.name === name);
                    const live = row?.state ?? null;
                    return (
                      <>
                        {live ? (
                          <StatusChip size="small" status={adminStatus(live)!} />
                        ) : (
                          t('interfaces:notInVpp')
                        )}
                      </>
                    );
                  },
                },
                {
                  key: '4',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.link')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const { 0: id } = gridRow;
                    const name = `${parent}.${id}`;
                    const row = items?.find((i) => i.name === name);
                    const live = row?.state ?? null;
                    return (
                      <>{live ? <StatusChip size="small" status={linkStatus(live)!} /> : '—'}</>
                    );
                  },
                },
                {
                  key: '5',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.addresses')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const { 1: sub } = gridRow;
                    return <>{[...(sub.ipv4 ?? []), ...(sub.ipv6 ?? [])].join(' ')}</>;
                  },
                },
                {
                  key: '6',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('interfaces:sub.actions')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const { 0: id } = gridRow;
                    return (
                      <>
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
                      </>
                    );
                  },
                },
              ]}
              tableProps={{
                size: 'small',
                'aria-label': t('tableLabel', { parent }),
                sx: CELLS,
              }}
              rowKey={(nativeRow, _gridIndex) => {
                const gridRow = nativeRow as unknown as GridRow;
                const { 0: id } = gridRow;
                return id;
              }}
              rowProps={(nativeRow, _gridIndex) => {
                const gridRow = nativeRow as unknown as GridRow;
                const { 0: id, 1: sub } = gridRow;
                return {
                  hover: true,
                  sx: { cursor: readOnly ? 'default' : 'pointer' },
                  onClick: () => !readOnly && onEdit(id, sub),
                };
              }}
              cellProps={(nativeRow, column, _gridIndex) => {
                const gridRow = nativeRow as unknown as GridRow;
                const { 1: sub } = gridRow;
                switch (column.key) {
                  case '0':
                    return {
                      dir: 'ltr',
                      sx: { textAlign: 'start', whiteSpace: 'nowrap' },
                    };
                  case '1':
                    return { sx: { whiteSpace: 'nowrap' } };
                  case '2':
                    return {
                      title: sub.innerVlanId === undefined ? t('noInner') : t('proto.dot1q'),
                    };
                  case '3':
                    return {};
                  case '4':
                    return {};
                  case '5':
                    return { dir: 'ltr', sx: { ...MONO, textAlign: 'start' } };
                  case '6':
                    return { sx: { textAlign: 'end' } };
                  default:
                    return {};
                }
              }}
              emptyContent={
                <>
                  <Typography color="text.secondary">{t('interfaces:sub.none')}</Typography>
                </>
              }
            />
          );
        })()}
      </Box>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
        {t('exactMatch')}
      </Typography>
    </Box>
  );
}
