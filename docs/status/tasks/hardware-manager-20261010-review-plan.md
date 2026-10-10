# Packaging correction review plan

Source checkpoint2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c; final D112 integration
commit will retain the identical product tree. Only product path changed:
`deploy/debian/ngfw/debian/control`: consume existing shlibs:Depends in ngfw-api.
Other changed paths are this task's envelope/WIP/review plan; no contracts changed.

Mandatory reviewers: R1 correctness/tests (host_37), R2 security (host_211),
R7 docs/evidence (fresh independent review after a slot becomes available),
R8 packaging (install_review). T1 always applies (host_37 independently verifies
complete hosted quick on final SHA plus actual fixture/build evidence).
No API/agent/compiler/web/VPP sources or forwarding behavior changed, so R3/R4/R5/R6
and T2/T3/T4 are not required for this metadata correction. Hardware installation
has separate R4/R8 safety review and remains blocked by ext4 corruption.

R8 found the native metadata failure and approved its narrow source correction.
Fixed native archive inspection is pending; old archives stay blocked. Do not merge
until every applicable reviewer APPROVE, T1 PASS, unchanged hosted quick green,
current main/expected PR heads verified, and reviewed history archived remotely.
