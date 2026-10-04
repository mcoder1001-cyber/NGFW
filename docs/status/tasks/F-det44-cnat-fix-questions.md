# F-det44-cnat-fix — questions / manager actions

## Q1 (V-entry for docs/vpp-code-track.md — I do not own that file): VPP 26.06 det44 delete re-enables its node

`src/plugins/nat/det44/det44.c`, `det44_interface_add_del()`, delete branch (read from `/root/vpp/src`, v26.06, nothing
touched):

```c
  if (is_del)
    {
      ...
      rv = ip4_sv_reass_enable_disable_with_refcnt (sw_if_index, 0);
      ...
      rv = vnet_feature_enable_disable ("ip4-unicast", feature_name,
					sw_if_index, 1, 0, 0);      /* <- enable=1 on the DELETE path */
      ...
      pool_put (dm->interfaces, i);
    }
```

Every `det44_interface_add_del_feature is_add=0` therefore puts one MORE `det44-in2out` (inside) / `det44-out2in`
(outside) on the interface's ip4-unicast arc instead of removing the one the add put there (add → 1, del → 2, add → 3,
del → 4; the sv-reassembly refcount is paired correctly). det44 forgets the interface, the nodes stay and drop every
packet no det44 map matches (`No translation`). Proof on fresh slot-16 interfaces, raw binary API, no agent:
`F-det44-cnat-fix-evidence/split-before.txt` lines 37-59. Suggested VPP fix (one line): pass `0` instead of `1` in that
call. Config-only fallback (done, `descriptors/det44`): repair the arc through `feature_is_enabled` /
`feature_enable_disable enable=0` after every det44 delete, clear leftovers before every add, report leftovers in
Retrieve. Please add it as a V-item (V-new "det44 delete re-enables the feature node").

## Q2 (VPP cnat behaviour, info + possible V-entry): stale cnat sessions keep matching for many minutes

VPP 26.06 cnat keeps a finished or failed flow's session (and its FIB rewrite, `lbi` of the adjacency it used) until
the session scanner frees it, and the data path still matches it although `last_seen + lifetime` is long past
(`cnat_lookup_create_or_return`, `cnat_inline.h`: a bihash hit is used as is and its timestamp refreshed). The
scanner works 100 µs per 1 s tick (`cnat_session_scan`, `10e-5`) over a session table sized for millions of entries
(`289 active elements`, `bytes: used 128.22m`), so a full pass takes many minutes. A new flow with the same 5-tuple
follows the stale rewrite towards interfaces that were deleted meanwhile and is lost: a rerun on ports 43000-43011
8 minutes after a 12/12 round connected 1 of 12 (`F-det44-cnat-fix-evidence/split-after-stale-ports.txt`, the session
dump in its diag shows `lifetime:5`, `last_seen` ~340 s old, `lbi:119` of the previous run). The learned return client
(`cnat-client:[10.16.1.2]`, an interposed /32 in table 0) also outlives the interface. Harness fix: every CNAT round
takes 12 source ports from a per-half-minute block (`cnatPorts`). For the product: after a translation or interface
delete, flows that reuse a 5-tuple within the scanner pass can fail; there is no per-owner session/client delete
(`cnat_session_purge` is VPP-global, never used here). Leftover after my runs, inside slot 16's range only:
`cnat-client:[10.16.2.100]` / `cnat-client:[10.16.1.2]` + their sessions (`leftovers-after.txt`) — no API removes
them per owner; they age out. Suggested V-item: cnat lookup should treat an expired session as a miss.

## Q3 (agent scheduler, not my files): every agent restart re-creates every det44 object

On a resync the reconciler re-applies the write-only `det44.enable/global` as a Create, and `executor.around()`
deletes and re-creates every live dependent around it — the det44 map and both det44 interfaces — on every agent
restart (agent log of the final run, line 109:
`"msg":"re-creating dependents","key":"det44.enable/global","dependents":3`). With the arc repair this is harmless for
the arc (ARC after the second restart: one node each), but the det44 map delete drops every det44 session on each
agent restart, and before this fix it added two arc nodes per restart. It also explains run 2's six nodes exactly:
rev 2 add (1) + raw loss delete (2) + resync add (3) + second restart delete/add (4, 5) + rev 4a delete (6). Suggested
follow-up (internal/scheduler): a resync re-apply of a write-only object should not run `around()` — the object is
re-applied idempotently, not new. Same pattern for any write-only global with dependents (nat64/nat66 enables).

## Q4 (info): suspect (b) is not needed — nothing changed in descriptors/cnat

On fresh interfaces with no det44 history the VIP connects 12 of 12 without any cnat interface feature
(`split-before.txt` cnat-fresh, `split-after.txt`): VPP 26.06 serves the VIP through the FIB (`cnat-client` DPO →
`ip4-cnat-tx`) and learns the return client itself. `cnat.interface-feature` (feature_cnat_enable_disable) exists in
the descriptor family but the document has no field for it; no schema change is needed for this bug.

## Q6 (permission refusal, manager action): `/tmp/g-w16` not deleted

The envelope says to delete the CI-only `TMPDIR=/tmp/g-w16` before finishing; the permission layer refused my command
that contained `rm -rf /tmp/g-w16` (21:51). Per the envelope I did not try another way. What is left: `/tmp/g-w16/`
with only `node-compile-cache/` (144 KiB, dated 2026-09-28 — from the predecessor's gate runs as well). Please remove
it or tell me the allowed form.

## Q5 (info, harness): two latent bugs of the predecessor's test fixed here

- `cleanup-through-agent` sent `"interfaces": {}` without `ApplyRequest.subsystems`, which the agent skips (D-041):
  the interfaces were never deleted through the agent. It now names `interfaces`, `vrfs`, `routing`, `nat`.
- The wan netns is both the MAP-E CE and the CNAT backends; the plain `10.16.1.0/24 dev <ip6tnl>` route sent the
  backends' SYN-ACKs into the tunnel (masked in runs 1/2 by the det44 nodes). Only the CE's shared address uses the
  tunnel now (source rule, netns-private table 16065).
