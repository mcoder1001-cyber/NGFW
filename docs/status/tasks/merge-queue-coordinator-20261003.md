# Coordinator merge queue — 2026-10-03

Merged product changes:
- PR111 finite test handoff and reserved short lane, main a64eab8b. Independent reviews and mandatory complete hosted quick passed. Current-main composed tree67f083f9:7 scheduling/process tests passed; standalone recipient8 tests passed103.9s.
- PR112 offline ISO scaffold and fail-closed inventory guard, main454dd312. Independent installer review and mandatory complete hosted quick passed. Post-pipeline composed tree929ccd4c:68 ISO checks and7 scheduling/process tests passed. Full signed ISO pool/build and disposable VM acceptance remain deferred; P14 is partial.

Pending merge queue:
- PR106 LCP ownership/recovery, remote29816653: scoped lint0 issues and race lifecycle tests pass, independent reviews approve. Mandatory hosted quick running after fixing full-lifecycle fake MFIB and mixed-namespace fixtures.
- PR114 Notifications, remote1da4fa19: local generation clean; API77 tests, schema11 tests, UI13 tests pass; typechecks/scoped lint and secret scan pass. Independent source reviews approve. Initial hosted quick failed only two unused schema test bindings (34/35 task success); corrected and new hosted gate required. No Telegram.
- PR116 PKI backend, remoted51f5948: API73 tests and YANG13 tests pass, strict generated client consumer compile passes, independent privilege/audit/publicDTO and crypto reviews approve. DER parent bounds, shared100k digest-round budget and OCSP freshness repaired. Hosted gate pending. Agent file-state/secret runtime and full UI remain incomplete.

New work:
- ISO reinstall label explicitly says largest disk; source23308a43, renderer/68-test suite pass; root independent source review approves. Label/flags/selectors preserved except accurate text.
- Standalone public PKI inventory panel76ca893b reviewed; route/expiry defects corrected in02cb4c1e, tests pending. Navigation not yet wired; panel is not shipped functionality.

All developer product work uses separate named branches/worktrees. Root owns finite tests; developers continue successors. Three child slots plus root form four workers; no persistent AI service claimed. Remote review histories for merged PRs retained archive/test-pipeline-reviewed-20261003 and archive/p14-installer-reviewed-20261003. Main CI was queried after each merge; run registration was not yet visible, so no main CI pass is asserted.
