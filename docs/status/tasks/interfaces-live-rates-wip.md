# Interfaces live rates — verified local fix

Branch: codex/interfaces-live-rates
Worktree: /home/mohammad/ngfw-worktrees/original-rebase
Local product commit: e06fae305; published equivalent product commit: 39ad0ed24274a066fa1bcd389bdeaeefa967fe61.
Owned files: apps/web/src/domains/interfaces/rates.ts, rates.test.tsx and task status documents.

Reproduced StrictMode updater replay discarding samples: regression failed before fix. Combined immutable state snapshots fix replay without changing backend contracts.
Validation: 9 focused tests PASS; web typecheck PASS; lint PASS; check gate PASS; production build PASS, 599.8 kB budget PASS, no-dev-routes PASS. Independent review APPROVE.
Live verification: local5174 against appliance211 remains logged in; RX/TX change and trend grows from20 to30 samples, replacing previous frozen4 samples. Screenshot /tmp/ngfw-interface-rates.jpg. Server session56741 runs this worktree with verified certificate/servername configuration.

Complete quick gate blocked by missing tools requiring sudo installation; no main merge or appliance deployment. Common-ui memo comparator separately fixed/tested on its own branch, but Interfaces disables that memoization; not needed or integrated here.
Recovery: run pnpm --filter @ngfw/web exec vitest run src/domains/interfaces/rates.test.tsx src/domains/interfaces/model.test.ts. Next integration step: complete unchanged quick CI in provisioned environment before merge.
