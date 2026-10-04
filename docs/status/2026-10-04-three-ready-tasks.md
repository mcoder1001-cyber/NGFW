# Three selected ready tasks — source integration, 2026-10-04

سه تسک انتخاب‌شده از صف آماده، پیاده‌سازی و مستقل ریویو شدند.
مرج هر سه تسک در گیت‌هاب مستقلاً تأیید شد؛ جدول زیر شناسه‌های دقیق را ثبت می‌کند.
درصد پیشرفت فقط تکمیل منبع ثبت‌شده است؛ پذیرش واقعی سخت‌افزار تأیید نشده است.
CI تجمیعی با دستور صریح مالک به تعویق افتاده و موفق اعلام نشده است.
آزمون‌های زنده و محدودیت‌های باقی‌مانده در برنامه پذیرش معوق ثبت شده‌اند.

| Task | Source integration | Verification and limits |
|---|---|---|
| F-capture-trace-host | PR137, merge `5bad63a4b2df303a57e35f909044ac989cfc9c51` | Independent source APPROVE; runner9 PASS; focused Go/API typecheck PASS; hosted quick run37148857490 SUCCESS on Capture head870a6e0. This does not certify the later combined main tree. New host test explicitly skips without dedicated socket. Actual API e2e, VPP packets/recovery, BPF and T4 screenshot NOT RUN. |
| F-igmp-mfib-host | PR138, merge `1a3d2ba47c4a93fa6fb4cdb269bd74daa3146515` | Independent source APPROVE; rebased six-package race and security/check gates PASS. Cold-start ownership regression independently PASS with race. Real VPP/IGMP packet/restart acceptance NOT RUN; FRR PIM sync remains F-pim-frrsync and BIER is not built. |
| F-bruteforce-block-host | PR139, merge `4ead8bd2ae2eb7881f311cc71a92218c129f15d7` | Independent exact integration-tree APPROVE; focused Go race, API23, typecheck and security/source checks PASS. Real VPP/nftables packet enforcement, detector and restart-loss acceptance NOT RUN. |

**Source progress: 74.2% by estimated hours (1170.0/1577.5 h), 74.4% by tasks (157/211).**

Board totals: merged157, review8, running13, ready10, parked8, failed0, todo15. Integrated product base: `4ead8bd2ae2eb7881f311cc71a92218c129f15d7`. Counts are source states, not proof of a live agent or completed hardware acceptance.

## Authorization and aggregate CI

The owner explicitly authorized publication to the existing public `mcoder1001-cyber/NGFW` repository and sequential merging of these three implementations while deferring aggregate CI. This narrow instruction supersedes waiting for the aggregate gate for these merges; it changes no workflow file or security/host privilege rule. Prior green quick results for earlier main trees do not certify these integration trees. Local complete quick attempts encountered Unix-socket permission failures and were not green.

Deferred cases are listed in [DEFERRED-ACCEPTANCE.md](DEFERRED-ACCEPTANCE.md). Re-run the unchanged complete aggregate gate on exact integrated main in a suitable environment, then real target acceptance with existing ownership, lock and explicit IGMP/dedicated-capture opt-in safeguards.

PR136's stale ready-to-running assignment proposal was not integrated. This final reconciliation changes exactly the three selected source rows to merged after observing their actual merges; it is not three additional product implementations. No persistent agent service or future continuous execution is claimed.
