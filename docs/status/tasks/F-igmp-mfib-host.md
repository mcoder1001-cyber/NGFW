# F-igmp-mfib-host — static mFIB and live IGMP wiring

Implemented multicast data-plane source against existing schema/proto: ownership-aware static mFIB CRUD, IGMP interface/join/proxy and globals projection, live MulticastState, event relay lifecycle and persistent ownership enforcement. Includes fake-client regression coverage and gated real-host static multicast lifecycle test.

Full four-family unit suites, focused six-package tests/race, targeted vet and six-package golangci-lint passed, with actual output in F-igmp-mfib-host-evidence. Mandatory complete quick is blocked by cloud Unix-socket restrictions; the owner explicitly deferred full CI to a later aggregate run on 2026-10-04. No live VPP acceptance was claimed. PIM FRR renderer/sync stays F-pim-frrsync; BIER not built.

Publication: the owner explicitly approved public publication and merge after an earlier approval rejection. Source be8bc983567fa9d8408830d149040b0b2e7a6460 is published on codex/igmp-host-20261003; reviewed archive 24f1f0fd22a3e2a1220647e037275f505e156b94 is retained. Latest-main rebase was conflict-free and six focused race suites passed again. Manager handles PR merge separately. Full recovery context and next commands are in F-igmp-mfib-host-wip.md.
