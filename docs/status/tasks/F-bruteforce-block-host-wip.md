# F-bruteforce-block-host recovery checkpoint

Branch `codex/autoblock-host-20261003`; base main cf486eff35e15dcf6fcd8a34711afb2695f03f9d.
Contract local bb59cbc1, published remote 3c1f9bbd58d03ff6cb79bedc65b1c7336b3703a9 (tree 0f4a3b89).
CLI push blocked by network403. Same repo connector public/admin/push authority verified by manager;
connector checkpoints use exact local trees and secret scan before publication.

Implemented WIP: additive runtime snapshot RPC, persisted TTL cache and existing GlobalBlocking VPP/nftables overlay;
security root persistence/projection dependency; API serialized snapshot publisher/retry and host event subscription;
trusted SSH/charon journal parser plus distinct-port scan detector and nftables observer chain.

Actual tests: proto regeneration succeeded with immutable shared verified-bin/buf1.73.0.
Initial full agent package Go run failed existing UNIX socket tests because sandbox socket() returns EPERM;
nftables package unit tests passed. This is not CI GATE PASSED.
API typecheck before dependency builds failed missing @ngfw/proto declarations. Build workspace dependencies next.

Remaining: targeted runtime service tests, journal detector tests, publisher DB ipCidr/address conversion regression,
source/projection edge-case review fixes, topology driver and documentation, mandatory unchanged quick gate,
independent review, published PR. Lab packet tests not run; no real VPP/nftables observed.

Exact next command: prepend toolchain/verified-bin,bin,go/bin,node_modules/.bin to PATH;
`pnpm --filter @ngfw/proto build && pnpm --filter @ngfw/schema build`, then targeted Go tests.

## Source completion checkpoint

Implemented real lifecycle: runtime snapshot bridge, cache replay, agent TTL expiry, VPP+nftables projection,
security-only/ACL-only commits and confirm rollback; API serialized republish/retry, mapped-address canonicalization;
SSH/charon provenance parser, nft distinct-port observer, native safe AUTH_FAILED peer/SPI detector; real topology
acceptance driver and user docs. No source is declared lab-only because of unavailable lab execution.

Focused actual evidence: Go runtime/detectors/nftables/agent race suites PASS; service fake-VPP test proves enforcement,
allowlist, update rejection/rollback+retry, security-only disable+confirm-revert, ACL-only preservation, retrieval purity,
cache replay, independent expiry and RPC owner/invalid-entry guards. API engine/publisher22 tests PASS before last
capacity/native additions; rerun final focused suites before final report. ESLint feature/client/fake-agent PASS with
only existing Node module-type warning. Driver syntax check PASS; live packet acceptance NOTRUN.

Host capability explicitly20000 runtime entries, preserving gRPC4-MiB boundary without changing privileges/transport.
Default10000 unchanged; higher configuration rejected during agent validation, never silently truncated. Capacity and
worst-case IPv6 proto-size tests added. Native detector requires the existing secret-safe state capability, dedups
owned failed SA SPI pairs and matches configured local/remote endpoints; polling may miss transient failures.

Unchanged full quick is running at `/tmp/autoblock-quick.log`; source is not merge-approved without its actual result.
Remote publication held by manager after automatic approval review rejected public publication; local checkpoints
remain available. Do not retry connector mutations until manager receives explicit user publication authorization.

Final follow-up: charon runtime excluded; disabled legacy user list compatibility (projection/retrieve/count) fixed;
soft configuration cap is API admission only (no agent truncation/refusal); hard cap20000 retained. Fake dataplane-loss
cache recreation regression passes. Final focused Go race outputs1.200/1.032/1.137/1.474s, golangci0issues, vetPASS,
APItypecheckPASS, focusedAPI23PASS, ESLintPASS. Fullquick baseline API socketEPERM/chownEINVAL failures observed,
rest of Turbo still pending; no full gate pass. Detailed final report F-bruteforce-block-host.md.
Exact next command: inspect `/tmp/autoblock-quick.log` and its Turbo log; obtain two final-source reviewers;
manager owns publish authorization/PR/full hosted gate/merge and lab-deferred acceptance.

Final source df50c039d7a4ea860c83d3d5898f2d11d9dc7b07 treea4a53df9. Full quick own session6534 interruptedCtrl-C;
actual exit130 after baseline API EPERM/EINVAL failure+Turbo stall, notPASS. R1 independent exact-source APPROVE
reported by manager with independent racePASS; R2 final review pending. No more source changes; docs-only handoff.
No pending full CI process or remote mutations from this worker. Manager owns next action.
R2 finalAPPROVE df50 exact with independent Go-race/API23/diffcheckPASS; both R1/R2 approve source. Final handoff
complete, no more worker actions. Next manager action requires publication authorization then hostedquick/lab.
