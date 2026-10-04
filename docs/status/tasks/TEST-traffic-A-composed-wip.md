# Durable checkpoint
Branch codex/ready-traffic-a-composed-20261004.
Worktree /root/.codex/worktrees/f796/work-traffic-a; owned traffic-a source/status scopes.
Completed: composed baseline/chain document builder for tagged LAN100 bridge+BVI,
slot VRF/static ECMP/static neighbor, strict uRPF, ACL object group, TCP8001 PBR→202,
NAT44-ED inside BVI/outside201+202. Scope linkage and unsafe-input tests added.
Actual tests: python3 test/topology/traffic-a/check.py:49 tests,0failures,0errors,0skips,OK.
Remaining real source: execute product transaction, private two-side capture lifecycle,
fixed peer/probe server commands, full pcap assertions, ED→EI switch, baseline/rig cleanup.
Live: NOTRUN, manager explicitly has not granted exclusive traffic window.
Next exact command: edit test/topology/traffic-a/execute.py; rerun check.py.
Local/remote SHA: resolve HEAD; publication outcome recorded once successful.

Checkpoint2: execute.py/peer.py and globals/ now implement lease-gated composed transaction, private two-side capture lifecycle, fixed TLS-independent TCP peers/probes, pcap outcome checks, empty-owned NAT fixture ED/EI transitions, baseline/original rollback and rig cleanup (refuses unsafe partial cleanup).
Actual source tests53 PASS. RootConfig generated schema validates chain with semantic[]. Helper unit/vet require final output check. Live NOTRUN (no exclusive manager window granted).
Remaining: independent review safety/coverage fixes; readback/counter attribution and host residue assertions before whole-chain claim. Driver presently keeps whole_chain_proven=false honestly.
Remote checkpoint1:81199250a891e7cda6fd839c8c16cc77dc712cd3 via connector, CLI403.

Final source checkpoint: executable lease-gated chain with private captures, fixed probes, correlated packet assertions, CLI bridge/BVI/FIB/ABF/ACL/NAT readback, strict-uRPF drop delta, retained raw allocated-interface counters, candidate hash/revision linkage, owned empty NAT fixture transitions with complete config fingerprint + EDVRF/users guards, normal rollback + table/pool/interface/namespace residue + unchanged VPP PID/restarts. PASS is emitted only after cleanup/fixture exit succeeds.
Actual final fixtures55tests,0failures,0errors,0skips. Go globals3tests PASS; go vet PASS. RootConfig validation success/semantic[]. Live NOTRUN by manager: no otherwise-idle traffic window granted. Source ready for independent final review, mandatory full quick pending root prerequisite fixes.

Final source APPROVE by integration_review on119d95cab148dfa5342723fc056f551023a6649e, independently12composedtestsPASS; full59sourcefixturegatePASS. Live NOTRUN. Root baseline prerequisite fdcac943e19c5a097a7fc959939ab55cd4a6256c integrated for complete unchanged hosted preflight. Local refs/archive/traffic-a-reviewed-20261004 preserves reviewable source history. Final current-main integration gate remains mandatory.

Safety followup published remote ee28a44ca432a0f9d802734e91d3f22782423dae (local d0cf1954b, tree5de880f40e25079dd2548c5f50a3a17d595a92ad). Canonical slot NAT lease now /run/ngfw-test/w14/nat44.lock shared with feature suites. Restore-original lowers all owned peer/host veths before any product host-interface deletion, including initial revision zero; regressions verify ordering and all-link attempts after failure. Actual strict source gate61tests PASS, zero failures/errors/skips. Independent followup approval requested. Earlier integrated prerequisite/preflight is superseded by manager final7fff162f8f6bf66c3b418cd6b4da12bf08b5fa19; no complete quick green claimed. Next exact command: git fetch origin, after manager confirms main prerequisite merged preserve reviewed history and prepare D112 single commit atop exact current main; then complete unchanged hosted quick. Live remains NOTRUN.

Independent integration_review APPROVE exact local d0cf1954b / remote ee28a44ca432a0f9d802734e91d3f22782423dae including canonical NAT lock and all-endpoint down-before-API cleanup regressions. Approval explicitly requires unchanged complete quick; live NOTRUN. Source frozen.

D112 integration prepared on exact main f8fcd6c2fc8cfddfe8c34397681acec44724225d after PR162 complete hosted quick passed. Reviewed history preserved locally under refs/archive/traffic-a-pre-final-20261004 and remotely archive/traffic-a-final-reviewed-20261004. Main prerequisite WIP conflict resolved by retaining main unchanged. Mandatory unchanged hosted quick pending; refresh onto latest main again when prior queue products merge. Live acceptance NOTRUN.

Final cumulative D112 integration on main4069b365d1b762411ebdfc1a07c37461de04cb18 after dataplane160 merge; all six upstream task products retained unchanged. No conflicts or traffic source changes (git diff against standalone green source empty). Existing actual61Python fixtures and Go3tests/vet PASS remain applicable source evidence; independent source approval unchanged. Standalone green e20afc archived remotely archive/traffic-a-standalone-green-20261004 and matching local ref. Complete unchanged cumulative hosted quick pending. No live forwarding window granted: live acceptance NOTRUN, no whole_chain_proven result generated.
