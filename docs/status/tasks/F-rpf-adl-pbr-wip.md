# F-rpf-adl-pbr — WIP log

Slot 10 (`w10`, tables/ABF ids 10000–10999), branch `task/F-rpf-adl-pbr` on `task/W-seed` (speculative, D-114/D-120:
main is not merged until the manager says P08 has landed; `task/W-seed@df67a8e` merged 18:09 as instructed).

## 2026-09-24 17:30 — start
- [x] read 00-CONTEXT, shared-host-rules, FEATURE-TEMPLATE, prompt, wave-A-hotspots, envelope
- [x] contract: schema ext + key lines, semantic rules, proto mirror (Interface 18/19, RoutingConfig 11, ServicesConfig 8), regen (b7439d4, 63be7be)
- [x] agent: V23 (a) in adl.interface Retrieve; adl.allowlist safe sequences + applied-once record; descriptors/auto_sdl;
      PBR name records; builder + assembler; registration; ACL bridge; unit tests (fake models duplicate adds)
- [x] host check (w10 loopbacks, flock -s lab lock): apply, Retrieve == desired, vppctl, restart simulation, rollback; NRestarts 0 → 0
- [x] API: feature module, `GET /api/v1/state/pbr`, OpenAPI, regenerated client + CLI table; e2e
- [x] UI: PBR screen, ADL / Auto-SDL screen, drawer group strings, en + fa; screenshots against the real stack
- [x] docs: docs/user/routing/rpf-adl-pbr.md, descriptor docs, V23 row + V-new
- [x] questions file (Q1–Q9, incl. the 18:41 crash: not slot 10)
- [x] CI `TMPDIR=/tmp/g-w10 tools/ci.sh --base main` → CI GATE PASSED at 456fd40 (this tree's ci.sh fails the contract
      guard on SIGPIPE, fixed on main 7edac8c / D-127; the green run used a scratch copy with exactly that fix)
- [x] F-rpf-adl-pbr.md with pasted output; cleanup (processes by PID, vrx_w10 dropped, nothing of w10 in VPP, dist removed)
