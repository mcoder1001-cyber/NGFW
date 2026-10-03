# Independent P14 console recovery regression review — 2026-10-03

Reviewed only new test_console_recovery.py and WIP in immutable9ef326cd1f3528a5d8b2f7b412835d5432e966eb against actual bootstrap/banner scripts. Verdict APPROVE source tests with aggregate coverage limit below. No product edits or test execution performed.

Tests use temporary image roots, actual bootstrap generation and actual banner render/check-login, not a reimplementation of cleanup. The existing query hook applies only when LIVE is false; supplied --root keeps console writes and systemctl changes disabled. Fixed fixture script reads response bytes from a file, avoiding shell interpolation of case content. Independent roots per uncertainty case are cleaned through unittest cleanup.

Partial zero output with nonzero database status and successful empty/negative/nonnumeric/multirow outputs must preserve credential and issue bytes and private issue0600 mode. The subsequent valid zero exercises recovery: password copy removed, secret absent from issue and captured stdout/stderr, issue0644 with HTTPS URL preserved. Another check verifies idempotent issue content. These are useful boundary/outcome assertions; they would detect premature deletion on partial or malformed database results and failed recovery.

Limits: password-file permissions are not directly asserted by these new tests (issue permissions are); initialization and final idempotent-call diagnostics are not separately secret-canary checked. Existing product writes remain unchanged. The root-scheduled standalone Python command executes these regressions now, but tests/run.sh explicitly invokes only disk_guard, render and vpp_manifest Python files and does not discover this new test. Coordinate adding the console recovery command to the shared aggregate before claiming durable hosted coverage; worker appropriately left that shared file outside its envelope. This is a test integration limit, not a false approval of live appliance first-login/database acceptance.

Root owns queued real tests and aggregate coordination. No databases, network, host root, services or shared host operations were accessed by reviewer.
