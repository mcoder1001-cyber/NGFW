# PKI API modern integration WIP — 2026-10-03

Branch `codex/pki-api-integration-20261003`, base contract checkpoint `866342af`, isolated worktree `/root/.codex/worktrees/0b16/developers/PKI-api`. No push/merge; root publishes checkpoints.

Transferred only standalone PKI API files and user documentation, with three app.module registration anchors and PKI fake-agent UNIMPLEMENTED handler. Fixed import accepting both key PEM and key reference: now rejects at `/privateKeyRef` before any parsing/read/storage. Added five secret-mutating routes to existing privileged write-ahead audit list, admin route matrix and 503 documentation. Added HTTP offline tests for all five operator refusals, all five audit-unavailable refusals and successful redacted CSR audit. Corrected documentation SAN example to unprefixed hostname required by schema.

Owned shared hunks beyond app.module: only five PKI entries in audit privileged route list and admin route test matrix; exact PKI fake-agent handler. No management lifecycle/other feature changes. No agent/renderers transfer and no host access.

Validation pending root execution of immutable code checkpoint. Command: `/root/.codex/worktrees/0b16/NGFW/tools/heavy.sh bash .scratch/pki-api-check.sh` from this worktree. The script uses installed shared binaries, builds local schema/proto/yang, runs PKI and global route-guard tests, then API typecheck and scoped ESLint. Expected duration 3–8 minutes plus semaphore queue. Dependencies are private ignored node_modules directories, with local workspace links.

Contract evidence from frozen modern checkpoint: repository schema/proto generator passed (54.628s), proto build passed, schema VPN/PKI 127 tests passed, proto 106 tests passed, buf lint passed. Earlier historical API 27 tests/typecheck passed; no current API passing claim yet. Remaining: current API failures if any, independent review, generated REST client and modern quick gate, plus agent wiring/UI/live integration beyond this subset.
