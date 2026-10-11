# Setup WAN defaults and remove LAN step — work in progress

Branch: codex/setup-system-wan-no-lan-20261011
Local commits: 2ce83db29 (contract), 7f73c292c (API); UI changes pending commit.
Remote checkpoint: pending.

Implemented: six-step WAN-only UI; configured and live WAN defaults; editable fields protected from late observation refreshes; English/Persian copy; updated browser preview script and getting-started guide. Contract allows omitted LAN fields while preserving legacy callers and policies.

Validation reported by contract helper: schema tests 17/17; API tests 17/17; package lint pass. Route reuse correction tests 19/19; latest preservation additions pending final run. Parent UI tests and dependency generation/typecheck running slowly on shared host.

Independent source review found and tracked: ambiguity fallthrough and VRF mixing corrected; initial observation edit race corrected by disabling WAN controls until initial requests settle; ordinary static route duplication correction underway; address-only gateway detection corrected.

Remaining: integrate final route/preservation fix; regenerate API artifacts; finish focused UI/typecheck and quick gate; final independent review; publish reviewable checkpoint/PR. Hardware negotiation and confirmed commit acceptance require isolated lab and have not been run.

Next: poll current test/generation sessions; cherry-pick contract helper correction; regenerate, test and commit final consumers.
