# Independent remaining-work audit — 2026-10-09

Read-only source and acceptance audit; no CI, tests, appliance commands or product
edits were performed. Base: completion `11a7125dab5c84e3b28db1dd3180dc81fec7998e`;
additional inspected carrier checkpoint `42b0c993` and helper `216ee225`.
This report is not final whole-tree approval or proof of their combined behavior.

## Findings

- The completion board has 212 rows: 197 merged, 8 review, 2 running, 5 parked.
  These are source workflow counts, not release readiness or live worker counts.
- Carrier source is absent from the published completion checkpoint. Its separate
  checkpoint contains explicit VLAN/QinQ resolution and live classification
  verification; distinct raw/transit TAPs; isolated carrier registration preserving
  the RA wrapper; PD dynamic registration and verified admission binding; private
  resolver staging; missing-TAP recovery; and PPP WAN default-route ownership.
  Final integration must preserve the completion branch's sealed credentials,
  unnumbered registration, WAN adapter, packaging and existing guard boundaries.
- The runtime WAN membership test exercises removal of the previous mirrored
  default before stopping the PPP unit, and restores the prior manifest's default
  policy. The agent routing-only test exercises projection metadata only. These
  are useful separate controls, but do not themselves demonstrate a failed
  combined scheduler transaction, rollback, and ordering against WAN route writes.
  The final combined reviewer should check that seam and actual runtime adapter
  conformance; this audit identifies an evidence gap, not a proven source defect.
- Helper `216ee225` mounts only per-session resolver state writable over the
  read-only PPP configuration tree. Carrier checkpoint prepares that regular
  single-link, no-follow file before service start. Final package fixtures must
  inspect these exact integrated assets, including installed writable paths.
- Final generated contracts, independent combined review, unchanged quick and
  applicable fixture gates remain non-laboratory work. Restricted local process
  namespaces/foreign UID failures are not PASS and must not become lab deferrals.
- Product LICENSE/copyright authority remains a separate release input; do not
  invent it or describe the overall remainder as laboratory-only while unresolved.

## Current documentation requiring reconciliation

`STATUS-FINAL.md` still describes an earlier six-task campaign and owners; it is
not a current final certificate. `DEFERRED-ACCEPTANCE.md` current header still
lists credential source open, and old paragraphs list now-addressed TD19 trust
and DHCP source gaps. Preserve historical receipts, but replace current-status
summaries using exact final merge and validation evidence.

The current OSPF guide says production MD5 awaits PENDING-secret-channel, while
the completion API delivery already admits OSPF/RIP MD5 references and the FRR
secret-generation regression covers rotation/historical rollback. Update that
current limitation after combined validation. The Multi-WAN guide still says PPP
handoff unavailable; update only when the real carrier binding is integrated.

## Genuine deferred target work

The five parked rows describe actual target evidence: RA supplier/session and
identity acceptance; private FRR mgmtd/200-route current proof; Global Blocking
packet/cost/browser proof; NAT46 packet/FIB/restart/API evidence; and OSPF native
peer/FIB/restart/API evidence. Keep prior failed native observations visible.
A missing lab does not establish that their suspected causes are fixed.

The central deferred campaign additionally retains actual package installation,
boot, privileges, physical/NAT/ACL forwarding, PPP discovery/authentication/
reconnect, delegated-prefix/RA behavior, dynamic WAN failover, appliance HA and
real browser acceptance. All require exact target SHA and observed outcomes.
No mock, unit or fixture result closes them.

VDOM, NTS server, Ansible, NETCONF, external certification and the deliberately
excluded full QA/72-hour campaigns are not silently expanded into this source
completion request. No new omission in those excluded scopes is asserted.

Next: combine carrier/helper and reviewed completion source; verify the named
seams; regenerate; independent review; one final CI campaign; merge the tested
tree; then refresh board and current reports with source/target boundaries.
