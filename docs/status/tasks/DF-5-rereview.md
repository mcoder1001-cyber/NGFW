# DF-5 — re-review after the fix round

Reviewer: independent re-review agent. I did not write this code or the first review. Branch `task/DF-5` @ `7f6bdf5`. The
first review (`DF-5-review.md`, BLOCK) was @ `e553199`. I looked at the fix round as `git diff 333eb6e..HEAD`: `333eb6e` is the
`git merge main` that the fix round started with, and `ac84fb7..HEAD` also carries main's unrelated SDK/P07b changes. Checklist
used: prompts/REVIEW-PROMPT.md. Rules checked against: 00-CONTEXT and LOG D-051, D-071, D-076, D-080, D-089, D-095, D-096.
VPP semantics were read (read-only) in `/root/vpp/src/vnet/ipsec`.

## What I ran (on the host, slot 4, one package at a time; the lab lock was held only for each run)

```
$ tools/ci.sh --base main                        (worktree /root/ngfw-wt/DF-5, HEAD 7f6bdf5)
no contract files changed in the 28 commit(s) of HEAD since main (ca63114)
ok: no secret-shaped strings
ok: gitleaks — scanned ~631132 bytes (631.13 KB) in 891ms no leaks found
  warnings: review(DF-5): findings  (main's own subject)
  mode quick · wall time 3m31s · logs /root/ngfw-wt/logs/ci/DF-5-20260924-052112-3006936
CI GATE PASSED                                    (matches the gate pasted in DF-5.md for c283f5e)

$ eval "$(tools/lab env 4)"; VRX_INTEGRATION=1 go test -count=1 -run OnHost -v ./internal/descriptors/<pkg>/
ipsec     exit=0 NRestarts 5 -> 5
  charon sweep (charon running): SPDs [], policies 1, SAs [4501], in use []
  charon sweep (charon stopped): SPDs [4501], policies 0, SAs [4502], in use []
  AckRestart after the completed sweep: ok (calls 1)
  after the charon sweep (our SAs untouched): P05 plan … 0 create, 0 update, 0 delete, 10 unchanged
  --- PASS: TestIpsecOnHost (0.15s)
ikev2     exit=0 NRestarts 5 -> 5   --- PASS: TestIkev2OnHost (0.31s)  --- SKIP: TestIkev2GlobalsOwnerOnHost
wireguard exit=0 NRestarts 5 -> 5   --- PASS: TestWireguardOnHost (0.55s)
```

VRX_DF5_GLOBALS was not set. No packets were sent. VPP was not restarted. Afterwards `show ipsec sa` and `show ipsec spd` were empty.

**Leak check on my own host-test output.**

* There are 8 references: ipsec 5 `hmac:`, ikev2 2 `hmac:`, wireguard 1 `hmac:` and 1 `x25519:`.
* There are 0 `sha256:` references and 0 `VRX_TEST_PSK` strings.
* The plain `sha256` of every DF-5 test placeholder (`VRX_TEST_PSK_DF5`, `_ikev2`, `_ikev2_b`, `_int`, `_wg_`, `_0123…`,
  `_fingerprint_key`, `_scheduler_plant`) occurs 0 times, in the host logs and in the CI logs.
* Every 64-hex or base64-32 string in the output is either an `hmac:` value or a WireGuard *public* key (the peer key and the
  `x25519:` reference).

## Status of the original findings

| # | Status | Evidence |
|---|---|---|
| **H1** keyed fingerprints (D-096) | **FIXED** | See "H1 details" below the table. |
| **H2** sweep guard | **FIXED** | See "H2 details" below the table. |
| **H3** double unlock / UAF | **PARTIAL** | See "H3 details" below the table. |
| **M1** echo of pasted values | **FIXED** | See "M1 details" below the table. |
| **M2** D-096 ordering / ack | **PARTIAL** | See "M2 details" below the table. |
| **M3** stale SPD binding row | **PARTIAL (deferred)** | Only documented (ipsec.md §"Known limitation → TD-3"). Neither the fixture cleanup (unbind before loopback delete) nor the `SpdInterface.Delete` error was done. Acceptable as a TD-3 hand-off. It is not blocking, and the manager should confirm TD-3 carries it. |
| **M4** write-ahead records | **FIXED** | See "M4 details" below the table. |
| **L1** govpp buffers | **FIXED** (doc) | `vpn/doc.go:41-42`. |
| **L2** globals lock | **FIXED** | `ipsec/integration_test.go:231`. |
| **L3** owner `-` collision | **FIXED** | `ikev2/profile.go` `checkName` refuses an owner that is empty or contains `-`. |

### H1 details — FIXED

* **Keyed references.** `vpn.Keyer` computes HMAC-SHA256 references (`hmac:<hex>`). The keyer is injected with `WithKeyer` in
  ipsec, ikev2 and wireguard. There is no package global. Every Retrieve path takes the keyer and refuses to run without one
  (`ErrNoKeyer`).
* **Old logs and plain SHA-256.** The old logs had `sha256(VRX_TEST_PSK_DF5_ikev2)` in them. The new host output has 0
  occurrences (see "What I ran").
* **Formatting and logging.** The `Keyer` and `MapResolver` types format as fixed strings and implement `LogValuer`. The key
  sits behind a pointer.
* **Scheduler edit (P05 file).** The change is minimal: +26/−1 in `reconciler.go`, and `diffErr` is the only line that
  changed.
  * `DiffSummary` prints the key, the message type and the names of the top-level fields that differ. It never prints values.
  * I found no other place in the scheduler that formats a value. `PlannedOp.Value` still carries values, but those are
    keyed refs now, and hiding them is P08's display concern.
  * The cost to operators: a verification error on *any* descriptor (for example an MTU mismatch) no longer shows want/got.
    It shows only `fields [mtu]`. That is acceptable for a security default. Later, an opt-in debug diff with per-field
    redaction (a proto field option for secret refs) could restore the values. That is not needed for DF-5.
  * `TestErrorsAndLogsNeverPrintValues` covers the transaction error, the op results and the slog output.
* **Key file.** 0600 perms, `O_EXCL` create, a symlink refused via `Lstat` + `IsRegular`, and a wrong size refused. It has
  crash-safety gaps; see N4.

### H2 details — FIXED

* `NewCharonSweeper` (`orphans.go:76-92`) refuses each of these configurations: a zero or inverted range, `Hi ≥ 0x80000000`,
  `cfg.IDs` unset, any overlap, a non-persisted store (only `*dfkit.FileBootStore` or `Persistent()`), and no keyer.
  `TestCharonSweeperValidation` covers these refusals, and the host test checks the overlap refusal.
* Any record, valid, pending or stale, exempts an id (`recorded` = `Boot.Get`, `:105`). SAs and SPDs carry no tags. Tunnel
  protections exempt their SAs.
* One residual: policies in *foreign* SPDs are deleted. See N2.

### H3 details — PARTIAL

**What is fixed in the running-charon path:**

* A tunnel-protection check runs first.
* The policies of the SA are deleted, then a fresh dump of all SPDs must show no reference.
* The SA is re-read (same SPI, still unrecorded, not live) and then unlocked once.
* Any failure skips that SA and writes no marker.
* `Sa.Delete` refuses while a policy or protection references the SA.
* `TestCharonSweepNeverFreesReferencedSA`: a failing policy delete, run twice, leaves locks at 2 with no UAF.

This matches VPP:

* `ipsec_sa_unlock_id` is `fib_node_unlock` (`ipsec_sa.c:1137-1195`).
* A policy add takes `ipsec_sa_find_and_lock` (`ipsec_spd_policy.c:227`).
* A policy delete runs `ipsec_sa_unlock(vp->sa_index)` (`:308`), and the fast path does the same (`:280-289`, `:893`, `:956`).
* A protection locks its SAs (`ipsec_tun.c:487/498`) and unlocks them (`:530/534`, `:698-699`).

**What is wrong:** the fake models **SPD delete** as releasing its policies' SA locks, and VPP does not. See N1. So the
"models VPP lock counting" part of the fix is wrong exactly where the new stopped-mode sweep depends on it.

### M1 details — FIXED

* `CheckRef` (`hmac:`64-hex, `x25519:`base64-32, legacy `sha256:`) runs first in:
  * the SA key refs (`sa.go:265-272`);
  * the IKEv2 psk (`profile.go:249`);
  * the WireGuard preshared key (`peer.go:102`);
  * `Resolve` and `Verify`.
* A malformed value gets a bare `ErrBadRef`.
* **Redact grammar.**
  * A well-formed `hmac:` or `x25519:` ref → prefix + 8 characters. That is 32 bits of an HMAC, or a public key: harmless.
  * A legacy `sha256:` ref → prefix only.
  * A D-051 name → shown as is.
  * Anything else → `<redacted>`.
* Every `Redact` caller in the Resolve/Verify paths runs after `CheckRef`. The only exceptions are `MapResolver.Resolve`
  (test/helper) and `PublicKeyOfRef`, and those are still safe by the grammar.
* `TestPastedPlaintextNeverEchoed` and `TestPastedPlaintextKey` cover this.

### M2 details — PARTIAL

**Fixed:**

* The API is split into `Sweep(ctx, token, live)` and `AckRestart(ctx, token, acker)`.
* The ack is refused without a completed Sweep for the same token on the same VPP identity (a persisted marker).
* The marker is dropped after the ack, so a second ack needs a new sweep.
* The documented order is stop → Sweep(nil) → start → AckRestart.
* The host test proves that an ack without a sweep is refused (`calls 0`).

**Not correct:** the stopped-mode SPD removal does not free the orphan SAs in real VPP (N1). The residual RF-2 window (Q13)
is assessed below (N3).

### M4 details — FIXED

The SPD, SA and SPD-binding Creates all follow this sequence: existence check → `PutPending` → add → `Put`. A pending
record counts as ownership (`records.go:51-71`).

* **Crash between the record and the add.** The object is absent. Retrieve iterates only the dumped objects, so it reports
  nothing. The next Create finds nothing, rewrites the pending record and adds. This converges. The sweeper also never
  touches the id, because any record exempts it.
* **Crash between the add and the confirm** (or a lost reply). The pending record makes the object ours. Retrieve reports it,
  and Create adopts it only when it is identical: for an SA, same SPI/protocol and `proto.Equal`, including the keyed refs.
  Delete cleans it up. `TestWriteAheadRecords` covers this case.
* **Failed add.** The record is dropped only when a follow-up dump *succeeds* and shows the object absent. A ctx timeout makes
  the dump fail too, so the record stays. That is the safe side: VPP processes a connection's messages in order.
* Residual risk: see N6 (Low).

## NEW findings (ranked)

### N1 — HIGH: the stopped-charon sweep deletes the SPD first, but VPP's SPD delete does not release its policies' SA locks. The orphan SAs survive in VPP, get reported as deleted, and the restart is acked. The fake models the opposite.

Code involved:

* `ipsec/orphans.go:128-144` (step 1 deletes charon SPDs before the SA pass), and the claim in its comment at `:32-33` and
  `:128` ("drops the policies' SA locks").
* `ipsec/fake_test.go`, the `ipsec_spd_add_del` delete branch (the `defer v.unlockSA(id)` per protect policy).
* `ipsec/orphans_test.go:94,113-121`, which asserts that SA 4502, protected by a policy in charon SPD 4500, is gone.
* The same claim is in `docs/agent/descriptors/ipsec.md` ("charon SPDs, then unrecorded charon SAs").

**VPP source.** `ipsec_add_del_spd` delete (`ipsec_spd.c:24-104`) unbinds the interfaces, then `vec_free`s the policy index
vectors and `pool_put`s the SPD. It never calls `ipsec_sa_unlock` and never `pool_put`s the policies. Only the per-policy
delete (`ipsec_spd_policy.c:308`) unlocks.

**Reproduced on the host VPP** (slot-4 ids 4590/4591, `vppctl` only, under the shared lab lock, no packets, NRestarts 5 → 5,
cleaned up after):

```
control (no policy): ipsec sa add 4591 …; ipsec sa del 4591           → (gone)
ipsec sa add 4590 spi 6590 esp; ipsec spd add 4590
ipsec policy add spd 4590 … outbound action protect sa 4590           → show ipsec sa <idx>: locks 2
ipsec spd del 4590                                                     → locks 2   (SPD gone, SA lock NOT released)
ipsec sa del 4590   (= the sweep's single ipsec_sad_entry_del)         → [4] sa 4590 … still present, locks 1
ipsec sa del 4590   (cleanup, 2nd unlock)                              → none left
```

**Failure scenario.** This is the main D-096 case.

1. charon crashes. Its SPD still holds protect policies for its CHILD_SAs.
2. P11 stops charon and calls `Sweep(ctx, token, nil)`.
3. The SPD delete keeps one lock per protect policy. `policiesUsing` now sees no policy, because the SPD is gone. Each SA is
   unlocked once and stays in VPP with lock ≥ 1.
4. The sweep returns `DeletedSAs=[…]` with no error, writes the marker, and the ack goes through.
5. The restarted charon allocates SA ids from its range base again (Q10/D-096). Its `ipsec_sad_entry_add` gets
   ENTRY_ALREADY_EXISTS on those ids, so the tunnels do not come up. That is exactly the collision the stopped sweep exists to
   prevent.
6. The SAs are only freed after N more sweeps, one lock per sweep.

The leaked policy structs are unreachable once the SPD is gone, so I see no packet-path UAF. The damage is a false success
report plus stuck SA ids.

The host test misses this because its stopped sweep runs after the running sweep has already removed the only protect policy.
It logs `policies 0`.

The same hazard exists in `Spd.Delete` (`spd.go:95-114`). If it is ever reached with protect policies still in the SPD (for
example policies not modelled as dependents), `Sa.Delete`'s new "no policy references" check passes and its single unlock
leaves the SA in VPP. Its record is then dropped, and Create later gets "exists, not ours".

**Fix.**

* In stopped mode, delete every protect policy of an unrecorded charon-range SPD one by one (they unlock) before
  `ipsec_spd_add_del` del. Alternatively, run the per-SA pass (which deletes policies in charon SPDs) *before* the SPD pass.
* After an SA unlock, re-dump. If the SA is still present, report it (e.g. `Leaked`/an error, no marker) instead of in
  `DeletedSAs`.
* `Spd.Delete` refuses while the SPD still holds protect policies, or deletes them first.
* Fix the fake. Its SPD delete must drop the policies without unlocking (D-076: fakes model VPP's real behaviour), and
  `TestCharonSweepStopped` must then still pass.
* Add a host-test case with a protect policy still in the charon SPD at stopped-sweep time. After the sweep, assert that the
  SA is absent from `ipsec_sa_v5_dump`.

### N2 — MEDIUM: the sweep deletes policies in foreign SPDs (neither ours nor in the charon range)

`ipsec/orphans.go:206-224` (`releasePolicies` exempts only SPDs *we* recorded). `orphans_test.go:95` asserts that the policy
in "someone's unrecorded SPD outside the charon range" (3002) is deleted.

**Failure scenario.** An SPD outside both ranges belongs to some other owner: another agent, another slot on the shared host, or
an operator. Deleting its policy touches a foreign object, which D-071 says never to do. It can also turn that owner's
protected traffic into a policy miss (which becomes bypass or discard, depending on the other policies). The first review's
H3 fix said to report such a reference as `InUse` and skip the SA. It did not say to delete the reference.

**Fix.** Delete policies only in unrecorded SPDs **inside the charon range**. A reference from any other SPD → `InUse`. Change
the unit test accordingly.

### N3 — MEDIUM (for RF-2/P11, not DF-5 code): Q13's ack window is real, and option (a) is the right answer

`CharonSweeper.AckRestart` gates on the sweep token, but RF-2's `AckRestart(ctx)` records charon's start time *at ack time*.

**Failure scenario.**

1. charon is started.
2. charon installs its SPD/SAs, crashes and is restarted by systemd.
3. P11 acks the restart.

The second instance's objects are acknowledged without a sweep, and its successor collides on ids. That is N1's outcome
again, through a narrow but real window during a crash loop.

The DF-5 side cannot close this with RF-2's current interface. Option (a) in Q13 (RF-2 `AckRestart(ctx, since)` acks only the
start time P11 observed right after starting charon) closes it. DF-5's `RestartAcker` should then take `since`. I agree with
Q13's recommendation (a). It is not blocking DF-5, but P11 must not ship on option (b).

### N4 — LOW: the key file is not crash-safe, and some checks are loose

`vpn/secret.go:169-211`.

* **Partial write.** The key is written in place after `O_EXCL`. A crash between the create and the write/sync leaves a
  0-byte (or short) 0600 file. Every later start fails with "must be a regular 32-byte file" until someone deletes it by hand,
  and a second agent starting at the same time fails the same way. Fix: write to a temp file in the same dir, fsync it, then
  `link(2)` or `rename` it into place (`link` keeps the first-writer-wins behaviour), and fsync the dir.
* **Directory perms.** `MkdirAll(dir, 0700)` does not tighten an existing, wider directory.
* **TOCTOU.** `Lstat` followed by `ReadFile` has a time-of-check gap (a symlink swap). Open with `O_NOFOLLOW` and `Fstat` the
  descriptor instead.
* **Owner.** There is no owner-uid check.

All of these need write access to the state dir, so they are defence in depth.

### N5 — LOW: the legacy `sha256:` acceptance path (D-DF5-9) keeps a plain-SHA-256 input alive and does not converge

`vpn/secret.go:30-33,270-271`. Test: `ipsec/secrets_test.go:35`.

* No normaliser rewrites the desired value, and Retrieve always reports `hmac:`. So a desired state that carries `sha256:`
  never equals what is retrieved: every apply fails post-apply verification, and every plan shows an Update (ErrRecreate =
  SA flap).
* No producer of `sha256:` references was ever merged: DF-5 is unmerged and P08 does not exist yet.
* The path therefore only helps an input D-096 forbids ("never a plain sha256").

**Fix.** Refuse `sha256:` with `ErrBadRef`, which was D-DF5-9's first option, and delete the legacy code.

### N6 — LOW: pending records are broad and never garbage-collected

* A pending SPD-binding record stores `spdIndex = anyIndex` (`spd_interface.go:86-87,129`), and `record()` accepts it for
  *any* SPD bound to that sw_if_index. After a crash between `PutPending` and the bind, whatever binding later appears on that
  index is reported as ours, and Delete would unbind it. The fix is to match on the spd_id's pool index, which is known from
  `ipsec_spds_dump` before the bind.
* A pending record whose object is no longer desired stays in the store forever. The fix is to prune pending records with no
  matching object on the next Retrieve.

### N7 — LOW: pre-fix logs with plain SHA-256 of test PSKs remain on disk

`/root/ngfw-wt/logs/DF-5-integration.log` (00:48) and `DF-5-ipsec-integration.log` (Sep 23 16:03) have mode 0644. They
contain `sha256(VRX_TEST_PSK_DF5)`, `sha256(VRX_TEST_PSK_DF5_ikev2)` and `sha256(VRX_TEST_PSK_DF5_int)`. These are test
placeholders only, so nothing real leaks. Delete or regenerate the files so the evidence set matches D-096.

### N8 — LOW: `Sweep(ctx, token, nil)` trusts the caller that charon is stopped

When `live == nil`, the sweep deletes every unrecorded charon-range SPD, including a running charon's live SPDs, which drops
every tunnel. Consider taking an explicit `CharonStopped` assertion from P11, for example the RF-2 state showing charon
inactive, or documenting this as a P11 precondition in bold.

## Verdict

H1, H2, M1 and M4 are fixed and verified. CI and the three host tests are green, and I found no leaks. However, the
stopped-mode sweep, which is the D-096 procedure M2 asked for, does not work on real VPP (N1, reproduced above). It reports
orphan SAs as deleted while VPP keeps them locked, and the fake and unit test assert the opposite of VPP's behaviour. That
leaves H3/M2 only partially fixed. N2 also touches foreign SPDs (D-071).

A short fix round is needed on N1 (reorder or delete the policies before the SPD, check the SA is gone after the unlock, fix
the fake, add a host case with a live protect policy) and N2. N4–N8 are optional. The re-review will be limited to N1 and N2.

**BLOCK**
