# Agent timing instrumentation prototype — D172 option

Own NGFW-ci-agent-timing-prototype / task/CI-agent-timing-prototype-20261002,
base actualmaincb5cf4c1e21ffbc60a8129692c02437eebe715bb. Prior measured report
corrected4e9ff90c independently corroborated: combined agent5m55–6m06 cannot be
split among vet/lint/race/build because its artifact has no command timestamps.

Draft scope only apps/agent/Makefile: constant VRX_TIMING BEGIN/END labels plus
UTC timestamps immediately before/after existing vet, lint dispatch, race test
and build commands. Each original recipe/flags/order remains byte-identical;
markers are separate recipes, so failure aborts before its END marker and cannot
be masked by printf. No wrappers/background/global shell state/cache/gate edits.
No Turbo summary flag added without independently verifying pinned CLI support.

Normal tools/ci.sh captures successful output into agent artifact08-agent.log;
these timestamped markers will be available there, not streamed live by default.
With existing verbose mode, job log also shows them. No verbose/gate change here.
END means a command returned normally under make, not a new acceptance claim;
missing END identifies failure/interruption, and lint fallback is still forbidden
by existing CI verification. No hosted validation or measured speedup claimed.

Review via make dry-run/recipe command-stream comparison and diff syntax only;
no local Go compile/lint/race or dependency download. Fresh independent applicable
review needed. Do not merge/apply optimization while featurePR84→transaction→
cleanup serial gates are pending; manager owns eventual hosted instrumentation
experiment and exact-head gate. No D172/log/board/main/feature-ref edits.

Actual verification: make -n lint test build VERSION=timing-dryrun shows original
command order/flags and eight separate markers. Removing only marker lines yields
byte-identical original Makefile. bash -n parses the dry-run command stream EXIT0;
git diff --check clean. No Go command executed and no PASS of hosted tests claimed.
