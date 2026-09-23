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

## Q1 — dependency key names for interfaces and VRFs

`natcommon/scope.go` uses `interface/<name>` and `vrf/<id>` as dependency keys, as the DF-3 prompt specifies. The
README examples use `interface.loopback/<name>` and `ip.table/<id>`. Once DF-1/DF-2/P05 merge their descriptor
names, change the two variables `natcommon.InterfaceDescriptor` / `natcommon.VRFDescriptor` (one place). Nothing is
blocked; the default is the prompt's scheme.

## Q4 — `nat44_ed_vrf_tables_v2_dump` answers with v1 details on VPP 26.06

On 26.06 the v2 dump replies with `nat44_ed_vrf_tables_details` (v1) messages, the v1 dump replies with nothing, and
the generated v2 client rejects the v1 type. `nat44-ed.vrf-table` Retrieve therefore drives the v2 dump on a raw
stream and accepts either details type. No binapi change is requested. This may be worth a V-item too.

## Q5 — scope: npt66 and dslite

The envelope scope lists nat44_ed, nat44_ei, nat64, nat66, det44, map, cnat and pnat. The factory prompt also
mentions npt66 and dslite. I followed the envelope (higher precedence), so npt66 and dslite are **not** built.
Please schedule them in a follow-up DF task if they are wanted.

## Q6 — cnat objects without a VPP getter

`cnat.snat-policy`, `cnat.snat-interface` and `cnat.snat-exclude-prefix` have no dump or getter in VPP 26.06. The
same goes for the write-only translation flags (`flags`, `is_real_ip`, `flow_hash_config`). The descriptors keep an
in-process cache, so after an agent restart they re-apply once (idempotent, except that an excluded prefix's refcount
is bumped). Doing better needs a VPP API addition (`cnat_snat_policy_get`, `cnat_snat_policy_if_dump`,
`cnat_snat_exclude_pfx_dump`, flags in `cnat_translation_details`). Possible vpp-code-track item; not blocking.

## Q7 — cnat crash hazards (guarded in DF-3, worth a V-item)

`cnat_set_snat_policy` and `cnat_snat_policy_add_del_exclude_pfx` dereference a NULL default SNAT entry (release
builds compile `ASSERT` out), so sending either one before `cnat_set_snat_addresses` crashes VPP.
`cnat_translation_update` with `n_paths = 0` underflows `vec_validate`. The DF-3 descriptors guard all three; any
other caller (vppctl scripts, other tasks) must do the same.
