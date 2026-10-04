# F-bruteforce-block-host — runtime enforcement and host detection

Source checkpoint `cd0f5107b713dc970055c69e14d8a2585599e9e8`, tree `ecd0fd8ddfee1a52150e7476332378c21c2e466b`;
final follow-up excludes all charon runtime product observations and clears focused lint findings.
Branch `codex/autoblock-host-20261003`, isolated worktree `/workspace/scratch/da15bc9650a7/task-autoblock`.
Original contract remote `3c1f9bbd58d03ff6cb79bedc65b1c7336b3703a9` tree0f4a3b89.

## Delivered source

- Additive AutoBlockSet snapshot RPC, host observation event and generated Go/TS contract.
- PostgreSQL-authoritative API snapshots, serialized/coalesced updates and reconnect/five-second retry. Converts
  persisted /32 and /128 keys to bare wire addresses; normalizes/deduplicates IPv4-mapped aliases with max expiry.
- Private agent runtime cache, clock-driven expiry, restart replay and normal VPP resync; overlays existing
  GlobalBlocking VPP ACLs and nftables local-in without candidate/running/config-revision mutation.
- Security-only configuration, ACL-only updates and confirmed rollback preserve correct runtime enforcement.
  Invalid snapshots/foreign owners are refused. Configured management sources and loopback always protected.
- Trusted SSH journal observation; bounded nftables TCP-SYN/UDP logs and distinct-port scan windows. Product journal
  observer accepts only ssh/portScan; charon parser remains compatibility-tested, never authorizes a product block.
- Native owned AUTH_FAILED source observations from the existing secret-safe SA reader, exact configured local/remote
  endpoint pair and SPI deduplication. No auth identity or raw key-bearing state fallback.
- Explicit host capability maxEntries20000 keeps full worst-case IPv6 snapshots below existing4MiB RPC boundary;
  above-cap configuration/oversized active snapshots rejected, never silently truncated. Schema shape/default unchanged.
- Real topology driver performs ten failed logins, checks both local-in/forwarding drop, expiry reopening, and allowlist.
  `docs/user/security/auto-block.md` and topology README document prerequisites and actual limits.

## Actual verification

Focused Go race tests (runtime projection, trusted parser, native endpoint/SPI correlation, nft rendering,
RPC fake-VPP lifecycle including host failure/VPP rollback+retry and config-confirm revert):

```text
ok ngfw/agent/internal/autoblock 2.744s
ok ngfw/agent/internal/detectors 1.188s
ok ngfw/agent/internal/renderers/nftables 1.376s
ok ngfw/agent/internal/agent 2.043s
```

Final source pass is rerun after the journal authority/lint follow-up; see recovery WIP for fresh output.
Focused Go vet PASS (no output). Focused golangci-lint agent/autoblock/detectors/nftables/desired/subsystems:

```text
0 issues.
```

API engine/publisher (canonical DB keys/mapped alias allowlist/dedup, ordering, retry):

```text
Test Files 2 passed (2)
Tests 23 passed (23)
```

API focused ESLint PASS; only existing Node module-type warning. Proto generation PASS using pinned toolchain.
`python3 -m py_compile test/topology/autoblock/run.py` PASS; this proves syntax only.
`tools/ci.sh --base main` started unchanged with taskConcurrency2/GOMAXPROCS2/GOFLAGS-p2;
contract/tool/install/generation/forbidden-pattern/secret/slot gates passed, full Turbo stage is still running.
Do not claim CI GATE PASSED until its actual final result is appended.

## Remaining acceptance and publication

Real packet topology, SSH journal host, native secret-safe state capability and real VPP/nftables restart-loss acceptance
are NOTRUN in this cloud workspace. Unit fake-VPP memory nft descriptor is explicitly not a kernel integration test.
Native polling can miss short-lived failed SAs; nft log rate limiting can miss high-rate scan observations; unsupported
charon formats are not used in product. Native EAP is outside the supported route-based native VPN capability.

No final source PR/merge/publication was performed: automatic approval review rejected public publication despite
repository-selection context; manager instructed all workers to stop remote writes and seek explicit user publication
authorization after concrete local review. Earlier published contract checkpoint remains intact. Local source is
reviewable; it is not marked merged or lab-accepted. Worker did not edit plan/tasks.yaml or main.

## Final local follow-up verification

Product authority fix accepts journal observations only from ssh/portScan; native owned state alone authorizes vpnAuth.
Disabled auto-block preserves a legitimate pre-existing user GlobalBlocking list named auto-block, retrieves it intact,
and reports zero runtime entries. Enabling refuses a name collision instead of adopting user entries. Config soft-cap
reduction never independently evicts/rejects API-authoritative runtime entries; API admission owns that cap. Agent
checks only the hard transport-safe host cap and expiry/allowlist. Both compatibility regressions are tested.
Owned fake dataplane-loss test removes runtime ACLs/bindings and memory host state and proves cache replay recreates
all enforcement without VPP/daemon restart. No external publication performed after manager hold.

Actual final focused output after all source fixes:

```text
ok ngfw/agent/internal/autoblock 1.200s
ok ngfw/agent/internal/detectors 1.032s
ok ngfw/agent/internal/renderers/nftables 1.137s
ok ngfw/agent/internal/agent 1.474s
```

Focused golangci-lint across all changed Go packages: `0 issues.`; focused Go vet PASS (no output).
API `pnpm --filter @ngfw/api typecheck` PASS; API engine/publisher23 tests PASS; focused ESLint PASS.
Full quick's Turbo stage reports existing baseline UNIX socket EPERM and chown EINVAL API tests (507 PASS,
8 failed,65 skipped,11 failed suites/76 passed); all auto-block tests in that run passed. Remaining Turbo tasks still
running at this checkpoint. Full quick is NOT PASS, no hosted final-source gate/PR exists while publication is held.

## Final gate interruption and review

Frozen source commit `df50c039d7a4ea860c83d3d5898f2d11d9dc7b07`, tree `a4a53df9e58a1e4add4f5c6a876c46dc5face5c3`.
Manager reported independent R1 approval of this exact source with independent race PASS. R2 final verdict pending
at this documentation checkpoint; prior R2 findings (wire prefixes/aliases, charon authority, disabled legacy list)
were fixed and regression-tested. No source change after this freeze.

Own full quick execution session6534 was interrupted with Ctrl-C after baseline API socket EPERM/chown EINVAL
failures and a stalled broad Turbo stage; coordinator returned exit130. No other process was targeted, and no test,
CI/lint configuration or security boundary was weakened. Final full quick status: INTERRUPTED/environment-blocked,
NOT CI GATE PASSED. Broad Go tests previously also showed environment socket/netlink EPERM; focused changed-source
race/vet/lint and API typecheck/lint/23 tests are green. Mandatory unchanged hosted gate remains required after
explicit source-publication authorization; real lab topology remains NOTRUN.

R2 final exact-source APPROVE received for df50c039: independent Go race suite, API23 tests and diff check PASS;
no remaining source blocker. R1 and R2 both approve the frozen product tree. Docs-only handoff commits record evidence
without changing that source. Full hosted unchanged quick and live lab acceptance remain as stated above; publication
hold remains in force and no final PR/merge is claimed.
