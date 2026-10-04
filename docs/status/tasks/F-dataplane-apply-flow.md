# F-dataplane-apply-flow source completion

Branch: codex/ready-dataplane-apply-20261004; PR #162.
Built root Unix HTTP executor, single-use 120-second approvals bound to authenticated
actor/document/generated SHA/installed SHA, fixed product apply invocation, admin-only
write-ahead audited API actions, confirmation UI with en/fa wording, generated client,
and installation socket/service plus Python runtime dependency.

Independent reviewers igp and wan_pppoe: APPROVE on correctness/security. No live
appliance apply, VPP restart or host installation was performed. Live apply/health
rollback acceptance is deferred to a dedicated appliance window.

Actual verification:
- python3 -m unittest discover -s deploy/vpp -p test_apply_executor.py -v: Ran 6 tests, OK.
- API dataplane mapping + auth route guard: 2 files, 11 tests passed.
- API approved action routing: 1 file, 3 tests passed.
- Web dataplane + locale + product text: 3 files, 8 tests passed.
- Package staging fixture: 3 tests, OK; hosted full packaging fixtures PASS at aa73e52122a.
- Mandatory unchanged complete quick gate running from final committed source;
  no gate PASS or merge claimed until the actual final output is recorded.

Inherited main blockers were repaired without changing the gate: the IS-IS
interface-default fixtures now expect implemented ipv4/ipv6 and RIPv2 defaults; tunnel
product messages describe actual restrictions without implementation branding or lab
environment flags. A superseded local gate was stopped after source repairs; the final
clean run is /root/ngfw-wt/apply-quick-final.log.

Independent integration_review: APPROVE; executor + staging 9 tests OK.
Schema default + 200000-entry parser targeted run: 2 files, 30 tests PASS.
The final local gate bounds task scheduling to two CPUs to avoid shared-host
contention; it runs every unchanged check. No timeout or assertion was weakened.
