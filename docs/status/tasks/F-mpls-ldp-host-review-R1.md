# R1 correctness and tests

Reviewed final branch head: `8ddc3fd0bb9114cc8ed20bdd6ed8a28d83b06dc5`; final product change `cb2560ed` (the difference is review documentation only).

No unresolved product correctness findings. The fixed FRR reader joins selected/active IPv4 RIB nexthops to in-use label bindings and link discovery, validates bounded input and rejects unsupported nonempty/error-shaped JSON. FRR's lazily-created empty array objects are explicitly recognized. Translation is deterministic, deduplicates paths and rejects label/FEC collisions and unsupported label cases. Snapshot hold-down distinguishes read failure from a successful withdrawal. Dynamic desired state uses current configuration identities and filters removed/remapped interfaces and labels outside a changed label range. The scheduler seam owns application, with retry after a failed apply of an unchanged observation. Named descriptor tests prove isolation, static/IP binding collision rejection, restart retrieval and withdrawal.

Resolved preliminary findings:

- Failed scheduler application did not retry an unchanged snapshot: dirty state plus regression added.
- Cached adjacency survived LCP remapping: current-mapping filtering plus scoped regression added.
- Wrong/error-shaped JSON was accepted as an empty snapshot: required-field/type checks and negative tests added.
- Source registration/config dependency wiring lacked tests: scoped LDP source/projection tests added.

Independent verification on the frozen product paths:

```text
go test -race -count=1 ./internal/frrsync/ldp ./internal/descriptors/mpls ./internal/renderers/frr/ldp
ok ngfw/agent/internal/frrsync/ldp 1.343s
ok ngfw/agent/internal/descriptors/mpls 1.052s
ok ngfw/agent/internal/renderers/frr/ldp 1.041s

go test -race -count=1 ./internal/subsystems -run TestLdp
ok ngfw/agent/internal/subsystems 1.087s

tools/ci.sh check --base main
check PASSED (0m04s)
```

The source/descriptor/renderer commands ran immediately before the final label-range filter commit; git diff proves those tested paths unchanged at the stated head. The subsystem tests were rerun after the final commit.

Complete local quick: BLOCKED-ENV. The reviewer independently attempted it on the companion PIM branch and the manager on baseline; API Unix gRPC socket tests fail `listen EPERM`, JWT ring ownership tests fail `chown EINVAL`. A broader subsystem run also reproduced baseline PBR missing `acl.acl/lan-b` failures. No full quick pass or live VPP/FRR/topology acceptance is claimed. The manager must obtain the complete hosted quick result before integration. EOS-only scope and laboratory limitations are explicit in the task evidence; remaining scale work is recorded in the developer's scoped debt note.

Verdict: APPROVE for product correctness at the stated head; integration remains conditional on the mandatory complete quick gate.

Final count delta reviewed: RPC rejects a disconnected VPP session; installed count comes from the named descriptor's current Retrieve under a ten-second bounded context, rather than cached desired route count. Tests prove configuration suppression reports zero even while observation survives, and failed application reports actual surviving objects. Reader failure returns unavailable rather than fabricating a zero count. A missing registered runtime remains explicitly unavailable at the RPC boundary.

```text
go test -race -count=1 ./internal/frrsync/ldp ./internal/subsystems ./internal/agent -run 'Test(Ldp|Read|HoldDown|FailedApply|MplsLdp)'
ok ngfw/agent/internal/frrsync/ldp 1.216s
ok ngfw/agent/internal/subsystems 1.098s
ok ngfw/agent/internal/agent 2.454s
```
