# F-mpls-srmpls — questions and notes for the manager

Worker slot 5, branch `task/F-mpls-srmpls`. Nothing here blocks the task; each item says what I did meanwhile.

## Q1 — contract committed on the branch (please review)
`contract(schema): routing mpls` (2e57555c) and `contract(proto): routing mpls, MplsState` (df92f99c), described in
`F-mpls-srmpls-contract.md`. Numbers exactly as wave-BC-numbers.md: RoutingConfig 15; MplsConfig 1–6 used, 7–9 spare
(this task's), 10 left to F-mpls-ldp with a comment + `// wave-BC: F-mpls-ldp` anchor (no `reserved 10;`, so their
change stays an insert). No EventKind, no ActionRequest member. I kept building against the branch.
