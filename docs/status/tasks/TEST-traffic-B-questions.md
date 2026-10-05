# TEST-traffic-B prerequisites observed through production REST

## WireGuard secret channel — actual product failure, 2026-10-05

The first owned REST attempt failed HTTP 403 `license-required` at
`/vpn/wireguard/interfaces/site`. The fixture now issues and verifies a one-day
signed test licence using the existing production licence tool and a trust key
confined to the owned API process. No licensing guard is disabled. Signing
material, tokens and raw private logs remain outside Git.

The unchanged packet driver then passed entitlement checking, uploaded the key
through production `POST /secrets?replace=true`, and committed its candidate.
Actual response: HTTP 422 `https://ngfw.dev/problems/apply-failed`, agent status
`rolled_back`, running unchanged. The reason was:

> wg2760 private key key/w27tb-a: no secret material in the agent

The agent additionally reported `PENDING-secret-channel`. Shared VPP PID and
restart count were unchanged. Private log:
`.scratch/traffic-b-rest-wg-license/wireguard.private.log`.

This is a real prerequisite failure, not laboratory deferral. Proposed minimal
separate product task: deliver referenced sealed API secrets over the existing
agent secret bundle during REST commit, with bounded material access, owner
validation, no diagnostic disclosure, and rollback/restart tests. The manager
owns its separate implementation/review/PR. This test task will not substitute
manual gRPC secret injection or label this REST phase passed.

## Native IPsec/PKI unsupported warnings

The manager identified potentially stale unsupported warnings in the WireGuard
converter for IPsec/PKI fields. No native REST result has yet been observed after
licensing setup. Changed-path warning checks remain strict; an actual warning
will be recorded here before any separately reviewed product correction.

### Native REST result now observed

At source a45391efc, the native PSK campaign reached the production REST commit.
The strict check refused the actual warning at `/vpn/ipsec`:
`vpn.ipsec is not implemented by this agent build (P11)`, rule
`agent.unsupported-field`. Private run summary:
`.scratch/P11-host-20261005-181530-394018/summary.json`; responder diagnostics in
that same owned run directory. Shared VPP MainPID1014/NRestarts0 unchanged.

Proposed separate product correction: remove the obsolete blanket unsupported
warnings only for subsystems actually handled by the current registered native
IPsec/PKI implementations, retaining unsupported errors for genuinely missing
fields. Independent tests must demonstrate active changed IPsec/PKI configuration
no longer emits stale warnings and genuinely unsupported fields remain rejected.
The test task does not modify that product converter or waive its warning check.

### WireGuard nullable timestamp observation

The tagged fixture's REST response reports actual peer `established: true`,
`status: established`, `eventsActive: false`, `lastHandshake: null`. The production
API contract explicitly makes lastHandshake nullable because it records when the
agent observed an event; it is not VPP's handshake timestamp. Acceptance uses the
actual established state plus a positive kernel `wg latest-handshakes` timestamp,
required ping and UDP capture. Missing event timestamp remains explicitly recorded;
no synthetic timestamp is generated. The first over-strict assertion failure is
preserved in `.scratch/traffic-b-rest-native-state/wireguard.private.log`.
