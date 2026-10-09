# Owner-authorized completion campaign

Owner direction, 2026-10-08: complete the non-laboratory work, merge reviewed
changes, and reconcile board/documentation. Run the final aggregate CI campaign
after source corrections are complete; no intermediate workflow was requested.
The earlier PR206 navigation-only hold is superseded by this explicit all-merge
direction. This instruction does not certify unfinished code or laboratory tests.

Integration branch: `codex/completion-20261008`, PR214. Main at campaign start:
`417e8fcd0c82fae7906da0fba1c0622a8de017f7`. Checkpoints carry `[skip ci]`.
The required quick gate is not weakened. Offline Python fixture coverage is
extended to include the new committed release lock and provenance contracts.

## Source staged for final validation

| Scope | Reviewed source provenance | Evidence and boundary |
|---|---|---|
| FRR slot cleanup | PR209 `64171651` | Three race regressions passed; prior source CI37812533974 passed. Does not resolve native mgmtd deadline or current 200-route proof. |
| PPP lifecycle | PR210 `baa12c54` | Reviewed fencing, parent lifetime and transition retries; does not implement missing carrier. |
| PPPoE setup wizard | `6ecc6469`, remote `cf8cddf8` | R2-approved reference-only setup, distinct logical WAN and physical parent, collision guards and safe rerun;39 focused tests PASS under verified Node22.23.2. |
| PPP carrier packaging | `152cc08f`, remote `f4131162` | R7-approved fixed helper/unit/hook/tmpfiles staging and WAN probe receipt preservation;6 focused fixtures PASS. Requires final helper source integration. |
| Web navigation | PR206 `0df43e59` | Independent source reviews and earlier full local/hosted quick passed; en/fa navigation source staged unchanged. |
| TD19 Python release | PR212 `bfe917b6` | Six direct pins,22 runtime wheels,24 upstream artifacts and reproducible derived wheel. Independent source/security approval and offline rebuild. Actual Ubuntu26.04 install/boot/packet remains unverified. |
| Dynamic Multi-WAN | PR215, product local `edd8d0d1` | R4-approved DHCP renewal/release/error withdrawal, cleanup retry protection, fixed probe executable and egress-generation fencing. Actual PPP runtime binding remains pending. |
| Production sealed credentials | `9d61005d`, remote `eebb6254` | Independent R2 combined approval and R4 CA-key isolation approval;36 API tests and eight focused race packages passed. WG/SNMP/BGP/NTP/TLS syslog/host namespace startup and projection bound to existing versioned store. |
| IP unnumbered | `675cd9ea`, integration remote `036ca641` | R4-approved generated-API descriptor, ownership/dependency guards and Apply/Retrieve/restart/rollback lifecycle race tests. Native VPP acceptance still unverified. |
| Daemon ownership/identity | PR213, product local `4b39ad1f` | R2-approved capability/mount boundary and atomic public-link migration;10 local Python cases and sysident race passed. Two real foreign-UID/capability cases require final hosted execution. No appliance service changed. |

## Previous CI failure repaired without changing production guards

Earlier PR210 workflow37812958217 on `a9398271`, started before CI deferral,
failed solely at `TestPppoeIPv6MirrorFollowsHookState`: the render-only non-owner
fixture invoked a guarded hook without supervisor admission. The real hook
correctly refused to publish. Integration commit `3d03bab6` explicitly admits
the simulated session through `ResumeIPv6` before the existing hook assertions.
Author and independent R1 each ran this test three times under race detection,
all passing, no skips (1.337s/1.340s). The current final aggregate gate has not
run. The earlier local procfs/PID namespace limitations remain recorded and are
not converted into passing process-lifetime acceptance.

## Still in progress / unavailable

- Accepted WBS D1.3 IP unnumbered and the six production credential consumers
  now have reviewed source staged above. The discovered syslog CA-key alias
  escape is corrected before private-key lookup, independently rechecked.
  NTS server certificates remain outside the accepted chrony feature scope.
- PPP kernel carrier: exclusive physical L2 transport, distinct logical VPP
  transit interface, owned namespace and pre-local return routing, lifecycle
  readiness/withdrawal, effective policy identity, dynamic WAN probing and PD
  LAN address/RA application. Allocator or helper fixtures alone do not complete it.
- Product license: owner selected use of existing text, but supplied no text or
  identifier, and no authoritative product LICENSE/copyright was found. No
  license or copyright holder is invented. This is a release input, not a lab test.
- Final unchanged quick gate and relevant fixture workflows are deferred until
  source is ready. Native RA supplier/identity, P12 mgmtd, PPP and other packet,
  installed-service, restart/rollback and browser acceptance remain unverified.

Next: finish and independently review carrier/PD/WAN integration, consolidate
source, then run final CI on the exact cumulative candidate. Preserve failures,
fix their actual causes and obtain passing required checks before integration.
Refresh this receipt and task states from actual merge and validation results.

Board reconciliation: historical merged receipts are retained, but P08 is reopened
to review. WG/SNMP/host services/BGP now have independent source approval and
are in review awaiting final integration; Multi-WAN remains running for actual
PPP runtime binding. Task counts therefore reflect incomplete work rather
than preserving an inaccurate completion percentage.

Integrated-tree focused check after credential/unnumbered registration merge:
`go test -race -count=1 ./internal/agent -run
'TestHostCredentialProjectionSelectsOwnerAndRotation|TestUnnumberedDomainApplyRetrieveRevoke'`
PASS 1.191s; no skips and no full CI run.

## Recovered cumulative candidate — 2026-10-09

The source candidate `33db7c63` consolidates the previously separately published
carrier, PD, VLAN, helper, packaging and WAN ownership work. The actual runtime
now satisfies the WAN forwarding interface. Recovery prerequisite ordering has
independent R2 approval. Current source tests reproduce and correct WAN route
write ordering, NCP replacement during verification, and stale forwarding on
invalid observation; native boundaries in these tests are fakes. Source generation
completed thirteen tasks. Detailed commands/results are in
[carrier-finish-20261009-wip.md](carrier-finish-20261009-wip.md) and the final review
receipts. Earlier pending-source statements above are checkpoint history, not the
current implementation inventory.

No CI has run on this candidate yet. Final cumulative R2/R4 reviews, the unchanged
hosted gate and relevant fixture workflows, and exact tested-source merge remain
required. Product license authority remains a release input; no license is chosen
by this campaign. All native acceptance listed centrally remains NOT RUN.

Final R2 and R4 source approvals are now recorded for candidate33db7c63, with no unresolved findings in their scopes. Only outcome documentation changed after that product freeze. The final hosted campaign and merge receipt follow in ../2026-10-09-completion.md.

## Final integrated source receipt — 2026-10-09

Source candidate `30de26ee6a5697a3713fe4375399b86c5588a472` passed the unchanged complete hosted quick gate
[37903333143](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333143) and all applicable fixture workflows.
[PR214](https://github.com/mcoder1001-cyber/NGFW/pull/214) merged as `d58db1a673d716ebcf595a9e49847a54991c58da`; merge tree equals tested tree
`d1de8b7c01220ce03b311d0a7dcafd5fe12ab1ad`. Reviewed history is preserved at
`codex/archive-completion-corrected-20261009` (`3f98838119ed8d67aab9c226308829ac49af9def`).
Final R2/R4 correction receipts approve the scoped source; initial failed CI and
local environment failures remain historical evidence, not retroactive PASS.

| Final check | Result |
|---|---|
| Mandatory quick | PASS, complete unchanged hosted gate |
| [Packaging37903333245](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333245) | 81 fixtures, zero failures/errors/skips; seven gate-policy controls PASS |
| [Provisioning37903333184](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333184) | 46 strict +23 offline Debian +11 trusted installer +18 portable export PASS |
| [Python37903333309](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333309) | 14 synthetic-wheel +5 release-contract tests PASS |

Board: **205 merged,7 parked,0 review,0 running**, total212. The eight reviewed
source rows are merged. PPP-host and MultiWAN-host source is integrated; their
native acceptance joins the five already parked rows. No live source worker is
claimed. All older pending-source/CI statements in this document are historical
and superseded by this receipt.

Native acceptance of this cumulative source remains **NOT RUN**. Known prior RA
supplier/post-ACK identity failures and P12 mgmtd startup failure at the unchanged
30-second deadline before the200-route proof still require diagnosis, any necessary
fix and rerun on the real target. No unit/fixture result closes these cases.
Product license text/name/copyright authority remains a separate release input
under `docs/decisions/PENDING-P10-product-license.md`; no license is invented.
Plan exclusions remain unchanged. This is source completion, not release certification.
