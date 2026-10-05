# TEST-traffic-C — incomplete driver checkpoint

سناریوی موج C شروع شده است؛ تکمیل پذیرش ادعا نمی‌شود.
بخش تراکنش API و تحلیل شواهد بسته پیاده‌سازی شده است.
۹ آزمون مستقل از آزمایشگاه موفق بوده‌اند.
هیچ تغییری در VPP مشترک یا سرویس‌های مشترک ایجاد نشده است.
اجرای واقعی، fixture و جمع‌آوری rider هنوز نیاز به کدنویسی دارند.

Board snapshot at base: 91.2% by hours (1438.0/1577.5 h), 91.0% by tasks
(192/211). This driver does not change the board counts.

## Implemented

- Assigned-slot local product API client; no direct VPP writes.
- Strict applied receipt checks, `notApplied: []`, unsupported-field rejection.
- Clean candidate requirement; committed baseline rollback in `finally`, including
  an independent rollback attempt if candidate discard fails.
- MPLS/SRH parser checks correlate header and owned inner tuple in one line.
- VRRP timestamp-gap check includes interval boundaries and rejects duplicate replies.
- Three-colour QoS counters and WAN/sent count checks; no rate/performance claims.
- Dry-run explicitly reports remaining code and `live_acceptance: false`.

## Actual checks, 2026-10-05

```
python3 -m unittest discover -s test/topology/traffic-c -v
Ran 9 tests in 0.005s
OK
python3 test/topology/traffic-c/driver.py --slot 14 --dry-run
"mode": "DRY_RUN_ONLY"
"live_acceptance": false
python3 test/topology/traffic-c/driver.py --slot 14
live orchestration incomplete: no host mutation is authorized by this driver
(exit 1; expected refusal)
```

Parser text is synthetic unit fixture data, not real packet evidence. Live VPP,
NRestarts, feature counters, keepalived, globals and cleanup evidence: NOT RUN.

## Remaining code and acceptance

Owned rig/slot-stack bootstrap and cleanup, manager-window authority, global
before/after restoration, real tcpdump and traffic collection, VRRP product
commit disable/enable plus keepalived fixture, QoS live state collection,
IPFIX/capture/IGMP/metrics riders, residue inspection and TD-H18 host proof.
Only after that code exists may the actual quiet-window run be deferred as
laboratory acceptance. One VPP plus keepalived peer; no two-VPP HA claim.

`tools/ci-slot.sh` is absent at this base; use the current unchanged
`tools/ci.sh --base main` quick gate. Full gate is not yet recorded here.
