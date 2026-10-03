# Independent Capture review — 2026-10-03

Reviewer 2 reviewed the final integration diff read-only, excluding unrelated native IPsec and VRRP changes. **Scoped approval: no actionable blocker found in stop, retention or interrupted-capture recovery.** This approves the reviewed changes for integration review; it does not claim merge or real capture host acceptance.

Reviewed `capture.go`, capture RPC/service/startup wiring, API AgentClient/controller, UI queries/page/locales, and their focused regressions. In particular:

- Failed VPP stop preserves the running slot and durable running record. Failed filter restoration has a separate marker tied to the VPP boot; recovery retries it on the same boot and drops stale markers without changing a new boot's globals.
- Refusing a foreign source or already-kept file becomes handled only after the terminal error record saves successfully. Metadata-save failures remain retryable. Recovery can inspect a file moved before its record was committed.
- The running-slot release checks record identity, so an older completion cannot release a newer capture. Retention excludes running records and preserves the newest kept capture; oversized requested plans fail before capture mutation.
- HTTP Stop is admin-only and audited, cancels only the exact stream held by this API process, and returns 202 as a request acknowledgment. Unknown IDs yield 404; listed captures without a locally held stream yield 409. It does not pretend to stop another API process's stream. UI confirmation and continued polling match these semantics.
- Capture RPCs retain owner checks; globals authorization comes from startup-resolved service configuration. The stream's late error log contains only capture ID and gRPC code.

Independent verification, through the heavy semaphore:

```text
tools/heavy.sh go -C apps/agent test -race -count=1 ./internal/actions/capture-trace/
ok ngfw/agent/internal/actions/capture-trace 13.876s
```

No VPP mutation or restart was performed for this review. Existing separately documented limitations remain: buffered download, temporary VPP-file permissions, actual capture host/API acceptance/screenshots, and lack of a binary-API packet-count/status getter. This review does not expand approval to those follow-up tasks or to power-loss durability of filesystem writes.
