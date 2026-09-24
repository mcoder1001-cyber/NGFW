# DF-5 — independent review (ipsec, ikev2, wireguard + vpn)

Reviewer: independent review agent (did not write this code). Branch `task/DF-5` @ `e553199`, base `main` @ `5f022f5`
(merge-base `58fe694`). Checklist: prompts/REVIEW-PROMPT.md; rules: 00-CONTEXT, factory prompt DF-5, envelope, LOG D-051,
D-055, D-063…D-096.

## What I ran (on the host, slot 4, one package at a time; the lab lock was held only by each run itself)

```
$ tools/ci.sh --base main                       (worktree /root/ngfw-wt/DF-5, HEAD e553199)
no contract files changed in the 20 commit(s) of HEAD since main (58fe694)
ok: no secret-shaped strings
ok: gitleaks — ... no leaks found
  mode quick · wall time 3m37s · logs /root/ngfw-wt/logs/ci/DF-5-20260924-045501-2831494
CI GATE PASSED                                   (matches the gate output pasted in DF-5.md)

$ eval "$(tools/lab env 4)"; VRX_INTEGRATION=1 go test -count=1 -run OnHost -v ./internal/descriptors/<pkg>/
ipsec      NRestarts 5 → 5   --- PASS: TestIpsecOnHost (0.15s)
             agent restart (fresh agent, persisted records): ... 0 create, 0 update, 0 delete, 10 unchanged
             after loss (SA + NIC binding deleted via the API): plan create ipsec.spd-interface/loop402; create ipsec.sa/4002
             charon sweep: deleted SAs [4501], policies 1, in use []; AckRestart calls 1
ikev2      NRestarts 5 → 5   --- PASS: TestIkev2OnHost (0.18s)   --- SKIP: TestIkev2GlobalsOwnerOnHost (VRX_DF5_GLOBALS unset)
wireguard  NRestarts 5 → 5   --- PASS: TestWireguardOnHost (0.56s)
```

VRX_DF5_GLOBALS was not set. No packets were sent. VPP was not restarted. (NRestarts was already 5 when the review started. That is
the 04:50 crash in D-095, which DF-5.md also mentions.)

Leak check on my own host-test logs: no raw test vector and no `VRX_TEST_PSK` string. But the logs contain
`psk: "sha256:3751fdd20365…"`, and `printf VRX_TEST_PSK_DF5_ikev2 | sha256sum` → `3751fdd20365…`, which proves the finding H1 below.

## Checklist summary

| # | Check | Result |
|---|---|---|
| 1 | Contract | No change in packages/proto, packages/schema or generated code. `vpn/pb/vpn.proto` is agent-internal (D-055, Q6). OK |
| 2 | Real verification | The host tests go through P05's real reconciler against `/run/vpp/api.sock` and assert on Retrieve and on the VPP dumps. The redacted `vppctl show` evidence is in DF-5.md. OK |
| 3 | Restart safety | The simulation is pasted and I reproduced it: fresh agent, empty plan; an agent without our records adopts nothing; objects lost behind the agent's back are re-created. OK. One caveat: this only holds with a persisted record store (see H2 and Q12) |
| 4 | binapi provenance | `apps/agent/binapi/` and `tools/binapi-gen.sh` are untouched. Every message comes from the generated packages (the build proves it). OK |
| 5 | Shared-host rules | Prefix `w4`, ids 4000–4599, ports 204xx, cleanup in `t.Cleanup`, no pkill. Minor issues: L3, M3 |
| 6 | Security (secrets) | **H1 (D-096), M1 (errors echo the value)**. Everything else is good (details below) |
| 7 | Transaction / sweep | **H2, H3, M2** |
| 9 | Scope | `ikev2.liveness` (split out of the profile) and the SA state helper stay within D6.3. OK |
| 11 | Tests actually run | CI green, and my own run matches the pasted output. OK |

Security points that hold (verified in code):

* The dumps that return material are handled correctly. `ipsec_sa_v5_dump` (sa.go:201, 328-330), `ikev2_profile_dump` (profile.go:175,
  197, 207, 608), `wireguard_peers_v2_dump` (peer.go:353, 434) and the ikev2 SA/child SA derived keys (state.go:79, 117) are all reduced
  to a reference, or zeroed, before any filtering or `continue`. That includes the keys of other owners.
* `wireguard_interface_dump` is always called with `ShowPrivateKey: false`.
* Resolved material is zeroed with `defer` on every Create path (sa.go:86-87 and 268, profile.go:442/450, interface.go:81,
  peer.go:273/277).
* `MapResolver` sits behind a pointer, so `%+v` on it is safe.
* Fixtures use `VRX_TEST_PSK_DF5*` or labels hashed from it.
* The vppctl evidence is redacted and passed a leak guard. I found no key-shaped hex or base64 in `/root/ngfw-wt/logs/DF-5-*`.

## Findings (by severity)

### H1 — Secret fingerprints are unkeyed SHA-256, which violates D-096. Low-entropy IKEv2 PSKs can be guessed offline

`vpn/secret.go:115` (`Ref` = `sha256:<hex>`). It is used by `ipsec/sa.go:371`, `ikev2/profile.go:626`, `wireguard/peer.go:283` and
`Verify` (`secret.go:152`).

* **Failure scenario.** The reference is stored in the desired state and returned by every Retrieve. It then goes into:
  * plan and verify errors: `scheduler/reconciler.go:823` prints `want {%v} got {%v}` with the full values;
  * test logs (`integration_test.go:71/73` prototext);
  * any P08 state or audit output.

  Anyone who can read those can run a dictionary attack against an IKEv2 PSK offline. I showed above that the logged `psk:` reference
  is exactly `sha256(VRX_TEST_PSK_DF5_ikev2)`. D-096 decided: "never a plain sha256 of the secret". DF-5 is named as affected.
* **Fix.**
  * Replace `Ref` with HMAC-SHA256 under an agent-local key: a 0600 file in the state dir, created on first use, injected through
    `Config`/`With…` (no package global). Use the prefix `hmac:<hex>`.
  * `Verify` and `Resolve` use the same key.
  * `MapResolver.Add` takes the keyer.
  * Update `vpn/doc.go`, the P11 secret contract in `docs/agent/descriptors/ipsec.md`, and the tests. Add a test that the reference of
    a known PSK is not `sha256(psk)`, and one that two keys produce different references.
  * Keep `x25519:<public key>` for the WireGuard private key. It is a public value, not a fingerprint.

### H2 — The charon sweep can delete this owner's own SAs, or another owner's. Its only guard is a record that is in-memory by default

`ipsec/orphans.go:57-85`, `ipsec/ipsec.go:82-84` (the default is `NewMemoryBootStore`), `vpn/keys.go:43-47` (a zero `IDRange` owns every
id).

* **Failure scenario.** An orphan is any SA in `sw.IDs` that has no record in *this* store. Nothing checks that `sw.IDs` is disjoint
  from `cfg.IDs`. In production `cfg.IDs` is the zero range, which means all ids, so the two ranges always overlap. The unit test even
  places an owned SA (4501) inside the charon range.
  * P11 calls `SweepAndAck` after a charon restart.
  * If the agent was also restarted with the default in-memory store, or the store file was lost or reset, every agent-created SA in
    the charon range has no record. The sweep then unlocks those SAs and live tunnels drop.
  * The same happens to any other owner's SAs (another agent or slot, or a second ipsec.sa owner with a different store) whose ids fall
    in the range. That breaks D-071 ("never touch foreign objects").
  * VPP-native IKEv2 child SAs (ids ≥ 0x80000000, `ikev2.c:2218`) are only spared while they are attached to a tunnel protection.
* **Fix.**
  * `SweepCharonOrphans` refuses the sweep when `cfg.IDs` is zero or overlaps `sw.IDs`, and when `sw.IDs` reaches into bit 31 (the
    ikev2 plugin's id space).
  * It also refuses when `cfg.Boot` is an in-memory store: add a `Persistent()` marker, or pass the store explicitly.
  * Document that the descriptors' range and charon's range must be disjoint (this matches D-096's "charon gets its own id range").
  * Add unit tests for these refusals.

### H3 — The sweep can unlock an SA twice. That frees an SA a policy still points to, which is a use-after-free crash on the packet path

`ipsec/orphans.go:124-137` and `:139-165`. In VPP, `ipsec_sad_entry_del` is an **unlock**, not a delete (`ipsec_sa.c:1182` →
`ipsec_sa_unlock`), and every protect policy holds its own lock (`ipsec_spd_policy.c:227`, released at `:308`).

* **Failure scenario A.**
  1. `deletePolicy` fails. The re-encoding may not round-trip charon's exact policy, or VPP returns an error.
  2. The loop does `continue` but leaves the SA in `orphans`, and the SA is "deleted" anyway. That drops the creator's lock, so the SA
     stays alive with only the policy's lock and still shows in the dump.
  3. The sweep returns an error, so there is no ack, and P11 retries.
  4. On the retry the SA is still unrecorded and not live, so `IpsecSadEntryDel` runs again. This drops the *policy's* lock and frees
     the SA while the policy's `sa_index` still points to it.
  5. The next matching packet crashes VPP. This is the same class as V19/D-095, on a shared VPP.
* **Failure scenario B.** The protect policy that uses the orphan sits in an SPD the sweep never scans (outside `sw.IDs`, or recorded
  by us). The SA is unlocked but stays, and the next sweep frees it. Same crash.
* **Fix.**
  * Before unlocking an SA, dump the policies of **all** SPDs. The existing sweep only covers the charon range. If any policy
    (protect, with `sa_id` = orphan) or any tunnel protection still references the SA, skip it and report it in `InUse`.
  * If a policy delete fails, drop that SA from `orphans`.
  * Add a unit-test case "policy delete fails → SA not deleted; second sweep → still not deleted".
  * The fake's `ipsec_sad_entry_del` should model VPP's lock counting (D-076 says fakes must model VPP's real add/delete behaviour).
    That way the double unlock becomes visible in tests.
* The same lock semantics apply to `Sa.Delete` (sa.go:127). It is safe today only because of dependency ordering and the record drop.
  Add the same "no remaining policy or protection reference" check before the unlock. Then a retried delete after a timed-out first
  one cannot drop someone else's lock.

### M1 — Secret-reference errors echo the reference whole when it has no `:`, which is the typical pasted-plaintext case

`vpn/secret.go:192-196` (`Redact` returns `ref` unchanged when there is no `:` or at most 8 characters follow it). It is used in
`secret.go:96, 158, 173`.

* **Failure scenario.** A user or P08 puts plaintext into a secret field, for example `psk: "Summer2026!"`. Neither the SA key refs nor
  `Ikev2Auth.psk` are format-checked before `Resolve`: `profile.go:238` only checks that the value is non-empty. The error becomes
  `ikev2: profile w4-x psk: vpn: secret not found: Summer2026!` and goes into the plan result, logs and the API. The first 8
  characters of a pasted value with a `sha256:`/`x25519:` prefix leak the same way. RF-2 explicitly never echoes malformed references
  (strongswan/secrets.go:20).
* **Fix.**
  * Validate the reference format (`^(hmac|x25519):[0-9a-f…]{n}$`) up front in every encode or validate path. On failure, return
    `ErrBadRef` without the value.
  * `Redact` returns only the prefix, or `<malformed>`, when the reference is not well-formed.
  * Add a test that a pasted value never appears in any error.

### M2 — `SweepAndAck` does not match D-096's ordering, and its ack can swallow a second charon restart

`ipsec/orphans.go:190-198`. RF-2 `AckRestart` (strongswan/state.go:299-313).

* **Failure scenario 1.** D-096 fixes the order: stop charon → sweep charon SPDs and SAs → start charon → AckRestart. `SweepAndAck`
  acks immediately after the sweep, and `AckRestart` needs VICI, so it fails while charon is stopped. The helper cannot be used in the
  decided order.
* **Failure scenario 2.** The sweep does not remove charon SPDs or bypass policies (D-DF5-6). D-096 now requires that.
* **Failure scenario 3.** `AckRestart` records charon's start time *at ack time*. If charon restarts again between the sweep and the
  ack, the new restart's orphans are acknowledged without ever being swept.
* **Fix.**
  * Split the API. P11 calls `SweepCharonOrphans` with charon stopped (`Live` = none), then starts charon, then acks.
  * Add a stopped-mode sweep that removes unrecorded charon-range SPDs. Unbinding is implicit, because `ipsec_spd_add_del` del unbinds.
  * Make the ack conditional on the start time observed before the sweep, for example `AckRestart(ctx, since)` in RF-2 (a question for
    RF-2/P11).
  * Update ipsec.md §"Orphaned SAs" and answer Q10 accordingly.

### M3 — An SPD binding whose interface disappeared first leaves a stale binding in VPP (a V19-class inherited row); the fixtures can produce it on the shared VPP

`ipsec/spd_interface.go:347-349` (interface gone → drop the record, nothing else). `vpn/vpntest/vpntest.go:80-82` (the loopback cleanup
deletes the loopback without unbinding first).

* **Failure scenario.** VPP has no interface-delete hook for `spd_index_by_sw_if_index` (only `ipsec_tun_interface_add_del` exists,
  ipsec_tun.c:774). If a bound interface is deleted before its binding, the row stays:
  * The next interface that gets the same sw_if_index "has" an SPD in `ipsec_spd_interface_dump`.
  * Nobody can bind an SPD to that interface (`SYSCALL_ERROR_2`).
  * When the SPD is later deleted, VPP runs feature disable on that reused index.

  In the host test this happens whenever the safety-net apply fails before the fixture cleanups run. After that, the loopback of
  another slot inherits the row. D-095(c) asks restart simulations and fixtures to delete dependents first.
* **Fix.**
  * The fixture cleanups unbind any SPD from their loopback before deleting it.
  * `SpdInterface.Delete` returns an error, instead of silently dropping the record, when the interface is gone but VPP still lists a
    binding row for our recorded index and pool index. The binding is then cleaned by deleting our SPD, which VPP unbinds.
  * Add "stale SPD binding row on a reused sw_if_index" to TD-3's sanitize list. `ipsec.itf` and `wireguard.interface` create
    interfaces at reused indexes too.

### M4 — A failure after a successful VPP add leaves an object that is ours but has no record, and a stuck Create

* `ipsec/spd_interface.go:308-318`. The binding is added, then `dumpBindings` or `rec.Put` fails. The function returns without writing a
  record. Every later Create fails with "SPD already assigned", and Retrieve never reports the binding. Delete refuses with
  `ErrNotOurs`. It is stuck until a VPP restart.
* `ipsec/sa.go:92-94` and `spd.go:64-66`: the same pattern after `rec.Put` fails. A persisted store can hit disk errors.
* **Fix.** On a post-add failure, roll the add back in the same call. We know it is ours. That is what `itf.go:72` and
  `wireguard/interface.go:94` already do for a tagging failure. Alternatively, write the record before returning the error.

### L1 — Material stays in govpp's encode/decode buffers

This applies to `ipsec_sad_entry_add_v2`, `ikev2_profile_set_auth`, `wireguard_*_create/add` and the three details dumps. The generated
`DecodeBytes` copies out of the receive buffer (wireguard.ba.go:323-324), and neither buffer is zeroed. This is outside DF-5's code.
Document it in `vpn/doc.go` as a known limitation and file a govpp item. No code change is required in DF-5.

### L2 — The ipsec host test reads a VPP global without the globals lock

`ipsec/integration_test.go:230` reads `ipsec_backend_dump` without `vpntest.LockGlobals(t, false)`, which D-082 requires (ikev2 does
take it, at :138). Add the one-line lock.

### L3 — IKEv2 profile ownership by the `<owner>-` name prefix can collide across owner names

`ikev2/profile.go:196`. Owner `w4` with profile `a-b` and owner `w4-a` with profile `b` both map to `w4-a-b`. This is harmless with the
slot names in use (`w<N>`, product owner). Refuse `-` in owner names, or document the constraint.

### Note — gitleaks history rewrite (Q1, D-DF5-8)

Per D-067, only the branch's own unmerged commits were recreated. I verified that `8fecca0`, `f15ea5f`, `d84ff22` and `450e960` are
not on any branch and that main's ancestry (`58fe694`) is intact. The old objects stay in the object store until gc. Nothing to do.

### Open items for the manager (not blocking DF-5)

Q2 (V-track: govpp NUL-truncated IKEv2 ids), Q3, Q5, Q8, Q9, and Q12. For Q12, P08 must install a persisted `WithBootStore` store for
ipsec and ikev2. After H2's fix the sweep refuses to run without one.

## Verdict

H1 is a manager decision (D-096) that DF-5 has not implemented yet, and it changes the reference format P11 and P08 will build on. H2
and H3 let the D-089 sweep tear down other owners' or our own live tunnels, or crash the shared VPP. Please do one fix round on H1, H2,
H3, M1 and M2. M3 and M4 should go in the same round. L1–L3 are optional. The re-review is limited to those items.

**BLOCK**
