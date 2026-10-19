# Common UI migration WIP

Integration branch: `codex/common-ui-migration-20261008`; base `417e8fcd`.
Latest product checkpoint: `c7078948`. Remote SHA: none. Root publication attempts on
`ef1623b5` and `1a1f922f` failed HTTP403: repository denied authenticated account hightwig.
Worker branches have the same publication failure. No GitHub connector available.
Owned files/scope: common-ui-migration-envelope.md.

Completed code: responsive real Common UI shell/breadcrumb/navigation preserving
all routes; portable Common UI source workspace package; controlled Pg schema
primitives/panels/actions; full-capability server grid; shared Common UI styling
for tables/dialogs/tabs/cards; all handcrafted text fields/buttons migrated,
including developer demo. Existing API/auth/datastore/config transactions preserved.
Nested forms keep RHF/schema validation; usePgForm remains available for simple Pg
forms. Style caches now reused per document/direction; normal read-only styling uses
MUI state classes. Accessible collapsed disclosure targets remain mounted without
retaining hidden menu contents. No unfinished local implementation known.

Verified results:
- Common UI strict TypeScript, authored-source lint and JS/declaration build pass.
- UI-kit TypeScript/build/lint pass. Existing90tests validated:84 passed full run,
  six WebSocket tests passed sandbox-escalated retry. New theme remount2tests pass
  for LTR/RTL, stylesheet reuse, visible controls and document language/direction.
- Web639tests validated across full run and final targeted retries: initial full
  run102/108files,627/639tests pass; remaining6files/55tests all pass after navigation,
  viewport and style-performance fixes. Final retry304.47s with maxWorkers1, no
  assertions/timeouts weakened. Covers App, Interfaces, Bonds, Bridging, LLDP/
  mirroring/nsim, Neighbours. Demo passes14.71s; all15Interface tests pass.
- Final web production build passes,588.5kB initial gzip under unchanged600kB budget,
  138lazy chunks; developer-route exclusion passes. Web lint/logical CSS pass461files.
- Policy-only gate `tools/ci.sh check --base origin/main` passes,212task board rows.
- Independent applicable reviews approve forms/shell/grid and cache-lifetime fix.

Actual commands/evidence: `pnpm --filter @ngfw/web build`; `pnpm --filter @ngfw/web
lint`; `node node_modules/vitest/vitest.mjs run --maxWorkers=2` in apps/web;
then same Vitest command with maxWorkers1 for the six named files above. Full log
/private/tmp/web-all-tests.log; final retry/private/tmp/web-retry-tests-final.log.
Build/private/tmp/ngfw-final-build.log. No running test workers remain after retry.

Remaining acceptance: actual feature-page visual verification against a real device
API; remote checkpoint publication, hosted unchanged quick CI and PR/merge. Current
app preview http://127.0.0.1:5189/ (Vite session28884) shows the existing unavailable
API retry surface because localhost3000 does not answer. Approved isolated sample
remains on5188. No production fake API or authentication bypass added.

Full unchanged quick attempted: stopped at missing golangci-lint/gitleaks; pinned
GitHub downloads failed DNS resolution. Host also lacks Go/buf. This is an unresolved
mandatory gate, not deferred lab acceptance. No merge and no published PR.
Next action: restore authorized remote write/device access, publish the existing
branch, run hosted unchanged quick CI, then visually verify en/fa, light/dark and
mobile against actual API. Resume this branch; do not rebuild or create duplicate work.

Final review follow-up: legacy PgText now passes readOnly to both MUI InputBase
and native htmlInput, preserving state-class styling and keyboard/copy semantics.
Common UI declaration build passes after this small correction.

Owner requested local consolidation: merge2df604a3 joins shell/forms/grid histories
into the existing migration branch using the tested integration tree.
`git diff HEAD^1 HEAD` verified no product/file changes from this merge.
Both real app5189 and approved sample5188 listeners verified HTTP200.
Preview workspace duplicate YAML key removed so its pnpm scripts remain usable.
