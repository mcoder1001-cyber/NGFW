# Task: F-nat44-ed — NAT44 endpoint-dependent   (prepend 00-CONTEXT.md)

## Goal
Outbound NAT (source NAT / PAT), static 1:1 mappings and port forwards using VPP's `nat44-ed` plugin,
with a live session browser. Reference: TNSR "NAT: outbound, 1:1, port forwards"; VPP plugin `nat44_ed`.

## Inputs to read first
- Contract PR first (label `contract`): `nat{ mode: "ed", inside: [ifaceName], outside: [ifaceName],
  pools: [{name, range: "a.b.c.d-a.b.c.e", vrf?}], staticMappings: [{name, local{ip,port?}, external{ip|pool, port?},
  protocol?, vrf?, twiceNat?}], timeouts{udp,tcpEstablished,tcpTransitory,icmp}, sessionLimit }`
  + matching proto message. Get it approved before touching the agent.
- `apps/agent/binapi/nat44_ed/` — verify every message name there (`nat44_ed_plugin_enable_disable`,
  `nat44_interface_add_del_feature`, `nat44_add_del_address_range`, `nat44_add_del_static_mapping_v2`,
  `nat44_ed_set_timeouts`? — confirm in binapi, do not assume)
- VPP docs: https://s3-docs.fd.io/vpp/26.06/ → NAT44-ED

## Scope — build exactly this
1. **Schema**: semantic rules — inside/outside interfaces exist and are disjoint; pool ranges valid and non-overlapping;
   static mapping external port requires protocol; session limit ≥ 1024.
2. **Agent**: descriptors `nat44-enable` (plugin enable with mode ED + session limit), `nat44-interface-feature`,
   `nat44-address-pool`, `nat44-static-mapping`, `nat44-timeouts`; dependencies interface → feature → pool → mapping.
   Retrieve from the `*_dump` messages. Session table read via `nat44_ed_user_session_dump`/equivalent with
   server-side paging in the agent (never ship 1M sessions over gRPC in one message — stream or page).
3. **API**: config via pointer routes; `GET /api/v1/state/nat/sessions?page&filter` (paged), `DELETE …/sessions/{id}`
   (kill session → agent Action), `GET /api/v1/state/nat/summary` (counts, pool utilisation).
4. **UI**: NAT page with tabs Outbound / Static & Port Forwards / Pools / Sessions (ServerDataGrid with kill);
   pool utilisation bar; en+fa.
5. **Docs**: `docs/user/nat/nat44.md` with the three classic scenarios.

## Acceptance (paste the evidence)
- [ ] host-lan → host-wan TCP via iperf3: `tcpdump` on wan shows the pool address, not the lan address;
      `vppctl show nat44 sessions` shows the session; UI session browser shows the same 5-tuple
- [ ] Port forward: connection from wan to external:8080 reaches lan host:80 (`trace` shows `nat44-ed-out2in`)
- [ ] 1:1 mapping works both directions
- [ ] `tools/lab restart-vpp vrx-a` → NAT config back within 30 s; sessions are expected to be lost (document it)
- [ ] Rollback disables the feature on interfaces and removes pools (Retrieve)
- [ ] Overlapping pools → 400 with pointer

## Out of scope (do not build)
NAT44-EI, NAT64/66, DET44/CGNAT, MAP, CNAT, hairpinning tuning, ALGs beyond VPP defaults, HA session sync, IPFIX logging.

## Open questions
Which behaviour for outbound traffic that matches no pool — drop or bypass? Default to VPP behaviour and document.
