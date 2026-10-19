# Interfaces live rates checkpoint
Branch: codex/interfaces-live-rates; base/local parent 493ecc1dfd55eecd129f4623491b1a6a32ebe104. Remote publication pending.
Owned files: rates.ts, rates.test.tsx and interfaces-live-rates status documents.
Reproduced: StrictMode regression loses all rates before fix (1 failed, 1 passed).
Implemented: keep samples and rates in a single state snapshot; copy samples inside updater before folding, making updater replay safe.
Tests: rates + model = 9 passing. Typecheck pending completion. Independent review requested.
Browser before fix: Ethernet5 SVG stuck on four points across several minutes while REST counters advanced.
Remaining: live browser verification, lint/build/quick gate, publish checkpoint.
Next command: pnpm --filter @ngfw/web lint
No main merge authorized by passing CI yet.

Verification update: typecheck, web lint (487 files) and tools/ci.sh check --base HEAD passed. Independent reviewer diagnose_contracts APPROVE. Browser after fix: RX/TX changed between observations and Ethernet5 history grew to 20 then 30 points; screenshot /tmp/ngfw-interface-rates.jpg. Local5174 now serves this worktree with existing verified TLS settings for appliance211; no appliance changes.
Full quick gate blocked: tool installation needs sudo password (golangci-lint/gitleaks and other prerequisites); no main merge. Production web build pending.
