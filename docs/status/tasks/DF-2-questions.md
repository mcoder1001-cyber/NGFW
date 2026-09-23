# DF-2 — questions for the manager (non-blocking; work continued)

1. **ip6nd_proxy_add_del crashed the shared VPP.** On 2026-09-23 15:52:38 the first `ip6nd_proxy_add_del`
   (loop305, 2001:db8:3:5::99, after `ip6nd_proxy_enable_disable`) was followed by a VPP 26.06 abort / systemd restart
   (recorded by the previous DF-2 worker). The host integration test `TestProxyNdOnHost` is therefore skipped unless
   `VRX_DF2_PROXY_ND=1`; the descriptor is unit-tested on the fake. Proposal: the manager reproduces it under the
   exclusive lock after handover and files it in `docs/vpp-code-track.md` (DF-2 must not edit that file).
2. **Write-only descriptors (no dump in the VPP 26.06 API):** `adl.interface`, `adl.allowlist`,
   `classify.interface-ip-table`, `classify.interface-l2-tables`, `classify.output-acl`. Their `Retrieve` returns
   `df2.ErrRetrieveUnsupported` (no cached desired state faked). P05 needs a rule for them — proposal: the reconciler
   treats `ErrRetrieveUnsupported` as "actual unknown": always (re)apply desired objects of such a descriptor after a
   restart, never plan Deletes from them, and exclude them from the verify step. Contract question for P05 (descriptor.go
   is frozen; nothing changed).
3. **Normalisation contract.** `proto.Equal` diffing needs the desired value in Retrieve's canonical form. DF-2 exports
   `NormalizeRaConfig`, `NormalizeRaPrefix`, `NormalizeDad`, `NormalizePolicy`, `NormalizeTable`, `NormalizeSession`,
   `sessionredirect.Normalize`; the F-* wiring / API layer must call them (or P05 adds an optional
   `Normalize(proto.Message) proto.Message` hook — contract change, manager's call).
4. **Key strings of other tasks.** DF-2 uses `interface/<name>`, `vrf/<id>`, `interface-ip/<if>/<prefix>` (task prompt)
   and `acl.acl/<name>` (DF-4 acl.md). If P05/DF-1 settle on `interface.loopback/…`-style keys, only
   `internal/descriptors/df2/keys.go` changes.
5. **classify Store location.** Classify tables have no tag; the owner's name ↔ index map (plus memory_size,
   current_data, session action/metadata that VPP does not report) lives in `classify.FileStore`. P05/P08 must give it a
   path in the agent state dir (proposal: `$STATE_DIR/classify-<owner>.json`).
6. **Shared VPP instability during DF-2 runs.** VPP aborted again at 2026-09-24 00:26:04 in `gtpu_plugin.so`
   (another slot's test, not DF-2) while a DF-2 held test had objects on it; the DF-2 run failed on vanished loopbacks.
   Re-ran green afterwards. No DF-2 leftovers remain (VPP restart wiped them; post-run checks below in DF-2.md).
7. **Registry list.** Each DF-2 package exposes `Register(r, client, owner, …)`; there is no central descriptor list on
   the base (P05 in progress), so wiring the eight `Register` calls is left to P05/P08 (signatures in DF-2.md).
