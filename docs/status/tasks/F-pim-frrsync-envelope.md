# F-pim-frrsync envelope

Branch: task/F-pim-frrsync-20261004. Worktree: /workspace/scratch/e4f791ef53f7/pim.
Owner: PIM developer. Manager authorized source construction and shared wiring additions.

Scope follows plan/tasks.yaml F-pim-frrsync: existing PIM contract, FRR RP section and S2 interface lines, bounded show ip mroute adapter, seam S1 dynamic source, exclusive mfib.route.pim descriptor, scoped pimd integration harness, unit evidence and documentation.

Owned packages: renderers/frr/pim, frrsync/pim; own subsystems/pim*.go; approved shared hunks mfib/mfib.go, desired/bgp.go, subsystems/subsystems.go. No board, binapi, contract, API or UI change.

Default-VRF PIM only, matching current contract. Product global ID scope required for table zero; numbered slots own no dynamic global table routes. Harness config verification uses test namespace and child pimd, never a system FRR instance. No live VPP restart or host mutation in this cloud workspace.
