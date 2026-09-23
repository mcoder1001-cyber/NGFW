# DF-4 — WIP log

- 15:10 start; read P05a interfaces, binapi acl/acl_types/ip_types/ethernet_types/interface, acl.c (v26.06 tag) for handler semantics.
- 15:35 first commit 9948f7d: package `internal/descriptors/acl` (6 descriptors + stats reader + plugin info), key contract doc.
- 15:43 unit tests green on the fake; first host run: 5/7 subtests green; learned `macip_acl_del` auto-unbinds (VPP) → fake/tests/doc aligned.
- 15:47 host integration rerun: 8/8 subtests PASS, flag restored, no leftovers. 15:50 ci.sh --base main: CI GATE PASSED (40ee367). Status report written.
