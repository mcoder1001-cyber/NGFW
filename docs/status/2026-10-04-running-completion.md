# Running tasks — completion campaign, 2026-10-04

تونل‌ها و IS-IS/RIPng تکمیل و در PR151 مرج شدند.
دو ریویوی مستقل سورس را تأیید کردند؛ تست‌های متمرکز و typecheck موفق بودند.
رویدادهای native IPsec و کانال رمز IS-IS هم تکمیل شدند.
HA و Multi-WAN در شاخه‌های جدا در حال تکمیل واقعی هستند.
P10 و IPsec گواهی‌محور همچنان تصمیم امنیتی می‌خواهند؛ آزمایشگاه به‌عنوان PASS ثبت نشد.

PR151 merged4508d38f1, verified main tree equals reviewedf9a9cb37e28d42be90918891910c57df7470ca90.
Source counts173/211 merged,6 running,7 ready,15 todo,10 parked,0 review.
TD19 teardown fixture corrected after current helper change; strict36/36 pass,
packaging30/30 pass, FRR pin suite13/13 and Go module6/6 pass.
Exact independent review artifacts are in docs/status/tasks/complete-running-independent-review-20261004.md
and running-security-review-20261004.md. Full CI inherited waiver remains documented;
no complete-suite/lab pass claimed. Repository current guard passes.

P10: isolated product VPP build in Packaging-complete (no host installation),
privilege/atomic-global-file ownership decision and license authority unresolved.
TD19: current fourth FRR signer and NodeSource key authority not inferred from
self-downloaded keys. Official source evidence and refused defaults retained.
Native certificates: local certificate/remoteCa cannot implement VPP peer-leaf
signature trust; explicit peer-pin + global signing-key model awaits owner answer.

## Final source merge reconciliation

- PR151: tunnels + IS-IS/RIPng + native SA events, main4508d38f1.
- PR152: completed-row reconciliation and TD19 fixture, mainc9207eb8b.
- PR153: rebuild API/web before package staging, main2fd82320a.
- PR154: owned WAN routes/NAT/PBR, main9994929bb; independent approval and integrated focused tests/build pass.
- PR155: HA live roles/events and atomic authenticated peer sync, mainad6749b86; independent approval, API50/UI6 tests, Go race, dependency build14/14 and final typecheck/source guard pass.

Board: merged175, running0, ready7, todo15, parked14, review0 (211 total). Four original running rows are now source-complete. Four are parked on unanswered owner decisions: P11/F-ikev2-native certificate trust and shared local key; P10 file-writing privileges and authoritative product license; TD19 initial repository trust. Exact alternatives in docs/decisions/PENDING-native-ipsec-certificate-trust.md, PENDING-P10-agent-file-ownership.md, PENDING-P10-product-license.md and PENDING-TD19-repository-trust.md. Decision policy always-PENDING boundaries apply; no response is assumed from elapsed time. P11 title/scope reflects the already-authorized route-based scope instead of the superseded strongSwan build.

Isolated artifact progress: observed builder PID3349754, log /tmp/ngfw-packaging-vpp-build-active-20261004.log in Packaging-complete, after pinned dependencies completed and VPP source compilation began. No product deb manifest was produced at this checkpoint; no installation or persistent-runner guarantee. Exact next read-only check: ps -p3349754 -o pid,etime,args and tail -30 of that log, then deploy/vpp/verify.sh --require-files on actual output if produced. All worker source changes/reviews have durable remote checkpoints; worker agents have finished (packaging reviewer later hit model capacity, its observed build process remained live).

Full hosted CI remains waived under the prior owner instruction; no hosted green result claimed. Lab-only checks listed in DEFERRED-ACCEPTANCE.md remain NOTRUN. Shared system VPP/host privileges/trust were not modified.
