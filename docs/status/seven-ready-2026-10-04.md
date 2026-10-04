# Seven ready tasks — 2026-10-04

درخواست مالک: تکمیل، مرج و Done کردن هفت تسک اولیه، هر کدام روی شاخهٔ جدا.
کد هر هفت تسک تکمیل و مستقل بازبینی شده؛ هر هفت تسک اکنون مرج شده‌اند.
تمام هفت نسخهٔ مستقل گیت کامل hosted quick را گذرانده‌اند.
مرج‌ها ترتیبی است؛ هر ترکیب جدید محصول باید گیت کامل را بگذراند.
پذیرش واقعی دستگاه/مرورگر/شبکه اجرا نشده و در کمپین مرکزی باقی است.

**Overall: 88.1% by hours (1390.0/1577.5 h), 87.7% by tasks (185/211)**

This report covers only the seven initially-ready rows. Auto-readied dependents
are separate work. Done means reviewed source completion, not deployed or live
laboratory acceptance. See [central deferred acceptance](DEFERRED-ACCEPTANCE.md).

| Initial task | Published task branch | PR | Current source/gate evidence | Merge / board |
|---|---|---|---|---|
| F-dataplane-apply-flow | codex/ready-dataplane-apply-20261004 | [162](https://github.com/mcoder1001-cyber/NGFW/pull/162) | c434481c; complete quick [37217602615](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37217602615) PASS | f8fcd6c2; Done board f6386e4b |
| F-igp-followups | codex/igp-followups-20261004 | [164](https://github.com/mcoder1001-cyber/NGFW/pull/164) | 14771daa; final complete quick [37220789126](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37220789126) PASS | 8c537ce2; Done board588eaafe |
| F-multiwan-wiring | codex/F-multiwan-wiring-20261004 | [159](https://github.com/mcoder1001-cyber/NGFW/pull/159) | 2dd48e0a cumulative quick37222055557 PASS; standalone7f56e543 quick37219016499 PASS | bee8ed4b; Done boarde372cf6c |
| F-pppoe-client-wiring | codex/F-pppoe-client-wiring-20261004 | [165](https://github.com/mcoder1001-cyber/NGFW/pull/165) | 38cef0cb cumulative quick37223559490 PASS; d33edd3c standalone quick37219021909 PASS | 12472a11; Done board50973b20 |
| F-management-ui-host | codex/ready-management-ui-host-20261004 | [163](https://github.com/mcoder1001-cyber/NGFW/pull/163) | fc6d896c cumulative quick37224837911 PASS; 375f85b0 standalone quick37219065597 PASS | d1aba878; Done board49a347fb |
| F-dataplane-ui-host | codex/ready-dataplane-ui-host-20261004 | [160](https://github.com/mcoder1001-cyber/NGFW/pull/160) | 4eaaa17a cumulative quick37226354347 PASS; a990ad73 standalone quick37219069928 PASS | 4069b365; Done board4aa71108 |
| TEST-traffic-A | codex/ready-traffic-a-composed-20261004 | [166](https://github.com/mcoder1001-cyber/NGFW/pull/166) | ab4ef8d5 cumulative quick37227941852 PASS; e20afc2d standalone quick37219072067 PASS | 1f094f63; Done in final seven-task board closeout |

Each developer retained local and published archive refs before final D112
single-commit integration. Final branches are published; local commits alone
are not recovery evidence. Product branches remain frozen during their gates.
Root performs expected-head merges, developers do not merge their own work.

Historical full-main Gitleaks false positives were two SHA256 checksums of public
FRR/NodeSource repository signing keys in2fd82320. Independent reviewers verified
both public artifacts. [PR167](https://github.com/mcoder1001-cyber/NGFW/pull/167)
added exactly two fingerprint exceptions, retained existing exceptions, and
changed neither scanner rules nor depth. Full redacted history scan346commits
found zero remaining leaks. Complete quick37219414200 PASS; merge38fa5d4e;
postmerge main quick37220694454 PASS. This maintenance PR is not an eighth task.

Recovery: current product main1f094f63; postmerge main CI gate37229296181 PASS.
Dataplane postmerge main37227875343 PASS.
Management postmerge main37226289859 PASS.
PPP corrected postmerge main37224995962 PASS.
WAN postmerge main37223476801 PASS.
IGP postmerge main37221999334 PASS.
Final seven-task board/status checkpoint branch is codex/board-seven-ready-closeout-20261004;
earlier queue history is retained on codex/seven-ready-integration-20261004.
Final reviewed board closeout records all seven initial rows as merged/Done.
Board totals: 185 merged, 7 ready, 8 todo, 11 parked, 0 running. The seven ready
rows are newly unlocked dependents outside this request. The closing metadata
commit receives its own unchanged hosted main CI gate after publication.
No live VPP restart, production apply, PPPoE dial or forwarding campaign was run.

All 19 cumulative IGP and PPPoE delivery tests passed after strictly
sequential proto and schema builds (15.07s). Initial local failures were the RIP
positive/negative cases because pre-IGP proto dist dropped the new RIP auth leaf;
OSPF rejection was already correct. No product or assertion was changed to make
them pass. Build/test logs: `/root/ngfw-wt/pppoe-tmp/proto-integrated-build.log`,
`schema-integrated-final-build.log`, `api-integrated-igp-built.log`.

Historical scanner correction: the natural-language nineteen-test summary in
board/status commit e372cf6c was concatenated without spaces and flagged as a
credential. That text contains no credential. Its exact historical fingerprint
is excepted; the wording is corrected and scan rules/depth remain unchanged.

The natural-language finding was independently approved by two reviewers; an
unchanged full-history redacted scan requested the latest 500 commits and
actually scanned 345 commits / 43.99 MB with zero findings. Correction c079a7d9
changes only the report and one exact historical ignore entry. Failed main
runs37224613173 and37224778242 were the same prose false positive; current main
passed complete quick37224995962 before PPP board closeout publication.
