# TD-24 review — APPROVE WITH CHANGES

Reviewed: branch `task/TD-24` @ `d80cf4bf`, base `task/TD-11c@e83c6318`, CI green at `315e957a`.
Diff read in full (`git diff e83c6318 HEAD`); tests re-run locally (below); coretest model checked
against `/root/vpp` src/plugins/dhcp/{client.c,dhcp_api.c}; scope checked against
`task/TD-23:apps/agent/internal/descriptors/core/coretest/fakevpp.go`.

## 1. Correctness

- **Exactness.** `apps/agent/internal/descriptors/core/ifaddr.go:352` filters
  `leases[in.Index][p]` — keyed by `sw_if_index` *and* the exact canonical prefix string. A stray
  address on the same DHCP interface, or a different address on the same index, is not caught by
  the filter and stays subject to reconcile. Confirmed by
  `TestStaticAddressesNextToDHCPLeaseStillReconciled` (dhcplease_test.go:197). For untagged
  interfaces the exclusion never even fires: `Env.logical` (core.go:266) only treats an untagged
  address as "ours" if it is individually claimed, so an unclaimed DHCP lease on an untagged/physical
  interface was already excluded before TD-24, by the existing claim check — the new filter is only
  reachable for tagged interfaces, which matches the bug report (D-069/tag-based ownership).
- **Fail-closed dump.** A non-`UnknownMsgError` failure from `dhcp_client_dump` returns an error and
  aborts Retrieve (ifaddr.go:373–378); `TestDHCPLeaseDumpFailureAndMissingPlugin` shows the top-level
  `Apply` also fails closed, with zero deletes. Good — matches D-063/D-076 fail-closed conventions.
- **No plugin.** `unknownMsg` catches `*adapter.UnknownMsgError` both on the initial call and inside
  the `Recv` loop and returns an empty set with no error (ifaddr.go:371,383). Matches
  `dhcp_client_lease_encode` in `/root/vpp/src/plugins/dhcp/dhcp_api.c:246` — that function is only
  reachable when the plugin and message exist, so "unknown message" is the correct signal for "no
  plugin, no leases."
- **No addresses → no dump.** `leases` stays `nil` until the first "ours" address is seen
  (ifaddr.go:320,347); `TestDHCPLeaseDumpFailureAndMissingPlugin`'s last case confirms zero
  `dhcp_client_dump` calls when an owner has no addresses at all.
- **Fake vs. real VPP.** Checked `dhcp_client_lease_encode` (dhcp_api.c:246–271): `lease.host_address`
  / `lease.mask_width` always come from `client->installed`, memset to zero until
  `dhcp_client_acquire_address` runs, regardless of `state`. `coretest/dhcp.go`'s `dhcp_client_dump`
  handler mirrors this exactly (zero `HostAddress`/`MaskWidth` unless `Lease.IsValid()`), and
  `dumpLeaseAddrs`'s `a.IsUnspecified()` check matches the always-zero-when-unbound encoding. Good
  fidelity for what this fix actually reads. Note the fake never models `RENEWING`/`REBINDING` states
  — harmless here, since the fix does not look at `state` at all (correctly: `client->installed` stays
  populated through those states too, per `client.c:121-155`, so keying on the installed address
  instead of state is actually more correct than a state check would be).
- **Race — not covered, not documented.** `client.c:219-225` (`dhcp_client_addr_callback`) shows a
  renewal to a different address does `dhcp_client_release_address` then
  `dhcp_client_acquire_address` — VPP's `installed` copy and the interface's address table change
  together, but Retrieve reads them with **two separate API calls** (`ip_address_dump` at
  ifaddr.go:327, then a lazily-triggered `dhcp_client_dump` at ifaddr.go:348) with no VPP-side lock
  between them. If the client renews to a different address in the gap between the two calls, Retrieve
  can pair a *stale* address (already superseded in VPP, captured by the earlier `ip_address_dump`)
  with the *new* lease from `dhcp_client_dump` — the stale address then looks like an undesired
  address of ours, and the next reconcile issues `sw_interface_add_del_address is_add=0` for an
  address VPP has already removed on its own. Consequence is a failed delete op (not silent data
  loss, and not a wrong deletion of a still-installed address — VPP itself already removed it), and it
  self-heals on the next Retrieve. None of the 7 new tests exercise this interleaving (they call
  `BindLease` synchronously between Retrieve calls, never mid-Retrieve), and neither
  `TD-24-questions.md` nor the README paragraph mentions it. Given the `ip_address_dump`-first,
  `dhcp_client_dump`-lazy ordering is required by D-132 ("no dump without an address of ours"), I
  don't think there's a free fix here — but it should be written down next to Q1–Q3 as a known,
  narrow, self-healing race, the same way Q3 documents the SLAAC/DHCPv6 gap. **This is the one
  "changes" item for the verdict.**
- **F-kea's schema rule.** Confirmed absent from both `main` and this branch:
  `packages/schema/src/domains/interfaces.ts` has no `dhcp-client-no-static`-style rule anywhere in
  the repo outside `task/F-kea-dhcp-relay`. Q4's description ("static IPv4 == lease fails at apply
  with VPP `DUPLICATE_IF_ADDRESS`, a loud rollback, not a silent delete") is accurate and honestly
  disclosed rather than silently left as a gap.

## 2. D-132 (bounded, no polling)

Confirmed one `dhcp_client_dump` per Retrieve: the dump is triggered once (`leases == nil` guard,
ifaddr.go:347) and reused for every remaining interface/address in that Retrieve call, across both
the IPv4 and IPv6 passes. `TestInterfaceAddrRetrieveOmitsDHCPLease` asserts
`len(CallsNamed("dhcp_client_dump")) == 1`. No new timers, tickers or background polling added
anywhere in the diff (checked the full `ifaddr.go` diff). Compliant.

## 3. Tests

- Re-ran locally: `cd apps/agent && go test -race -count=1 ./internal/descriptors/core/... ./internal/agent/...`
  → both packages `ok` (core 1.4s, agent 15.6s), no race reports.
- Spot-checked the base-fails-first evidence in TD-24.md against the actual test bodies: the 6
  `core/dhcplease_test.go` tests plus `internal/agent/dhcplease_test.go`'s one test line up with the
  7 base-failure transcripts (wrong Retrieve set, non-empty plan, address deletes during
  reconcile/resync, wrong count with a stray/lost static, wrong renewal set, Retrieve succeeding
  without a lease list). Matches.
- Mutations: M1 (`if leases[in.Index] != nil { continue }`, index-only match) is a real
  weaker-filter mutation of the actual `leases[in.Index][p]` check and is caught by 5 of the 6 core
  tests, as claimed. M2 (`leases = nil` moved inside the interface loop) breaks the "one dump per
  Retrieve" invariant and is caught by the explicit call-count assertion. Both are plausible,
  meaningful mutations of the real code, not straw men.
- `coretest/dhcp.go` model fidelity: see §1 above (checked directly against
  `/root/vpp/src/plugins/dhcp/{client.c,dhcp_api.c}`).

## 4. Host proof design (test/topology/interfaces/dhcplease_test.go)

- dnsmasq is started via `ip netns exec <ns> dnsmasq --keep-in-foreground ...` with its own
  lease/pid/log files under `/run/vrx-test/<p>/td24` and stopped by PID (`t.Cleanup(func(){p.stop(t)})`,
  dhcplease_test.go:263-282). No `systemctl`/host dnsmasq unit anywhere in the file — confirmed
  slot-local only.
- Sequence matches the envelope and TD-24.md: bind (`rev1`), one re-apply (`rev1-again`, asserts zero
  object results), two agent restarts each followed by a start-up resync
  (dhcplease_test.go:454-470); `check()` verifies the lease address, IPv6 static, absence of the
  lease key in the agent log, and that Retrieve never reports the lease, after every step.
- Would fail on the base: on base `ifaddr.go`, the re-apply's reconcile would see the lease as an
  undesired address of ours and emit a Delete for it, so `len(res) != 0` at line 458 would already
  fail, and `check()`'s address/DHCP-state assertions would fail next. This is the same mechanism the
  unit tests demonstrate against the base, just through the full product/host path.
- Safety: NRestarts checked before/after (`t.Cleanup` at line 356); no trace/classify commands in the
  file; cleanup removes `dhcpClient` first (VPP releases the lease) then the interface/table, with
  veth down before interface delete (D-101). Names (`ns-<p>-dhcp`, `<p>d0/<p>d1`, table `<N>024`)
  don't collide with the P08 rig, as claimed. I did not run this test (host proof is correctly gated
  behind TD-25 and the "never run host tests" rule for this review).

## 5. Scope / conflicts with TD-23

- `coretest/fakevpp.go`: TD-24 adds one field, `DHCP *DHCPClient`, at the end of the `Iface` struct
  (fakevpp.go, after `IsSub bool`). Diffed `task/TD-23`'s copy of the same file against this branch's
  base (`e83c6318`): TD-23 touches the `VPP` struct (adds `installingExt`/`onOwner` fields and the
  whole extension-registry block) and does **not** touch `Iface` at all — its copy of the `Iface`
  struct is byte-for-byte identical to the base. No overlap; a rebase merge of this hunk is a clean
  context-line insertion.
- `coretest/ifext.go`: TD-24 replaces the stub `dhcp_client_dump` handler with
  `v.installDHCPClient()` at the line the base has as `v.On("dhcp_client_dump", ...)`. Diffed
  `task/TD-23`'s `ifext.go` against the same base: byte-for-byte identical (zero diff) — TD-23 does
  not touch this file at all. Confirms Q5's claim.
- This is a direct edit to a function only one branch (TD-24) currently changes, not a case that
  needed D-134's new shared extension registry (that mechanism is for seams several branches extend
  independently in the same cycle; only TD-24 touches this one).

## 6. Q1–Q3 — recommended owner/severity

- **Q1** (VRF change on a DHCP interface refused/rolled back): agree with **M**, owner **F-kea /
  dhcp/**\*\* (per D-133, F-kea already owns dhcp/**). Confirmed TD-24 does not touch
  `apps/agent/internal/descriptors/dhcp/**` (empty diff) — correctly out of scope for this branch.
  The one-line fix (`ClientDescriptor.Dependencies` returning an optional dependency on
  `interface-ip.table/<if>`) is plausible and the failure mode described (loud rollback, lease
  intact) is a real improvement over the pre-TD-24 silent behavior, so this is not a regression
  TD-24 introduces.
- **Q2** (`dhcpClient: {}` without a hostname fails at apply on main): agree with **M**, owner
  **P08 successor or F-kea**. This is a genuine, independently-reproducible bug in
  `desired/interfaces.go:242` (passes `""` where the schema promises "absent = system hostname"),
  found incidentally while writing the TD-24 agent test — correctly flagged as out of TD-24's file
  ownership rather than fixed inline.
- **Q3** (no dump for SLAAC/DHCPv6-installed addresses): agree with **L**. Verified directly:
  `apps/agent/internal/descriptors/dhcp/dhcp6.go` and `register.go` already define
  `dhcp.dhcp6-client`, `dhcp.dhcp6-pd-client`, `dhcp.dhcp6-pd-address` descriptors (DF-8, pre-existing
  on this base), but `apps/agent/internal/subsystems/subsystems.go` registers only `dhcp.NameClient`
  (grep confirms no other `dhcp.Name*` constant appears there) — so these descriptors are dead code
  from the product's point of view today, and Q3's "no product path enables them" claim holds. The
  proposed V-item and the "fallback: skip non-desired global IPv6 on an interface where that feature's
  object exists" rule are reasonable and correctly placed as someone else's (`docs/vpp-code-track.md`
  owner's) follow-up.

## Verdict: APPROVE WITH CHANGES

The fix is correct, exact, D-132-compliant, fail-closed, architecturally clean (no dhcp descriptor
import into core, no `subsystems.go` change, binapi-only names), and well tested (7/7 new tests fail
on the base, 2/2 sampled mutations caught, `-race` clean). Scope is clean against TD-23. Q1–Q3 are
correctly triaged and out of this branch's ownership.

**Required change before merge:** add the `ip_address_dump` / `dhcp_client_dump` TOCTOU race
(§1, "Race — not covered, not documented") to `TD-24-questions.md` (or the README paragraph) as a
known, narrow, self-healing limitation — one line is enough: Retrieve's two dump calls are not
atomic, a renewal landing between them can make one Retrieve report an address VPP has already
superseded, which fails a delete op harmlessly and corrects itself on the next resync. This is a
documentation-only addition; I found no test or code change needed to justify blocking the merge
over it, and D-132's "no dump without an address of ours" requirement rules out the obvious fix
(dump leases first, unconditionally).
