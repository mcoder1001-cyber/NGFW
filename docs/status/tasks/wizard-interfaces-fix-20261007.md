# Wizard interface discovery fix
Factory configurations without saved interfaces previously showed empty WAN/LAN selectors and an undefined-string validation error. The wizard now combines configured interfaces and eligible live physical interfaces, displays translated discovery/selection feedback and excludes host-owned, local0, subinterfaces and the opposite selection. Preview and stage independently validate live identity; new interface defaults appear in the exact candidate diff and confirmed commit remains the mutation boundary. D-239.

Independent R1/R2/R3/R6/R7 APPROVE; independent complete unchanged quick PASS (15m46s), API716/web623 tests,35Turbo tasks, agent/CLI lint/race/test/build,27unit-only Go modules,149 fake-host checks. Exact final output: CI GATE PASSED. Detailed reports preserve actual earlier failures and fixes. Independent product tree matches root source.

D112 remote archive, single-commit integration, root gate, final hosted quick and expected-head merge remain required. Actual appliance/browser and packet acceptance explicitly deferred; no deployed service change claimed.
