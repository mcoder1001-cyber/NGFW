# Interfaces discovery WIP

Branch: codex/interfaces-discovery-20261007
Last published local/remote contract SHA: c450739df14ee4abd97f021b71d5a8104222fa00. UI checkpoint published a3d2322f7284f49ff5d58173a18c6fbd4e824f6f; final reviewed source published4292daa826c5904aaf851f22f9f586f9c6112c57. Report-only reviews and deferred appliance/browser acceptance published9f9793e99ae59ed2ae67cb208731bb8bb8759e0c.
Owned files: see task envelope.
Completed: additive read-only host inventory controller response/client, graceful separate inventory/dataplane availability plus source diagnostics, EN/FA automatic host/management labels and read-only drawer, collision regression and user documentation.
Actual tests: pnpm gen PASS (13 tasks), API state suite PASS (17 tests), API and web typechecks PASS, targeted new EN/FA/unknown-link UI3 PASS, Go vppstartup PASS. Independent R1–R7 APPROVE reviewed source4292daa82. Obsolete quick cancelled after reviewed source changes; short-path complete quick session20037 failed web lint on source9075; preserve interfaces-discovery-quick-shorttmp.log and CI directory20261007-100722-2600221. Source lint fix checkpoint follows this commit; fresh complete quick next.. Duplicate full target run stopped to reduce host load; complete gate will certify all final tests.
Remaining: final complete unchanged quick gate, hosted quick gate, manager integration. Real appliance/browser acceptance deferred explicitly.
Current failure: complete short-path gate detected i18next/no-literal-string for literal JSX status="up"; fixed with typed HOST_LINK_UP constant and targeted web lint PASS (check-logical-css455 files OK). Fresh complete quick must rerun. Prior source regression native-vmxnet3 outer DPDK guard was fixed and final API17 PASS. Long-TMPDIR full gates intentionally cancelled before completion; short path avoids observed Unix socket length failure in separate wizard task. Complete gate still unverified.
Exact next command: TMPDIR=/ift tools/ci.sh quick --base origin/main

Acceptance limit: automatic host inventory covers physical PCI NICs; Linux-only virtual devices require future agent inventory extension. Existing VPP virtual interfaces continue to show. No live host changes performed.
