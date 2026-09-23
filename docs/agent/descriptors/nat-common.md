# NAT family: ownership, globals and write-only objects (DF-3, applies to every NAT doc)

This page covers package `natcommon` and every family in `apps/agent/internal/descriptors/{nat44ed,nat44ei,nat64,nat66,det44,mapnat,cnat,pnat}`.
Every family constructor takes `natcommon.Option`s: `New(client, owner, opts...)` / `Register(r, client, owner, opts...)`.

## VPP-global singletons (D-071)

The globals are:
- plugin enable/disable: `nat44-ed.enable`, `nat44-ei.enable`, `nat64.enable`, `nat66.enable`, `det44.enable`;
- timeouts: `nat44-ed.timeouts`, `nat44-ei.timeouts`, `nat64.timeouts`, `det44.timeouts`;
- forwarding: `nat44-ed.forwarding`, `nat44-ei.forwarding`;
- `nat44-ei.ipfix`, `map.params`, `cnat.snat-addresses` (the default SNAT entry) and `cnat.snat-policy`.

They are built by `natcommon.Global` and behave according to `natcommon.WithGlobalsOwner(bool)`:

| | globals owner (`WithGlobalsOwner(true)`, the product agent on a real box) | every other owner (default; every test slot on the shared host) |
|---|---|---|
| Create / Update | sets the value (enable, set timeouts, …) | **requires** it: reads VPP and returns `ErrGlobalNotSet` / `ErrGlobalMismatch` if it is off or different; sends nothing |
| Delete | resets it: timeouts/params to VPP defaults, forwarding off, policy `none`, default SNAT entry removed. A plugin **disable only after a complete emptiness check across all owners** (`Plugin.Empty`), otherwise skipped; the plugin stays enabled | no-op |
| Retrieve | the real value (`Absent` = VPP defaults, so no object is reported for them); write-only where VPP has no getter | `ErrRetrieveUnsupported`, so the reconciler re-checks the requirement on every resync and never deletes on absence |

**Emptiness checks (review finding 1)** count every object kind the disable destroys, for all owners:
- nat44-ed: in/out interfaces, output-feature interfaces, pool and twice-NAT addresses, interface-address pools, static,
  identity and LB mappings, VRF tables;
- nat44-ei: the same without LB mappings and VRF tables;
- nat64: interfaces, prefixes, pool addresses, static BIBs;
- nat66: interfaces and static mappings;
- det44 is **never** disabled, not even by the globals owner (VPP crash, D-068 / V9).

A plugin Update (VRF or flag change) needs disable+enable and is refused with `ErrNotEmpty` while any object exists.

**Unobservable globals:** VPP has no "enabled" getter for nat64, nat66 or det44, and no getter for the cnat SNAT policy
or the IPFIX domain/port. A non-owner's requirement is satisfied when an object of the plugin exists (proof that it is
enabled). When nothing is observable, it is accepted without touching VPP (documented limitation).

A write-only enable never gets an Update. A Create whose VRFs differ from the VRFs this process enabled with is refused
(`nat66.ErrVRFChange`, `det44.ErrVRFChangeUnsafe`) instead of being reported as applied (review finding 6). If the
plugin was enabled before this process started, its VRFs cannot be verified.

nat44 ED and EI are mutually exclusive: the owner's enable is refused with `ErrOtherVariant` while the other is on.

## Claim rule (D-071, review finding 5)

- **Own tag → ours.** This applies to tagged mappings and domains (`<owner>:<name>`), and to interface-bound objects on
  interfaces tagged `<owner>:…`.
- **Foreign tag → never touched.** Create refuses an interface tagged by anyone else (`natcommon.ErrForeignInterface`)
  and Retrieve never reports it. This now includes the production owner, which used to claim everything.
- **Untagged → only via a claim.** This covers pools, prefixes, BIBs, maps, translations, bindings and objects on
  untagged interfaces. The generic `natcommon.Descriptor` records the key in the `ClaimStore` on Create (released on
  Delete). Retrieve reports an untagged object only if its key is claimed. A test slot `w<N>` additionally owns its
  address and table range (10.N/16, fd00:N::/32, N000–N999): the shared-host rules are its standing claim. The default
  store is in memory; P05 passes a persisted one with `natcommon.WithClaims`.

Interfaces are named and resolved by their **logical name** (D-065 / D-069): DF-1's `iface.ResolveName` is used on
Create, and Retrieve reports the owner-tag id for our interfaces and the VPP name for untagged ones.

## Unique keys and identity re-verification (review findings 2 and 3)

- Retrieve fails with `ErrDuplicateKey` when two objects map to one key, and `nattest.Apply` / `AssertPlan` fail too.
  Interface-bound nat44 static and identity mappings are dumped twice by VPP (the resolved entry plus the to-resolve
  record); they are collapsed to the interface-bound record.
- Duplicates that VPP genuinely holds (a retried Create) are reported as `<id>#<index>` extras so the scheduler deletes
  them (the D-066 pattern). This applies to pnat bindings (same match tuple) and MAP domains (same tag).
- Deletes by index or id re-verify identity immediately before the delete message:
  - `map.domain` and `map.rule` check the tag at the index;
  - `cnat.translation` checks the VIP, port and protocol at the id;
  - `pnat.binding` checks that the index is live and holds the same match and rewrite;
  - `pnat.attachment` checks the binding id at the index.
  On a mismatch nothing is sent and the delete counts as done. A pnat binding still attached anywhere is not deleted
  (`ErrAttached`).

## Write-only re-application (D-063, D-076)

Write-only Creates run again on every resync:
- the nat64, nat66 and det44 enables ("already enabled" is tolerated), nat44-ei IPFIX (VPP compare-and-swap), the cnat
  SNAT policy (assignment) and cnat snat-interface (a bitmap bit) are idempotent in VPP;
- the one non-idempotent add is `cnat.snat-exclude-prefix`, because every add bumps a per-prefix-length refcount. Its
  Create records `<key>@vpp<main-thread PID>` in the ClaimStore (`natcommon.VPPIdentity`, like DF-4's
  acl.stats-enable) and skips the add while the same VPP process runs. After a VPP restart the identity changes and
  the prefix is re-added once.
- The unit tests model the duplicate add (the fake's refcount) and show that three resyncs leave one instance.

## Host-wide lock

Mutations that dereference the cnat default SNAT entry take `/run/lock/vrx-nat-cnat.lock`:
- shared: `cnat_set_snat_policy` and `cnat_snat_policy_add_del_exclude_pfx`, together with their guard read;
- exclusive: creating or deleting the entry.

This way the guard and the crashing message cannot interleave with another owner's delete (review finding 8). The
lock directory can be changed with `natcommon.WithLockDir`.

## Tests on the shared host

Integration tests run as **non-owners**:
- The plugins are **test fixtures** (`nattest.EnsurePlugin`). A plugin is enabled if it was off and disabled again only
  if the test enabled it and it is completely empty; det44 is never disabled.
- Globals are exercised as requirements: the current value is accepted, a different one fails with `ErrGlobalMismatch`,
  and the values read before and after are asserted equal (the previous value is restored by never changing it).
- The cnat default SNAT entry is a fixture, created only if absent and removed under the exclusive lock.
- The H1 regression is in `nat44ed_integration_test.go`. A non-owner Delete leaves nat44-ed enabled. If the test
  enabled the plugin itself, an output-feature interface on a loopback tagged by a second owner (`w9b`) must also make
  the **globals owner's** Delete skip the disable.
