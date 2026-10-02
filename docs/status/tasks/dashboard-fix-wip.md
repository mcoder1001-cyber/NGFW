# Dashboard merge checkpoint — 2026-10-02

Branch: codex/dashboard-fix-20261002. Base: origin/main 2312bd4a (recovery policy and deferred campaign preserved). Reviewed predecessor remote: 18a5b5d9c7750328160ffc14ee3178c8ac68bed9, archived as archive/dashboard-reviewed-20261002 before final squash. Manager owns PR58 merge queue; no board edits in this branch.

Owned scope: existing dashboard host stats/listener files and task docs; MPLS Persian regression test required by complete CI.

Hosted predecessor gate 37004866519 failed only the MPLS Persian tab assertion (556 other tests passed). Pinned Node22.23.2 focused baseline reproduced no failure: 7/7 pass. Exact timing cause is not proven. The case bypassed real language settings using direct i18n mutation, leaving theme/document language English despite claiming RTL acceptance. Updated it to select Persian through the existing Settings UI and close its modal. Exact Persian tab/title assertions retained; added real html dir=rtl and lang=fa checks. No skip, timeout increase, production mock, or reduced gate.

Actual tests: frozen install passed; schema/UI-kit/API-client builds passed; focused original MPLS 7/7 and updated MPLS 7/7 (10.55s) passed under pinned Node22.23.2. Default Node24 is unsuitable for this repo's jsdom/router tests: native Request rejects jsdom AbortSignal before route mount; that environment failure is not product acceptance. Targeted lint/typecheck in progress; full unchanged hosted quick and fresh independent R1/R6/R7 review pending.

All real VPP stats/listener/alarm/restart/browser acceptance remains NOT RUN in docs/status/DEFERRED-ACCEPTANCE.md. User authorized laboratory deferral; genuine code/CI failures still block merge. Remaining broader dashboard capability/daemon acceptance is explicit in inherited task report.

Next: publish coherent remote checkpoint, independently review new test delta, run unchanged hosted complete gate on current-main integration tree, fix any real failures, merge sequentially only after green. Remote commit SHA must be read from branch after publication (self-referential SHA is not invented here).
