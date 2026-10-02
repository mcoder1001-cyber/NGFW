# Notifications R3 optional correction verification

Original R3 APPROVE report is preserved in `notifications-resume-review-R3.md` (commit `fcf77e24`). Its only optional MINOR finding was the undocumented 400 response for an invalid channel-name path parameter.

Independently inspected `87784db6eda3f99744fd05fd66cbbb2bf60e3d4e` and manager integration `80c6fcd9` through `git show` from the review workspace. The controller now declares `@Protected(400, 404, 409, 503)`. The generated `Notifications_test.responses` now includes 400 with `application/problem+json` and the shared `Problem` schema; successful and existing error responses remain intact. The correction commit contains the matching nine-line generated-client addition. This is additive documentation of existing runtime behavior, with no request, success-response or config-shape change.

This is a source/delta verification, not an independently rerun full generator or quick gate on the newer checkpoint. The earlier review independently regenerated all relevant outputs on the original tree; developer generation and the exact hosted gate remain integration evidence for the newer tree. No product edits.

**Verdict: APPROVE.** Optional R3 MINOR is resolved. Other reviewer findings and subsequent product changes retain their own verification requirements.
