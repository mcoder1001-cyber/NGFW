import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import DragIndicatorIcon from '@mui/icons-material/DragIndicator';
import EditIcon from '@mui/icons-material/Edit';
import FileDownloadIcon from '@mui/icons-material/FileDownload';
import FormatListNumberedIcon from '@mui/icons-material/FormatListNumbered';
import UploadFileIcon from '@mui/icons-material/UploadFile';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import FormControlLabel from '@mui/material/FormControlLabel';
import IconButton from '@mui/material/IconButton';
import Menu from '@mui/material/Menu';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Switch from '@mui/material/Switch';
import TextField from '@mui/material/TextField';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import {
  ServerDataGrid,
  type GridColDef,
  type ServerDataGridProps,
  type ServerPageRequest,
} from '@ngfw/ui-kit/data-grid';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { ImportDialog } from './ImportDialog';
import {
  ACL_POLL_MS,
  addressText,
  DEFAULT_RULE_PAGE_SIZE,
  emptySelection,
  MAX_SEQUENCE,
  nextSequence,
  planDrop,
  ruleCounters,
  RULE_PAGE_SIZES,
  rulesQuery,
  selectedIds,
  serviceText,
  toRuleRow,
  type BulkBody,
  type GridRowId,
  type RuleRow,
  type RuleSource,
  type RulesPage,
  type SelectionModel,
} from './model';
import { ConfirmDialog, CounterCell, LiveAlerts, Mono, PendingChip, RefreshButton } from './parts';
import { RuleDialog, type RuleEdit } from './RuleDialog';
import { aclKeys, exportCsv, fetchRules, useAclEdit, useAclLists, useBulk } from './queries';

type GridApi = NonNullable<NonNullable<ServerDataGridProps<RuleRow>['apiRef']>['current']>;
type Meta = Omit<RulesPage, 'items'>;

const SOURCES: readonly RuleSource[] = ['candidate', 'running'];
const PACKETS = 'packets' as const;
const BYTES = 'bytes' as const;
const ACTION_COLOR = { permit: 'success', deny: 'error', reflect: 'info' } as const;
const DISABLED_CLASS = 'acl-rule-disabled';
const DND_TYPE = 'text/plain';
const MOVE = 'move' as const;
type DialogKind = 'move' | 'renumber' | 'delete' | 'import';

/** Start dragging a rule by its handle (the drop is handled by the grid container). */
function startDrag(e: DragEvent, row: RuleRow, dragged: { current: RuleRow | null }) {
  dragged.current = row;
  e.dataTransfer.effectAllowed = MOVE;
  e.dataTransfer.setData(DND_TYPE, String(row.sequence));
}

/** Allow a drop only on a rule row while a rule is being dragged. */
function allowDrop(e: DragEvent, dragging: boolean) {
  if (dragging && rowIdAt(e.target) !== null) {
    e.preventDefault();
    e.dataTransfer.dropEffect = MOVE;
  }
}

/** Grid row id under a DOM node of the grid (MUI rows carry `data-id`). */
function rowIdAt(target: EventTarget | null): number | null {
  const el = target instanceof Element ? target.closest('[data-id]') : null;
  const v = el?.getAttribute('data-id');
  return v !== null && v !== undefined && v !== '' && Number.isInteger(Number(v))
    ? Number(v)
    : null;
}

/** Rules tab: pick a list, then the rule editor for it. */
export function RulesTab({
  list,
  onSelectList,
}: {
  list: string | null;
  onSelectList: (name: string) => void;
}) {
  const { t } = useTranslation('acl');
  // names only: the grid below has its own 30 s poll, the list of lists need not walk VPP as well
  const lists = useAclLists({ poll: false });
  const names = useMemo(() => {
    const s = new Set((lists.data?.lists ?? []).map((l) => l.name));
    if (list) s.add(list);
    return [...s].sort((a, b) => a.localeCompare(b));
  }, [lists.data, list]);
  const item = lists.data?.lists.find((l) => l.name === list);
  return (
    <>
      <Stack direction="row" gap={2} alignItems="center" sx={{ mb: 1 }} flexWrap="wrap">
        <TextField
          select
          size="small"
          label={t('rules.list')}
          value={list ?? ''}
          onChange={(e) => onSelectList(e.target.value)}
          sx={{ minInlineSize: 240 }}
        >
          {names.map((n) => (
            <MenuItem key={n} value={n}>
              <bdi>{n}</bdi>
            </MenuItem>
          ))}
        </TextField>
        {item?.description ? (
          <Typography variant="body2" color="text.secondary">
            {item.description}
          </Typography>
        ) : null}
        {item ? <PendingChip pending={item.pending} /> : null}
      </Stack>
      {list === null ? (
        <Alert severity="info">{names.length > 0 ? t('rules.pickList') : t('rules.noLists')}</Alert>
      ) : (
        <RuleEditor key={list} list={list} />
      )}
    </>
  );
}

/**
 * The rule editor of one list on ui-kit's ServerDataGrid: the server pages, searches and counts (a page is at most
 * 1000 rules, rows are virtualised), so a 100 000-rule list is never loaded whole. Counters refresh every 30 s and on
 * Refresh (D-132). Reorder by dragging a row's handle onto another row, or select rules and "Move to sequence".
 */
function RuleEditor({ list }: { list: string }) {
  const { t } = useTranslation('acl');
  const fmt = useFormatters();
  const perms = usePermissions();
  const qc = useQueryClient();
  const bulk = useBulk(list);
  const edit = useAclEdit();
  const [source, setSource] = useState<RuleSource>('candidate');
  const [hitsOnly, setHitsOnly] = useState(false);
  const [search, setSearch] = useState('');
  const [selection, setSelection] = useState<SelectionModel>(emptySelection);
  const [meta, setMeta] = useState<Meta | null>(null);
  const [ruleEdit, setRuleEdit] = useState<RuleEdit | null>(null);
  const [dialog, setDialog] = useState<DialogKind | null>(null);
  const [deleteOne, setDeleteOne] = useState<RuleRow | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [dialogError, setDialogError] = useState<unknown>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [exportAnchor, setExportAnchor] = useState<HTMLElement | null>(null);
  const apiRef = useRef<GridApi | null>(null);
  const page = useRef<{ rows: RuleRow[]; page: number; contiguous: boolean }>({
    rows: [],
    page: 0,
    contiguous: true,
  });
  const seqById = useRef(new Map<GridRowId, number>());
  const dragged = useRef<RuleRow | null>(null);
  const appliedFilter = useRef('');
  const canEdit = perms.editConfig;
  const readOnly = !canEdit || source === 'running';

  // the search box drives the grid's quick filter (which resets to the first page); the words go to the API as `filter`
  useEffect(() => {
    const text = search.trim();
    const id = setTimeout(() => {
      if (text === appliedFilter.current) return;
      appliedFilter.current = text;
      apiRef.current?.setQuickFilterValues(text ? [text] : []);
    }, 300);
    return () => clearTimeout(id);
  }, [search]);

  const fetchPage = useCallback(
    async (req: ServerPageRequest, signal: AbortSignal) => {
      const data = await fetchRules(list, rulesQuery(req, source, hitsOnly), signal);
      const rows = data.items.map(toRuleRow);
      page.current = {
        rows,
        page: req.page,
        contiguous: !hitsOnly && req.quickFilter.join('').trim() === '',
      };
      for (const r of rows) seqById.current.set(r.id, r.sequence);
      setMeta({
        list: data.list,
        source: data.source,
        page: data.page,
        pageSize: data.pageSize,
        total: data.total,
        size: data.size,
        applied: data.applied,
        mappingKnown: data.mappingKnown,
        countersAvailable: data.countersAvailable,
        countersReason: data.countersReason,
        agentError: data.agentError,
      });
      return { rows, total: data.total };
    },
    [list, source, hitsOnly],
  );

  const countersAvailable = meta?.countersAvailable ?? false;
  const countersReason = meta?.countersReason ?? '';
  const applied = meta?.applied ?? false;
  const selectedSeqs = selectedIds(
    selection,
    page.current.rows.map((r) => r.id),
  )
    .map((id) => seqById.current.get(id))
    .filter((s): s is number => s !== undefined);

  const resetSelection = () => setSelection(emptySelection());
  const changeSource = (s: RuleSource) => {
    setSource(s);
    seqById.current.clear();
    resetSelection();
    apiRef.current?.setPage(0);
  };
  const changeHitsOnly = (v: boolean) => {
    setHitsOnly(v);
    resetSelection();
    apiRef.current?.setPage(0);
  };
  const refresh = () => void qc.invalidateQueries({ queryKey: aclKeys.rules(list) });

  const runBulk = async (body: BulkBody) => {
    setError(null);
    setDialogError(null);
    setNotice(null);
    try {
      const r = await bulk.mutateAsync(body);
      resetSelection();
      setDialog(null);
      setDeleteOne(null);
      setNotice(t('bulk.done', { count: r.changed, n: fmt.integer(r.changed) }));
    } catch (e) {
      if (dialog !== null || deleteOne !== null) setDialogError(e);
      else setError(e);
    }
  };

  const onDrop = async (e: DragEvent) => {
    const from = dragged.current;
    dragged.current = null;
    const id = rowIdAt(e.target);
    if (!from || id === null) return;
    e.preventDefault();
    const rows = page.current.rows;
    const i = rows.findIndex((r) => r.id === id);
    const target = rows[i];
    if (!target) return;
    const predecessor = i > 0 ? rows[i - 1]!.sequence : page.current.page === 0 ? 0 : null;
    const plan = planDrop(from, target, predecessor, page.current.contiguous);
    if (plan.kind === 'none') return;
    setError(null);
    setNotice(null);
    try {
      if (plan.kind === 'patch')
        await edit.mutateAsync({
          method: 'PATCH',
          path: ['acl', 'lists', list, 'rules', plan.index],
          body: { sequence: plan.sequence },
        });
      else await bulk.mutateAsync({ op: 'move', sequences: plan.sequences, to: plan.to });
      resetSelection();
      setNotice(
        t('rules.moved', {
          sequence: fmt.number(from.sequence, { useGrouping: false }),
          before: fmt.number(target.sequence, { useGrouping: false }),
        }),
      );
    } catch (err) {
      setError(err);
    }
  };

  const openDialog = (d: DialogKind) => () => {
    setDialogError(null);
    setDialog(d);
  };
  const openImport = openDialog('import');
  const openRenumber = openDialog('renumber');
  const openMove = openDialog('move');
  const openDelete = openDialog('delete');
  const bulkSelected = (op: 'enable' | 'disable' | 'delete') => () =>
    void runBulk({ op, sequences: selectedSeqs });
  const enableSelected = bulkSelected('enable');
  const disableSelected = bulkSelected('disable');
  const deleteSelected = bulkSelected('delete');
  const moveSelected = (to: number) => void runBulk({ op: 'move', sequences: selectedSeqs, to });
  const renumber = (start: number, step: number) => void runBulk({ op: 'renumber', start, step });
  const deleteRow = () => {
    if (deleteOne) void runBulk({ op: 'delete', sequences: [deleteOne.sequence] });
  };

  const openAdd = async () => {
    setError(null);
    try {
      const first = await fetchRules(list, { page: 1, pageSize: 1, source: 'candidate' });
      const last =
        first.total > 0
          ? (await fetchRules(list, { page: first.total, pageSize: 1, source: 'candidate' }))
              .items[0]?.sequence
          : undefined;
      setRuleEdit({ row: null, suggested: nextSequence(last) });
    } catch (e) {
      setError(e);
    }
  };

  const doExport = async (s: RuleSource) => {
    setExportAnchor(null);
    setError(null);
    try {
      await exportCsv(list, s);
    } catch (e) {
      setError(e);
    }
  };

  const columns = useMemo<GridColDef<RuleRow>[]>(() => {
    const seq = (v: number) => fmt.number(v, { useGrouping: false });
    const any = (
      <Typography component="span" variant="body2" color="text.secondary">
        {t('match.any')}
      </Typography>
    );
    const notInVpp = t('counters.notInVpp');
    const cols: GridColDef<RuleRow>[] = [
      {
        field: 'drag',
        headerName: t('col.drag'),
        width: 48,
        disableColumnMenu: true,
        renderHeader: () => (
          <DragIndicatorIcon fontSize="small" color="disabled" titleAccess={t('col.drag')} />
        ),
        renderCell: (p) =>
          readOnly ? null : (
            <Box
              component="span"
              draggable
              role="button"
              aria-label={t('rules.dragHandle', { sequence: seq(p.row.sequence) })}
              onDragStart={(e) => startDrag(e, p.row, dragged)}
              onDragEnd={() => {
                dragged.current = null;
              }}
              sx={{
                cursor: 'grab',
                display: 'inline-flex',
                alignItems: 'center',
                blockSize: '100%',
              }}
            >
              <DragIndicatorIcon fontSize="small" color="action" />
            </Box>
          ),
      },
      {
        field: 'sequence',
        headerName: t('col.sequence'),
        width: 96,
        valueGetter: (_v, row) => row.sequence,
        renderCell: (p) => seq(p.row.sequence),
      },
      {
        field: 'enabled',
        headerName: t('col.enabled'),
        width: 90,
        renderCell: (p) =>
          p.row.rule.enabled === false ? (
            <Chip size="small" label={t('rules.off')} />
          ) : (
            <Typography variant="body2">{t('rules.on')}</Typography>
          ),
      },
      {
        field: 'action',
        headerName: t('col.action'),
        width: 100,
        renderCell: (p) => (
          <Chip
            size="small"
            variant="outlined"
            color={ACTION_COLOR[p.row.rule.action] ?? 'default'}
            label={t(`enum.action.${p.row.rule.action}`, { defaultValue: p.row.rule.action })}
          />
        ),
      },
      {
        field: 'ipVersion',
        headerName: t('col.ipVersion'),
        width: 96,
        renderCell: (p) => t(`enum.ipVersion.${p.row.rule.ipVersion ?? 'any'}`),
      },
      {
        field: 'source',
        headerName: t('col.source'),
        minWidth: 150,
        flex: 1,
        renderCell: (p) => {
          const text = addressText(p.row.rule.source);
          return text === null ? any : <Mono>{text}</Mono>;
        },
      },
      {
        field: 'destination',
        headerName: t('col.destination'),
        minWidth: 150,
        flex: 1,
        renderCell: (p) => {
          const text = addressText(p.row.rule.destination);
          return text === null ? any : <Mono>{text}</Mono>;
        },
      },
      {
        field: 'service',
        headerName: t('col.service'),
        width: 150,
        renderCell: (p) => {
          const text = serviceText(p.row.rule.service);
          return text === null ? any : <Mono>{text}</Mono>;
        },
      },
      {
        field: 'schedule',
        headerName: t('col.schedule'),
        width: 120,
        renderCell: (p) =>
          p.row.rule.schedule ? (
            <bdi>{p.row.rule.schedule}</bdi>
          ) : (
            <Typography component="span" variant="body2" color="text.secondary">
              {t('match.always')}
            </Typography>
          ),
      },
      {
        field: 'description',
        headerName: t('col.description'),
        minWidth: 140,
        flex: 1,
        valueGetter: (_v, row) => row.rule.description ?? '',
      },
      {
        field: 'log',
        headerName: t('col.log'),
        width: 70,
        renderCell: (p) => (p.row.rule.log ? t('rules.logOn') : ''),
      },
      {
        field: 'pending',
        headerName: t('col.pending'),
        width: 110,
        renderCell: (p) => <PendingChip pending={p.row.pending} />,
      },
      {
        field: 'status',
        headerName: t('col.status'),
        width: 150,
        renderCell: (p) => {
          const live = p.row.live;
          if (live)
            return (
              <Chip
                size="small"
                variant="outlined"
                color={live.status === 'applied' ? 'success' : 'default'}
                label={t(`status.${live.status}`)}
              />
            );
          const why =
            source === 'candidate' && p.row.pending
              ? t('status.notCommitted')
              : applied
                ? t('status.unknown')
                : t('status.notApplied');
          return (
            <Typography component="span" variant="body2" color="text.secondary">
              {why}
            </Typography>
          );
        },
      },
      {
        field: 'vppRules',
        headerName: t('col.vppRules'),
        width: 100,
        renderCell: (p) => (p.row.live ? fmt.integer(p.row.live.vppRules) : '—'),
      },
      {
        field: 'packets',
        headerName: t('col.packets'),
        width: 110,
        renderCell: (p) => (
          <CounterCell
            value={ruleCounters(p.row.live, countersAvailable)?.packets ?? null}
            kind={PACKETS}
            reason={countersAvailable ? notInVpp : countersReason}
          />
        ),
      },
      {
        field: 'bytes',
        headerName: t('col.bytes'),
        width: 110,
        renderCell: (p) => (
          <CounterCell
            value={ruleCounters(p.row.live, countersAvailable)?.bytes ?? null}
            kind={BYTES}
            reason={countersAvailable ? notInVpp : countersReason}
          />
        ),
      },
      {
        field: 'actions',
        headerName: t('col.actions'),
        width: 96,
        disableColumnMenu: true,
        renderCell: (p) => (
          <Stack direction="row" alignItems="center" sx={{ blockSize: '100%' }}>
            <IconButton
              size="small"
              aria-label={t(readOnly ? 'rules.viewNamed' : 'rules.editNamed', {
                sequence: seq(p.row.sequence),
              })}
              onClick={() => setRuleEdit({ row: p.row })}
            >
              <EditIcon fontSize="small" />
            </IconButton>
            <IconButton
              size="small"
              aria-label={t('rules.deleteNamed', { sequence: seq(p.row.sequence) })}
              disabled={readOnly}
              onClick={() => {
                setDialogError(null);
                setDeleteOne(p.row);
              }}
            >
              <DeleteIcon fontSize="small" />
            </IconButton>
          </Stack>
        ),
      },
    ];
    return cols.map((c) => ({ ...c, sortable: false, filterable: false }));
  }, [t, fmt, readOnly, source, applied, countersAvailable, countersReason]);

  const n = (v: number) => fmt.integer(v);
  return (
    <>
      <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap" sx={{ mb: 1 }}>
        <ToggleButtonGroup
          size="small"
          exclusive
          value={source}
          onChange={(_e, v: RuleSource | null) => v && changeSource(v)}
          aria-label={t('rules.sourceLabel')}
        >
          {SOURCES.map((s) => (
            <ToggleButton key={s} value={s}>
              {t(`rules.source.${s}`)}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
        <FormControlLabel
          control={
            <Switch
              size="small"
              checked={hitsOnly}
              onChange={(e) => changeHitsOnly(e.target.checked)}
            />
          }
          label={t('rules.hitsOnly')}
        />
        <TextField
          size="small"
          type="search"
          label={t('rules.search')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          helperText={t('rules.searchHelp')}
          sx={{ minInlineSize: 260 }}
        />
        <Box sx={{ flex: 1 }} />
        <RefreshButton onClick={refresh} busy={false} />
      </Stack>
      <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap" sx={{ mb: 1 }}>
        <Tooltip title={canEdit ? '' : t('readonly')}>
          <span>
            <Button
              variant="contained"
              size="small"
              startIcon={<AddIcon />}
              disabled={readOnly}
              onClick={() => void openAdd()}
            >
              {t('rules.add')}
            </Button>
          </span>
        </Tooltip>
        <Button
          size="small"
          variant="outlined"
          startIcon={<UploadFileIcon />}
          disabled={!canEdit}
          onClick={openImport}
        >
          {t('rules.import')}
        </Button>
        <Button
          size="small"
          variant="outlined"
          startIcon={<FileDownloadIcon />}
          onClick={(e) => setExportAnchor(e.currentTarget)}
          aria-haspopup="menu"
        >
          {t('rules.export')}
        </Button>
        <Menu
          anchorEl={exportAnchor}
          open={exportAnchor !== null}
          onClose={() => setExportAnchor(null)}
        >
          {SOURCES.map((s) => (
            <MenuItem key={s} onClick={() => void doExport(s)}>
              {t(`rules.exportFrom.${s}`)}
            </MenuItem>
          ))}
        </Menu>
        <Button
          size="small"
          variant="outlined"
          startIcon={<FormatListNumberedIcon />}
          disabled={readOnly}
          onClick={openRenumber}
        >
          {t('rules.renumber')}
        </Button>
        {meta && (
          <Typography variant="body2" color="text.secondary" sx={{ marginInlineStart: 1 }}>
            {meta.total === meta.size
              ? t('rules.count', { count: meta.size, n: n(meta.size) })
              : t('rules.countFiltered', { total: n(meta.total), size: n(meta.size) })}
          </Typography>
        )}
      </Stack>
      {selectedSeqs.length > 0 && !readOnly && (
        <Paper variant="outlined" sx={{ p: 1, mb: 1 }}>
          <Stack
            direction="row"
            gap={1}
            alignItems="center"
            flexWrap="wrap"
            role="toolbar"
            aria-label={t('bulk.label')}
          >
            <Typography variant="body2" sx={{ marginInlineEnd: 1 }}>
              {t('bulk.selected', { count: selectedSeqs.length, n: n(selectedSeqs.length) })}
            </Typography>
            <Button size="small" disabled={bulk.isPending} onClick={enableSelected}>
              {t('bulk.enable')}
            </Button>
            <Button size="small" disabled={bulk.isPending} onClick={disableSelected}>
              {t('bulk.disable')}
            </Button>
            <Button size="small" disabled={bulk.isPending} onClick={openMove}>
              {t('bulk.move')}
            </Button>
            <Button size="small" color="error" disabled={bulk.isPending} onClick={openDelete}>
              {t('bulk.delete')}
            </Button>
            <Button size="small" onClick={resetSelection}>
              {t('bulk.clear')}
            </Button>
          </Stack>
        </Paper>
      )}
      {meta && (
        <LiveAlerts
          agentError={meta.agentError}
          countersAvailable={meta.countersAvailable}
          countersReason={meta.countersReason}
        />
      )}
      {meta && !meta.applied && !meta.agentError && (
        <Alert severity="info" sx={{ mb: 1 }}>
          {t('rules.notApplied')}
        </Alert>
      )}
      {notice && (
        <Alert severity="success" sx={{ mb: 1 }} onClose={() => setNotice(null)}>
          {notice}
        </Alert>
      )}
      {error !== null && <ProblemAlert error={error} sx={{ mb: 1 }} />}
      <Paper
        variant="outlined"
        sx={{ blockSize: 600 }}
        onDragOver={(e) => allowDrop(e, dragged.current !== null)}
        onDrop={(e) => void onDrop(e)}
      >
        <ServerDataGrid<RuleRow>
          apiRef={apiRef}
          aria-label={t('rules.gridLabel', { list })}
          columns={columns}
          queryKey={[...aclKeys.rules(list), { source, hitsOnly }]}
          fetchPage={fetchPage}
          refetchInterval={ACL_POLL_MS}
          initialPageSize={DEFAULT_RULE_PAGE_SIZE}
          pageSizeOptions={RULE_PAGE_SIZES}
          checkboxSelection={!readOnly}
          keepNonExistentRowsSelected
          rowSelectionModel={selection}
          onRowSelectionModelChange={(m) => setSelection(m)}
          disableColumnFilter
          disableColumnSorting
          onRowDoubleClick={(p) => setRuleEdit({ row: p.row })}
          getRowClassName={(p) => (p.row.rule.enabled === false ? DISABLED_CLASS : '')}
          sx={{ [`& .${DISABLED_CLASS}`]: { color: 'text.secondary' } }}
        />
      </Paper>
      <RuleDialog
        list={list}
        edit={ruleEdit}
        readOnly={readOnly}
        onClose={() => setRuleEdit(null)}
      />
      <ImportDialog list={list} open={dialog === 'import'} onClose={() => setDialog(null)} />
      <MoveDialog
        open={dialog === 'move'}
        count={selectedSeqs.length}
        busy={bulk.isPending}
        error={dialogError}
        onClose={() => setDialog(null)}
        onMove={moveSelected}
      />
      <RenumberDialog
        open={dialog === 'renumber'}
        busy={bulk.isPending}
        error={dialogError}
        onClose={() => setDialog(null)}
        onRenumber={renumber}
      />
      <ConfirmDialog
        open={dialog === 'delete'}
        title={t('bulk.deleteTitle', { count: selectedSeqs.length, n: n(selectedSeqs.length) })}
        text={t('bulk.deleteText')}
        confirmLabel={t('delete')}
        busy={bulk.isPending}
        error={dialogError}
        onConfirm={deleteSelected}
        onClose={() => setDialog(null)}
      />
      <ConfirmDialog
        open={deleteOne !== null}
        title={t('rules.deleteTitle', {
          sequence: deleteOne ? fmt.number(deleteOne.sequence, { useGrouping: false }) : '',
        })}
        text={t('bulk.deleteText')}
        confirmLabel={t('delete')}
        busy={bulk.isPending}
        error={dialogError}
        onConfirm={deleteRow}
        onClose={() => setDeleteOne(null)}
      />
    </>
  );
}

const LTR_NUMBER = { dir: 'ltr', inputMode: 'numeric', min: 1, max: MAX_SEQUENCE } as const;

function isSequence(v: string): boolean {
  const n = Number(v);
  return v.trim() !== '' && Number.isInteger(n) && n >= 1 && n <= MAX_SEQUENCE;
}

/** "Move to sequence": the selected rules get `to`, `to + 1`, …; rules already there shift up. */
function MoveDialog({
  open,
  count,
  busy,
  error,
  onMove,
  onClose,
}: {
  open: boolean;
  count: number;
  busy: boolean;
  error: unknown;
  onMove: (to: number) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation('acl');
  const fmt = useFormatters();
  const [to, setTo] = useState('');
  const ok = isSequence(to);
  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>{t('bulk.moveTitle', { count, n: fmt.integer(count) })}</DialogTitle>
      <DialogContent>
        <DialogContentText sx={{ mb: 2 }}>{t('bulk.moveHelp')}</DialogContentText>
        <TextField
          autoFocus
          fullWidth
          type="number"
          label={t('bulk.moveTo')}
          value={to}
          onChange={(e) => setTo(e.target.value)}
          error={to !== '' && !ok}
          slotProps={{ htmlInput: LTR_NUMBER }}
        />
        {error !== null && <ProblemAlert error={error} sx={{ mt: 2 }} />}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('cancel')}</Button>
        <Button variant="contained" disabled={!ok || busy} onClick={() => onMove(Number(to))}>
          {t('bulk.moveRun')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

/** Renumber the whole list: start, start + step, … in the current order (10, 20, … by default). */
function RenumberDialog({
  open,
  busy,
  error,
  onRenumber,
  onClose,
}: {
  open: boolean;
  busy: boolean;
  error: unknown;
  onRenumber: (start: number, step: number) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation('acl');
  const [start, setStart] = useState('10');
  const [step, setStep] = useState('10');
  const ok = isSequence(start) && isSequence(step);
  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>{t('renumber.title')}</DialogTitle>
      <DialogContent>
        <DialogContentText sx={{ mb: 2 }}>{t('renumber.help')}</DialogContentText>
        <Stack direction="row" gap={2}>
          <TextField
            type="number"
            label={t('renumber.start')}
            value={start}
            onChange={(e) => setStart(e.target.value)}
            error={!isSequence(start)}
            slotProps={{ htmlInput: LTR_NUMBER }}
          />
          <TextField
            type="number"
            label={t('renumber.step')}
            value={step}
            onChange={(e) => setStep(e.target.value)}
            error={!isSequence(step)}
            slotProps={{ htmlInput: LTR_NUMBER }}
          />
        </Stack>
        {error !== null && <ProblemAlert error={error} sx={{ mt: 2 }} />}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('cancel')}</Button>
        <Button
          variant="contained"
          disabled={!ok || busy}
          onClick={() => onRenumber(Number(start), Number(step))}
        >
          {t('renumber.run')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
