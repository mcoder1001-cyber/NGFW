# NGFW progress — 2026-10-07

Fresh checkpoint at16:01UTC. Owner requested advancing available tasks and previously explicitly authorized all ready merges without fresh CI. Focused unchanged tests and independent source reviews retained; no manually launched aggregate CI, host installations, shared VPP/service/NIC changes or production deployment. Automatically triggered GitHub workflows are observed separately.

| Work | Actual outcome | Evidence and limits |
|---|---|---|
| PPPoE prerequisites | PR201 merged1c3092efbe411566d1ab5815790a47cfd8d62352 | Direct agent ppp/pppoe dependencies; author and independent packaging2/2 PASS. Discovery/encapsulation/PD and transition-admission/parent identity code failures remain. |
| P12 safety recovery | PR203 merged188a2ea9835298466e9fc2601add2bfa587204b1 | Actual nsfs handle inventory, strict fallback and cleanup failures recovered; independent producer identity BLOCK fixed and round2 APPROVE1409c4739. Latest combination Python11/11 PASS0.041s; Go race agent1.857s/BGP1.128s PASS. Current mgmtd30s failure cause and native200route proof still unknown/open. |
| Remote-access VPN | PR202 merged19052bb130ab46977dc5b7aa7be25e976fb5aaf9 | Exact45-path reviewed final-union delta restored; bounded authenticated PID1 property transport, session pagination, lazy renderer and localized counters. Independent source APPROVE97f83b97b. Latest integration Go race runtime3.341s/renderer7.145s/agent74.701s; packaging3/3 PASS2.632s. Author UI11/11, tsc/scoped ESLint PASS after documented initial failures. Actual runtime READY/EAP/TLS/API/browser/restart acceptance remains open. |
| TD19 installers | PR204 draft, not approved at545f118af | Alternate-root/dry-run seam and pinned build tools added, but independent review112fa6382 BLOCK: unchecked rooted executable/PATH and arbitrary regular stubs can escape recording. New9/9 PASS missed these failures; strict45 had44PASS/1FAIL copied helper fixture. Live developer repairing source and fixture; no actual installation. Python dependency hash closure still requires authoritative supplied lock; default apply fails closed. |

Reviewed history preserved on remote codex/archive-pppoe-readiness-20261007, codex/archive-p12-recovery-20261007 and codex/archive-ra-union-20261007 before single-commit squash integrations. Main never rewritten. RA's existing approved source decision docs/decisions/DEC-ra-pid1-property-readback-20261006.md preserved in203.

Board reconciliation: TD19's repository-trust decision was already answered in DEC-238-td19-repository-trust.md; stale parked trust reason corrected. TD19 currently running means observed source work; RA running means remaining functionality/acceptance, with source workers completed. Fresh counts205/212 merged,2running,5parked; seven original rows remain open. Original remaining estimate27.5hours is not measured time-to-finish or operational readiness.

Observed live inventory: root manager/developer and TD19 developer running; independent reviewers completed and available, no active tester claimed. RA/PPP developers completed. Global chat worker inventory unverifiable; no persistent supervisor process/checkpoint evidence asserted. Native work awaiting resume requires existing authorized host/slot preflight; handover remains pending.

Automatic main workflows on19052bb13 (37649374922/37649374791) in_progress at this checkpoint; previous main1c3092efb workflows37648058922/37648059006 SUCCESS. This is neither fresh full gate certification nor a claim that final main CI has passed.

Next: publish TD19 safety repair, rerun unchanged focused45 plus independent adversarial negatives, merge only approved repaired source, refresh this report and board. Remaining host acceptance (global blocking, NAT46, OSPF, P12, PPP, RA), P10 privilege/file ownership decision, PPP datapath/PD and multi-WAN gateway handoff are still explicit. No laboratory failure relabeled PASS.
