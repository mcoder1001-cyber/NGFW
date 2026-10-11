# WAN-only setup contract checkpoint

Branch: `codex/wan-only-contract`; initial local SHA: `d2d55984d`.
Remote publication: pending parent integration; parent explicitly requested no branch push yet.
Owned files listed in `wan-only-contract-envelope.md`.

Completed code: optional all-or-none legacy LAN input, WAN-only preservation, single WAN inventory selection; PPPoE cleanup refuses migration when preserved policy references would dangle.
Actual tests: dependency installation passed. Schema build and focused schema/API unit tests running; results pending.
Remaining: finish focused tests/lint, contract-first commit, API commit, parent handoff.
Current failure: none observed.
Next command: `pnpm --filter @ngfw/schema exec vitest run src/setup.test.ts`.
