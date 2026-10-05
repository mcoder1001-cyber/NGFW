# TEST-traffic-C independent process/protocol review

Reviewed frozen local216a1330c / remote3bd70b3941b14b03b7a822355c54c6d427aca0b0,
exact tree e47b0ff29c1de621213e6ad348535c6e70d2b9aa. Read task prompt,
shared-host rules, envelope, driver/executor/peers/globals helper and inherited
wave-A API, process and candidate-claim primitives. Reviewer changed no source.

Verdict: BLOCK pending P2 owned-process interruption correction.
execute.py entry has no SIGTERM handler. Manager TERM exits without Python
finally close, leaving separate-session children, candidate/rig state and modified
globals. Override entry SIGTERM into controlled interruption. Inherited stop()
only escalates if the leader wait times out; if leader exits before grandchildren,
remaining owned group survives. Local stop override should preserve SIGINT
capture accounting, then always clean surviving group and reap. Add interruption
and gracefully-exiting-leader descendant regressions. Findings sent promptly to
worker/manager. Laboratory availability cannot defer this source cleanup gap.

Independent actual checks:

- python3 -B -m unittest discover -s test/topology/traffic-c -v: 13 PASS,0.011s.
- execute.py --slot14 --dry-run: PASS; modeDRY_RUN_ONLY/live_acceptancefalse.
- tools/heavy.sh go -C test/topology/traffic-c/globals test -count=1 ./...:
  PASS,0.020s (waited40s for scheduler slot).
- tools/heavy.sh go -C test/topology/traffic-c/globals vet ./...: PASS.
- diff whitespace check against d314f0728 for owned code: PASS.

Protocol/parser boundary inspection: receipt requires applied/notAppliedempty,
positive revision, warnings/results shape and unsupported-field refusal. MPLS
label+owned tuple and SRH/SID+inner tuple correlate within packet lines; QoS
requires real UDP sent/received/WAN agreement and all three counter deltas;
VRRP requires real commit disable/enable, timed ping gap≤3s, MASTER/BACKUP and
virtual-MAC evidence. Rider collector validates source and IPFIX framing, product
capture is downloaded/decoded, IGMP group/mFIB and scoped metrics are checked.
Parsers fail closed on unknown packet formats; live formatting is NOT RUN.

Authority: lease is bounded root0600/unaliased, exact task/slot/boot/daemon/
globals/quiet fields and nonce; lab shared/slot exclusive/globals exclusive;
no trace or restart. Candidate ownership before commit/discard protects foreign
writer; partial/lost/expired/revoked scenarios refuse PASS and can require manager
recovery. Cleanup independently attempts original config and global restoration.
Global helper accepts inherited canonical exclusive lock, snapshot captures
exporter/flowprobe/SR globals/table0, and readback equality is required after
binary-API restore. Refuses existing flowprobe bindings and table0 per documented
product-adoption constraint; does not certify acceptance on an occupied shared
host. This limitation is explicit, not silently converted to completed live proof.

No live host/global mutations executed by reviewer. Mandatory quick/current
integration and manager-owned live campaign remain outstanding; passing these
pure checks cannot close live acceptance or TD-H18.
