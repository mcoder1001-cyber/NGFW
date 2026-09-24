# RF-2: review (independent reviewer, 2026-09-24)

Branch `task/RF-2` @ `8d980d7` (base `main@ff6b91a`). I reviewed it on the host, slot 3 (`w3`), and wrote none of this code.
`git merge-tree` against the current `main` (`4ff135f`) has **one conflict**: `apps/agent/internal/renderers/ALLOWLIST.md`
(RF-3 rows landed on main). It is textual only: keep both sets of rows. `go.mod`/`go.sum` merge cleanly.

## What I ran

| check | result |
|---|---|
| `tools/ci.sh --base main` (my run, log `/root/ngfw-wt/logs/ci/RF-2-20260924-020940-1828891`) | `CI GATE PASSED`, wall time 1m01s. `ok ngfw/agent/internal/renderers/strongswan 6.464s`. gitleaks: "no leaks found". Contract guard: "no contract files changed in the 8 commit(s)". Matches the run pasted in `RF-2.md` |
| `VRX_INTEGRATION=1 go test -count=1 -v -run TestStrongswanIntegration` (slot 3, stock 6.0.4 charon-systemd from `/run/vrx-test/w3/swan-stock`) | PASS (2.48 s). Same steps and output as pasted: Validate uses the scratch charon v; Apply a+b; IKE_SA ESTABLISHED on both sides, AES_GCM_16/CURVE_25519; two ESP xfrm states in `ns-w3-a`; up events; idempotent re-Apply keeps IKE_SA #1; a failed Apply rolls back byte for byte; SIGTERM restart plus re-Apply; terminate and unload leave Retrieve and xfrm empty; planted secret found in 0 of 14 sources. `grep -c VRX_TEST_PSK` over my test log returns `0` |
| My throw-away probes (`zz_review_probe_test.go` with real charons through `swantest`, and `zz_probe_unit_test.go`; both deleted, never committed) | findings H1, M1, M3 and L1 below |
| After all runs | No process under `/run/vrx-test/w3/swan` (`pgrep -af`). No `ns-w3-*`. Host `ip xfrm state` is empty. `systemctl is-active strongswan strongswan-starter` returns `inactive`/`inactive`. `/etc/strongswan.conf`, `/etc/swanctl` and `/etc/strongswan.d` are absent before and after, and `ls /etc` is identical. `findmnt` shows no `swan-stock` overlay in the host namespace. `/run/vrx-test/w3` holds only `swan-stock` and `swan.lock`. Worktree clean |

Checklist:
1. Contract: no hits. `go.mod` is not a contract path.
2. Real verification: yes. Two real charons talk IKE over a veth pair in rig namespaces. The test asserts on VICI `list-sas`/`list-conns`/`get-shared` and on `ip xfrm state/policy`. No VPP is involved, and none is required (VPP IPsec is P11/DF-5).
3. Restart safety: charon restart plus re-Apply is evidenced. There are no VPP objects. A charon crash is not covered: see M3.
4. binapi: not used.
5. Shared host: per-slot flock (`swan.lock`, fixes RF-1 M4). Kills are SIGTERM to the verified child PID; the runner context kills only as a fallback. Namespaces carry the prefix. `t.Cleanup` handles teardown. `/etc` is untouched. `ip` is allow-listed test-only, and daemons enter the netns by `setns`, not `ip netns exec` (fixes RF-1 L1). See M2 for the product/test boundary.
6. Security: no `exec.Command`, `sh -c` or `bash -c`. Escaping and the strict parser are solid. Secret handling is good. Findings: L1, L2 and L4.
7. Transactions: H1, M1 and L3.
8. and 10. No UI.
9. Scope: `Initiate`/`Terminate`, pools/authorities (shape only), AH and IKEv1 rendering are all allowed by the prompt. Nothing to remove.
11. My CI and integration runs match the pasted output.

---

## Findings, ranked by severity

### H1: `Apply` reports success while the running SAs keep the old traffic selectors, PSK and proposals. The convergence check looks only at loaded config, never at SAs
`apply.go:183-207` terminates SAs only for connections that are removed completely. `apply.go:246-294` (`converged`) compares only `list-conns` to the files. The comment at `apply.go:30-31` covers unchanged connections only.

**Probe (real charons, w3):** a↔b is ESTABLISHED with `10.3.1.0/24 ↔ 10.3.2.0/24`, start_action none. Then:
```
P1 Apply(b, remote_ts narrowed to 10.3.1.0/25) err=<nil>
P1 b: conns [w3-ab/w3-ab lts=[10.3.2.0/24] rts=[10.3.1.0/25]] | SAs [ike#1 ESTABLISHED child#1 INSTALLED lts=[10.3.2.0/24] rts=[10.3.1.0/24]]
   xfrm policy b still: src 10.3.1.0/24 dst 10.3.2.0/24 …
P2 PSK rotated on both sides: Apply a err=<nil> b err=<nil>; ike#1 / child#1 unchanged (still authenticated with the old PSK)
```
**Failure scenario:** an operator removes a subnet from a tunnel, drops a weak proposal, or rotates a leaked PSK. The commit succeeds, and Retrieve's `conns` shows the new config. The kernel (P11: VPP) keeps forwarding the old, wider selectors under the old keys. charon rekeys a CHILD_SA and IKE_SA from the SA's own (old) config object, and `reauth_time` is 0 unless `rekey.reauth` is set. So in practice the old policy runs until the IKE_SA dies (DPD, restart). This is the same pattern as RF-1 H2 and RF-3 H1 (success without convergence), here at the SA level, and it is security-relevant.

**Fix:** keep the previous plan (already parsed in `previousPlan`) and diff each surviving connection. When the security-relevant parts of a connection changed (children set, TS, mode, proposals, esp_proposals, version, addrs, ids, auth, or its shared secret), then after `load-conn`: terminate its IKE_SAs (`terminate ike=<name> force=yes`), or `rekey reauth=yes` where make-before-break matters, and let the start action or the peer re-establish. Extend `converged` so that every INSTALLED CHILD_SA of a loaded connection has TS ⊆ the configured TS, and add a `staleSAs` count to `State`. Add a probe-style integration step (narrow TS on the responder → the old CHILD_SA is gone, the new one carries the /25).

### M1: every change to a `start_action=start` connection (the model default) tears the IKE_SA down, and changing start_action none→start duplicates the CHILD_SA
`model.go:438` defaults start_action to `start`. charon's replace path undoes and re-runs start actions for a changed connection.

**Probe:**
```
P3a Apply(a, start_action none→start) err=<nil>  → a and b: ike#1 child#1 INSTALLED + ike#1 child#2 INSTALLED (duplicate CHILD_SA, same TS)
P3b Apply(a, dpd 30→20 only)          err=<nil>  → ike#1 gone, ike#2/child#3 new on both sides (full re-initiation)
```
**Failure scenario:** a cosmetic edit (DPD interval, rekey time) causes an outage of the whole tunnel on commit, and a rollback causes a second one. Duplicate CHILD_SAs are harmless with xfrm. With kernel-vpp they are two SAs behind one tunnel-protect, which P11 must handle.

**Fix:** document it in README/strongswan.md, since the commit engine and UI should say "tunnel will re-establish". Fold it into the H1 diff: tell the caller which connections will flap. For start_action transitions, check for an existing INSTALLED CHILD_SA of that child before letting charon start another one. At minimum, cover it with a test.

### M2: the renderer "owns everything in charon", and the default Paths are the product paths. This is safe in the product today, but not on this shared host once P11 installs `vrx-strongswan`
`renderer.go:138` (`paths: ProductPaths()` default), `apply.go:183-241` (unload every conn, shared secret, pool and authority not rendered), `apply.go:78-83` (a failed first Apply unloads everything).

**Failure scenario:** P11 installs the system charon on this host (`/var/run/charon.vici`), and its topology test drives that charon, because it must talk to the host VPP. Any `strongswan.New(...)` without `WithPaths`, whether a test in any slot or a second slot's P11 run, overwrites `/etc/strongswan.conf` and `/etc/swanctl/conf.d/vrx*.conf` and unloads every other slot's connections and PSKs. The own-all semantics mean two slots cannot share one charon. In the product, own-all is equivalent to `swanctl --load-all` and is acceptable.

**Fix:** (a) no silent product default. Make `Paths` required, or refuse `ProductPaths()` when `VRX_TEST_PREFIX` is set (the same shape as RF-1 L4). (b) Add an optional ownership filter (`WithOwnedPrefix("w3-")`) so that `unloadStale`/`converged` ignore foreign names. P11's shared-host test then uses the prefix, and the product uses none. Record the rule in `docs/lab/shared-host-rules.md` via the manager.

### M3: after a charon crash, SAs stay in the kernel (P11: in VPP). Nothing detects this, and Apply does not clean it up (Q6)
**Probe:** SIGKILL of charon a (a PID the test spawned), then restart, then Apply, then Apply(empty):
```
P4 after SIGKILL: xfrm state a: proto esp spi 0xc15d6379 … / proto esp spi 0xce532253 …
P4 restarted a: conns 0 SAs 0 unlisted 0 (renderer sees no residue)
P4 after re-Apply err=<nil> and after Apply(empty): the same two ESP states remain
```
**Failure scenario:** with kernel-vpp, the stale VPP SAs, which hold plaintext keys, and the tunnel-protect entries survive. charon re-establishes with new SPIs and reqids, so VPP ends up with orphans, possibly steering traffic into a dead SA. Retrieve reports a clean state.

**What the renderer should do (answer to Q6):** it cannot see kernel or VPP state, and it should not (DF-5 owns it). It must make the restart observable. `Watch` already emits `daemon down/up`. Also emit `restarted=true` when `stats.uptime.since` changed, and persist the last-seen `since` so an agent restart does not miss a charon restart. Then P11/DF-5 reconciles on that event: VPP SAs and SPD entries not referenced by `list-sas` (SPI and reqid) are deleted, and Apply is re-run. For the stock kernel-netlink path (tests, `swantest`), the harness should flush xfrm in the namespace when it restarts a daemon it killed. Put this on the board for P11 as a required step; it does not block RF-2.

### M4: `Watch` loses events silently when its consumer is slow, and never resyncs while subscribed
`events.go:231` (256-slot channel), `events.go:184-191` (`send` blocks on `out`). govici `client_conn.go:497-505` drops events non-blockingly (`select { case c <- ev: default: }`).

**Failure scenario:** when the `StreamEvents` consumer stalls (gRPC backpressure, a slow API), the 256-slot buffer fills and govici drops `ike-updown down`. The UI status chip stays "up" indefinitely, because polling runs only while the event socket is down.

**Fix:** while subscribed, run the `pollOnce` diff every 30–60 s as well (it already emits `KindPoll` changes), or trigger an immediate resync when `len(ch) == cap(ch)`. Keep the 1 Hz poll for the down case.

### L1: tunnel names of 61–63 characters with a PSK are accepted by the schema and fail at render time
`model.go:309` names the secret `"ike-" + cname`. `escape.go:36` caps section names at 64, and `objectName` allows 63. Probe: `name len 61 (objectName ok=true): render err=… secret section "ike-w3-xxx…" must be ike-<name>`. **Fix:** allow 68 in `nameRe`. strongSwan has no such limit.

### L2: the redactor replaces every substring equal to a PSK in Retrieve, events and errors. This corrupts state and gives API readers an oracle
`secrets.go:99` (`strings.ReplaceAll` over the JSON in `state.go:294`). PSKs are ≥ 8 bytes. If a PSK equals or contains a visible value (a peer FQDN, an IP, `ESTABLISHED`), that field reads `<redacted>`, which tells every GET reader what the PSK is. **Fix:** Retrieve and events carry no key material by construction (VICI never returns it). Redact only error strings and the `secret = …` pattern there, not structured state. At minimum, do not redact values that occur in `vrx.conf`.

### L3: bounded listings fail the whole operation instead of degrading
`vici.go:37,259` (`MaxSAsPerConn = 64`) is used by `State` (`state.go:171`) and by `unloadStale` (`apply.go:198`). A connection with more than 64 IKE_SAs (a `%any` responder, the pools/remote-access shape the model already renders) makes every Retrieve fail, and makes an Apply that removes it roll back. `unloadStale` only needs to know whether any SA exists: use `terminate` unconditionally (it is a no-op without SAs), or stop at the first SA. `State` should return the first 64 and set a `truncated` flag, following RF-3's L1 pattern.

### L4: secrets outside the 0600 file in the checker path (test-only today)
The checker stages the secrets file under `os.MkdirTemp("")` (`helpers_files.go:234`, usually `/tmp` on disk; 0700 dir, deleted after). The scratch charon keeps the PSKs of every validated candidate in memory, including rejected ones, until it restarts. The product has no checker, so this is acceptable. Document it in the README, and stage under the slot dir (`/run`, tmpfs) in `swantest`.

### L5: `strongswan.conf` changes are never applied, and no restart request is returned (D-079)
`apply.go:32-33`. `DaemonConfig` (plugins, ports, log) is agent configuration, not document data, so this bites only when the agent config changes (P11 switches the plugin list to kernel-vpp). Apply then returns nil while charon runs the old plugin set. **Fix:** compare the running charon's `stats.plugins` with the rendered `load` list, and return the shared `ActionRequired{restart}` (chrony/unbound shape, persisted per D-079) when they differ.

### L6: harness orphans on `go test -timeout`
`swantest/harness.go:283`: when `go test` times out it panics, `t.Cleanup` does not run, and the charons, namespaces and xfrm state stay. `New` refuses to reuse a live slot, which is good, but a human has to clean up. Set `Pdeathsig: SIGTERM` for the daemon (the runner needs a `SysProcAttr` hook), or document the manual cleanup in the README.

### I1: `prfsha256` default for AEAD IKE proposals without a PRF (`escape.go:215-218`)
This is correct, because charon rejects an AEAD IKE proposal without a PRF, and it is consistent with D-067 (IKEv1 + GCM is rejected in both schema and renderer, `model.go:283`). However, the schema help (`vpn.ts:143-146`, "Defaults to the PRF matching the integrity algorithm") says nothing about AEAD. P11's contract PR should update the help text ("AEAD: prfsha256"), and the manager should log it. Retrieve shows the effective `prfAlg`. `CheckProposal("ike", …)` does not know the IKE version, so a hand-edited file with IKEv1 + GCM passes Validate (defence-in-depth only).

### I2: the questions I was asked to rule on
- **`.`→`+` (Q5):** correct and collision-free. `objectName` never contains `+`, so `a.b` and `a+b` cannot both exist. `a.b` and `a-b` stay distinct, and the `ike-` secret namespace is separate from connection names. Do **not** change the schema: rejecting `.` would reshape an existing contract (always PENDING) for no safety gain. The cost is cosmetic: charon logs show `a+b`, and events and Retrieve carry both `conn` and `tunnel`. The name `include` (any case) is rejected at render with `ErrUnsafe`, which is acceptable.
- **Injection:** I checked every field: names (`nameRe` plus the `include` check), ids (shape validators, then quoted), addresses/TS (`netip` canonical), proposals (keyword enum), descriptions (comment, `Line`), plugins/paths (regex), the PSK (base64 only). Braces, newlines, `#` and `=` cannot reach a structural position. The strict parser (one tab per level, byte-exact round trip, known keys only, quoting per key) is re-run in Apply as well as Validate, so a tampered file never reaches VICI. strongswan.conf(5) on the host has no `${…}` expansion; the only non-literal syntax is `:` references and `include`, and both are excluded.
- **Secrets:** 0600 file, temp file created 0600 before rename (`CreateTemp`), owner from Paths (`root:root` in the product). The file sits under a 0700 dir in tests and the packaged `/etc/swanctl/conf.d` in the product. The PSK is base64 only. `sharedSecret.String()` is redacted. charon log level is capped at 1 in strongswan.conf, and the product journal keeps charon-systemd's default of 1. govici does not log. The planted-secret test covers files, charon logs, the renderer log, Retrieve, events and errors. Apart from L2 and L4, I found nothing.
- **govici caps:** the 1 MiB per-packet bound (`boundedConn`) actually prevents govici's `make([]byte, announced)`: the header is never handed over when it is too large. With per-connection streams capped at 64 (below govici's 128 buffer) there is no silent truncation for listings, apart from L3's hard-fail behaviour. The event path is not covered (M4).
- **go.mod:** `github.com/strongswan/govici v0.8.2` is MIT (LICENSE checked) and has zero transitive dependencies. `golang.org/x/sys` was already in the graph; direct is correct. Accept both.
- **Environment (Q1):** keep the extract-into-/run approach until P11's `vrx-strongswan` package lands. Installing the stock package would create `/etc/swanctl` and enable a unit. The private-mount-ns overlay is well contained (propagation private first, asserted by the test).

---

## Required before merge
H1 (SA-level convergence: terminate/reauth changed connections, plus a TS check in `converged`, plus an integration step), M4 (periodic resync while subscribed), L1 (one regex), and resolving the ALLOWLIST.md merge conflict. Each is local to `apply.go`/`events.go`/`escape.go`.

## Required before P11 starts (on the board; may be a follow-up)
M1 (document the flap and avoid duplicate CHILD_SAs), M2 (explicit Paths plus an ownership prefix for the shared-host P11 test), M3 (restart detection via `uptime.since`, and the VPP-side reconcile in P11/DF-5), L2, L3, L5.

**APPROVE WITH CHANGES**
