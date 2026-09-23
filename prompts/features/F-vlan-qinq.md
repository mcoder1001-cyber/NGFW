# Task: F-vlan-qinq — 802.1q sub-interfaces and QinQ   (prepend 00-CONTEXT.md)

## Goal
Implement VLAN sub-interfaces (802.1q) and stacked QinQ (802.1ad outer + 802.1q inner) end to end.
Reference: TNSR "VLAN 802.1q / QinQ 802.1ad", VPP `create sub-interfaces` / `sw_interface_add_del_address`.

## Inputs to read first
- `packages/schema/src/interfaces.ts` — `subinterfaces` already exists (vlanId, innerVlanId?) — extend only if a field is missing
- `packages/proto/vrx/dataplane.proto` — `Interfaces.subinterfaces`
- `apps/agent/internal/descriptors/subinterface.go` (P05 stub) and `apps/agent/binapi/interface/` —
  messages `create_subif`, `create_vlan_subif`, `delete_subif`, `sw_interface_dump` (verify names in binapi)
- VPP docs: https://s3-docs.fd.io/vpp/26.06/ → "sub-interfaces"

## Scope — build exactly this
1. **Schema**: semantic rules — vlanId 1–4094; innerVlanId requires vlanId; (vlanId, innerVlanId) unique per parent;
   sub-interface inherits parent VRF unless set; MTU ≤ parent MTU.
2. **Agent**: `subinterface` descriptor: Create via `create_subif` with exact-match flags for dot1q / dot1ad+dot1q;
   Update = recreate when tags change (return ErrRecreate); Delete via `delete_subif`; Retrieve from
   `sw_interface_dump` filtering `sub_id != 0` and decoding `sub_outer_vlan_id/sub_inner_vlan_id/sub_dot1ad`.
   Dependencies: parent interface. IP/MTU/admin descriptors must accept sub-interfaces as targets (test it).
3. **API**: nothing new beyond pointer routes; `/state/interfaces` must list sub-interfaces nested under the parent.
4. **UI**: in the interface drawer, sub-interface table (vlan, inner vlan, addresses, state) with add/edit/remove
   via SchemaForm; en+fa.
5. **Docs**: `docs/user/interfaces/vlan-qinq.md`.

## Acceptance (paste the evidence)
- [ ] After commit: `Retrieve()` == desired and `vppctl show interface <parent>.<sub>` shows dot1q 100 / dot1ad 200 dot1q 100 with the addresses;
      after rollback no sub-interfaces remain (Retrieve empty). Optional, not required: scapy tagged frames from `ip netns exec ns-<p>-lan`
- [ ] Agent-restart simulation → sub-interfaces recreated with addresses within 30 s
- [ ] Rollback deletes the sub-interfaces (Retrieve shows none)
- [ ] Duplicate (vlanId, innerVlanId) → 400 with pointer to the second entry

## Out of scope (do not build)
Bonding, bridge domains, L2 cross-connect on sub-interfaces, VLAN rewrite/tag-rewrite, LLDP.

## Open questions
Default `exact-match` semantics: we use exact-match for both dot1q and dot1ad — flag if TNSR behaviour differs.
