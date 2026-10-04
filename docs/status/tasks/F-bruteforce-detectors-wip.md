# Recovery checkpoint

Branch contract/F-bruteforce-detectors-20261004; base 06e4368c.
Contract local aff92170, published equivalent remote 4cb740d3ad04959045141fbd43dea18743ddd853
(manager confirmed identical tree). Product local5c88cb5f, equivalent published remote93508f8a2c7a9332d10179ca2052126ff5a069a4
PR143; verified tree d6d31bd3de4b4ef22ddd7270f4266d9015e73655. Source frozen.
Independent R1/R2/R4/R5 reviews approved source (reports manager-owned).

Owned edits: agent detectors (host/native + tests), granted existing host watcher,
rpc_events_autoblock helper/tests, API auto-block engine/service/tests, topology driver
and unit tests, user docs, task prompt and task recovery files. No enum/generated changes:
EventKind25 already on main, map attribute extension documented in contract checkpoint.

Preexisting main source at 4ead8bd2: host SSH/charon parsing, native secret-safe owned VPN
watcher, watcher lifecycle, API subscriber, enforcement and web/expiry topology driver.
New work: scan destination-port observations, API distinct-port refresh window and
source/aggregate/per-source caps; invalid-port fail-closed; StreamEvents and actual subscriber regression
coverage; SSH/manual topology mode with strict auth rejection evidence.

Actual completed checks: Go focused detectors/agent tests passed (nonrace); API focused
6 new tests passed; Python topology-driver 4 unit tests passed; driver syntax compiles;
source check PASSED (gitleaks unavailable at the earlier check, no false full-CI claim).
API full typecheck and targeted ESLint passed after dependency builds; earlier missing dependency declarations and partial Event fixture were corrected. Broader API suite 29 tests passed; latest budget suite passed:31totalAPItests. Go focused race and vet passed for detectors/agent. Source check with pinned gitleaks PASSED. Complete quick BLOCKED-ENV: generation clean, gates passed; concurrent Turbo killed
API test/typecheck processes (typecheck exit137). Manager instructed no repeat, own
session67971 interrupted(exit130). Logs /tmp/ngfw-ci/detectors-20261004-045756-2.
Next: manager publishes report, finishes final applicable review, integration per owner
CI deferral; run real lab topology later.

Remaining: manager publish final report and integrate source; lab packet acceptance deferred.
Live lab not available; local-in/forwarding/allowlisted actual packets NOTRUN. Native VPN
requires already-established safe SA state support; charon compatibility parser does
not authorize product blocks. Deploy agent/API together: legacy port-less observations
are ignored because their distinct-port evidence cannot be trusted.
