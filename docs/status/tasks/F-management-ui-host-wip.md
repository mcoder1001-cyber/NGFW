# Durable recovery
Branch: codex/ready-management-ui-host-20261004.
Worktree: /root/.codex/worktrees/f796/work-management-ui-host. Base d5557440c.
Owned: management-ui topology driver/tests/README and matching host status files.
Completed code: bounded loopback API-key client; shared lab/per-slot lock; no-op candidate
lock acquisition; ephemeral certificates/keys; real TLS1.3 fingerprint handshake and TLS1.2
refusal after actual commit; loaded revision; mismatch validation400 field pointer; public
response/audit secret scrub; unchanged API PID/start-time; baseline rollback and secret cleanup.
Live acceptance: NOTRUN, isolated slot stack/dedicated API key not provisioned.
Browser/nav/tab screenshots and API-log scrub: NOTRUN. Product code is integrated;
these are acceptance obligations, not product source fixes.
Next: run host-independent tests, publish checkpoint/PR, independent review and full unchanged quick gate.
Local/remote SHA: resolve HEAD; publish outcomes recorded after connector success.

Reviewed source remote36c58acab6d6fee804285e294668340be5b0b040 PR163. Root+igp independently APPROVE candidate revision/partial apply fixes, actual5testsPASS. Local refs/archive/management-ui-host-reviewed-20261004 preserves reviewed history.
Root prerequisite fdcac943e19c5a097a7fc959939ab55cd4a6256c integrated for unchanged complete preflight CI; no source check bypass. Final exact-current-main gate mandatory before merge.

D112 integration prepared on exact main f8fcd6c2fc8cfddfe8c34397681acec44724225d after PR162 complete hosted quick passed. Reviewed history preserved locally under refs/archive/management-ui-host-pre-final-20261004 and remotely archive/management-ui-host-final-reviewed-20261004. Main prerequisite WIP conflict resolved by retaining main unchanged. Mandatory unchanged hosted quick pending; refresh onto latest main again when prior queue products merge. Live acceptance NOTRUN.

Cumulative final integration refreshed onto actual PPP merged main12472a11ca21d9f0bc5ab330b72250069159c72e, preserving upstream Apply/IGP/WAN/PPP unchanged. No conflicts and no product source changes. Standalone green375f85b archived remotely archive/management-ui-host-standalone-green-20261004 and locally refs/archive/management-ui-host-standalone-green-20261004. Full unchanged cumulative hosted quick pending. Live/browser/log acceptance NOTRUN.
