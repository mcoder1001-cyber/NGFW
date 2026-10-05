# TEST-traffic-C — runnable source, acceptance pending

سناریوی موج C و مسیرهای اجرای محصول پیاده‌سازی شده‌اند.
تغییر تنظیمات، تحلیل بسته و پاک‌سازی خطاپذیر پوشش داده شده‌اند.
۱۳ آزمون مستقل از آزمایشگاه موفق بوده‌اند؛ dry-run موفق است.
اجرای واقعی VPP و داور مستقل هنوز تأیید نشده‌اند.
پیش‌شرط table0 در فایل پرسش‌ها صریح است؛ وضعیت Done ادعا نمی‌شود.

Board snapshot at base: 91.2% by hours (1438.0/1577.5 h), 91.0% by tasks
(192/211). This source checkpoint does not change the board counts.

## Implementation

`test/topology/traffic-c/execute.py` uses the proven Wave-A command/process/API
primitives with its own strict C manager lease and C product transaction. It
implements MPLS-labelled requests, SRv6 SRH/inner tuple, VRRP commit failover with
one VPP and an owned keepalived namespace, three-colour policer counters and
real sent/captured/received UDP accounting, plus live IPFIX, product capture,
IGMP/mFIB and owned metrics riders. Each stage rolls back its API configuration;
exact binary-API globals restoration, NRestarts/PID identity and final residue
are mandatory. TD-H18 invokes the existing real host test with explicit globals
opt-in, rejects SKIP, and requires restore evidence. No VPP trace/restart, module
load, shared daemon restart, secret output or foreign-object deletion.

The combined global-owner agent cannot adopt existing foreign table0. This
executor refuses preexisting table0 before mutations. A manager-provisioned
private/otherwise idle VPP with absent table0 can execute the full source path;
shared existing-table0 acceptance remains constrained, see questions.

## Actual source checks, 2026-10-05

```
python3 -m unittest discover -s test/topology/traffic-c -v
Ran 13 tests in 0.006s
OK
python3 -m py_compile test/topology/traffic-c/execute.py test/topology/traffic-c/peer.py
(exit 0)
python3 test/topology/traffic-c/execute.py --slot 14 --dry-run
"mode": "DRY_RUN_ONLY"
"live_acceptance": false
"executor": "execute.py"
tools/heavy.sh go -C test/topology/traffic-c/globals vet ./...
(exit 0; after waiting for heavy slot)
tools/ci.sh check --base main
check PASSED (0m22s) [initial foundation checkpoint]
```

These checks use synthetic parser fixtures and mocked API transport. No real
packets, VPP changes, globals or services were exercised. Globals Go unit checks,
current full unchanged quick gate, independent review and live manager-window
acceptance remain pending; no invented host result is included. Runtime evidence
belongs in a private `TEST-traffic-C-evidence` export as text; never commit pcap.
