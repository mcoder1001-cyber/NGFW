# OSPF controller error-contract test successor

Immutable `619bca2a`, tiny test-only change vs `fad3717c`.
Verdict: APPROVE WITH LIMITS.

The test captures the rejection and verifies actual ProblemError class, HTTP 503,
and exact public body type/title/status/detail. This asserts the observable error
contract more directly than object identity. Catch returns its unknown rejection
unchanged. A resolved healthy observation (or undefined) fails the first instanceof
assertion; the test does not silently accept resolution or replace the error with
a fabricated passing fixture. Type assertions only affect TypeScript and do not
bypass runtime checks. No production behavior changes in this successor.

Source-only review, no tests run by reviewer. Root's focused retest was still
running when assigned; no passing claim is made here. Prior compatibility approval
remains scoped to the already reviewed Full/- and NBMA partial-observation fixes.
