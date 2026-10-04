# F-igmp-mfib-host — static mFIB and live IGMP wiring

Implemented multicast data-plane source against existing schema/proto: ownership-aware static mFIB CRUD, IGMP interface/join/proxy and globals projection, live MulticastState, event relay lifecycle and persistent ownership enforcement. Includes fake-client regression coverage and gated real-host static multicast lifecycle test.

Full four-family unit suites, focused six-package tests/race, targeted vet and six-package golangci-lint passed, with actual output in F-igmp-mfib-host-evidence. Mandatory complete quick is blocked by cloud Unix-socket restrictions and remains required. No live VPP acceptance was claimed. PIM FRR renderer/sync stays F-pim-frrsync; BIER not built.

Publication: public GitHub commit was rejected by automatic approval review; branch source is local only. No PR or merge claimed. Full recovery context and next commands are in F-igmp-mfib-host-wip.md.
