# F-igmp-mfib-host recovery checkpoint

Branch: codex/igmp-host-20261003. Base: cf486eff. Worktree: task-igmp.
Remote SHA: unavailable. The GitHub connector commit operation was rejected by automatic approval review because public publication permission was judged missing; no PR exists. Local implementation checkpoint 23bbcfd7000454777392ddc88c75b17027538697 (source frozen; final evidence-only commits may follow). Do not claim durable publication.

Owned: descriptors/{mfib,igmp} (IGMP gap-only ownership/state), desired/igmp_mfib, subsystems/igmp_mfib, agent/rpc_igmp_mfib and related tests/docs. Shared hunks: projection.go IGMP build/assembly; subsystems.go registration/Connected/AfterResync; df7/events.go bounded unsubscribe; test-only coretest/{igmp_mfib,lcp_mfib} and reachability table.

Completed: generated-binding static mFIB CRUD/retrieve with raw prefix collision guard, owned table/range checks, persistent boot claims and failed-write restoration; IGMP interface/join/proxy projection and owner-only SSM ranges; live membership/mroute RPC; membership event retry/cancellation across reconnection/shutdown. Persistence declarations enforce production stores. Existing IGMP static joins canonicalize source addresses numerically. Cold-start static retrieval excludes learned router groups until the interface mode is known host, preventing accidental deletion before reapply.

Historical claims of mfib integration refer to unavailable local-host history; current GitHub main lacked source. Remote codex/igmp-pim-host-resume-20261002 carries only partial PIM renderer. PIM renderer/sync belongs to F-pim-frrsync; empty PIM neighbours are explicitly documented. BIER optional, not built.

Actual verification: full mfib/igmp/df7/desired unit suites PASS; focused golangci-lint PASS and targeted go vet PASS. Six feature packages focused tests PASS and targeted race PASS (evidence directory), including raw-undecodable foreign route exclusion, multiple groups independent rollback, failed-write stale claim restoration, reboot ownership, empty SSM, router live group dedupe/filtering, RPC ownership/connectivity/error/success, persistent registration and bounded unsubscribe and direct reconnect/unsubscribe ordering. Broader suites initially exposed absent multicast fake handlers/persistence declarations and were fixed. Remaining broad failures use Unix sockets denied by cloud sandbox; see mandatory quick result below.

Mandatory complete quick attempted with unchanged tools/ci.sh --base main. Generation/drift and forbidden patterns/gitleaks passed; API tests fail on unavailable Unix sockets (11 failed/75 passed files, 8 failed/502 passed/65 skipped). The gate is NOT green and hosted quick remains required before merge. After these real failures the stale broad gate was terminated by its exact spawned process IDs. No tests weakened or skipped to manufacture green.

Live laboratory execution unavailable (no /run/vpp/api.sock). A normal gated static multicast host test now verifies create/retrieve/rollback/recreate without sending IGMP. Actual lab static/pimd/restart/IGMP packet evidence remains deferred. Existing IGMP engine host test requires explicit opt-in because recorded router-alert VPP crash.

Remaining: independent final review, exact-tree hosted quick, authorized publication and merge; real lab acceptance under normal/IGMP opt-in windows.
Next command: run tools/ci.sh --base main with hosted resource settings in an environment permitting Unix sockets, then actual lab TestStaticMulticastOnHost and opt-in TestIGMPOnHost. Preserve source and evidence without overwriting these restrictions.
