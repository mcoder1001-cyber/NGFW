# WAN-only setup contract checkpoint

Branch: `codex/wan-only-contract`; contract local SHA: `453e33a2fe2dfa8a01998e8333a1740d17c334d7`.
Remote publication: pending parent integration; parent explicitly requested no branch push yet.
Owned files listed in `wan-only-contract-envelope.md`.

Completed code: optional all-or-none legacy LAN input, WAN-only preservation, single WAN inventory selection; PPPoE cleanup refuses migration when preserved policy references would dangle.
Actual tests: dependency installation passed; `pnpm --filter @ngfw/schema build` passed; focused schema setup tests 20/20 passed (including default-route adoption and WAN prefill preservation); focused API setup controller tests 17/17 passed; `git diff --check` passed. Package schema/API lint passed before route/prefill follow-ups; schema lint rerun and API typecheck pending. `tools/ci.sh check --base main` passed (local main is historical, parent must use current origin/main for final gate).
Remaining: finish schema lint rerun/API typecheck, parent handoff. Parent must regenerate client and run complete quick gate before merge.
Current failure: none observed.
Next command: `pnpm --filter @ngfw/api typecheck`.
