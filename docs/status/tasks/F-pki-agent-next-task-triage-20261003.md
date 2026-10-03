# PKI agent next-task triage — 2026-10-03

Read-only comparison: main6b7fdda2 and combined PKI3bcf981b. No agent/internal product diff between those refs; current main still marks strongswan pendingP11 in subsystems/reachability_test.go. No product/test/generator/service modifications or heavy execution performed.

## Exact remaining gaps

The generated RPC is named PkiFileState, not GetPkiFiles. Generated Go/TS messages and Dataplane methods exist. Agent server embeds UnimplementedDataplaneServer and has no concrete PkiFileState method, so requests currently return inherited UNIMPLEMENTED. API agent-files.ts already calls that RPC with owner and3s deadline, translating errors into the public state agentFiles.unavailable branch; testing/fake-agent.ts likewise returns UNIMPLEMENTED intentionally.

No modern PKI materializer package, pki.files/vrx scheduler object, manifest lifecycle, stale removal/recreation, file permission/directory confinement, private keyed fingerprint state or retrieve observer is wired. strongswan/model.go already maps cert-auth names to certificate/CA .pem filenames but does not create those files. Paths.SwanctlDir exists; renderer writes config/secrets files separately. Product subsystem remains pendingP11. These gaps are not resolved by API certificate generation/storage or successful public inventory tests.

Secret resolution currently remains feature-local; shared product API-to-agent transport/cache is externally owned P11 scope (historical PENDING-secret-channel records the restart-before-first-resync invariant). PKI requires certificate/CA/key/CRL content availability, stable agent-local D096 keyed fingerprints, restart-safe replay, materialization before charon config loading, rollback semantics and no secret echo. Do not independently add DB/master-key access, new secret RPC/bundle fields, sealed-cache format, strongSwan Apply/renderer ownership changes or scheduler startup wiring.

## One bounded independent next task

Implement the existing read-only PkiFileState RPC honestly for an unwired agent: a NEW apps/agent/internal/agent/rpc_pki.go method using existing owner validation and service clock, returning correct owner/retrieved_at, empty root/files and fixed public unavailable reason (materializer not wired). Add NEW rpc_pki_test.go tests for owner mismatch, valid owner, empty-owner convention matching neighboring feature methods, deterministic timestamp, empty inventory, no server/data-plane/file mutation and no secret-bearing error fields. A bufconn adapter-level test should establish that this RPC no longer silently inherits UNIMPLEMENTED. Keep returned reason accurate; never scan arbitrary host directories or imply installed certificates.

Own only those two new files plus unique WIP. No schema/proto/generated/shared subsystem/P11 files, fake-agent edits, live host services or sockets outside offline test transport. This delivers accurate existing API/UI unavailable state and a concrete RPC boundary now, without inventing a transport. Estimated implementation/review range:45–90min plus root-owned test queue; estimate only.

Cannot independently claim file inventory/materialization complete: nonempty manifest observer and scheduler wiring must coordinate with P11 provider/strongSwan integration ownership; actual private-key delivery/restart-safe materializer requires that shared transport/cache contract and deployment acceptance. The small RPC task must remain explicitly unavailable until those dependencies exist. Parent may prefer parallel offline materializer library work afterward, but its input provider and key/manifest ownership should be agreed first.
