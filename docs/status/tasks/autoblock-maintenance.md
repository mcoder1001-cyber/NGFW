# Inactive auto-block heartbeat correction

Hosted complete gate37177580917 failed only TestRequestedResyncsAreRateLimited:
seams_test1084 received an unexpected RECONCILE_START. Hosted artifact09-agent.log
also recorded mode=auto-block,domains=[acl] at first1s maintenance tick. Requested
resync logs showed normal immediate/deferred/coalesced operations.

The fresh runtime's fingerprint starts empty. watchAutoBlock compared it with the
fingerprint of an empty/inactive configuration and performed an ACL-authoritative
runtime transaction even when auto-block was unconfigured/disabled, entriesempty,
and dirtyfalse. This could remove an unrelated same-owner ACL, as well as emit the
unexpected event. Filtering or relaxing the test would conceal a real product bug.

The8line locked guard skips only clean inactive runtimes with no retained entries.
Enabled protection still runs; dirty authoritative empty snapshots still clean stale
rules; retained cached/expired entries still drive cleanup/retry/recovery. Existing
reconciliation, fingerprinting, permissions and event contracts are unchanged. The
original resync rate-limit/no-extra-event assertions are completely unchanged.

Deterministic new tests start only the real maintenance watcher over the existing
scheduler/coretest model (no external Unix socket):
- Unconfigured and explicitly disabled clean runtimes emit no reconciliation and
  preserve the seeded external ACL across a real1s heartbeat.
- A dirty empty snapshot removes stale system-owned overlay ACL, clears dirty, and
  records the applied fingerprint after the real watcher transaction.
- Active cached entry expires on the injected clock; the actual watcher removes VPP
  ACL and host table enforcement, rather than a test calling reconciliation directly.

Before fix, both inactive cases failed with exact `auto-block [acl]` event evidence
(/tmp/autoblock-inactive-before.log,2.130s). After fix, own inactive+dirtyempty race-count3
GOMAXPROCS2 passed12.372s; independent R1 expanded maintenance/lifecycle/scope count3
passed12.771s, R2maintenance/lifecycle passed3.725s. Own existing lifecycle/scope-count2
passed1.593s. Final own GOMAXPROCS2 `go test -race -count=3 ./internal/agent -run
'^Test(InactiveAutoBlockMaintenance|DirtyEmptyAutoBlockMaintenance|ActiveAutoBlockMaintenance)'`
passed15.310s; /tmp/autoblock-maintenance-final.log. Pinned golangci-lint2.13.2 on
internal/agent/... returned0issues; source/security check with gitleaks passed.
Independent R2 real watcher active-expiry test passed1.051s.

Original resync rate-limit test locally fails before behavior at seams_test1069 because
Unix socket creation is denied (EPERM);10repetitions37.603s, no skipped/modified test.
Its hosted behavior and unchanged complete gate must be verified after publication.
No full CI GATE PASSED claim and no live packet/lab acceptance claim.
