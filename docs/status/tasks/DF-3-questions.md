# DF-3 — questions / notices for the manager

Rewritten by the continuing worker on 2026-09-24. The first worker's questions file was never committed;
the numbering below keeps Q4, which `docs/agent/descriptors/nat44-ed.md` already cites.

## Q0 — URGENT NOTICE: the DF-3 det44 integration test crashed the shared VPP twice (fixed)

- **What happened:** `TestDet44OnHost` crashed VPP with SIGSEGV on 2026-09-23 16:03:41 (first worker) and on
  2026-09-24 00:19:48 (my first run after I took over). systemd restarted VPP both times (restart counters 2 and 3
  in `journalctl -u vpp`), which wiped every slot's VPP objects. The 00:19:53 crash that followed was in
  `gtpu_plugin.so` and did not come from DF-3.
- **Root cause (VPP 26.06 bug, `src/plugins/nat/det44/det44.c`):** `det44_plugin_disable()` walks
  `vec_dup (dm->interfaces)`, but `dm->interfaces` is a **pool**, so the copy includes freed slots. Deleting a stale
  slot fails, and the error path calls `det44_log_err ("… %U del failed", unformat_vnet_sw_interface, …)`, which
  uses an *unformat* function as a *format* → SIGSEGV in `va_unformat`. So the crash happens whenever det44 is
  disabled after any det44 interface was removed. A second bug in the same file: `det44_interface_add_del(is_del=1)`
  calls `vnet_feature_enable_disable (…, 1 /* enable */ …)`, so the `det44-in2out` / `det44-out2in` feature is never
  taken off the interface.
- **Fix in DF-3 (commit after ee8d620):** `det44.enable` Delete **never** sends `det44_plugin_enable_disable(enable=0)`.
  It releases the singleton on the agent side only, and the plugin stays enabled but idle until the next VPP restart.
  `det44.enable` Update, which changes the VRFs, returns `ErrVRFChangeUnsafe` instead of disabling and re-enabling.
  The integration test no longer disables det44, and it only touches the det44 timeouts when they are at VPP's
  defaults.
- **Request:** please add this as a V-item in `docs/vpp-code-track.md` (I do not own that file): "det44 disable
  crashes: pool iterated as a vector + unformat used as format; det44 interface delete re-enables the feature".
  Until it is fixed upstream, nobody should call `det44_plugin_enable_disable` with `enable=0`, and that includes
  the manager's nightly cleanup sweep.

## Q1 — dependency key names for interfaces and VRFs (RESOLVED by D-065)

`natcommon/scope.go` uses `interface/<name>` and `vrf/<id>` as dependency keys, as the DF-3 prompt specifies. The
README examples use `interface.loopback/<name>` and `ip.table/<id>`. D-065 settles interfaces: the alias key
`interface/<name>` stays. `vrf/<id>` still follows the prompt; if DF-1/P05 name the table descriptor differently,
it is one variable (`natcommon.VRFDescriptor`).

## Q4 — `nat44_ed_vrf_tables_v2_dump` answers with v1 details on VPP 26.06

On 26.06 the v2 dump replies with `nat44_ed_vrf_tables_details` (v1) messages, the v1 dump replies with nothing, and
the generated v2 client rejects the v1 type. `nat44-ed.vrf-table` Retrieve therefore drives the v2 dump on a raw
stream and accepts either details type. No binapi change is requested. This may be worth a V-item too.

## Q5 — scope: npt66 and dslite

The envelope scope lists nat44_ed, nat44_ei, nat64, nat66, det44, map, cnat and pnat. The factory prompt also
mentions npt66 and dslite. I followed the envelope (higher precedence), so npt66 and dslite are **not** built.
Please schedule them in a follow-up DF task if they are wanted.

## Q6 — objects without a VPP getter (D-063 applied)

These are write-only (Retrieve returns `ErrRetrieveUnsupported`, with no cache echo): `cnat.snat-policy`,
`cnat.snat-interface`, `cnat.snat-exclude-prefix`, `nat64.enable`, `nat66.enable`, `det44.enable` (none of these has an
"is enabled" or VRF getter) and `nat44-ei.ipfix` (domain id and source port have no getter). The re-apply on every
resync is idempotent in VPP for all of them. The exception is an excluded cnat prefix, whose per-length refcount grows
(search order only).
Translation `flags`, `is_real_ip` and `flow_hash_config` are **not modelled**: they are not in
`cnat_translation_details`, so VPP defaults are sent. If F-* features need them, a VPP API addition is required
(flags in `cnat_translation_details`, `cnat_snat_policy_get`, `cnat_snat_policy_if_dump`,
`cnat_snat_exclude_pfx_dump`, a nat64/nat66/det44 "running config" getter). This is a possible vpp-code-track item;
it does not block anything.

## Q7 — cnat crash hazards (guarded in DF-3, worth a V-item)

`cnat_set_snat_policy` and `cnat_snat_policy_add_del_exclude_pfx` dereference a NULL default SNAT entry (release
builds compile `ASSERT` out), so sending either one before `cnat_set_snat_addresses` crashes VPP.
`cnat_translation_update` with `n_paths = 0` underflows `vec_validate`. The DF-3 descriptors guard all three; any
other caller (vppctl scripts, other tasks) must do the same.

## Q8 — pnat: missing binding index in details, flow-hash crash, detach bug (worth V-items)

- `pnat_bindings_details` has no binding index. The DF-3 descriptor recovers it from the `pnat_bindings_get` cursor
  semantics (binary search). An upstream fix would add `binding_index` to the details.
- `pnat_flow_lookup` and `pnat_binding_detach` crash VPP if no binding was ever attached: the `bihash_16_8` flow hash
  is not lazily instantiated. This is guarded in DF-3.
- `pnat_binding_detach` disables the interface's attachment point even when other bindings remain attached there.

## Q9 — `ErrRetrieveUnsupported` is a local copy until P05 merges

`natcommon.ErrRetrieveUnsupported` has the same text as `scheduler.ErrRetrieveUnsupported` on task/P05, as DF-2 and
DF-6 do. When P05 merges, replace it with `var ErrRetrieveUnsupported = scheduler.ErrRetrieveUnsupported` (one line,
`natcommon/errors.go`) so that the reconciler's `errors.Is` matches.

## Q10 — D-064 compliance record

`systemctl show vpp -p NRestarts` before/after the first host run of each new plugin (all times 2026-09-24):
- det44: the first run (00:19) raised NRestarts: **crash caused by DF-3** (`det44_plugin_enable_disable` with
  enable=0, see Q0). Fixed so that det44 is never disabled, and the test is gated behind `VRX_DF3_DET44=1`. Re-runs
  after the fix left VPP unchanged (ActiveEnterTimestamp 00:19:53 → 00:19:53, and later 00:26:04 → 00:26:04).
- map: NRestarts 1 → 1 (00:25:17). cnat: 2 → 2. pnat: 2 → 2. The restarts at 00:24:53 (manual), 00:25:16 and
  00:26:04 were gtpu (DF-6), not DF-3.
- The crashing messages for docs/vpp-code-track.md: `det44_plugin_enable_disable(enable=0)` after any det44
  interface delete (Q0). Guarded but never triggered: `cnat_set_snat_policy` /
  `cnat_snat_policy_add_del_exclude_pfx` without a default SNAT entry, `cnat_translation_update` with n_paths=0
  (Q7), and `pnat_flow_lookup` / `pnat_binding_detach` before the first pnat attach (Q8).
