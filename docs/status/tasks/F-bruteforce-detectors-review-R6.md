# F-bruteforce-detectors independent R6 review — 2026-10-04

Reviewed final local source 5c88cb5f + docs1df528d4 against local main. Review covered requested
docs/evidence/scope aspect only; no product files edited. Read AGENTS/shared
review rules and applicable reviewer prompt.

## Findings

No findings. Existing user guide now explains destination-port repeat refresh, source-bound scan evidence, deployment compatibility, 4096 ports/source,10000 sources,100000 timestamps and threshold>4096 disable/reload warning. Source DistinctPortWindows constants and AutoBlockService.reload warning match those claims; saturation discards evidence and may miss attacks. NativeVPN capability and charon test-only guidance clear. Topology README CLI uses pinned SSH host keys, disposable rejected key and shared-slot lock and distinguishes401/permission denial from transport errors. No UI changes; screenshot/RTL checks not applicable.

## Review evidence

Read task final/WIP reports, scope envelope and relevant renderer/user/topology
docs, and compared described limits with committed source.
`git diff --stat main...HEAD` confirms no web/UI changes in these deliveries.
Pasted developer test evidence was inspected; this reviewer did not rerun those
tests, full quick CI or live lab. No new test-success claim is made.
Remote publication equivalence must be checked separately by manager.

Verdict: **APPROVE**.
