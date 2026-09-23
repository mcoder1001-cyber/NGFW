# DF-2 — questions for the manager (non-blocking; work continued)

1. **ip6nd_proxy_add_del aborted the shared VPP — please record it in `docs/vpp-code-track.md` (D-064; DF-2 does not own that file).**
   - When: 2026-09-23 15:52:38, VPP 26.06 (pid 1109) on vrx-a, first host run of `TestProxyNdOnHost` (slot 3).
   - Calls: `ip6nd_proxy_enable_disable` (loop305, enable) → `ip6nd_proxy_add_del` (loop305, 2001:db8:3:5::99, is_add),
     loop305 = loopback with 2001:db8:3:5::1/64, admin up.
   - Journal: `Out-of-memory, calling os_panic()` → SIGABRT; stack `os_panic ← clib_mem_heap_realloc_aligned ← _vec_realloc_internal
     ← vlib_put_next_frame ← vnet_interface_output_node_fn_x86_64_v3 ← vlib_main` — data-path frame growth in interface-output,
     consistent with an ND proxy loop on the loopback. systemd restarted VPP (restart #1).
   - Done in DF-2: `ip6-nd.proxy` is out of the default `Register` (opt-in `RegisterProxyNd`), host test only with `VRX_DF2_PROXY_ND=1`,
     doc says "unverified on host". Proposal: the manager reproduces it under the exclusive lab lock; if loopbacks are the trigger,
     DF-2 adds a Create guard refusing loopback / non-ethernet interfaces in a follow-up.
2. **Write-only descriptors — answered by D-063.** `adl.allowlist`, `classify.interface-ip-table`, `classify.interface-l2-tables` stay
   write-only and are only registered via `RegisterWriteOnly` (the D-063 reconciler must opt in). `adl.interface` and
   `classify.output-acl` now read presence back through `feature_is_enabled` (review H2) and are in the default `Register`.
3. **Normalisation contract.** `proto.Equal` diffing needs the desired value in Retrieve's canonical form. DF-2 exports
   `NormalizeRaConfig`, `NormalizeRaPrefix`, `NormalizeDad`, `NormalizePolicy`, `NormalizeTable`, `NormalizeSession`,
   `sessionredirect.Normalize`; the F-* wiring / API layer must call them (or P05 adds an optional normalise hook — contract, manager's call).
4. **Key strings.** Interfaces: alias `interface/<name>` (D-065, DF-1). ACLs: `acl.KeyACL` = `acl.acl/<name>` (D-066). VRFs:
   `vrf/<id>`, interface addresses `interface-ip/<if>/<prefix>` (P05 core; only `df2/keys.go` changes if they differ).
5. **State-dir paths for P05/P08.** Two DF-2 stores need a path in the agent state dir: the classify `FileStore`
   (proposal `$STATE_DIR/classify-<owner>.json`) and the claim store for objects on untagged interfaces (`df2.FileClaimStore`,
   proposal `$STATE_DIR/claims-df2-<owner>.json`, passed to every DF-2 `Register` via `df2.WithClaims`). DF-4's etype
   whitelist could share the same store (it claims by interface name, DF-2 by object key — no collision).
6. **Shared VPP instability during DF-2 runs.** VPP aborted at 2026-09-24 00:25:16 / 00:26:04 in `gtpu_plugin.so` (DF-6, recorded
   in D-064). Review-fix host runs: `NRestarts` 2 before and after.
7. **Registry wiring.** No central descriptor list exists yet; the `Register` calls (and the opt-in `RegisterProxyNd` /
   `RegisterWriteOnly`) are for P05/P08. Signatures in DF-2.md.
8. **Global singletons — answered by D-071.** `ip-neighbor.config` and `ip6-nd.dad` are only in `RegisterGlobals`
   (globals owner only); the default `Register` of ip_neighbor / ip6_nd no longer contains them.
9. **D-069 (re-review N6).** DF-2 resolves interfaces by the VPP name. When DF-1's logical-name resolver lands, DF-2 switches
   to it; persisted claim keys embed the interface name and need a one-off key migration then.
10. **D-073(c).** `df2.ErrRetrieveUnsupported` becomes an alias of the scheduler's error once P05 merges.
