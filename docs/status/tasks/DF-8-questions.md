# DF-8 — questions for the manager (written while continuing; nothing waits on them)

Answered by D-077: Q1 (tracedump / tracenode / prom exporter / Trace Path dropped → F-capture-trace,
F-dashboard-prom-alarms), Q3/Q4 (http_static / DUID host runs opt-in; http_static then removed from DF-8 with prom,
review M5), Q5 (dfkit is the shared base), Q7 (dhcp relay stays per-VRF).

## Q2 — VPP bugs found (for docs/vpp-code-track.md; none crashes, D-064 does not apply)
1. `ipfix_classify_stream_details` / `ipfix_classify_table_details` are sent without `REPLY_MSG_ID_BASE`
   (`src/vnet/ipfix-export/flow_api.c`) → clients receive an unrelated message id; DF-8 made both objects write-only (V16).
2. `lcp_itf_pair_get_v2` with sw_if_index ~0 replies with the v1 reply id (`lcp_api.c`); DF-8 uses v1.
3. `sflow_interface_details` carries only `hw_if_index`; no API maps hw → sw index. Fix: add `sw_if_index`.
4. `show dns servers` prints the IPv6 list from the IPv4 vector (CLI only).
5. **New:** `lcp_default_ns_get` returns uninitialised bytes while no default netns is set (`REPLY_MACRO_DETAILS2` does
   not zero `netns`; host run 2026-09-24 got `"\xfd\x11"`). DF-8 treats an invalid name as unset. Fix: zero the reply.

## Q6 — scheduler sentinel
`dfkit.ErrRetrieveUnsupported` has the same text as P05's `scheduler.ErrRetrieveUnsupported`; alias it when P05 lands
(review L4: `df2.ErrRetrieveUnsupported` is a third copy — one consolidation task).

## Q8 — persisted stores for P05/P08
Untagged-interface claims (`iface.Claims`, shared with DF-1; claim holders are qualified with the D-080 boot identity and
the sw_if_index) are in memory by default — P05/P08 install a persisted one with `iface.SetClaimStore`. The pcap
BootStore is now an explicit constructor argument (review M4): the agent passes
`dfkit.NewFileBootStore(<state dir>/df8-boot.json)`. `ClientDescriptor.Reconnected()` must be called from P05's
reconnect hook (review M6).

## Q9 — lab-wide lock for read-first VPP-global tests (review M3)
Tests that read a VPP-global with a getter (flowprobe params, sflow globals, IPFIX exporter 0), skip when someone else
holds it and otherwise set and restore it now take an exclusive flock on `/run/lock/vrx-globals.lock` for the whole
test. Getter-less globals (DNS, BPF filter, pcap filter function, IPFIX classify stream, lcp default netns) and the lcp
replace transaction are opt-in only (`VRX_DF8_GLOBALS=1`, `VRX_DF8_LCP_REPLACE=1`, manager window). Options:
(a) adopt `/run/lock/vrx-globals.lock` for every factory's global tests (add it to shared-host-rules.md);
(b) use the lab lock exclusively instead. Recommendation (a).

## Q10 — follow-ups from the review (not done in DF-8)
- L3: the lcp host tap is untagged; tagging it would make DF-1's tapv2 descriptor delete it — P12 decides its ownership.
- L4: consolidate `df2` and `dfkit` (claim stores, address helpers, atomic writes, the sentinel) into one kit.
- L7: `dfkit.BootIdentity` implements D-080 (boot_id + PID + start time); P08 moves it into the agent's vpp package.
- DHCPv6 objects are write-only: a foreign DHCPv6 client enabled on an untagged interface cannot be detected (VPP's
  enable is idempotent), so the first successful enable claims it. Documented; needs a VPP dump to fix.
