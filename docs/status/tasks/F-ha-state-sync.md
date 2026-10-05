Final integration 2026-10-05: actual BFD MAINbb785c4c14dd01dbc3553853b75a9d1971ef6135 treeabf7. Preserves external PR182 P12 status and all five campaign feature merges; BFD actual board closure included. Current generated BFD+HA product union unchanged from normal generation proofs. Exact frozen current-main independent complete local T1 and hosted quick remain mandatory; no final PASS yet.

Prospective refresh 2026-10-05: parent frozen BFDc6cf87d19616e8ba25a66fd0d121b8fcaa62e1d7 incorporates external board reconciliation on actual mainba17cb2b86d5a1672dd5a2f36291f2785fc96945. HA product and generated union unchanged; actual BFD merge and complete final local/hosted gates still pending. Historical generation/test pins below remain explicit.

# Current integration review state

Manager-owned codex/integrate-ha-final-20261005 in /dev/shm/ngfw-integrate-ha-final-20261005, prospective frozen BFD parent38d53a14 on actual mainf445f57c; final must be rebased to then-current main before complete gates. Product API834eed6f and UI075c062b include separately assigned permission and stale-observation repairs. Source histories archived; current R3/T2 APPROVE/PASS23unique actual API scenarios with permission403/audit repeated twice, preserved409503, real API restart and actual stream7cases (remotece852bcf). Author691APItests/typecheck/check PASS. Prior exact48e5 wholequick PASS32m02 is not latest full-source gate. Fresh R1 permission closure19311b07, whole R2 security74ba755d, whole R4 native ownership52fd2795 and R6/T4 UI closure4ca796dd all APPROVE/PASS; scoped reports adjacent. Fresh whole R1 APPROVE remote6631a42f and final integration R7 APPROVE remotecebc94fb recorded; scoped-ledger and current driver-guide minor addressed. Independent full T1 on frozen current-main integration and hosted complete quick remain mandatory. R5 native timers/observations and R8 tooling unchanged since48e5 carry only that exact scope; topology acceptance16fixtures independently R4/T3 PASS preserved. Actual second appliance ngfw-b unprovisioned, two-node forwarding continuity NOTRUN; no source or mandatory gate deferred.

Historical chronology follows, superseded where dated.

# F-ha-state-sync — source delivery and acceptance

Draft PR178: https://github.com/mcoder1001-cyber/NGFW/pull/178 .
Native binary API delivery is partial as required by the task: NAT44-EI listener
and failover globals with getter readback, globals-owner boundaries, bounded
completion-correlated resync and truthful configured/observed REST/RPC/UI state.
NAT44-ED and reflexive ACL session sync remain unavailable; native IPsec requires
rekey after failover and does not replay SA sequence/anti-replay state.

Additive contracts and native product source are recorded in the contract/wip
files. Manager integration03deae02 corrected40 generated YANG lines via the
normal generator after the unchanged quick rejected the missing artifact. The
next complete quick found one actual product-text branding assertion; the en/fa
locale correction retains the unsupported behavior and unchanged assertion.

The concrete acceptance entry is test/topology/ha-state-sync/acceptance.py. It
maintains one challenge-verified TCP socket, matches the live exact NAT tuple on
A and B, observes VRRP role transfer and B forwarding counters, and rejects
reconnection/truncated/ambiguous evidence. Priority mode uses an unconfirmed
60-second transaction with guarded candidate lease/revision cleanup and confirms
automatic restoration. The optional VPP fault helper requires a root-owned
isolated-VM marker, expected boot/PID, handover nonce, strict SSH identity and
pidfd; its real execution is authorized only after D-012 manager handover.

Actual repair checks:13 offline Python cases PASS0.031s, including persistent
real loopback socket and safety/replay/tuple/failover refusal; concrete10 cases
PASS0.029s as UID65534; workspace build8/8 PASS1m17s; focused WEB6/6 PASS9.26s.
No actual appliance failover or signal is claimed. Root-owned repair branch
codex/ha-acceptance-probes-20261005 has durable remote checkpoints; previous
source history remains on codex/integrate-ha-20261005 and the original task.

Remaining merge requirements: complete unchanged quick on the frozen repair
and final current-main integration tree; independent applicable reviews and
tester reports, D112 history archive/single-commit integration/expected-head
merge and main CI. The original generic-hook driver is historical; its command
exit codes are not acceptance proof. Detailed setup and deferred real second-node
run are explicit in test/topology/ha-state-sync/README.md and DEFERRED-ACCEPTANCE.

Probe timing update: the bounded single-socket read now permits10 seconds for a
normal VRRP master-down interval; a real3.2-second delayed echo test preserves
the same connection.14 offline cases PASS3.241s. Root complete quick on prior
c423 failed only the existing unbound socket-path environment bound108vs107;
short private executable temp/cache retry remains required. Older independent
13-case reviews remain provenance, with this timing-only delta awaiting verify.
