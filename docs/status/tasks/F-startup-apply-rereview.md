# F-startup-apply — re-review after fix round 1 (independent review agent, 2026-09-24)

Branch `task/F-startup-apply` @ 7d81982 (fix commit b0a2a90, evidence 7d81982, merge of main 8adfa86); findings of
`F-startup-apply-review.md` (edd93a5). Run on the host, read-only: `apply-startup.sh` was **never** run with `--apply`
against real paths, VPP not restarted, no NIC bound/unbound, nothing written under `/etc`. Break attempts used a scratch
copy of the fake-host harness (outside the repo, deleted). Real-host actions: `vrx-vppcheck bootid`, `systemctl show vpp`,
`ss -Htn state established dst <peer>`, `ip -j route`, one `ping` to the gateway.

## What I ran

| check | result |
|---|---|
| `tools/ci.sh --base main` | **CI GATE PASSED** (quick, wall 3m26s, logs `/root/ngfw-wt/logs/ci/F-startup-apply-20260924-045747-2839043`) — matches the pasted run (only warning: the manager's `review(...)` subject) |
| `shellcheck deploy/vpp/apply-startup.sh deploy/vpp/test-apply-startup.sh` | clean, exit 0 |
| `deploy/vpp/test-apply-startup.sh <gen>` | see "Fake-host test" at the end |
| `vrx-vppcheck bootid` (branch build) on vrx-a | `b7712a53-…/2808617/5808198`, exit 0; `systemctl show vpp` → `MainPID=2808617` (identity PID = MainPID, as claimed), `Type=simple`, `Restart=always`, `RestartUSec=100ms`, `NRestarts=5` |
| mgmt facts on vrx-a | `SSH_CONNECTION` peer 172.30.126.196 (on-link via ens192, `ss` shows the ESTABLISHED :22 session); default route `via 172.30.126.1 dev ens192 onlink`; gateway ping → exit 1 (still drops ICMP) |
| contract / binapi | branch diff vs main touches none of `packages/schema`, `packages/proto`, generated code, `apps/agent/binapi`, `tools/binapi-gen.sh`; `bootid` uses the existing `internal/vpp/bootid` (control_ping from generated `memclnt`) |
| scope | vrx-vppcheck, the two scripts, the renderer doc, status files — all owned |

## Original findings

| # | verdict | evidence |
|---|---|---|
| H1 NRestarts reset by `systemctl restart` | **FIXED** (one gap → N3) | NRestarts no longer used. `vpp_restarted_ok` (`apply-startup.sh:443-453`) requires a new D-080 identity whose PID equals `MainPID`; `check_health` (`:455-460`) requires `MainPID`+`ActiveEnterTimestampMonotonic` and the identity unchanged. PID reuse: harmless — the triple carries the `/proc` start time, so an equal PID with a new start time is a new identity. MainPID vs VPP: control_ping `vpe_pid` = 2808617 = `MainPID` on the real host (Type=simple, `/usr/bin/vpp` is the main process). Crash + systemd auto-restart **inside the window**: both the unit tuple and the identity change → rollback (scenario 10 passes). Not covered: a crash **between `systemctl restart` and the first identity read** (N3) |
| H2 gateway drops ICMP | **FIXED for the happy path; the session-close path is unsafe-noisy** (→ N1) | `auto` on vrx-a picks `ssh-peer` (verified: the real `ss` query returns the manager's session) and otherwise refuses with exit 3 before anything changes (gateway-ping only when it answers now) — viable. Exact address/route comparison added (`mgmt_state_ok :423-431`). But if the manager's SSH session closes during the window the apply rolls back, the rollback is judged INCOMPLETE by the same probe, and the dead-man restarts VPP again N times and ends with "console access needed" — reproduced (N1). The doc says only "keep it open" (`vppstartup.md:114`) |
| M1 locks free between run and dead-man | **FIXED in the systemd path, NOT in the setsid fallback** (→ N2) | Separate holder unit (`start_holder :378-391`, `stage_hold :401-412`); dead-man kills the run but not the holder; holder gone → dead-man takes the locks first; FORCED only with a foreign holder, `lslocks` logged. Scenario 26 and my rerun: a queued `flock -s` waiter got the lab lock only after `systemctl start vpp`. When `systemd-run` is unavailable the holder is a setsid **child of the run**; `kill_run` SIGKILLs its `sleep` child, the holder dies under `set -e`, the locks go free mid-rollback (N2). A holder killed from outside while the run is still going is not noticed (N5) |
| M2 approval / gate | **PARTIAL** (→ N4) | `approval_ref :306-315` requires `PENDING-<slug>.md` on main and a LOG row mentioning it; gate record sealed; `--stage run` recomputes the gate and must equal the sealed record (`:635-637`). Forged gate file → refused (scenario 5 + mine). But: (a) **an old, already-executed approval is reusable forever** — `PENDING-handover` (the one-off plugin enable of D-060, already done) authorises any future startup.conf apply; (b) any row that merely **mentions** the slug counts: the real gate record resolves to `LOG D-058 D-060`, and D-058 is not an answer, it only cites "PENDING-handover option 1" in its rationale; (c) the seal is a checksum in the same directory — rewriting `settings` (e.g. `WINDOW=0`) and recomputing it is accepted (`COMMITTED: healthy for 0s`); (d) a rolled-back work dir can be replayed with `--stage run` (seal and gate still valid) → another VPP restart. (c)/(d) need root and a deliberate act; (a)/(b) are the real gap |
| M3 budget / retries | **FIXED (arithmetic optimistic)** (→ N6) | Explicit `budgets :565-571`; defaults: ITER 129 s, RB_BUDGET 760 s, RUN_BUDGET 397 s, DEADMAN_AFTER 1346 s, HOLD_MAX 4466 s. Run makes one attempt, the dead-man loops `RB_RETRIES` with doubling backoff, then releases — bounded (scenario 31, my run A). The "worst case" is not one: RB_BUDGET counts 3 svc calls (rollback_once makes 4: stop, kill, reset-failed, start, each `SVC_TIMEOUT+5`) and has no per-PCI term (6 data NICs × 4 sysfs/driverctl calls × 12 s) nor per-management-interface term; ITER does not scale with interfaces/gateways. Worst case ≈ 1.2 ks vs 760 s. Consequence only: the dead-man interrupts a very slow rollback and restarts it (the holder keeps the locks) |
| L1 fake-host test / shellcheck not in CI | **NOT FIXED** (manager-owned `tools/ci.sh`; proposed in the status file) | `grep test-apply-startup tools/ci.sh` → nothing |
| L2 dry run vs apply interface set | **FIXED** | `SSH_CONNECTION` peer added in `main` before `dry_run` (`:782-785`); scenario 2 |
| L3 lows | **FIXED** | `ere_escape` (`:159`, used in `netmgr_of`, plugin and approval regexes); dry-run `trap` (`:337`); `noprefixroute` kept; addresses before routes; `--foreground` refused over SSH without `--console` |

## New findings (by severity)

### N1 — MEDIUM: with `ssh-peer` (what `auto` picks on vrx-a) a closed SSH session turns a healthy apply into a rollback loop and a false "console needed"
`apply-startup.sh:259-263` (`probe_ok ssh-peer`), `:441` (`check_mgmt`), `:546` (rollback verdict), `:758-766` (dead-man)

The probe proves only that a TCP session with the manager's IP is in the kernel table. Detaching is advertised as
surviving the SSH session's death (scenarios 24/25), yet with `ssh-peer` a session that ends during the window — the user's
laptop sleeps, the tmux/ssh link blips, the operator logs out — is read as "management path lost". Reproduced in the fake
harness (gateway drops ICMP, peer 10.0.0.9, the restart hook drops the session):
```
ROLLBACK: management path: no ESTABLISHED TCP session with the manager (10.0.0.9)
ROLLBACK INCOMPLETE (management path: no ESTABLISHED TCP session with the manager (10.0.0.9))
the dead-man retries at its deadline (locks stay held)
dead-man (attempt 1/2) … INCOMPLETE … (attempt 2/2) … INCOMPLETE
dead-man: ROLLBACK INCOMPLETE after 2 attempts — console access needed; releasing the locks
vpp restarts/starts total: 4
```
On vrx-a with defaults that is: rollback, then both locks held ~22 min until the dead-man, then 3 more VPP stop/starts
with backoff (lab lock exclusive for up to ~1 h, CI blocked), ending in a false console alarm — the same class as the
original H2, now triggered by a normal event. Conversely, a session stays ESTABLISHED for up to `tcp_retries2` (~15 min)
after the path dies, so `ssh-peer` cannot detect a dead path within a 60 s window (mitigated by the exact address/route +
driver comparison, which is what actually catches a stolen NIC). Fix: for `ssh-peer`, test the peer's reachability rather
than the session object — `ip neigh` REACHABLE/`arping` of the on-link peer (172.30.126.196 is on-link) or of the gateway
(it answers ARP) — or at least never let "session gone" alone make a **rollback** incomplete (the file is restored, VPP is up,
addresses/routes are exact: that is healthy); document in `--help` and the doc what a closed session does.

### N2 — MEDIUM: setsid fallback: the dead-man kills the lock holder's child, the holder exits, locks go free during the rollback
`apply-startup.sh:383` (holder started with `setsid … &` from the run → a child of the run), `:410` (`sleep 1` under `set -e`),
`:727` (`kill_run` SIGKILLs every descendant except the holder PID itself)

`setsid` does not reparent: the holder is still a descendant of the run, and so is its `sleep 1`. `kill_run` spares the
holder's PID but kills its `sleep`; `sleep` returns 137, `set -e` ends `stage_hold`, both locks are released — after the
dead-man already logged "lock holder … still owns the locks", so it does not take them itself. Reproduced (fake
`systemd-run` failing, run hung in `systemctl restart`):
```
holder 2856712  ppid 2856401 (= the run);  descendants(run) = 2856422 2857497(holder's sleep 1) …
dead-man: killing the run (pid 2856401) and its children
HOLDER-DEAD
LAB-FREE
```
(Scenario 25/26 do not combine the fallback with a dead-man kill.) Exactly the M1 failure mode, on the path taken when
systemd-run is broken. Fix: exclude the holder's whole subtree in `kill_run` (or start the holder with a double fork so it
is not a descendant), and make `stage_hold`'s wait loop immune to a killed `sleep` (`sleep 1 || true`, or `read -t`).
Add the fallback + dead-man scenario.

### N3 — LOW/MEDIUM: a crash between `systemctl restart` and the first identity read is accepted
`apply-startup.sh:688-693`, `vpp_restarted_ok :443-453`

vrx-a has `Restart=always`, `RestartUSec=100ms`. If the new VPP crashes once during `wait_api` (up to `API_WAIT`), systemd
brings it back and `vpp_restarted_ok` records the **second** instance as the baseline. Reproduced (crash at the first
`vppcheck` after the restart): `rc=0 committed=yes nrestarts-now=1`. A config that crashes VPP once on startup (or on the
first packet — D-095 class) commits. Fix: read `NRestarts` right after `svc restart vpp` returns (it is 0 then — H1's
evidence) and require it to stay 0 in `vpp_restarted_ok` and every `check_health`; model it in the fake (it already sets
`nrestarts=1` on a crash).

### N4 — LOW/MEDIUM: the approval does not bind to this change (M2 remainder)
`approval_ref :306-315`

See M2 (a)/(b). Fix: require the PENDING file to name the change — e.g. the `--expect-new-sha256` (or the document's
sha) in the answered PENDING file or in the D-row — and accept only a row whose text answers it (`PENDING-<slug> answered`
/ the decision column), not any mention; refuse a PENDING whose answer has already been executed (a `committed` work dir
whose gate names the same D-row).

### N5 — LOW: the run never checks that the holder is still alive
`stage_run :640-712`. If the holder dies (killed, OOM, `HOLD_MAX` reached in a pathological run) the run goes on and
restarts VPP without either lock. Reproduced: holder SIGKILLed in the restart hook → `LAB-LOCK-FREE-DURING-RESTART`,
`rc=0 committed=yes`. Fix: `holder_alive` before `install`/`restart` and in each window iteration; if gone, re-take or roll back.

### N6 — LOW: budgets understate the worst case (M3 remainder); `--window 0` accepted
`budgets :565-571`: count 4 svc calls at `SVC_TIMEOUT+5`, `k × (CMD_TIMEOUT+2)` per PCI device in `drivers` and per
management interface/restore line, and scale ITER by interfaces + gateways; also `HOLD_MAX` must cover the dead-man's
real worst case or the holder can expire during its last attempt. `parse_args :145-148` lets `WINDOW`/`INTERVAL` be 0
(commit after one check); require `WINDOW ≥ INTERVAL > 0`.

### N7 — LOW: small items
- `--mgmt-probe tcp:127.0.0.1:22` (or any host not routed through a management interface) is accepted and always passes;
  check `ip route get HOST` → a protected interface.
- `vrx-vppcheck bootid` exits 0 with an incomplete identity (unreadable `/proc` → start time 0); check
  `Identity.Complete()` and exit 1.
- The fake-host test depends on the live `/root/ngfw` main (`PENDING-handover.md`, `LOG.md` D-060, host flag `pending`);
  a later LOG/handover change breaks scenarios 4/5/8+ — use a fixture repo via a test-only root.
- L1 still open (manager): add `deploy/vpp/test-apply-startup.sh` + shellcheck to `tools/ci.sh`.

## Answers to the focus questions
- **H1 identity after restart:** PID reuse — safe (start time). MainPID vs main thread — equal on the real host. Crash +
  auto-restart inside the window — caught. Crash before the first identity read — missed (N3).
- **H2 on vrx-a:** `auto` → `ssh-peer`, viable while the session lives; without a peer → refused, exit 3, nothing changed.
  Session closing during the window: documented only as "keep it open"; outcome is safe for the config (backup restored)
  but produces a false INCOMPLETE, repeated VPP restarts and ~1 h of exclusive lab lock (N1).
- **M1 interleave with a queued `flock -s`:** correct with the holder unit; broken in the setsid fallback (N2); a holder
  killed from outside is not detected by the run (N5); the dead-man itself handles a missing holder correctly.
- **M2:** forged gate file → refused; re-sealed settings → accepted (root-only); replay of an old work dir → accepted;
  PENDING file that exists but whose D-row only mentions it → accepted (D-058); old executed approval → accepted (N4).
- **M3:** retries bounded (RB_RETRIES, doubling backoff, then release); arithmetic optimistic but the failure mode is a
  restarted rollback under held locks, not a lost lock (N6).

## Fake-host test
`deploy/vpp/test-apply-startup.sh <branch-built vrx-startupgen>` → `apply-startup tests: 91 passed, 0 failed` (7m16s,
run concurrently with ci.sh) — matches the pasted 91. My extra scenarios (A: session closes → N1; B/rr2: setsid holder
+ dead-man → N2; C: early crash → N3; D: holder killed → N5; E: replay of a rolled-back work dir; F: re-sealed settings)
ran from a scratch copy of the harness, removed afterwards.

Two MEDIUM findings remain (N1 is on the path vrx-a will actually use; N2 on the fallback path). Neither leaves a bad
config installed, but N1 turns a routine event into repeated VPP restarts plus a false console alarm, and N2 reopens the
M1 lock race. Both are small fixes; no third full review round needed — the manager can verify N1–N3 with added scenarios.

**APPROVE WITH CHANGES**
