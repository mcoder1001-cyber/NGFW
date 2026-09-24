# DF-1 — WIP log

## 2026-09-24 — continuation (previous worker stalled after 51073b1)
State found: 4 commits on task/DF-1 (models, iface layer, 20 descriptors across interface / l2 / bond / memif /
tapv2 / af_packet / l3xc, unit tests on the fake, integration tests green on host). Missing: docs/agent/descriptors/*.md,
DF-1.md, questions file (l3xc.go already points at Q1), restart-simulation evidence, vppctl evidence, CI gate.

- [x] merged main into task/DF-1 (40 commits behind; P09 ci.sh, P03 proto) — unit + integration green after merge
- [x] restart simulation on host: fresh connection + fresh descriptors → Retrieve == desired, Meta equal, empty plan
- [x] registry test: every Register() into one MapRegistry, no duplicate names
- [x] docs/agent/descriptors/{interface,bond,l2,l3xc,memif,tapv2,af_packet}.md
- [x] DF-1-questions.md (Q1 vrf key, Q2 interface key scheme vs DF-2..DF-6 `interface/<name>`)
- [x] vppctl show evidence (VRX_DF1_HOLD)
- [x] tools/ci.sh --base main → CI GATE PASSED; DF-1.md

Closed 2026-09-24: restart simulation found 4 drift bugs + the mtu Delete bug (fixed 85588b5); lint clean;
CI GATE PASSED; report in DF-1.md.
