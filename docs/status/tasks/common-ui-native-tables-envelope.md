# Native Common UI table task envelope

Request: replace every visible NGFW page table with ReactGeereedMaterial from the owner's Common UI, so the user can review the result in the already authorized API-free development preview.

Reuse existing integration branch codex/common-ui-migration-20261008. Root owns integration, page imports, verification and status; grid worker owns server grid adapter; forms worker owns declarative composition adapter/tests; shell worker owns pure page row helper conversions and native selection/filter callbacks. Worker histories remain on their named branches; root integrates coherent commits sequentially. Independent review is read-only.

Acceptance: actual Common UI table renders both server grids and handcrafted page tables; preserve server query/paging/selection, row actions, forms, events, refs, colSpan/rowSpan, nested content, accessibility and read-only guards. Build and relevant tests must pass; production has no preview transport. Main merge is outside this local preview task and remains gated by required CI/reviews.

Recovery: see common-ui-native-tables-wip.md for concrete commands, results, checkpoints and remaining verification. Remote publication must be reported accurately; previous attempts denied HTTP403.
