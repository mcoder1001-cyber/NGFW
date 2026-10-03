# Independent PKI expiry alarm correction review

Reviewed immutable `95371b7c` in PKI-api-gap versus base `b2c214b5`: two severity/message expressions, new expiry.test.ts, unique status only. Verdict **APPROVE WITH VALIDATION LIMITS**, no concrete blocking finding.

Critical severity now uses exact notAfter<now instead of rounded daysLeft<=0. A still-valid certificate's final partial day and exact boundary equality stay warning; +1ms is critical. This matches existing import validity check pki.service.ts:563 and mounted inventory's exact timestamp comparison. Rounded days remain a display metric and configured alert-window comparison. Message follows corrected severity, so valid final-day facts are no longer called expired.

Seen map, change detection by days/severity, repeat dedup and cleared events are unchanged. Exact-boundary severity transition is observable even when rounded metric remains the same. Added tests build real P256 certificate DER, project both CA and certificate from controlled mocked datastore/material providers, and check final-hour warning, expiry boundary, one millisecond later critical wording, and quiet repeated checks. This is meaningful service behavior rather than isolated arithmetic. No new private-key reads or outputs; only existing public certificate observation is used.

Reviewer ran no tests/lint/build/full gate and made no product edits. Root owns focused regressions and unchanged hosted quick gate. Live appliance expiry alarms and end-to-end delivery remain NOT RUN. Approval is for this narrow correction, not materializer/secret-transport implementation.
