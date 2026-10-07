# Interfaces discovery WIP

Branch: codex/interfaces-discovery-20261007
Last published local/remote contract SHA: c450739df14ee4abd97f021b71d5a8104222fa00. UI checkpoint published a3d2322f7284f49ff5d58173a18c6fbd4e824f6f; reviewed fixes checkpoint follows this commit.
Owned files: see task envelope.
Completed: additive read-only host inventory controller response/client, graceful separate inventory/dataplane availability plus source diagnostics, EN/FA automatic host/management labels and read-only drawer, collision regression and user documentation.
Actual tests: pnpm gen PASS (13 tasks), API state suite PASS (16 tests before latest candidate regression), API and web typechecks PASS, targeted new EN/FA UI2 PASS, Go vppstartup PASS. Full InterfacesPage and final API17/newUI3 running. Obsolete quick cancelled after reviewed source changes; final quick next.
Remaining: web targeted result, full unchanged quick gate, independent reviews and manager integration.
Current failure: none observed; implementation unverified.
Exact next command: TMPDIR=/root/ngfw-wt/logs/interfaces-discovery-20261007-tmp tools/ci.sh quick --base origin/main

Acceptance limit: automatic host inventory covers physical PCI NICs; Linux-only virtual devices require future agent inventory extension. Existing VPP virtual interfaces continue to show. No live host changes performed.
