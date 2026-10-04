# Recovery checkpoint

Source product guard8lines under existing service lock: inactive/clean/noentries skips
maintenance; enabled, dirtyempty, cached and expired-entry cases keep current path.
No seams_test.go edits. New no-socket real-watcher tests cover unconfigured/disabled
externalACL preservation, authoritative dirtyempty teardown, and enabled cached expiry.

Actual pre-fix focused race failure: both inactive cases emitted exact message
`auto-block [acl]` after first1s tick; /tmp/autoblock-inactive-before.log2.130s.
Hosted failure had same mode/domain, while requested resync rate/coalescing was correct.
Actual pre-fix bug was unconditional inactive ACL runtime transaction, not just testnoise.

After guard: own inactive+dirtyempty GOMAXPROCS2race-count3PASS12.372s; independent R1
maintenance+scope/lifecycle-count3PASS12.771s; R2maintenance/lifecyclePASS3.725s.
Own lifecycle/scope regression-count2PASS1.593s; pinnedlint agentpackage0issues before
last positive expiry test. Final maintenance incl active expiry-count3PASS15.310s; final pinnedlint0issues and source/securitycheckPASS. Logs /tmp/autoblock-maintenance-final.log, /tmp/autoblock-maintenance-lint.log.

Original rate-limit test repeated-count10 locally stops at Unixsocket EPERM before
behavior (seams_test1069); /tmp/autoblock-maintenance-after.log37.603s. No rate-limit
assertions weakened and no claim it passed locally; hosted unchanged full gate required.
Source frozen; next root publish/review/unchanged full hosted gate.
