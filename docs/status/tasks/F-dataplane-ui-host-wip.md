# Durable checkpoint
Branch: codex/ready-dataplane-ui-host-20261004; worktree /root/.codex/worktrees/f796/work-dataplane-ui-host.
Owned: test/topology/dataplane-ui/** and F-dataplane-ui-host status files.
Product completion already integrated at 15ce0d28b (#147); stale source branches not replayed.
Implemented: bounded loopback API driver, developer-slot validation, clean candidate precondition,
observed runtime requirement, worker preview digest/diff/gated apply checks, invalid corelist semantic pointer,
startup digest and NRestarts invariance, finally candidate discard and clean-diff verification.
Tests: python3 -m unittest discover -s test/topology/dataplane-ui -v: 3 tests OK.
Live endpoint/browser: NOTRUN; slot stack/token not provisioned. Slot 14 allocated; lab status no rig objects.
Mandatory quick hosted gate required before merge. Next: lock boundaries, independent review/CI.
Publication SHA: resolve git HEAD and remote branch; publication outcome recorded after successful push.

Published checkpoint: e83c24fb5d827c8925454884ece886dbe94e3178, PR #160, via GitHub connector; CLI push403.
Independent review correction: dedicated ApiKey authentication, no-op PATCH acquires candidate lock; require API-key owner, recheck lockedAt/owner before edit and cleanup; compare baseRevision. applyAvailable accepts boolean as root enables Apply. Browser remains NOTRUN.

Final source independently reviewed by root after candidate-key locking correction. Remote dcec09ed6d4589fc56851fbd7011426a7e954847 PR160; local archive refs/archive/dataplane-ui-host-reviewed-20261004 preserves reviewed history. Three source tests PASS. Source includes root applyAvailable boolean contract (no disabled-admin assumption); browser acceptance NOTRUN.
Full original-base quick failed only known root baseline schema-group/tunnel locale tests; installation inode failure was resolved by worktree relocation. Root prerequisite fdcac943e19c5a097a7fc959939ab55cd4a6256c integrated as preflight, final current-main CI still mandatory.

D112 integration prepared on exact main f8fcd6c2fc8cfddfe8c34397681acec44724225d after PR162 complete hosted quick passed. Reviewed history preserved locally under refs/archive/dataplane-ui-host-pre-final-20261004 and remotely archive/dataplane-ui-host-final-reviewed-20261004. Main prerequisite WIP conflict resolved by retaining main unchanged. Mandatory unchanged hosted quick pending; refresh onto latest main again when prior queue products merge. Live acceptance NOTRUN.

Cumulative D112 integration on main d1aba878ca5f0f2420e3ca146ffaaa4873a64e1a after management163 merge. All upstream Apply/IGP/WAN/PPP/management preserved verbatim; no conflicts or source changes. Targeted driver3tests PASS. Standalone green a990ad73 retained in archive/dataplane-ui-host-standalone-green-20261004 and local corresponding ref. Full unchanged cumulative hosted quick pending; live/browser acceptance NOTRUN.
