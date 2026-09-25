# TD-11b: questions and notes for the manager

None of these blocks the branch. Where the envelope and the facts disagreed, I chose the option that keeps the data plane safe (see Q2).

## Q1. Hunks outside `files you own`
- **`subsystems/subsystems.go`: 3 small hunks.**
  - `func Register` → `func register`, plus its doc line. The exported `Register` now lives in `stores.go`. It wraps `register` with the persistence guard, so the agent refuses to start. This way no hunk touches the W-seed anchors in front of `return w, nil`.
  - The `NewIndexCache` refresh closure takes the caller's `ctx` instead of its own `WithTimeout(Background, 5 s)`. This is R2-stores: the closure lives at `subsystems.go:118-119`, which the D-118 tech-debt line cites.
  - The doc of `Wiring.IfaceClaims` no longer says "also df6.WithClaims" (see Q3, df6).
- **New files:**
  - `descriptors/dfkit/persist/` (the guard protocol, under dfkit/**);
  - `descriptors/dfkit/claimfirst.go` (`FileBootStore.Persistent` is kept out of `boot.go`, which TD-9 owns);
  - `subsystems/claims_td11b_test.go`;
  - `agent/claims_restart_integration_test.go` (the host proof; a new file with no hunk in TD-9's agent files);
  - one `*claimfirst*_test.go` / `claims_td11b_test.go` per family.
- **Test helpers touched:**
  - `subsystems/stores_test.go` (the `fixedIndex` resolver takes a ctx);
  - `df6/bypass_test.go` (the `failV6Enable` switch on the counting fake).

## Q2. Deviation: a partial Create opts in with `scheduler.PartialCreate(err)`
The envelope says "the scheduler journals a Create that returns Meta together with an error". I built exactly that first, and the full unit run caught what it breaks:
- **The failure:** `agent TestRollbackReported` went DEGRADED.
  - core `interface-ip` returns `IfMeta{}` with a *failed* add ("Address in use").
  - The rollback then deleted an address that had never been added and got "No such entry".
- **Other Creates return a Meta with a pre-write error too:**
  - `l2/fib_entry.go:92` and `l2/flags.go:157`;
  - the ipsec adopt paths;
  - `bond/member.go:70`.
  - For these, journaling the Meta could also delete an object that existed before.
- **What the scheduler does now:** it journals the Meta only when the error is marked `errors.Is(err, scheduler.ErrPartialCreate)`. This is documented on `Descriptor.Create`, and there is a test showing that an unmarked Meta+error is not journaled.
- **Consequences for other rows:**
  - F-wireguard (3.3b): `wireguard/peer.go:124-129` must return `PeerMeta{…}, scheduler.PartialCreate(err)`. Its envelope obligation is otherwise unchanged.
  - Any family whose Create can fail *after* the add does the same. In my files: df6 bypass (one family enabled, the other failed) and natcommon, which passes a marked error through and keeps the claim for the rollback's Delete.
- **Proposed LOG entry:**
  - (a) journal any non-nil Meta: rejected, because it deletes objects that were never created;
  - (b) explicit opt-in: chosen;
  - reversal cost: low.

## Q3. Obligations for other rows (the guard makes forgetting them fail at start where it can)
- **dhcp.client (live since P08).**
  - Problem: it still writes first and claims after (`dhcp/client.go:122`). A failed claim leaves the client in VPP, unclaimed. It is a DF-8 / wave-A file, so I did not touch it.
  - Fix (5 lines): `c, err := tg.ClaimFirst(ctx)` before `d.config(…)`; on INVALID_VALUE use `c.Adopt()` instead of `tg.Adopt()`; on any other error `return nil, c.Undo(err)`; drop the later `tg.Claim()`.
  - Also add `func (d *ClientDescriptor) CheckPersistent() error { return dfkit.CheckClaims(NameClient, d.owner) }`.
  - Owner: F-kea-dhcp-relay, or a one-hour amendment here if you prefer.
  - Its claims already go to the persisted interface store in the product (same per-owner store the DF-1 checks cover), so what is missing is the claim order only.
- **Other DF-7/DF-8 call sites that claim after the add:** lldp, vrrp, igmp, qos, span, policer, mpls, bfd, lcp, flowprobe, sflow, lb. Each row switches to `Target.ClaimFirst` and adds `CheckPersistent` when it wires its family.
- **df2 consumers (urpf, adl, abf, arp, ip6_nd, classify bindings):**
  - add `CheckPersistent() error { return d.opts.CheckPersistent(Name) }`;
  - use `df2.ClaimFirst(claims, untagged, key)` → `undo()` on a failed call. This is on F-rpf-adl-pbr's fix list; F-neighbors-ra covers arp/ip6_nd.
- **df6 families (F-tunnels / F-lisp / F-srv6):** they must register with `df6.WithClaims(w.PairClaims("df6"))`.
  - New finding: df6 keyed and per-boot claim ids (`<sid>`, `<iface>@<idx>/ip4`) are not interface names. Through the product's `IfaceClaims` (the df6 default, and the old doc's advice) every one of those claims failed with `ErrClaimUnbound`.
  - The guard now refuses to start with that store (`df6.ErrClaimStoreKind`). `PairClaims` shares the family file with `KeyedClaims` and is pruned on a VPP boot change.
- **Others:**
  - F-acl: acl descriptors get `CheckPersistent` over `Wiring.KeyedClaims("acl")`.
  - F-nat44-ed-sessions / F-nat44-ei: nothing to add; `natcommon.Descriptor` already carries the check, and without `natcommon.WithClaims(Wiring.KeyedClaims("nat"))` the agent refuses to start.
  - F-bridge-l2: `l2/fib_entry.go:92` and `l2/flags.go:157` should return nil Meta with a failed write. This is harmless now (not a PartialCreate), but it goes against the documented contract.

## Q4. Not done (stays tech-debt: not claim-store hygiene, or outside these files' scope)
- DF-2 N3: classify fingerprint.
- DF-6 N4: write-only parameter drift / fingerprint.
- DF-6 N6: cross-type adopt in `IfDescriptor.Create`.
- DF-6 N7 remainder:
  - bypass Delete with stale Meta;
  - `FileClaimStore` pruning. The product uses `PairClaims`, which *is* pruned on a boot change.
  - `SafeToDisable` coverage.
- The claim-first order is **not** applied to df6 bypass per-boot records or to cnat's applied records. Those are D-076 "applied" records: claiming before the write would make a crash between claim and write skip the enable forever.

## Q5. Merge notes
- **TD-11c:** it batches `KeyedClaims` writes in `stores.go`. My hunks add a `ctx` parameter to `fileClaims.claim/claimed`. Whoever merges second rebases; the conflict is mechanical.
- **TD-9:**
  - it owns `ApplyWith`/rollback; my `reconciler.go` hunk is `executor.create` plus the helper `isNilMeta` below it;
  - its planned `DefaultReplyTimeout` complements the ctx-bounded refresh;
  - the context-less `Claim`/`Claimed` (Retrieve through `iface.Table.Owns`) keep a 5 s cap (`legacyBound`).
