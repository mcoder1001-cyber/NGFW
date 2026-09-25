# REVIEW-2026-09-24: verdict per item and the row that carries it

The product owner's full-code review of 2026-09-24 has 66 items. Each item was re-verified on main (16b622a; 35f3a96 changes none of these files) and on the branch tips, and was then either planned or rejected (D-125). The board rows are in `plan/tasks.yaml`. The review plan's TD-12/13/14 became **TD-17/18/19**, because board TD-8 (agent seams) and TD-12 (test de-flake) were already taken.

## خلاصهٔ فارسی
- **نتیجهٔ بررسی ۶۶ قلم گزارش:**
  - ۴۵ قلم درست است؛
  - ۱۹ قلم تا حدی درست است (بخشی از آن‌ها قبلاً اصلاح شده یا عمداً چنین طراحی شده)؛
  - ۱ قلم نادرست است (4.2، اسکلت nftables)؛
  - ۱ قلم روی شاخهٔ P08 اصلاح شده است (3.1).
- **کجا رفتند:** همهٔ قلم‌های درست یا نیمه‌درست یا به ردیف‌های جدید بورد رفتند، یا به دامنهٔ ردیف‌های موجود، یا به پرونده‌های PENDING برای تصمیم شما.
- **ردیف‌های جدید:** TD-9، TD-10a/b، TD-11a/b/c، TD-13، TD-15، TD-16، TD-17، TD-18، TD-19، SEC-auth، UI-domain-editor، TEST-traffic-A/B/C و LAB-vpp-per-slot.
- **منتظر تأیید شما:** دو قاعدهٔ تازه در docs/11 بخش ۵ (قاعدهٔ ترافیک هر موج و تعریف تازهٔ «انجام‌شده») و حذف Ansible از D8.9.

Verdicts: **correct** = the report is right; **partially** = the facts are right but part is already fixed, by design, or wrongly inferred (see the note); **wrong** = not a defect; **fixed-on-branch** = fixed on a branch that has not merged yet.

## 1. Agent core
| ref | item | verdict | carried by | note |
|---|---|---|---|---|
| 1.1 | govpp DefaultReplyTimeout=0; resync/revert/rollback have no deadline | correct | TD-9 | also P08's unbounded ControlPing (agent.go:229) |
| 1.1b | no resync retry, no periodic drift check | correct | TD-9 | N2 backoff + Plan-only drift gauge |
| 1.1c | link-event watcher never restarted | correct | TD-9 | |
| 1.1d | no gRPC panic-recovery interceptor | correct | TD-9 | lands together with 1.1e |
| 1.1e | scheduler does not recover a descriptor panic | correct | TD-9 | ErrDescriptorPanic |
| 1.2 | Apply answers APPLIED for skipped domains, drops warnings | partially | TD-9 | the API already compensates with DryRun warnings + notApplied; the gap is only at agent level |
| 1.3 | confirm-and-apply confirms before checking that VPP is connected | correct | TD-9 | |
| 1.4 | cancelled caller ctx → ROLLED_BACK cached under the txn_id | correct | TD-9 | D-entry amends proto.md §2 item 3 |
| 1.5a | ownertable.WriteAtomic: no directory fsync | correct | TD-9 | shared dir-fsync helper; TD-16 reuses it |
| 1.5b | confirm deadline from txn start, not applied_at | correct | TD-9 | |
| 1.5c | invalid VRX_LOG_LEVEL silently ignored | correct | TD-9 | |
| 1.5d | mpls checkPaths / l3xc update: no 255-path bound | correct | F-bridge-l2 (l3xc), F-mpls-srmpls (mpls) | envelope amendments |
| 1.5e | metrics :9101 unauthenticated | partially | TD-9 | defaults to 127.0.0.1 and exposes no secrets; TD-9 refuses a non-loopback address unless opted in |

## 2. API
| ref | item | verdict | carried by | note |
|---|---|---|---|---|
| 2.1 | checkPending treats "no longer pending" as reverted | correct | TD-10a | read Health.last_txn_id |
| 2.2 | rollback restoreSecrets not persisted in config_pending | correct | TD-10a | migration 0004 |
| 2.3a | lockout can lock the last admin; no break-glass | correct | TD-10b | (user, IP) key; last admin throttled only; root-only unlock tool |
| 2.3b | per-IP limit uses req.ip without trustProxy | correct | TD-10b | VRX_TRUST_PROXY |
| 2.3c | access tokens not revoked on logout, demotion, deletion | partially (part already fixed) | TD-10b + PENDING-session-revocation | disable is fixed on TD-4 (c299859); logout, demotion and deletion are still open |
| 2.3d | refresh sessions never hard-expire | correct | TD-10b | VRX_SESSION_MAX_SEC |
| 2.3e | audit gaps (logout/refresh, 401 mutations, swallowed write failures) | correct | TD-10b | |
| 2.3f | secret deletion race vs commits | correct | TD-10a | inside commits.exclusive |
| 2.3g | /api/docs unusable in a browser | correct | TD-10b | option (a) or (b) via LOG |
| 2.4a | web/CLI apply deadline shorter than the server's worst case | correct | TD-10a | |
| 2.4b | commit mutex in-process only | correct | TD-10a | advisory lock (D-111 follow-up) |
| 2.5 | surface agent warnings in the commit result | partially (part already fixed) | TD-10a (API part); agent/proto part → tech-debt | warnings already reach every CommitResult, the web UI and the CLI; gaps: confirm(), the 422 extras, the agent proto field |

The report's §2 line numbers predate the TD-2 merge; the substance was re-verified on main.

## 3. Reachability and claim safety
| ref | item | verdict | carried by | note |
|---|---|---|---|---|
| 3 | only core.Register is wired | partially | TD-11a | 78 of 79 Register funcs are unreachable (not 54); interfaces are fixed on P08; the other families are assigned to their feature rows by design (A1 anchors, D-109) |
| 3.1 | `interface/<name>` alias not wired | fixed-on-branch | P08 (a86d3f2, also in W-seed); doc fix in TD-11a | the open remainder is 3.1b |
| 3.1b | interface-ip on an untagged (DPDK) NIC fails at Create | correct | TD-11c | gates the first DPDK run |
| 3.1c | alias never a node in a delete plan | partially | TD-11c | the sub-interface part merges with F-vlan-qinq |
| 3.2 | claim/boot stores default to in-memory | partially (part already fixed) | TD-11b (guard), TD-11c (flush per txn) | P08 closes DF-1/6/7/8 |
| 3.3 | VPP written first, claim recorded after | correct | TD-11b (generic), F-rpf-adl-pbr (urpf/adl/abf call sites) | |
| 3.3b | wireguard/peer.go returns meta with an error after the add | correct | TD-11b (generic half), F-wireguard (envelope) | |
| 3.4 | ACL binding interface dependency optional | correct | F-acl (envelope), F-rpf-adl-pbr (adl.go:177) | |

## 4. FRR / nftables
| ref | item | verdict | carried by | note |
|---|---|---|---|---|
| 4.1 | RF-1b: renderers/frr has no protocol sections | partially (inference wrong) | P12 + F-ospf, F-isis-rip, F-bfd-redistribution, F-mpls-ldp, F-igmp-mfib | by design: RF-1.md:6-7/:60-61 put protocols out of RF-1; each row owns `frr/<proto>/**`; no RF-1b row |
| 4.2 | no nftables renderer skeleton | wrong | — (F-host-acl-nftables builds it) | F-hardening-lite only consumes it; its stale title was fixed |
| 4.3 | seam S2 frr.RegisterInterfaceLines absent, conditional in P12 | correct | P12 (at spawn: S2 + S3 mandatory) | |
| 4.4 | FRR renderer not invoked; warning claims RF-1 renders | correct | P12 | warning text change |
| 4.5 | derived community-list / as-path lines missing | correct | P12 | |
| 4.6 | frr docs list F-mpls-srmpls, omit F-mpls-ldp | correct | P12 (README.md + section.go:20) | |

## 5. Repo, install, CLI
| ref | item | verdict | carried by | note |
|---|---|---|---|---|
| 5.1a | remote mirror incomplete | partially | PENDING-remote-mirror | only the mirror is behind; the repo on main is complete |
| 5.1b | root .gitignore coverage | partially (part already fixed) | TD-18 | node_modules, dist, deploy/vpp/.build and *.deb are already ignored; .venv/, *.tfstate*, .terraform/, /.scratch/ are added |
| 5.2 | stray root files index.html, fail.out | partially | manager (tech-debt: fail.out) | never committed, hidden by .git/info/exclude; index.html belongs to the product owner (D-098) |
| 5.3 | no CI workflow on the mirror | partially | PENDING-remote-mirror; TD-18 (.gitlab-ci.yml only if GitLab) | the workflow exists on main (c09583c) and was never pushed; CI is local by design |
| 5.4a | tools/manager/merge.sh unsafe | correct | TD-18 | before the first wave-A merge |
| 5.4b | tools/operator/wt.sh unquoted args | correct | TD-18 | wrong line numbers: wt.sh has 21 lines, code at :14-19 |
| 5.5a | install scripts pull VPP from FD.io | correct | P10 (scripts/10, docs/09), TD-19 (FD.io part of scripts/00) | |
| 5.5b | curl\|bash as root, unpinned installers | correct | TD-19 | |
| 5.6 | tools/lab provision uses hard-coded debs | correct | TD-19 | |
| 5.7a | CLI ping/traceroute drop `<host>` | correct | F-vrf-static-ecmp (merge acceptance) | |
| 5.7b | CLI follows redirects with Authorization | partially | TD-10a | Go 1.26 already strips Authorization on a cross-host redirect; same-host downgrade and 307/308 body replay remain |
| 5.7c | pg-test.sh password on the command line | correct | TD-18 | |

## 6. Plan
| ref | item | verdict | carried by | note |
|---|---|---|---|---|
| 6.1a | new rows TD-9/10/11 | correct | TD-9, TD-10a, TD-10b, TD-11a, TD-11b, TD-11c | split by file ownership |
| 6.1b | every wave-A task depends on the agent-core and reachability rows | partially | "merge after TD-11a" on the 12 wave-A rows (row notes); TD-9 is a release gate (P10, INTEGRATE-E2E, SECURITY-REVIEW) | no start deps on wave A |
| 6.2 | UI-domain-editor | correct | UI-domain-editor | merges after ui-nav-collapse |
| 6.3a | F-ansible: Ansible collection cut | partially | board retitle of F-sdk-terraform-ansible; docs/11 D8.9 → 🟡; have-not register (tech-debt) | deliberately cut by D-085: a have-not, not missing work |
| 6.3b | missing API pieces answer 501 | partially | F-dashboard-prom-alarms (clear-counters), F-backup-restore (reboot/shutdown), P10 (vrx-power@), P12 (bgp/neighbors) | user CRUD lives in management.users by design; most 501s already had F-* owners |
| 6.4 | P10 underestimated | correct | P10 (10 → 20 h, headers), TD-17 | |
| 6.5 | LAB-vpp-per-slot | correct | LAB-vpp-per-slot (parked on PENDING-vpp-host-hardening) | |
| 6.6a | TEST-traffic per wave | correct | TEST-traffic-A/B/C; docs/12 S4 gate, docs/11 §5 | the docs/11 line waits for the product owner's confirmation |
| 6.6b | DoD: reachable through API and UI | partially | docs/12, docs/11 §5; TD-11a helper; prompt edits by the manager (tech-debt) | |
| 6.7 | move the auth part of SECURITY-REVIEW forward | correct | SEC-auth | |
| 6.8 | tech-debt entries | partially | docs/tech-debt.md (owner + due-before, D-125) | several listed items were already done |

## 7. Decisions for the product owner
| ref | item | verdict | carried by | note |
|---|---|---|---|---|
| 7.1 | PENDING-secret-channel content | correct | PENDING-secret-channel (refreshed) | socket wording fixed, no-echo rule added |
| 7.2 | PENDING-vpp-host-hardening content | correct | PENDING-vpp-host-hardening (refreshed) | option D moved out, option F added |
| 7.3 | new PENDING-agent-privileges | correct | PENDING-agent-privileges | |
| 7.4 | new PENDING-vpp-c-track | partially | PENDING-vpp-c-track | "6 upstream bugs" is really 6 crashes from 3 bugs; the pipeline exists, its product series is empty |

## Same day: architecture audit, where each finding went
| finding | carried by |
|---|---|
| A1 (wave-A branches lack the TD-5/D-113 ring fixes) | fixed by the W-seed re-merge (D-122) |
| A2 + A8 (RoutingConfig.l2 = 20; D-120 missing from LOG) | D-122, D-120 |
| ARCH-01 (failed save answers APPLIED; the API never checks last_txn_id) | TD-9 (agent half), TD-10a (API half) |
| ARCH-02 (no tier-3 validation before apply) | TD-13 |
| A3 + A4 (TD-8 seams, SlotIDRange fails open) | TD-8 (D-123) |
| ARCH-03 + A7 (SDK/generate checks missing from ci.sh) | manager (tech-debt) |
| ARCH-04, ARCH-11, ARCH-05 (API hygiene) | TD-15 |
| ARCH-06 (descriptors/kit, fsync, ParsePrefix) | TD-16 |
| ARCH-07, ARCH-09, ARCH-13, ARCH-14 (LOW) | tech-debt (ARCH-07 with PENDING-secret-channel) |
| ARCH-08, ARCH-10 | WEB-1 / WEB-2 (D-123) |
| always-PENDING: TD-4 Q2, TD-4 Q3, PLAN-1 | PENDING-tools-app-transport, PENDING-session-revocation, PENDING-system-identity |
| refuted | ARCH-12, ARCH-15, A5 |
