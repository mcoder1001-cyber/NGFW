# Agent integration fixture closeout, 2026-10-04

انتظارهای قدیمی تست با خروجی واقعی قابلیت‌های تازه تطبیق داده شد.
مقایسهٔ کامل protobuf و شمارش دقیق اشیا حفظ شد.
تست GC برای مالکیت LB فایل مستقل دارد و به TempDir حذف‌شده وابسته نیست.
تست اولیهٔ process و LB موفق شد؛ شمارش قدیمی idempotent نیز اصلاح شد.
اجرای نهایی همهٔ چهار تست روی VPP خصوصی با race موفق شد.

Branch `codex/closeout-agent-fixtures`; base/source SHA `6645443499d111d7cbbaa8ed2d5f6bd9105544af`.
Local acceptance checkpoint: `ad2ac6195` (fixtures and evidence); later status-only corrections via `git log -1`.
Remote publication: blocked HTTP403 observed by manager; no remote SHA or successful publication claimed.

Owned files are the two agent integration fixture files and `closeout-agent-fixtures*` status/evidence.
Product code is unchanged. Canonical Retrieve includes actual normalized system identity (hostname, UTC, banner, DNS defaultVRF) and empty DHCP/QoS service domains. Idempotent count is exactly13 (previous12 plus system.identity). TestAgentOnHost registers cleanup of its latest agent and second owner so a failed assertion cannot leave workers using removed temporary directories. LBOnHost clears its global store after Stop; the direct GC fixture installs its own persisted BootStore and resets it on cleanup.

First run: process-agent restart PASS, LBOnHost PASS, LB GC PASS after required65-second delay; AgentOnHost reached canonical convergence but exposed stale unchanged count12 vs current13. Corrected that exact count and reran.

Final run: exit0, no skips, race enabled; TestAgentOnHost PASS8.52s, TestAgentProcessOnHost PASS9.95s, TestLbOnHost PASS2.13s, TestLbGarbageCollectOnHost PASS65.11s; package PASS87.133s. Evidence: `closeout-agent-fixtures-evidence/host-tests.jsonl`. DisposableVPP PID3594262 stopped. SharedVPP still `MainPID=1014,NRestarts=0`. `tools/ci.sh check --base 664544349` passed10s, including gitleaks and slot/forbidden checks; log `closeout-agent-fixtures-evidence/check.log`. Complete unchanged quick gate and independent review remain manager-owned.

Next exact command:

```bash
NGFW_SLOT=9 NGFW_TEST_PREFIX=w9 NGFW_VPP_TABLE_BASE=9000 NGFW_INTEGRATION=1 NGFW_LB_HOST=1 NGFW_LB_GLOBALS=1 tools/heavy.sh python3 test/topology/hardware-smoke/isolated-vpp.py go -C apps/agent test -json -race -count=1 -timeout 8m -run '^(TestAgentOnHost|TestAgentProcessOnHost|TestLbOnHost|TestLbGarbageCollectOnHost)$' ./internal/agent
```

Independent root review (read-only): `0f8f40429` adds generated setup/OSPF schema YANG and adds `packages/yang/generated` to the shared generation gate. `@ngfw/yang` is already part of pnpm generation. Checked files have no uncommitted changes; no blocker found. Root runner had a source-provenance issue: it records current gitHEAD while executing any existing `.scratch/agent-host.test`. Manager added build-provenance sidecar, binarySHA validation and frozen sourceSHA; re-review confirms those changes address the original mismatch. Suggested additionally rejecting dirty tracked agent source at provenance capture/run, since HEAD alone cannot detect uncommitted source. No reviewer root edits made.
