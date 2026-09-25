# F-acl — work in progress (slot 3)

Updated 2026-09-25, fix round 1 (review d58b9105). Final status and evidence: `F-acl.md` (incl. "Fix round 1").

- Done: contract, agent, API, web, docs, host topology evidence (before the 04:27 VPP restart), CI green; fix round 1
  (H1 apply-only `acl.config`, M3 mandatory binding→interface dependency, M1 docs + V7 row, M2 counters flag saved and
  restored in the host tests, L2/L5/L6/L8/L10/L13).
- Pending: screenshots (TD-25), the 100k host step (manager window + Q2 limits), the rebase work M4 (TD-23
  `RegisterExtension`, PBR test through the agent, Q3 stand-in removal) and the Q14 fold with F-host-acl-nftables —
  when the manager says the bases are in.
