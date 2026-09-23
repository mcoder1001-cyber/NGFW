# VRX — an open-source TNSR-class secure router platform

> `VRX` is a placeholder codename. Replace it everywhere before starting.
> Alternatives: ParsRouter, NovaEdge, Hyperion, TerraNode, SepehrGate.

## What this is

A complete 0→100 development package for building a product equivalent to
**Netgate TNSR**: a high-performance (10–100+ Gbps) software router / secure
gateway running on COTS x86 hardware, built on VPP + DPDK, managed by a
Node.js control plane with a React + MUI web UI.

## Read in this order

| File | Purpose |
|---|---|
| [docs/00-MASTER-PROMPT.md](docs/00-MASTER-PROMPT.md) | **The main deliverable.** A self-contained prompt/spec to hand to an AI coding agent or a dev team. |
| [docs/01-architecture.md](docs/01-architecture.md) | Process/layer architecture, why Node.js must NOT touch VPP directly, reconciler design |
| [docs/02-oss-stack.md](docs/02-oss-stack.md) | Every open-source component, its role, its license, and the legal rules |
| [docs/03-roadmap.md](docs/03-roadmap.md) | 10 phases, milestones, acceptance criteria, team, timeline, cost |
| [docs/04-api-datamodel.md](docs/04-api-datamodel.md) | Config semantics (candidate/running/commit/rollback), REST conventions, core schema |
| [docs/05-ui-spec.md](docs/05-ui-spec.md) | React + MUI screen inventory, schema-driven forms, RTL/Persian, design rules |
| [docs/06-repo-skeleton.md](docs/06-repo-skeleton.md) | Monorepo layout, tooling, CI, dev environment, build/packaging |
| [docs/07-risks.md](docs/07-risks.md) | Honest list of what will hurt, and how to de-risk it |
| [docs/08-master-schedule-fa.md](docs/08-master-schedule-fa.md) | **زمان‌بندی جامع (فارسی)** — WBS کامل با برآورد نفر-روز برای همهٔ قابلیت‌های TNSR + همهٔ پلاگین‌های VPP 26.06، تیم، قطار انتشار، برنامهٔ سه‌ماهه و اسپرینتی، گیت‌های پذیرش |
| [docs/08-master-schedule-en.md](docs/08-master-schedule-en.md) | **Master schedule (English)** — effort, team, release train, critical path, capacity finding, CapEx, scenarios, gate criteria |
| [wbs/VRX-WBS.xlsx](wbs/VRX-WBS.xlsx) | **Editable WBS workbook** — 102 work items, 620 live formulas, 10 sheets: Legend, Assumptions, WBS, Quarter Plan, Summary, Releases, Sprints, Team, CapEx, Scenarios |
| [wbs/VRX-WBS-jira.csv](wbs/VRX-WBS-jira.csv) | Jira import: 13 epics + 102 stories with estimates, labels and due dates |
| [wbs/VRX-WBS-msproject.csv](wbs/VRX-WBS-msproject.csv) | MS Project / generic CSV: work, duration, start, finish, resources, dependencies |
| [docs/09-os-packages.md](docs/09-os-packages.md) | **بسته‌های سیستم‌عامل (فارسی)** — سه پروفایل Runtime/Build/Lab، مخازن APT، تنظیم کرنل و hugepage، آنچه نباید نصب شود |
| [scripts/](scripts/) | Runnable installers: `00-add-repos.sh`, `10-install-runtime.sh`, `20-install-build.sh`, `25-build-strongswan-vpp.sh`, `30-tune-dataplane.sh`, `40-install-lab.sh` |
| [docs/10-30-day-agent-plan-fa.md](docs/10-30-day-agent-plan-fa.md) | **برنامهٔ ۳۰ روزهٔ ایجنت‌محور (فارسی)** — چه چیزی شدنی است و چه نه، مدل اجرا با ایجنت‌های موازی، ۱۶ موج، گیت‌های انسانی |
| [prompts/](prompts/) | **Agent prompts** — `00-CONTEXT.md` shared preamble, P01–P14 task prompts, `FEATURE-TEMPLATE.md`, `REVIEW-PROMPT.md`, `INTEGRATOR-PROMPT.md`, filled examples in `features/` |
| [docs/11-compressed-plan-fa.md](docs/11-compressed-plan-fa.md) | **طرح فشردهٔ ۲۱ روزه (فارسی)** — جای طرح ۳۰ روزه؛ VM به‌جای Docker، وضعیت دقیق هر ۱۰۲ قلم (✅/🟡/🔧/⏳/❌)، ترک جدای کد VPP (V1–V6)، سیاست تست فشرده |

## The 60-second summary

- **Data plane:** FD.io VPP + DPDK (Apache-2.0). This is 70% of the product's value and 70% of the risk.
- **Dataplane agent:** Go, using `go.fd.io/govpp` (binary API + stats API) — **not** Node.js.
- **Routing:** FRRouting (BGP/OSPF/IS-IS/RIP/BFD) on the Linux stack, bridged to VPP via the `linux-cp` plugin.
- **IPsec:** strongSwan with the `kernel-vpp`/`socket-vpp` plugins (VPP-SSwan), or VPP's native IKEv2 plugin.
- **Services:** Kea (DHCP), Unbound (DNS), chrony (NTP), net-snmp, keepalived (VRRP).
- **Control plane:** Node.js 22 + NestJS + TypeScript + PostgreSQL + Redis, gRPC to the Go agent.
- **UI:** React 19 + Vite + TypeScript + MUI v7 + TanStack Query + react-hook-form/Zod + i18next (en/fa, RTL).
- **Config model:** copy TNSR's YANG/clixon semantics — candidate datastore, `commit`, `rollback`, confirmed commit — but implement it with JSON Schema + a declarative reconciler instead of clixon.
