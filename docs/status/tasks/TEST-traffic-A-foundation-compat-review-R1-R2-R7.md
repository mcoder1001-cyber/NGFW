# TEST-traffic-A foundation gate compatibility — R1 / R2 / R7

2026-10-02. **APPROVE R1/R2/R7** exact CI patch
`271e65a7ed1e89caee84d5d0127941037b08c847`. Own isolated worktree/branch
NGFW-traffic-foundation-compat-review / task/traffic-foundation-compat-review.
Only this report authored. No product source or arbiter authorship. No findings.

R1: independently extracted fda0ddc7 AST test methods from Foundation and Evidence:
ordered list equals ORIGINAL_NAMES exactly, nine plus seven =16, unique. No
original test is excluded. Loader requires16 unique names and16 loaded cases;
completed result requires16 and accepted() rejects all failure/error/skip/xfail/
xpass outcomes. Missing method becomes real loader error even with count16.
Expanded source must additionally pass full correlation33 (and producer40 on
that later composition); original16 alone never certifies expanded functionality.
The new patch removes the actual old module-count incompatibility without
weakening outcome guards or introducing skips.

R2: workflow pinned checkout, contents:read, persist-credentials:false, Ubuntu24
and five-minute timeout unchanged. Only original-case policy and source fixture
Python run; no credentials, package installation, host/network operation or
privilege introduced. Names are fixed repository constants, not external input.
R7: workflow label accurately says original named foundation cases. All traffic
source is byte-identical1427; only CI selection changes. All original regression
coverage remains while separate required full-module gates cover additions.
No new transaction/producer code, packet provenance or whole-task approval is
included. Final current-main composition and exact-head complete hosted quick
with all applicable phase gates remain mandatory before expected-head merge.

Actual independent execution:
- Original gate policy:11 PASS, including genuinely executed failure/error/skip/
 xfail/xpass/zero/reduced and missing loader controls.
- Original16 runner:16 PASS0.403s, all nonpassing outcome counts zero.
- Full correlation runner:33 PASS2.470s, all nonpassing counts zero.
- Extracted original fda AST ordered method identity16: PASS.
- Additional actual16 suite with exactly one original method missing: testsRun16,
 errors1, complete() rejects; control assertion PASS (expected negative result).
- Frozen1427 traffic source diff:empty; whitespace check clean.

No redundant producer40 run since producer source is absent in this freeze.
Later producer integration must run its full40; this patch does not waive it.
Hosted CI not run/queried here. No live SSH/network/ip/tcpdump/VPP/nft/lease or
host mutation occurred. Missing live executor/transaction/ownership work remains
source work; actual lab acceptance remains NOT RUN, not implied fixture PASS.
