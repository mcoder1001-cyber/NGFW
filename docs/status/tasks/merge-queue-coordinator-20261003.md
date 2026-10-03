# Coordinator merge queue — 2026-10-03

Merged product changes:
- PR111 finite test handoff and reserved short lane, main a64eab8b. Independent reviews and mandatory complete hosted quick passed. Current-main composed tree67f083f9:7 scheduling/process tests passed; standalone recipient8 tests passed103.9s.
- PR112 offline ISO scaffold and fail-closed inventory guard, main454dd312. Independent installer review and mandatory complete hosted quick passed. Post-pipeline composed tree929ccd4c:68 ISO checks and7 scheduling/process tests passed. Full signed ISO pool/build and disposable VM acceptance remain deferred; P14 is partial.

Additional verified merges:
- PR106 LCP ownership/recovery and exact restart ticks: main fab85bc2, reviewed head1c0fc6c3, complete hosted quick run37120345259 PASS. Local full Go race suite and lint passed at3d4099d2. Exact decimal uptime parsing prevents false current-process restart acknowledgement; rfkit/rsyslog passed30 race repetitions.
- PR114 email/webhook Notifications: main62cc2832, reviewed head62db5333, complete hosted quick run37120282265 PASS. Configuration failure remains latched after a timed-out worker settles. Current-main composed treeb9b6d7f8 matched the actual merge;40 focused API tests and API typecheck PASS. No Telegram or live transport acceptance claimed.
- PR117 ISO manifest completeness and accurate largest-disk reinstall label: main6b7fdda2, reviewed head618ad5e8, complete hosted quick run37120506926 PASS. Current-main composed tree78b6c4f6 matched the actual merge;69 offline ISO checks PASS. Real temporary Debian archives cover manifest/pool parity; full upstream/build provenance remains deploy/vpp/verify.sh.

PKI integration checkpoint (2026-10-03; query PR116 for current merge state):
- Backend/public DTOs and bounded crypto validation remain independently reviewed. DER parent bounds, shared100k digest-round budget and OCSP freshness are repaired; API73/YANG13 focused tests and strict client consumer compilation passed.
- Inventory is mounted at VPN tab pki, with centralized en/fa resources and exact expiry handling.12 panel/router tests, web typecheck/scoped lint and independent mounted-source review PASS.
- Combined source wiring reviewd3fc04ec approved; regenerated Go descriptor at37e48316 semantically matches the entire proto source, including Notifications tag7 and typed PKI RPCs, independently checked. Generator output is committed; no hand-maintained protobuf conflict resolution is claimed.
- Previous hosted PKI run37120588910 passed35/35 Turbo tasks and agent lint/tests/build but failed the stale CLI operations-table check. Actual OpenAPI regeneration adds8 PKI operations in42e301af; full CLI lint/race tests/build now PASS (finite jobdec3079a).
- Independent CLI review found missing public export selector metadata. Source98ca5158 derives optional kind enum/default from the actual Zod query; real generation produces SDK and CLI query support. CLI CA/CRL URL regressions and positive/negative typed consumer probes cover the omission. Source fix independently approved; private-key export remains403 before secret access.
- A fresh unchanged complete hosted quick gate is required for this final integration. Agent GetPkiFiles implementation/runtime secret transport and UI issuance actions remain genuine incomplete functionality; the read-only inventory UI is implemented.

New work implemented above: ISO archive manifest parity, Notifications recovery latch, exact restart acknowledgement and mounted public PKI inventory. Full signed ISO build/disposable VM acceptance remains deferred; P14 is partial.

All developer product work uses separate named branches/worktrees. Root owns finite tests; developers continue successors. Three child slots plus root allow four workers; idle reviewers are not counted as active developers and no persistent AI service is claimed. Reviewed heads are retained in archive/test-pipeline-reviewed-20261003, archive/p14-installer-reviewed-20261003, archive/lcp-reviewed-20261003, archive/notifications-reviewed-20261003, archive/p14-manifest-reviewed-20261003 and archive/pki-combined-reviewed-20261003. Main CI was queried after each merge, but the connector lists only pull-request-triggered workflows; main CI is unverifiable through this tool, and no main CI pass is asserted.
