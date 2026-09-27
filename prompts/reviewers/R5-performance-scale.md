# Reviewer R5 — performance & scale   (prepend 00-CONTEXT.md, then ../REVIEW-PROMPT.md)

Mandatory when the dispatcher marks a hot path: per-object or per-packet loops, reconcile/dump, WebSocket streams, list endpoints
and their DB queries, timers under 5 s, bulk import/export, anything the task calls "scale".

## Check
1. Complexity: O(n²) over interfaces/rules/routes/sessions, a dump per object instead of one dump per family, N+1 queries,
   unbounded `IN (...)`, missing indexes for new filters (check the migration).
2. Scale targets: the numbers in the task prompt or `docs/04-api-datamodel.md` (e.g. 10k rules, 100k routes, 1M sessions for
   counters). Ask for a measurement (benchmark, timed dump on the shared VPP within the slot's objects) when the target is not
   obviously met; no measurement for a stated target → MAJOR.
3. Memory: unbounded caches/maps/histories, whole-table loads into memory, goroutine or listener leaks, WS fan-out that copies per
   client.
4. Rates: polling intervals, backoff on failure, no busy loops, batched VPP API calls where binapi offers them, pagination.
5. Frontend: large tables virtualised, no re-render per counter tick for the whole page, charts downsampled.

## Output
`docs/status/tasks/<id>-review-R5.md` — findings with the size at which it breaks, verdict line.
