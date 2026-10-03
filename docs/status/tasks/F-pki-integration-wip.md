# PKI modern integration WIP — 2026-10-03

Branch: `codex/pki-integration-20261003`; base `origin/main` = `a2378278`. Publication is delegated to root; no push/merge authorized here.
Owned paths: additive PKI hunk in `packages/schema/src/domains/vpn.ts`, PKI schema contract tests, additive PKI messages/RPC in `packages/proto/vrx/v1/dataplane.proto`, PKI fixture and regenerated contract outputs, standalone API PKI paths and registration anchors, task-specific status/docs.

Completed sources: extracted the exact historical PKI schema additions (163 inserted lines, no unrelated IPsec change); inserted only 102 proto lines covering PKI fields, messages and read-only RPC while preserving modern domains. Added contract tests preserving reference-only configuration and checking algorithms, CSR fields, CA facts, alert validity and issuer metadata. No consumers transferred yet.

Running: `/root/.codex/worktrees/0b16/NGFW/tools/heavy.sh bash .scratch/pki-contract-check.sh` (log `.scratch/pki-contract-check.log`). Repository scheduler regenerates schema/proto; then local proto build, focused VPN/PKI schema tests, proto tests and buf lint. Dependency directories are private directories linking shared on-disk installed dependencies, with local workspace package overrides; never committed.

ETA: 20–40 minutes for contract regeneration/build evidence plus semaphore queue; API transfer/typecheck another 10–25 minutes if modern signatures differ. Current blocker: validation in progress/queue, no product failure observed yet. Next command: `tail -80 .scratch/pki-contract-check.log`. Contracts will be committed before API consumers.

Remaining: review generation diffs for unrelated drift; contract checkpoint; API transfer/registration and modern compatibility tests; independent review and root publication/quick gate; agent wiring/UI/client are separate unfinished scope. No VPP or existing host services accessed.
