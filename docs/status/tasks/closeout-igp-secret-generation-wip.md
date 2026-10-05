# Transactional FRR password generation

Branch codex/closeout-igp-secret-generation; base98b48a7e. Publication local only, manager handles remote.
Owned desired/bgp.go + frr_secret_generation.go/test contract, subsystems/frr.go/new generation tests,
agent/agent.go startup source binding/targeted integration tests as needed, this status.
No shared VPP/services/NIC/package/reference changes.

Real failure b868217d: same-reference new sealed MD5 bundle Apply APPLIED unchanged11,
no FRR reload, old-password peer retained100 routes after15s. Never relabel that pass.
Design independently approved in principle by traffic: internal per-reference keyed HMAC
bindings (active selected sealed cache.Ref), historical cache.Resolve for transactional
rollback, scoped immutable context binding, truthful saved-generation retrieval verified
against actual applied files. No plaintext API/user document/hash/debug output and no
posttransaction reapply-success shortcut. Exact coverage/unknown/malformed bindings fail.
Contracts commit before consumers. Contract code/test pending; no actual fix claim yet.
Next focused contract race through tools/heavy.sh, then independent exact-source review.

Contract ae0c70e0202e283cd6c66baf72c9d973e286d36a committed before consumers;
independent exact review e606d395. Consumer now projects only filtered FRRDoc reference
bindings, uses immutable scoped historical resolution for real reload/rollback, refuses
missing bindings in generation-enabled runtime, and retrieves last applied generations
with existing actual file convergence status. User DesiredState and all other scheduler
contracts unchanged. Existing restoreSecretSelection retains current+confirmed snapshots
only after transaction rollback returns, and preserves snapshots during degraded recovery.

Actual consumer checks: full race subsystems PASS28.084s, desired PASS33.931s,
agent PASS77.119s. Final focused tests PASS1.576s/1.562s include a real scheduler dependent
failure rolling back old password while the new cache candidate stays active; unavailable
bound history cannot fall back to active Text; no reload on missing history; unchanged
references preserve identity, referenced rotation changes identity, unrelated description
is ignored. Fixtures use generated random credential values, no logged secret material.
Earlier contract checks PASS1.601s and source check PASS11s.
Remaining: exact independent consumer review, lint/fresh production build, actual peer
rotation and old/new negative checks plus rollback/confirm behavior. No live fix PASS yet.
Next: tools/heavy.sh golangci-lint run --allow-serial-runners ./internal/desired ./internal/subsystems ./internal/agent
(from apps/agent), then owned fresh agent build and traffic's private acceptance.

Frozen consumer source 689af4239e5bb743f5df200f6fe6bfaa87be1700 independently approved
conditionally by review f4b10240. Scoped golangci-lint: 0 issues; fresh production build
PASS; source check PASS11s. Exact binary apps/agent/bin/ngfw-agent-igp-secret-generation
SHA256 a118b652c16c0182f39fd6e6880e50c83f2e7f4679be2bf177fccc3e9ac7a1d4.
Traffic owns the strict same-keyRef live campaign plus confirm-revert extension; pending.
No product edits since that source checkpoint. Full integration gate is still manager-owned.

Actual strict same-reference live evidence now d17ab733f582f514b53b5e6516596e81b74cd0bf
in independent codex-closeout-igp-auth-fixed-live. Executable fixture6202eabe unchanged;
real agent Start compiled from frozen689af423 (not prebuilt executable substitution).
PASS152.14s/package152.248s, no skips. Raw independently inspected: original ref
rotation updated1/unchanged10, old peer FRR+VPP routes100→0, new matching100;
actual agent restart100; real confirm30s armed/fired, candidate old100→0 upon
historical rotated restoration, rotated matching100; withdrawal/restoration,
auth removal rejects still-MD5 peer then plaintext matching100, routing rollback0.
Shared VPP1014/NRestarts0 unchanged, private instance stopped and namespaces removed.
Shutdown context-canceled dynamic-source log findings remain raw, no failed lifecycle
step relabelled. Scope OSPF IPv4/default-VRF MD5 only; RIP/ISIS/BGP and broader IGP
packet/authentication matrix remain unproven. Product author inspected independent
fixture/raw evidence; product approval remains separate traffic/root review plus full
integration gate. No full IGP task Done claim.
