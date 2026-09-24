# DF-5 — verification of fix round 2

Verifier: an independent VERIFY agent. I did not write this code or the earlier reviews. Branch `task/DF-5` @ `9edde7c`
(fix `bb56911`). The scope is narrow, as `DF-5-rereview.md` (`4a370b9`) set it: **N1**, **N2** and a CI run.

What I read:

* `DF-5-rereview.md` N1/N2;
* the "Fix round 2" section of `DF-5.md`;
* `git show bb56911`, the DF-5 part of `git diff 4a370b9..HEAD` (the rest is main's merge): `ipsec/orphans.go`, `spd.go`,
  `fake_test.go`, `orphans_test.go` and `integration_test.go`;
* VPP (read-only): `/root/vpp/src/vnet/ipsec/ipsec_spd.c` and `ipsec_spd_policy.c`.

I ran everything on the host, slot 4 (`eval "$(tools/lab env 4)"`), one package at a time. The lab lock was held only
during each run (`vpntest.Connect`). No packets were sent, VPP was not restarted, and `VRX_DF5_GLOBALS` was not set.

## N1 — FIXED

**VPP semantics, confirmed again.**

* `ipsec_add_del_spd` delete (`ipsec_spd.c:24-104`) unbinds the interfaces, `vec_free`s the policy index vectors and
  `pool_put`s the SPD. It never calls `ipsec_sa_unlock`.
* Only the per-policy delete unlocks: `ipsec_spd_policy.c:308` (`ipsec_sa_unlock (vp->sa_index)`), plus the fast-path
  `:893` and `:956`.
* A policy add locks the SA (`:227`, `:284`).

**Code.** `orphans.go` `Sweep` runs in this order when stopped:

1. Every policy of every unrecorded charon-range SPD is deleted one at a time. A re-dump must show the SPD empty;
   otherwise the error is recorded and the SPD is not deleted.
2. Each orphan SA goes through `releasePolicies`: a fresh dump must show zero references, then `stillOrphan` re-reads
   the SA, then one `IpsecSadEntryDel`, then a **re-dump**. An SA that is still present goes into `NotSwept` and adds an
   error.
3. Only the emptied SPDs are deleted.

Any error means no marker, so `AckRestart` returns `ErrSweepNotComplete`.

Two more changes:

* `Spd.Delete` refuses while its SPD holds protect policies.
* The fake's SPD delete no longer unlocks. `TestSpdDeleteKeepsSALocks` pins locks at 2 after the SPD delete.

**Independent reproduction.** This was a temporary probe test of my own in `ipsec_test`, deleted after the run. The
charon objects were created with raw binapi, as charon would create them, on slot-4 ids. The sweep used
`ipsecd.NewCharonSweeper` with a persisted `FileBootStore`, descriptors 4000–4499 and charon 4500–4579.

```
A before: spds [4550], spd 4550 policies 3, SAs [4551 4552]        (3 protect policies still in the charon SPD)
A sweep: err=<nil> res={DeletedSPDs:[4550] DeletedPolicies:3 DeletedSAs:[4551 4552] InUse:[] NotSwept:[]}
A after: spds [], SAs []                                            (ipsec_spds_dump / ipsec_sa_v5_dump)
AckRestart(probe-A): ok
B (lock leaked by a raw SPD delete of spd 4554 holding a policy on sa 4553):
B sweep: err=ipsec: orphan sa 4553 still present after its unlock (a lock is held elsewhere)
         res={DeletedSPDs:[] DeletedPolicies:0 DeletedSAs:[] InUse:[] NotSwept:[4553]}; SAs after [4553]
B ack: ipsec: no completed charon sweep for this restart; refusing AckRestart (calls 0)
--- PASS: TestVerifyProbeOnHost (0.04s)     NRestarts 5 -> 5; afterwards `show ipsec sa` / `show ipsec spd` empty
```

The author's host tests on the same slot:

```
ipsec     exit=0 NRestarts 5 -> 5
  charon sweep (charon stopped): SPDs [4501], policies 1, SAs [4502 4504], in use []
  sa 4504 (protect policy still in the charon SPD at sweep time): absent from ipsec_sa_v5_dump after the sweep
  --- PASS: TestIpsecOnHost   --- PASS: TestSpdDeleteKeepsSALocksOnHost (sa 4590 survives spd del + 1 unlock; 2nd unlock: gone)
ikev2     exit=0 NRestarts 5 -> 5   --- PASS: TestIkev2OnHost   --- SKIP: TestIkev2GlobalsOwnerOnHost
wireguard exit=0 NRestarts 5 -> 5   --- PASS: TestWireguardOnHost
unit: ok vpn · ok ipsec · ok ikev2 · ok wireguard
```

## N2 — FIXED

`releasePolicies` returns `InUse{By: "spd <id>"}` as soon as a referencing policy sits in an SPD that is not
`charonSPD` (not in the charon range, or recorded by us). It returns before deleting anything. The stopped-mode step 1
only touches charon-range, unrecorded SPDs. The unit test asserts that the policy of foreign SPD 3002 survives.

For the host check, the probe used foreign SPD 4590. It is inside slot 4 but outside both the descriptor range and the
charon range. Its protect policy references charon-range SA 4556.

```
C sweep (stopped): err=<nil> res={… DeletedPolicies:0 DeletedSAs:[] InUse:[{SA:4556 By:spd 4590}] …}; foreign spd 4590 policies 1; SAs [4556]
C sweep (running): err=<nil> res={… DeletedPolicies:0 … InUse:[{SA:4556 By:spd 4590}] …}; foreign policies 1
```

Nothing was deleted. The foreign SPD, its policy and the SA all remained. The probe cleaned them up afterwards.

## CI

```
$ tools/ci.sh --base main      (HEAD 9edde7c)
  gitleaks 1f3e15d..HEAD: no leaks found
  warnings: commit subjects of the review commits only
  mode quick · wall time 3m25s · logs /root/ngfw-wt/logs/ci/DF-5-20260924-053626-3059014
CI GATE PASSED
```

## New high issues

None. The probe file was deleted, and the worktree was clean before this document was written.

APPROVE
