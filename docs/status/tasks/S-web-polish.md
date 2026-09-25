# S-web-polish — web polish for SNMP, flow export, host stack, licensing, LISP

## What
- **fa locales**: en/fa key sets were already identical; fa strings reviewed (technical terms kept). New/changed keys added with natural Persian. One `locale.test.ts` per domain folder (snmp, ipfix-sflow, host-stack, licensing, lisp) asserts identical en/fa key sets and that no English sentence is left in fa.
- **States**:
  - SNMP: loading bar, problem+json alerts for candidate and state load failures (title/detail/`errors[].pointer` via `ProblemAlert`), empty-state guidance rows for communities, v3 users, trap receivers.
  - Host stack: loading bar, candidate load problem, live-state problem detail under the existing "Live state unavailable" warning.
  - Flow export and LISP already had loading/error/empty states (LISP save errors map the pointer onto the form); unchanged.
  - Licensing (D-151): community and expired/invalid status texts now say new configuration for gated features is refused at commit until a licence is installed, with an "Install a licence" link to `#license-upload` (the upload section, now a labelled region). The shell banner also shows for `community` (warning) and links to `/system/licensing#license-upload`.
- **a11y**: licence status chip has `role="status"` + aria-label; host-stack live chips grouped in `role="status"`; upload section `aria-labelledby`. `check-logical-css` passes (no physical CSS in these folders).

## Verification
```
npx turbo run lint typecheck test --filter=@ngfw/web --concurrency=1
 check-logical-css: OK (146 files, no physical left/right CSS)
 Test Files  26 passed (26)
 Tasks:    14 successful, 14 total
tools/ci.sh check -> check PASSED
```
(Without `--concurrency=1`, turbo's parallel `gen` tasks raced on a pnpm reinstall in this worktree: ERR_PNPM_PACKAGE_MANAGER_REMOVE_MODULES_DIR. This is an environment problem, not a code problem.)

## Out of scope
Shared components (ProblemAlert, SchemaForm), ui-kit, routes, other domains. Showing the licence banner in the shell for `community` affects every page when no licence is installed. That is intended under D-151.

## Open questions
- Should the shell banner for `community` be dismissible? Right now it stays visible until a licence is installed.
